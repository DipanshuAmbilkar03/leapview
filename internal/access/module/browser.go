package module

import (
	"net/http"
	"strings"

	accessui "github.com/flidai/leapview/internal/access/ui"
	"github.com/gorilla/csrf"
)

// Login renders the access-owned authentication entrypoint.
func (m *Module) Login(w http.ResponseWriter, r *http.Request) {
	if m != nil && m.auth != nil {
		if principal, _, ok := m.auth.Authenticate(r); ok && !m.auth.MustChangeLocalPassword(r, principal.ID) {
			http.Redirect(w, r, m.auth.authenticationRedirectTarget(w, r, "/"), http.StatusFound)
			return
		}
		if m.auth.SSOConfigured() && !m.auth.LocalAuthEnabled() && loginErrorCode(r) == "" {
			http.Redirect(w, r, m.auth.ssoLoginHref(), http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := accessui.LoginPage(m.LoginPageOptions(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (m *Module) LoginPageOptions(r *http.Request) accessui.LoginPageOptions {
	options := accessui.LoginPageOptions{
		LocalAuth:        m != nil && m.auth != nil && m.auth.LocalAuthEnabled(),
		DevelopmentLogin: m != nil && m.auth != nil && m.auth.DevelopmentLoginEnabled(),
		SSOAuth:          m == nil || m.auth == nil || m.auth.SSOConfigured(),
		ProviderLabel:    "Sign in with Microsoft",
		Presentation:     m.presentation,
		Assets:           m.assets,
		Error:            loginErrorMessage(r),
		ErrorCode:        loginErrorCode(r),
	}
	if m == nil || m.auth == nil {
		return options
	}
	options.CSRFToken = csrf.Token(r)
	options.ProviderHref = m.auth.ssoLoginHref()
	options.ProviderLabel = m.auth.ssoProviderLabel()
	if principal, _, ok := m.auth.Authenticate(r); ok {
		options.MustChangePassword = m.auth.MustChangeLocalPassword(r, principal.ID)
	}
	return options
}

// loginErrorMessage maps the deliberately small set of login redirect
// outcomes to branded, accessible copy. Unknown values are ignored so an
// attacker cannot inject arbitrary text into the login page through a URL.
func loginErrorMessage(r *http.Request) string {
	switch loginErrorCode(r) {
	case "invalid_credentials":
		return "Invalid email or password. Check your details and try again."
	case "session_expired":
		return "Your session expired. Sign in again to continue."
	case "forbidden":
		return "You do not have permission to access that page."
	case "signed_out":
		return "You have signed out of LeapView."
	case "sso_cancelled":
		return "Sign-in was not completed. Sign in again to continue."
	case "sso_failed":
		return "We could not complete your organization sign-in. Try again or contact your administrator."
	case "sso_unavailable":
		return "Your organization sign-in is unavailable. Contact your administrator."
	default:
		return ""
	}
}

func loginErrorCode(r *http.Request) string {
	if r == nil {
		return ""
	}
	switch code := strings.TrimSpace(r.URL.Query().Get("error")); code {
	case "invalid_credentials", "session_expired", "forbidden", "signed_out", "sso_cancelled", "sso_failed", "sso_unavailable":
		return code
	default:
		return ""
	}
}

func (m *Module) LoginBootstrapSignals(r *http.Request) map[string]any {
	return accessui.LoginBootstrapSignalsForOptions(m.LoginPageOptions(r))
}

func (m *Module) Begin(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.auth == nil {
		http.NotFound(w, r)
		return
	}
	m.auth.Begin(w, r)
}

func (m *Module) Callback(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.auth == nil {
		http.NotFound(w, r)
		return
	}
	m.auth.Callback(w, r)
}

func (m *Module) LocalLogin(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.auth == nil {
		http.NotFound(w, r)
		return
	}
	m.auth.LocalLogin(w, r)
}

func (m *Module) DevelopmentLogin(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.auth == nil {
		http.NotFound(w, r)
		return
	}
	m.auth.DevelopmentLogin(w, r)
}

func (m *Module) LocalPassword(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.auth == nil {
		http.NotFound(w, r)
		return
	}
	m.auth.LocalPassword(w, r)
}

func (m *Module) Logout(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.auth == nil {
		http.NotFound(w, r)
		return
	}
	m.auth.Logout(w, r)
}

func (m *Module) LogoutAll(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.auth == nil {
		http.NotFound(w, r)
		return
	}
	m.auth.LogoutAll(w, r)
}
