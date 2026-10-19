package module

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
)

type logoutRedirectRepository struct {
	access.Repository
	revokedPrincipal string
}

func (repo *logoutRedirectRepository) RevokeSessionsForPrincipal(_ context.Context, principalID string) error {
	repo.revokedPrincipal = principalID
	return nil
}

func (repo *logoutRedirectRepository) RunAuditedMutation(_ context.Context, mutation func(access.Repository) (access.AuditEventInput, error)) error {
	_, err := mutation(repo)
	return err
}

func TestLogoutAllUsesSignedOutLandingForSSOOnly(t *testing.T) {
	provider := OIDCProviderConfig{ID: "oidc", IssuerURL: "https://issuer.example", ClientID: "client", ClientSecret: "secret", RedirectURL: "https://app.example/auth/oidc/callback"}
	for _, tt := range []struct {
		name      string
		local     bool
		providers []OIDCProviderConfig
		want      string
	}{
		{name: "SSO only", providers: []OIDCProviderConfig{provider}, want: "/login?error=signed_out"},
		{name: "local only", local: true, want: "/"},
		{name: "local and SSO", local: true, providers: []OIDCProviderConfig{provider}, want: "/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &logoutRedirectRepository{}
			auth := mustNewAuth(t, repo, AuthConfig{LocalAuth: tt.local, OIDCProviders: tt.providers})
			request := httptest.NewRequest(http.MethodPost, "/auth/logout-all", nil)
			request = request.WithContext(WithPrincipal(request.Context(), Principal{ID: "signed-in-admin"}))
			response := httptest.NewRecorder()
			auth.LogoutAll(response, request)
			if repo.revokedPrincipal != "signed-in-admin" {
				t.Fatalf("revoked principal = %q", repo.revokedPrincipal)
			}
			if response.Code != http.StatusFound || response.Header().Get("Location") != tt.want {
				t.Fatalf("bulk logout = %d %q, want 302 %q", response.Code, response.Header().Get("Location"), tt.want)
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Name != auth.SessionCookieName() || cookies[0].MaxAge >= 0 {
				t.Fatalf("logout did not expire session cookie: %#v", cookies)
			}
			single := httptest.NewRecorder()
			auth.Logout(single, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))
			if single.Header().Get("Location") != tt.want {
				t.Fatalf("single logout redirect = %q, want %q", single.Header().Get("Location"), tt.want)
			}
		})
	}
}
