package module

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/flidai/leapview/internal/access"
)

// RevalidateStream closes authenticated page streams when their credential or
// principal loses access. Route authorization must run before this middleware.
func (m *Module) RevalidateStream(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Preserve the explicit local/injected authority configurations.
		if m != nil && (m.auth == nil || m.auth.devBypass) {
			next.ServeHTTP(w, r)
			return
		}
		if m == nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		principal, ok := m.CurrentPrincipal(r)
		if !ok || principal.ID == "" {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		r = r.WithContext(ctx)
		check := m.streamCredentialCheck(r, principal.ID)
		guard := &streamResponseWriter{ResponseWriter: w, ctx: ctx, cancel: cancel, check: check}
		if !guard.valid() {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if !guard.valid() {
						return
					}
				}
			}
		}()
		next.ServeHTTP(guard, r)
	})
}

func (m *Module) streamCredentialCheck(r *http.Request, principalID string) func(context.Context) bool {
	a := m.auth
	credential, hasAPI := APICredentialFromContext(r.Context())
	evidence, hasSession := SessionCredentialEvidenceFromContext(r.Context())
	var lifecycle func(context.Context) bool
	switch {
	case hasAPI && hasSession:
		lifecycle = func(context.Context) bool { return false }
	case hasAPI && credential.Authoring != nil:
		token := bearerToken(r)
		lifecycle = func(ctx context.Context) bool {
			if a.authoringAuth == nil {
				return false
			}
			current, err := a.authoringAuth.Resolve(ctx, token)
			return err == nil && current.ID == credential.Token.ID && current.Principal.ID == principalID && current.Session.ID == credential.Authoring.ID
		}
	case hasAPI:
		reader, supported := a.repo.(access.APITokenAuthorityEvidenceReader)
		lifecycle = func(ctx context.Context) bool {
			if !supported {
				return false
			}
			now := time.Now().UTC()
			current, err := reader.APITokenAuthorityEvidence(ctx, principalID, credential.Token.ID, now)
			if err != nil || current.ID != credential.Token.ID || current.PrincipalID != principalID || current.TokenFingerprint == "" || current.TokenFingerprint != credential.Token.TokenFingerprint || current.RevokedAt != "" || !permissionExpiresAfter(current.ExpiresAt, now) || current.PermissionProfile != access.PermissionCatalogProfile || current.Permissions == nil || access.ValidatePermissionPairs(current.Permissions) != nil {
				return false
			}
			for _, pair := range credential.Token.Permissions {
				if !access.PermissionSetAllows(current.Permissions, pair) {
					return false
				}
			}
			return true
		}
	case hasSession:
		reader, supported := a.sessions.(access.SessionAuthorityEvidenceReader)
		lifecycle = func(ctx context.Context) bool {
			if !supported || evidence.PrincipalID != principalID {
				return false
			}
			now := time.Now().UTC()
			current, err := reader.SessionAuthorityEvidence(ctx, principalID, evidence.ID, evidence.Fingerprint, now)
			return err == nil && current.ID == evidence.ID && current.PrincipalID == principalID && current.TokenFingerprint == evidence.Fingerprint && current.RevokedAt == "" && permissionExpiresAfter(current.ExpiresAt, now)
		}
	default:
		// Desktop cookies have no browser-session evidence. Resolve the binding
		// once, then read its lifecycle without renewing its idle deadline.
		if desktop, supported := a.sessions.(access.DesktopSessionRepository); supported {
			cookie, err := r.Cookie(a.SessionCookieName())
			if err == nil {
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				binding, err := desktop.DesktopSessionForToken(ctx, cookie.Value)
				cancel()
				if err == nil && binding.PrincipalID == principalID {
					reader, supported := a.sessions.(interface {
						DesktopSessionAuthorityEvidence(context.Context, string, string, time.Time) (access.Session, error)
					})
					lifecycle = func(ctx context.Context) bool {
						if !supported {
							return false
						}
						now := time.Now().UTC()
						current, err := reader.DesktopSessionAuthorityEvidence(ctx, principalID, binding.SessionID, now)
						return err == nil && current.ID == binding.SessionID && current.PrincipalID == principalID && current.Kind == access.SessionKindDesktop && current.RevokedAt == "" && permissionExpiresAfter(current.ExpiresAt, now) && permissionExpiresAfter(current.AbsoluteExpiresAt, now)
					}
				}
			}
		}
	}
	return func(ctx context.Context) bool {
		if lifecycle == nil || a.repo == nil || !lifecycle(ctx) {
			return false
		}
		current, err := a.repo.PrincipalByID(ctx, principalID)
		if err != nil || current.ID != principalID || current.AccessDisabled() {
			return false
		}
		var api *access.APICredential
		if hasAPI {
			api = &credential
		}
		return !a.mustChangeLocalPassword(r.WithContext(ctx), principalID, api)
	}
}

type streamResponseWriter struct {
	http.ResponseWriter
	ctx    context.Context
	cancel context.CancelFunc
	check  func(context.Context) bool
	mu     sync.Mutex
}

func (w *streamResponseWriter) valid() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.validLocked()
}

func (w *streamResponseWriter) validLocked() bool {
	if w.ctx.Err() != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()
	if !w.check(ctx) {
		w.cancel()
		return false
	}
	return true
}

func (w *streamResponseWriter) Write(p []byte) (int, error) {
	if !w.valid() || w.ctx.Err() != nil {
		return 0, context.Canceled
	}
	return w.ResponseWriter.Write(p)
}

// Datastar flushes headers during initialization and panics on flush errors.
// Cancellation is communicated through the request context and later writes.
func (w *streamResponseWriter) Flush() {
	if w.ctx.Err() == nil {
		if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
			w.cancel()
		}
	}
}

func (w *streamResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
