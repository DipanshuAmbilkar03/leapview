// Package qualificationbarrier defines the inert evaluation hook shared by
// native build/activation boundaries and the external qualification client.
package qualificationbarrier

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const BuildResponseBoundary = "native-build-sealed-before-http-response"

// BuildResponseEvidence identifies the exact sealed result whose HTTP
// response has not reached the client at the interruption boundary.
type BuildResponseEvidence struct {
	Boundary    string    `json:"boundary"`
	PlanID      uuid.UUID `json:"planId"`
	BuildID     uuid.UUID `json:"buildId"`
	CandidateID uuid.UUID `json:"candidateId"`
}

const (
	// BuildResponseArmedMarker pauses only after the native build and its
	// command evidence are durable, before returning the HTTP response.
	BuildResponseArmedMarker   = ".qualification-build-response-barrier.armed"
	BuildResponseReachedMarker = ".qualification-build-response-barrier.reached"
	// ArmedMarker asks an evaluation deployment to pause immediately before its
	// target activation compare-and-swap.
	ArmedMarker = ".qualification-activation-barrier.armed"

	// ReachedMarker proves that an armed evaluation deployment reached the
	// activation boundary and consumed ArmedMarker.
	ReachedMarker = ".qualification-activation-barrier.reached"
)

// WaitBeforeBuildResponse consumes an explicitly armed evaluation marker only
// after the sealed build and its command evidence are durable. It permits an
// external test to kill the server before the client receives the response.
func WaitBeforeBuildResponse(ctx context.Context, environment string, evidence BuildResponseEvidence) error {
	evidence.Boundary = BuildResponseBoundary
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	return waitBefore(ctx, environment, BuildResponseArmedMarker, BuildResponseReachedMarker, "build response", encoded)
}

// WaitBeforeActivation consumes an explicitly armed marker and pauses the
// caller immediately before the native target activation compare-and-swap. It
// is inert outside the evaluation environment and when LEAPVIEW_HOME is
// unset, so it cannot pause a normal production target.
func WaitBeforeActivation(ctx context.Context, environment string) error {
	return waitBefore(ctx, environment, ArmedMarker, ReachedMarker, "activation", nil)
}

func waitBefore(ctx context.Context, environment, armedMarker, reachedMarker, boundary string, evidence []byte) error {
	if strings.TrimSpace(environment) != "evaluation" {
		return nil
	}
	home := strings.TrimSpace(os.Getenv("LEAPVIEW_HOME"))
	if home == "" {
		return nil
	}
	armed := filepath.Join(home, armedMarker)
	reached := filepath.Join(home, reachedMarker)
	if _, err := os.Stat(armed); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect qualification %s barrier: %w", boundary, err)
	}
	// Fail closed rather than replacing evidence from a previous run. Rename
	// makes the one-shot armed -> reached transition atomic to observers.
	if _, err := os.Stat(reached); err == nil {
		return fmt.Errorf("qualification %s barrier reached marker already exists", boundary)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect qualification %s barrier reached marker: %w", boundary, err)
	}
	if evidence != nil {
		// docker cp arms the hook with a root-owned private file. The server
		// owns the directory, so replace the marker atomically rather than
		// requiring write access to that file or weakening its permissions.
		pending, err := os.CreateTemp(home, ".qualification-evidence-*")
		if err != nil {
			return err
		}
		defer os.Remove(pending.Name())
		if _, err := pending.Write(evidence); err != nil {
			_ = pending.Close()
			return fmt.Errorf("write qualification %s evidence: %w", boundary, err)
		}
		if err := pending.Close(); err != nil {
			return err
		}
		if err := os.Rename(pending.Name(), armed); err != nil {
			return fmt.Errorf("write qualification %s evidence: %w", boundary, err)
		}
	}
	if err := os.Rename(armed, reached); err != nil {
		return fmt.Errorf("consume qualification %s barrier: %w", boundary, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	return ctx.Err()
}
