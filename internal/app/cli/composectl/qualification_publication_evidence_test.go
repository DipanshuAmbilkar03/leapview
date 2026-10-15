package composectl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A durable publication can commit before runtime acquisition is ready. Exercise
// the actual generated HTTP client, including the middleware's plain-text 503.
func TestQualificationCanonicalEvidenceWait(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		structured bool
		persistent bool
		mismatch   string
		wantError  bool
	}{
		{name: "plain readiness 503", status: 503},
		{name: "structured readiness 503", status: 503, structured: true},
		{name: "plain rate limit 429", status: 429},
		{name: "structured rate limit 429", status: 429, structured: true},
		{name: "deadline", status: 503, persistent: true, wantError: true},
		{name: "unauthenticated", status: 401, wantError: true},
		{name: "forbidden", status: 403, wantError: true},
		{name: "missing", status: 404, wantError: true},
		{name: "conflict", status: 409, wantError: true},
		{name: "invalid", status: 422, wantError: true},
		{name: "publication changed on retry", status: 503, mismatch: "publication", wantError: true},
		{name: "plan changed on retry", status: 503, mismatch: "plan", wantError: true},
		{name: "candidate changed on retry", status: 503, mismatch: "candidate", wantError: true},
		{name: "generation changed on retry", status: 503, mismatch: "generation", wantError: true},
		{name: "generation no longer active", status: 503, mismatch: "status", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := QualificationCandidate{
				ID: "candidate:test", Revision: 7, TargetID: "target:test", PrincipalID: "principal:test",
				ArtifactDigest: "source-digest", ProvenanceDigest: "provenance-digest",
				PlanID: "plan:test", PlanDigest: "plan-digest",
			}
			pending := QualificationPublication{
				CandidateID: candidate.ID, DeploymentID: "publication:test", TargetID: candidate.TargetID,
				PlanID: candidate.PlanID, PlanDigest: candidate.PlanDigest,
			}
			committed := pending
			committed.GenerationID = "generation:test"
			committed.Status = "committed"
			reads := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method, "observation must never republish")
				require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
				parts := strings.Split(r.URL.Path, "/")
				require.Len(t, parts, 8)
				kind := parts[6]
				reads[kind]++
				body := map[string]any{
					"candidateId": candidate.ID, "planId": candidate.PlanID,
					"planDigest": candidate.PlanDigest, "targetId": candidate.TargetID,
				}
				switch kind {
				case "publications":
					body["id"], body["status"], body["generationId"] = pending.DeploymentID, "committed", committed.GenerationID
					if reads[kind] > 1 && test.mismatch == "publication" {
						body["generationId"] = "generation:other"
					}
				case "plans":
					body["sourceDigest"], body["provenanceDigest"] = candidate.ArtifactDigest, candidate.ProvenanceDigest
					if reads[kind] > 1 && test.mismatch == "plan" {
						body["provenanceDigest"] = "other-digest"
					}
				case "candidates":
					body["id"], body["status"], body["sourceDigest"] = candidate.ID, "ready", candidate.ArtifactDigest
					body["snapshotSealId"], body["servingArtifactDigest"] = "seal:test", "serving-digest"
					if reads[kind] > 1 && test.mismatch == "candidate" {
						body["planDigest"] = "other-digest"
					}
				case "generations":
					if reads[kind] == 1 || test.persistent {
						if test.structured {
							w.Header().Set("Content-Type", "application/problem+json")
							w.WriteHeader(test.status)
							require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"status": test.status, "code": "DELIVERY_READ_UNAVAILABLE", "title": http.StatusText(test.status)}))
						} else {
							http.Error(w, http.StatusText(test.status), test.status)
						}
						return
					}
					body["id"], body["status"], body["servingArtifactDigest"] = committed.GenerationID, "active", "serving-digest"
					if test.mismatch == "generation" {
						body["servingArtifactDigest"] = "other-digest"
					}
					if test.mismatch == "status" {
						body["status"] = "retired"
					}
				default:
					t.Errorf("unexpected evidence endpoint: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(body))
			}))
			defer server.Close()
			ctx := t.Context()
			if test.persistent {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			publication, deployment, err := waitQualificationCanonicalPublicationEvidence(
				ctx, server.Client(), qualificationAuthoringOptions{Target: server.URL, ProjectID: "project:test"},
				"token", candidate, pending, committed, time.Millisecond,
			)
			if test.wantError {
				require.Error(t, err)
				require.Zero(t, publication)
				require.Zero(t, deployment)
				if test.persistent {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				} else if test.mismatch != "" {
					require.Contains(t, err.Error(), "does not match")
					require.Equal(t, 2, reads["publications"], "identity mismatches must fail immediately")
				} else {
					require.Equal(t, 1, reads["publications"], "nontransient errors must fail immediately")
				}
				return
			}
			require.NoError(t, err)
			for _, kind := range []string{"publications", "plans", "candidates", "generations"} {
				require.Equal(t, 2, reads[kind], "all evidence must be revalidated: %s", kind)
			}
			require.NoError(t, verifyExactAuthoringCandidate(candidate, publication, deployment))
			require.Equal(t, committed.GenerationID, deployment.GenerationID)
			require.Equal(t, "seal:test", deployment.SnapshotSealID)
		})
	}
}
