package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/platform/http/cursorsigning"
	"github.com/flidai/leapview/internal/platform/http/idempotency"
)

// Model the durable HTTP record left by process loss. Native plan/build
// handlers must reach their own operation ledger; ordinary commands must
// retain the protocol's fail-closed quarantine.
type interruptedNativeHTTPStore struct {
	compositionCountingIdempotencyStore
}

func (s *interruptedNativeHTTPStore) Claim(_ context.Context, _, digest, _ string, _, _ time.Duration) (idempotency.Record, bool, error) {
	s.claims.Add(1)
	return idempotency.Record{State: "indeterminate", Digest: digest}, false, nil
}

func TestNativeDeliveryRetryReachesDomainAfterLostHTTPResponse(t *testing.T) {
	for _, tc := range []struct {
		name, path  string
		wantHandler bool
	}{
		{"plan", "/api/v1/projects/project/delivery", true},
		{"build", "/api/v1/projects/project/delivery/plans/0198f2c0-7c7a-7f00-8a11-000000000001/build", true},
		{"publication", "/api/v1/projects/project/delivery/candidates/0198f2c0-7c7a-7f00-8a11-000000000001/publish", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &interruptedNativeHTTPStore{}
			platform := &platformServices{}
			if err := configureAPIProtocol(&capabilityRoutes{}, testAPIProtocolRuntime(), platform, &httpPolicy{}, runtimeAssemblyInputs{InstanceID: "target:test", DefaultEnvironment: "test"}, t.Context(), apiProtocolPersistence{
				Idempotency: store, CursorSigning: cursorsigning.NewEphemeralInitializer(),
				BypassDurableIdempotency: nativeAPIIdempotencyOwners(),
			}); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer credential")
			request.Header.Set("Idempotency-Key", "same-interrupted-operation")
			request.Header.Set("Content-Type", "application/json")
			called := false
			response := httptest.NewRecorder()
			platform.apiProtocol.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(response, request)
			if called != tc.wantHandler {
				t.Fatalf("handler reached=%t, want %t; response=%d %s", called, tc.wantHandler, response.Code, response.Body.String())
			}
			if tc.wantHandler && store.claims.Load() != 0 {
				t.Fatal("native operation acquired a second HTTP replay authority")
			}
			if !tc.wantHandler && (response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "IDEMPOTENCY_OUTCOME_UNKNOWN")) {
				t.Fatalf("ordinary command quarantine weakened: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
