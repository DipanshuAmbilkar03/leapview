package module

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/flidai/leapview/internal/access"
	oidcauth "github.com/flidai/leapview/internal/access/oidc"
)

func (a *Auth) ssoProviderID() string {
	ids := a.oidcRegistry.ProviderIDs()
	for _, id := range ids {
		if id == "azureadv2" {
			return id
		}
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return "azureadv2"
}

func (a *Auth) ssoLoginHref() string {
	return "/auth/" + url.PathEscape(a.ssoProviderID())
}

func (a *Auth) ssoProviderLabel() string {
	if cfg, ok := a.oidcRegistry.Config(a.ssoProviderID()); ok {
		issuer, _ := url.Parse(cfg.IssuerURL)
		if cfg.ID == "azureadv2" || issuer != nil && strings.EqualFold(issuer.Hostname(), "login.microsoftonline.com") {
			return "Sign in with Microsoft"
		}
	}
	return "Sign in with your organization"
}

func (a *Auth) redirectSSOError(w http.ResponseWriter, r *http.Request, code string) {
	// Document navigations recover on the branded login surface; protocol
	// clients retain status-based failures without following a browser flow.
	if !strings.Contains(r.Header.Get("Accept"), "text/html") || wantsJSON(r) {
		status, message := http.StatusUnauthorized, "authentication failed"
		if code == "sso_unavailable" {
			status, message = http.StatusServiceUnavailable, "identity provider unavailable"
		}
		http.Error(w, message, status)
		return
	}
	http.Redirect(w, r, "/login?error="+url.QueryEscape(code), http.StatusSeeOther)
}

// A retained local credential cannot bypass external identity management.
func locallyManagedIdentity(ctx context.Context, repo access.Repository, principalID string) bool {
	reader, ok := repo.(access.PrincipalIdentityManagementRepository)
	if !ok {
		return false
	}
	identity, err := reader.PrincipalIdentityManagement(ctx, principalID)
	return err == nil && identity.Source == access.IdentityManagementLocal
}

func (a *Auth) configureSSO(cfg AuthConfig) error {
	providers := make([]oidcauth.Config, 0, len(cfg.OIDCProviders))
	for _, provider := range cfg.OIDCProviders {
		providers = append(providers, oidcauth.Config{
			ID: provider.ID, IssuerURL: provider.IssuerURL,
			ClientID: provider.ClientID, ClientSecret: provider.ClientSecret,
			RedirectURL: provider.RedirectURL, Scopes: append([]string(nil), provider.Scopes...),
		})
	}
	if cfg.AzureClientID != "" && cfg.AzureSecret != "" && cfg.AzureCallback != "" && !hasOIDCProvider(providers, "azureadv2") {
		providers = append(providers, oidcauth.AzureProviderConfig(cfg.AzureClientID, cfg.AzureSecret, cfg.AzureCallback, cfg.AzureTenant))
	}
	registry, err := oidcauth.NewRegistry(providers)
	if err != nil {
		return err
	}
	a.oidcRegistry = registry
	a.configured = registry.Configured()
	return nil
}

func (a *Auth) defaultLoginRedirect() string {
	if a.localAuth || !a.SSOConfigured() {
		return "/login"
	}
	return a.ssoLoginHref()
}
