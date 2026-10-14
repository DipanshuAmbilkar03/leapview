package module

import (
	"context"
	"net/http"
	"time"

	"github.com/flidai/leapview/internal/access"
)

// AuthenticateStream rechecks durable credential and principal state before
// every response write, including idle keepalives. Revocation closes the
// established stream instead of only rejecting its next reconnect.
func (m *Module) AuthenticateStream(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		r = r.WithContext(ctx)
		next.ServeHTTP(&authenticatedStreamWriter{ResponseWriter: w, check: func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := m.recheckStreamCredential(r); err != nil {
				cancel()
				return err
			}
			return nil
		}}, r)
	}))
}

type authenticatedStreamWriter struct {
	http.ResponseWriter
	check func() error
}

// Unwrap preserves ResponseController flushing and deadline support.
func (w *authenticatedStreamWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *authenticatedStreamWriter) Write(p []byte) (int, error) {
	if err := w.check(); err != nil {
		return 0, err
	}
	return w.ResponseWriter.Write(p)
}

func (m *Module) recheckStreamCredential(r *http.Request) error {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok || principal.ID == "" {
		return access.ErrForbidden
	}
	if principal.DevBypass {
		return nil
	}
	if m.auth == nil {
		return access.ErrForbidden
	}
	credential, hasAPI := APICredentialFromContext(r.Context())
	session, hasSession := SessionCredentialEvidenceFromContext(r.Context())
	if hasAPI && hasSession {
		return access.ErrForbidden
	}
	if hasSession {
		reader, ok := m.auth.sessions.(access.SessionAuthorityEvidenceReader)
		if !ok || session.Class != "session" || session.ID == "" || session.Fingerprint == "" || session.PrincipalID != principal.ID {
			return access.ErrForbidden
		}
		now := time.Now().UTC()
		current, err := reader.SessionAuthorityEvidence(r.Context(), principal.ID, session.ID, session.Fingerprint, now)
		if err != nil || current.ID != session.ID || current.PrincipalID != principal.ID || current.TokenFingerprint != session.Fingerprint ||
			current.RevokedAt != "" || !permissionExpiresAfter(current.ExpiresAt, now) {
			return access.ErrForbidden
		}
		return nil
	}
	if hasAPI && credential.Authoring == nil {
		reader, ok := m.auth.repo.(access.APITokenAuthorityEvidenceReader)
		if !ok || credential.Token.ID == "" || credential.Token.TokenFingerprint == "" || credential.Principal.ID != principal.ID || credential.Token.PrincipalID != principal.ID {
			return access.ErrForbidden
		}
		now := time.Now().UTC()
		current, err := reader.APITokenAuthorityEvidence(r.Context(), principal.ID, credential.Token.ID, now)
		if err != nil || current.ID != credential.Token.ID || current.PrincipalID != principal.ID || current.TokenFingerprint != credential.Token.TokenFingerprint ||
			current.RevokedAt != "" || !permissionExpiresAfter(current.ExpiresAt, now) {
			return access.ErrForbidden
		}
		return nil
	}
	// Authoring credentials have a separate durable authority. Re-resolve them
	// through that authority instead of interpreting them as ordinary API tokens.
	if hasAPI && credential.Authoring != nil {
		current, currentCredential, ok := m.auth.authenticate(r)
		if ok && current.ID == principal.ID && currentCredential != nil && currentCredential.Authoring != nil && currentCredential.Token.ID == credential.Token.ID {
			return nil
		}
	}
	return access.ErrForbidden
}
