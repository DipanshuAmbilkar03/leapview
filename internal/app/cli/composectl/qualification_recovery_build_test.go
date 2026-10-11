package composectl

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/deployment/qualificationbarrier"
	"github.com/stretchr/testify/require"
)

type buildBarrierExecutor struct {
	contents string
}

func (e buildBarrierExecutor) Execute(_ context.Context, request qualificationCommandRequest) ([]byte, error) {
	return nil, os.WriteFile(request.Arguments[len(request.Arguments)-1], []byte(e.contents), 0o600)
}

func TestQualificationRecoveryArmsBuildResponseBarrier(t *testing.T) {
	executor := &recordingQualificationExecutor{}
	c, err := New(Options{Root: t.TempDir(), DockerBin: "docker-probe", qualificationExecutor: executor})
	require.NoError(t, err)
	work := t.TempDir()
	require.NoError(t, c.armQualificationBuildResponseBarrier(t.Context(), "owned-container", work))
	require.Len(t, executor.requests, 2)
	want := []string{"exec", "owned-container", "rm", "-f", qualificationBuildResponseMarkerPath(qualificationbarrier.BuildResponseArmedMarker), qualificationBuildResponseMarkerPath(qualificationbarrier.BuildResponseReachedMarker)}
	if !slices.Equal(executor.requests[0].Arguments, want) {
		t.Fatalf("clear arguments: %v", executor.requests[0].Arguments)
	}
	require.Equal(t, []string{"cp", filepath.Join(work, qualificationbarrier.BuildResponseArmedMarker), "owned-container:" + qualificationBuildResponseMarkerPath(qualificationbarrier.BuildResponseArmedMarker)}, executor.requests[1].Arguments)
	contents, err := os.ReadFile(executor.requests[1].Arguments[1])
	require.NoError(t, err)
	require.Equal(t, qualificationBuildResponseBoundary+"\n", string(contents))
}

func TestQualificationRecoveryRequiresExactBuildBoundary(t *testing.T) {
	valid := `{"boundary":"native-build-sealed-before-http-response","planId":"0198f2c0-7c7a-7f00-8a11-000000000101","buildId":"0198f2c0-7c7a-7f00-8a11-000000000102","candidateId":"0198f2c0-7c7a-7f00-8a11-000000000103"}`
	for _, contents := range []string{valid, "wrong-boundary\n", `{}`} {
		c, err := New(Options{Root: t.TempDir(), DockerBin: "docker-probe", qualificationExecutor: buildBarrierExecutor{contents}})
		require.NoError(t, err)
		running := &qualificationRunningCommand{command: exec.Command("unused"), done: make(chan struct{})}
		_, err = c.waitForQualificationBuildResponseBarrier(t.Context(), "owned-container", t.TempDir(), running)
		if contents == valid {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "unexpected sealed-build")
		}
	}
	c, err := New(Options{Root: t.TempDir(), DockerBin: "docker-probe", qualificationExecutor: &recordingQualificationExecutor{}})
	require.NoError(t, err)
	done := make(chan struct{})
	close(done)
	_, err = c.waitForQualificationBuildResponseBarrier(t.Context(), "owned-container", t.TempDir(), &qualificationRunningCommand{done: done})
	if err == nil || !strings.Contains(err.Error(), "completed before") {
		t.Fatalf("early completion accepted: %v", err)
	}
}
