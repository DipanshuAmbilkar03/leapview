package qualificationbarrier

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildResponseBarrierIsInertOutsideArmedEvaluation(t *testing.T) {
	for _, environment := range []string{"prod", "production", "dev", ""} {
		t.Run(environment, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("LEAPVIEW_HOME", home)
			armed := filepath.Join(home, BuildResponseArmedMarker)
			if err := os.WriteFile(armed, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := WaitBeforeBuildResponse(t.Context(), environment, BuildResponseEvidence{}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(armed); err != nil {
				t.Fatalf("non-evaluation marker consumed: %v", err)
			}
		})
	}
	t.Setenv("LEAPVIEW_HOME", t.TempDir())
	if err := WaitBeforeBuildResponse(t.Context(), "evaluation", BuildResponseEvidence{}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildResponseBarrierProvesBoundaryAndIsOneShotAcrossRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEAPVIEW_HOME", home)
	armed, reached := filepath.Join(home, BuildResponseArmedMarker), filepath.Join(home, BuildResponseReachedMarker)
	if err := os.WriteFile(armed, nil, 0o400); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- WaitBeforeBuildResponse(ctx, "evaluation", BuildResponseEvidence{}) }()
	deadline := time.After(time.Second)
	for {
		if _, err := os.Stat(reached); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("build response escaped before interruption: %v", err)
		case <-deadline:
			t.Fatal("build response never reached its boundary")
		case <-time.After(time.Millisecond):
		}
	}
	if _, err := os.Stat(armed); !os.IsNotExist(err) {
		t.Fatalf("armed marker not consumed: %v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted boundary: %v", err)
	}
	// A replay after restart must not pause again on retained reached evidence.
	if err := WaitBeforeBuildResponse(t.Context(), "evaluation", BuildResponseEvidence{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(armed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WaitBeforeBuildResponse(t.Context(), "evaluation", BuildResponseEvidence{}); err == nil {
		t.Fatal("stale boundary evidence accepted")
	}
}
