package module

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/access"
	"github.com/flidai/leapview/pkg/pagestream"
)

// Exercise the authentication/stream boundary used by chat against the durable
// PostgreSQL authority, including patches queued before a credential changes.
func TestAuthenticatedStreamRejectsPatchAfterSessionRevocation(t *testing.T) {
	for _, kind := range []string{"session", "api-token"} {
		for _, change := range []string{"revoke", "expire", "block", "authority-unavailable"} {
			t.Run(kind+"/"+change, func(t *testing.T) {
				store := testStore(t)
				principal, err := store.repository.UpsertPrincipal(t.Context(), access.PrincipalInput{Email: "stream-revocation@example.com"})
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodGet, "/updates?route=chat", nil)
				var credentialID, secret string
				if kind == "session" {
					secret, err = store.repository.CreateSession(t.Context(), principal.ID, time.Hour)
					if err != nil {
						t.Fatal(err)
					}
					session, err := store.repository.CredentialForSessionToken(t.Context(), secret)
					if err != nil {
						t.Fatal(err)
					}
					credentialID = session.ID
					request.AddCookie(&http.Cookie{Name: "lv_session", Value: secret})
				} else {
					var token access.APIToken
					secret, token, err = store.repository.CreateScopedAPITokenWithMetadata(t.Context(), access.ScopedAPITokenInput{PrincipalID: principal.ID, Name: "stream", Permissions: []access.PermissionPair{}, ExpiresAt: time.Now().Add(time.Hour)})
					if err != nil {
						t.Fatal(err)
					}
					credentialID = token.ID
					request.Header.Set("Authorization", "Bearer "+secret)
				}
				auth := mustNewAuth(t, store.repository, AuthConfig{LocalAuth: true})
				module, err := newSurface(surfaceConfig{Repository: func() (access.Repository, error) { return store.repository, nil }, Auth: auth})
				if err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				var called bool
				module.AuthenticateStream(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					stream := pagestream.NewSignalStream(w, r)
					if err := stream.Patch(pagestream.SignalPatch{"beforeRevocation": true}); err != nil {
						t.Fatal(err)
					}
					pending := make(chan pagestream.SignalPatch, 1)
					pending <- pagestream.SignalPatch{"privateAfterRevocation": "must-not-be-delivered"}
					close(pending)
					switch change {
					case "revoke":
						if kind == "session" {
							err = store.repository.RevokeSessionForPrincipal(t.Context(), principal.ID, credentialID)
						} else {
							err = store.repository.RevokeAPITokenForPrincipal(t.Context(), principal.ID, credentialID)
						}
					case "expire":
						table := "access.session"
						if kind == "api-token" {
							table = "access.api_token"
						}
						_, err = store.pool.Exec(t.Context(), "UPDATE "+table+" SET expires_at=clock_timestamp()-interval '1 microsecond' WHERE id=$1::uuid", credentialID)
					case "block":
						_, err = store.pool.Exec(t.Context(), "UPDATE access.principal SET blocked_at=clock_timestamp() WHERE id=$1::uuid", principal.ID)
					case "authority-unavailable":
						store.pool.Close()
					}
					if err != nil {
						t.Fatal(err)
					}
					if _, _, ok := auth.authenticate(request); ok {
						t.Fatal("changed credential still authenticates a new request")
					}
					if err := stream.ForwardUpdates(r.Context(), pending); err == nil {
						t.Fatal("forward accepted a patch after credential invalidation")
					}
					if r.Context().Err() == nil {
						t.Fatal("rejected stream did not cancel its request")
					}
				})).ServeHTTP(response, request)
				if !called || !strings.Contains(response.Body.String(), "beforeRevocation") {
					t.Fatal("authenticated stream did not send its initial patch")
				}
				if strings.Contains(response.Body.String(), "privateAfterRevocation") {
					t.Fatal("already-open stream delivered private data after durable credential invalidation")
				}
			})
		}
	}
}

func TestAuthenticatedIdleStreamClosesAfterRevocation(t *testing.T) {
	store := testStore(t)
	principal, err := store.repository.UpsertPrincipal(t.Context(), access.PrincipalInput{Email: "idle-stream@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.repository.CreateSession(t.Context(), principal.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.repository.CredentialForSessionToken(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	auth := mustNewAuth(t, store.repository, AuthConfig{LocalAuth: true})
	module, err := newSurface(surfaceConfig{Repository: func() (access.Repository, error) { return store.repository, nil }, Auth: auth})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?route=chat", nil)
	request.AddCookie(&http.Cookie{Name: "lv_session", Value: token})
	response := httptest.NewRecorder()
	var called bool
	module.AuthenticateStream(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		stream := pagestream.NewSignalStream(w, r)
		if err := stream.Patch(pagestream.SignalPatch{"beforeRevocation": true}); err != nil {
			t.Fatal(err)
		}
		if err := store.repository.RevokeSessionForPrincipal(t.Context(), principal.ID, session.ID); err != nil {
			t.Fatal(err)
		}
		stream.Wait(r.Context())
		if !errors.Is(r.Context().Err(), context.Canceled) {
			t.Fatal("idle stream did not cancel after revoked-session keepalive")
		}
	})).ServeHTTP(response, request)
	if !called {
		t.Fatal("stream never opened")
	}
	if strings.Contains(response.Body.String(), ": keepalive") {
		t.Fatal("idle revoked stream sent a keepalive")
	}
}

func TestAuthenticatedStreamRequiresDurableEvidenceOutsideDevelopment(t *testing.T) {
	for _, test := range []struct {
		name      string
		principal Principal
		allowed   bool
	}{
		{name: "explicit development", principal: LocalDeveloperPrincipal(), allowed: true},
		{name: "missing durable evidence", principal: Principal{ID: "injected-user"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			module := browserGuardModule(nil, test.principal, true)
			response := httptest.NewRecorder()
			var called bool
			module.AuthenticateStream(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				err := pagestream.NewSignalStream(w, r).Patch(pagestream.SignalPatch{"private": true})
				if (err == nil) != test.allowed {
					t.Fatalf("patch error = %v, allowed = %t", err, test.allowed)
				}
			})).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/updates?route=chat", nil))
			if !called || strings.Contains(response.Body.String(), "private") != test.allowed {
				t.Fatal("unexpected stream delivery")
			}
		})
	}
}
