package composectl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flidai/leapview/internal/deployment/qualificationbarrier"
	"github.com/google/uuid"
)

const qualificationBuildResponseBoundary = qualificationbarrier.BuildResponseBoundary

func qualificationBuildResponseMarkerPath(marker string) string {
	return "/var/lib/leapview/home/" + marker
}

func (c *Controller) clearQualificationBuildResponseBarrier(ctx context.Context, containerID string) error {
	_, err := c.qualificationContainers.Existing(containerID).Exec(ctx, nil, "rm", "-f",
		qualificationBuildResponseMarkerPath(qualificationbarrier.BuildResponseArmedMarker),
		qualificationBuildResponseMarkerPath(qualificationbarrier.BuildResponseReachedMarker),
	)
	return err
}

func (c *Controller) armQualificationBuildResponseBarrier(ctx context.Context, containerID, workDir string) error {
	if err := c.clearQualificationBuildResponseBarrier(ctx, containerID); err != nil {
		return fmt.Errorf("clear build response barrier: %w", err)
	}
	armed := filepath.Join(workDir, qualificationbarrier.BuildResponseArmedMarker)
	if err := os.WriteFile(armed, []byte(qualificationBuildResponseBoundary+"\n"), 0o600); err != nil {
		return err
	}
	_, err := c.qualificationDocker(ctx, nil, "cp", armed,
		containerID+":"+qualificationBuildResponseMarkerPath(qualificationbarrier.BuildResponseArmedMarker))
	return err
}

func (c *Controller) waitForQualificationBuildResponseBarrier(ctx context.Context, containerID, workDir string, command *qualificationRunningCommand) (qualificationbarrier.BuildResponseEvidence, error) {
	reached := filepath.Join(workDir, qualificationbarrier.BuildResponseReachedMarker)
	var evidence qualificationbarrier.BuildResponseEvidence
	err := qualificationWait(ctx, 250*time.Millisecond, func(waitCtx context.Context) (bool, error) {
		if !command.Running() {
			return false, fmt.Errorf("delivery command completed before the sealed-build response boundary")
		}
		if err := os.Remove(reached); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		_, err := c.qualificationDocker(waitCtx, nil, "cp",
			containerID+":"+qualificationBuildResponseMarkerPath(qualificationbarrier.BuildResponseReachedMarker), reached)
		if err != nil {
			return false, nil
		}
		contents, err := os.ReadFile(reached)
		if err != nil {
			return false, err
		}
		if err := json.Unmarshal(contents, &evidence); err != nil || evidence.Boundary != qualificationBuildResponseBoundary || evidence.PlanID == uuid.Nil || evidence.BuildID == uuid.Nil || evidence.CandidateID == uuid.Nil {
			return false, fmt.Errorf("unexpected sealed-build response boundary evidence")
		}
		return true, nil
	})
	return evidence, err
}
