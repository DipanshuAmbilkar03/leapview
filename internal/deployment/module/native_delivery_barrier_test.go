package module

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/deployment"
	deploymentgen "github.com/flidai/leapview/internal/deployment/api/gen"
	deploymenthttp "github.com/flidai/leapview/internal/deployment/http"
	"github.com/flidai/leapview/internal/deployment/qualificationbarrier"
	"github.com/flidai/leapview/internal/servingstate"
	"github.com/google/uuid"
)

func TestNativeDeliveryBuildResponseBarrierFollowsVerifiedCompletion(t *testing.T) {
	for _, failedCompletion := range []bool{false, true} {
		t.Run(map[bool]string{false: "sealed replay", true: "failed verification"}[failedCompletion], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("LEAPVIEW_HOME", home)
			armed := filepath.Join(home, qualificationbarrier.BuildResponseArmedMarker)
			reached := filepath.Join(home, qualificationbarrier.BuildResponseReachedMarker)
			if err := os.WriteFile(armed, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			port := &nativeGuardMutationPort{build: NativeDeliveryBuild{
				ID: uuid.New(), PlanID: uuid.New(), PlanDigest: nativeDigest('a'), SourceDigest: nativeDigest('b'), ExecutionDigest: nativeDigest('c'),
				PhysicalPoolID: "pool", WriterLeaseID: uuid.New(), CandidateID: uuid.New(), SealID: uuid.New(), ServingArtifactID: "artifact-native", ServingArtifactDigest: nativeDigest('d'),
				ServingStateID: uuid.New(), Status: string(deploymentgen.DeliveryBuildStatusSealed), CreatedAt: now, UpdatedAt: now, TerminalAt: now, Revision: 7, CandidateRevision: 13,
			}}
			if failedCompletion {
				port.completeErr = errors.New("durable evidence mismatch")
			}
			m := nativeDeliveryHandlerModule(port)
			m.instanceEnvironment = servingstate.Environment("evaluation")
			var err error
			m.projectClaims, err = deployment.NewProjectClaimService(&candidateProjectClaimRepositoryStub{claim: deployment.ProjectClaim{
				ProjectID: "finance", Environment: "evaluation", ClaimedBy: "bootstrap-admin", ClaimedAt: now,
			}})
			if err != nil {
				t.Fatal(err)
			}
			m.handler = deploymenthttp.NewHandler(deploymenthttp.Options{InstanceEnvironment: "evaluation", CurrentPrincipal: func(*http.Request) (deploymenthttp.Principal, bool) {
				return deploymenthttp.Principal{ID: "operator"}, true
			}})
			ctx, guard := nativeGuardContext(t, deploymentgen.GenCommandOperationBuildDeliveryPlan().APIGenOperationID())
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				m.BuildDeliveryPlan(response, httptest.NewRequestWithContext(ctx, http.MethodPost, "/", nil), "finance", port.build.PlanID.String(), "build-key")
			}()
			if failedCompletion {
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("failed verification reached blocking boundary")
				}
				if _, err := os.Stat(reached); !os.IsNotExist(err) {
					t.Fatalf("failed verification claimed sealed boundary: %v", err)
				}
				if guard.Completed() || response.Code == http.StatusOK {
					t.Fatal("failed command accepted")
				}
				return
			}
			deadline := time.After(5 * time.Second)
		wait:
			for {
				if _, err := os.Stat(reached); err == nil {
					break
				}
				select {
				case <-done:
					t.Fatalf("response escaped boundary: %d %s", response.Code, response.Body.String())
				case <-deadline:
					t.Fatal("sealed build never reached response boundary")
				case <-time.After(time.Millisecond):
					continue wait
				}
			}
			select {
			case <-done:
				t.Fatal("response returned before interruption")
			default:
			}
			cancel()
			<-done
			if !guard.Completed() || port.buildCompletions != 1 {
				t.Fatal("boundary preceded verified command completion")
			}
			encoded, err := os.ReadFile(reached)
			if err != nil {
				t.Fatal(err)
			}
			var evidence qualificationbarrier.BuildResponseEvidence
			if err := json.Unmarshal(encoded, &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence.Boundary != qualificationbarrier.BuildResponseBoundary || evidence.PlanID != port.build.PlanID || evidence.BuildID != port.build.ID || evidence.CandidateID != port.build.CandidateID {
				t.Fatalf("interruption evidence differs from sealed build: %+v", evidence)
			}
			// Retained reached evidence must allow the exact sealed result on retry.
			replayCtx, replayGuard := nativeGuardContext(t, deploymentgen.GenCommandOperationBuildDeliveryPlan().APIGenOperationID())
			replayed := httptest.NewRecorder()
			m.BuildDeliveryPlan(replayed, httptest.NewRequestWithContext(replayCtx, http.MethodPost, "/", nil), "finance", port.build.PlanID.String(), "build-key")
			if replayed.Code != http.StatusOK || !replayGuard.Completed() || port.buildCompletions != 2 {
				t.Fatalf("sealed replay failed: %d %s", replayed.Code, replayed.Body.String())
			}
		})
	}
}
