package composectl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/app/cli/installationstate"
	"github.com/stretchr/testify/require"
)

func TestFirstInstallActivationWaitsForDockerReadinessAfterPublication(t *testing.T) {
	for _, https := range []bool{false, true} {
		t.Run(fmt.Sprintf("https=%t", https), func(t *testing.T) {
			controller, root := bootstrapLifecycleController(t, https)
			staleHealth, commandLog := bootstrapActivationHealthProbe(t, controller, root)
			waits := 0
			controller.sleep = func(ctx context.Context, _ time.Duration) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				waits++
				// Docker samples /readyz periodically; publication can become
				// ready before its cached unhealthy state is refreshed.
				return os.Remove(staleHealth)
			}
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			require.NoError(t, controller.activateFirstInstallAt(t.Context(), server.URL))
			require.Equal(t, 1, waits)
			marker, present, err := installationstate.ReadMarker(root)
			require.NoError(t, err)
			require.True(t, present)
			require.Equal(t, installationstate.PhasePublic, marker.BootstrapPhase)
			commands, err := os.ReadFile(commandLog)
			require.NoError(t, err)
			require.NotContains(t, string(commands), "PUBLIC_UP_WHILE_UNHEALTHY")
			require.Contains(t, string(commands), "PUBLIC_UP_HEALTHY")
		})
	}
}

func TestFirstInstallActivationKeepsPrivatePhaseWhenDockerReadinessDoesNotRecover(t *testing.T) {
	for _, outcome := range []string{"unhealthy", "cancelled", "readiness regressed"} {
		t.Run(outcome, func(t *testing.T) {
			controller, root := bootstrapLifecycleController(t, true)
			staleHealth, commandLog := bootstrapActivationHealthProbe(t, controller, root)
			var ready atomic.Bool
			ready.Store(true)
			waits := 0
			controller.sleep = func(context.Context, time.Duration) error {
				waits++
				switch outcome {
				case "cancelled":
					return context.Canceled
				case "readiness regressed":
					ready.Store(false)
					return os.Remove(staleHealth)
				default:
					return nil
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				if ready.Load() {
					response.WriteHeader(http.StatusOK)
				} else {
					response.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			defer server.Close()

			err := controller.activateFirstInstallAt(t.Context(), server.URL)
			require.Error(t, err)
			if outcome == "cancelled" {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.Positive(t, waits)
			require.LessOrEqual(t, waits, defaultHealthChecks)
			marker, present, readErr := installationstate.ReadMarker(root)
			require.NoError(t, readErr)
			require.True(t, present)
			require.Equal(t, installationstate.PhasePrivate, marker.BootstrapPhase)
			commands, readErr := os.ReadFile(commandLog)
			require.NoError(t, readErr)
			require.NotContains(t, string(commands), "PUBLIC_UP_", "failed readiness must not expose the public proxy")
		})
	}
}

func bootstrapActivationHealthProbe(t *testing.T, controller *Controller, root string) (string, string) {
	t.Helper()
	staleHealth := filepath.Join(root, "docker-health-is-unhealthy")
	commandLog := filepath.Join(root, "activation-docker.log")
	require.NoError(t, os.WriteFile(staleHealth, nil, 0o600))
	docker := filepath.Join(root, "activation-docker")
	contents := `#!/bin/sh
printf '%s\n' "$*" >> "$LEAPVIEW_TEST_ACTIVATION_LOG"
case "$*" in
  *"ps -q leapview"*) printf 'leapview-container\n'; exit 0 ;;
  *"inspect -f {{.State.Health.Status}} leapview-container"*)
    if [ -e "$LEAPVIEW_TEST_STALE_HEALTH" ]; then printf 'unhealthy\n'; else printf 'healthy\n'; fi
    exit 0 ;;
esac
if [ "$1" = compose ] && echo "$*" | grep -q ' up -d' &&
   ! echo "$*" | grep -q 'first-install.env'; then
  if [ -e "$LEAPVIEW_TEST_STALE_HEALTH" ]; then
    printf 'PUBLIC_UP_WHILE_UNHEALTHY\n' >> "$LEAPVIEW_TEST_ACTIVATION_LOG"
    if echo "$*" | grep -q 'compose.https.yaml'; then
      printf 'dependency failed to start: container leapview is unhealthy\n' >&2
      exit 17
    fi
  else
    printf 'PUBLIC_UP_HEALTHY\n' >> "$LEAPVIEW_TEST_ACTIVATION_LOG"
  fi
fi
exit 0
`
	require.NoError(t, os.WriteFile(docker, []byte(strings.TrimSpace(contents)+"\n"), 0o700))
	t.Setenv("LEAPVIEW_TEST_ACTIVATION_LOG", commandLog)
	t.Setenv("LEAPVIEW_TEST_STALE_HEALTH", staleHealth)
	controller.dockerBin = docker
	return staleHealth, commandLog
}
