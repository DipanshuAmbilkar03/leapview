package module

import (
	"net/http"
	"strings"

	"github.com/flidai/leapview/internal/access"
)

// protectInstance applies the platform role baseline to an instance-scoped
// operation and then attenuates it with the generated typed pair when the
// caller supplied a typed API token. Instance authority is durable and
// instance-audience based; it is never read from a project snapshot.
func (a *APIGenAuthorizer) protectInstance(operationID string, next http.Handler) http.Handler {
	return a.module.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := a.module.CurrentPrincipal(r)
		if !ok || strings.TrimSpace(principal.ID) == "" {
			writeAuthorizationStatus(w, r, http.StatusUnauthorized)
			return
		}
		// The first project claim is a one-time credential exchange, not an
		// ordinary platform-admin operation. A browser session (including a
		// development bypass session) cannot consume the claim authority.
		claimOperation := operationID == "bootstrapProjectClaim" || operationID == "exchangeProjectClaimPublisher"
		if claimOperation && bearerToken(r) == "" {
			writeAuthorizationStatus(w, r, http.StatusUnauthorized)
			return
		}
		if principal.DevBypass && !claimOperation {
			next.ServeHTTP(w, r)
			return
		}
		allowed, err := a.module.IsPlatformAdmin(r.Context(), principal.ID)
		if err != nil {
			writeAuthorizationStatus(w, r, http.StatusServiceUnavailable)
			return
		}
		if !allowed {
			writeAuthorizationStatus(w, r, http.StatusForbidden)
			return
		}
		credential, hasCredential := a.module.requestCredential(r)
		if !hasCredential {
			if claimOperation {
				writeAuthorizationStatus(w, r, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		// Authoring credentials and legacy capability-only API tokens cannot
		// invoke an operation that has migrated to instance typed authority.
		if credential.Authoring != nil || strings.TrimSpace(credential.Token.ID) == "" || credential.Token.PrincipalID != principal.ID || credential.Principal.ID != principal.ID {
			writeAuthorizationStatus(w, r, http.StatusForbidden)
			return
		}
		if credential.Token.PermissionProfile != access.PermissionCatalogProfile || credential.Token.Permissions == nil {
			writeAuthorizationStatus(w, r, http.StatusForbidden)
			return
		}
		instanceID := a.instance(r)
		pairs, err := a.typed.ResolveInstancePairs(operationID, instanceID)
		if err != nil || !permissionPairsAllowAll(credential.Token.Permissions, pairs) {
			writeAuthorizationStatus(w, r, http.StatusForbidden)
			return
		}
		requirement, ok := a.typedRequirement(operationID)
		if !ok {
			writeAuthorizationStatus(w, r, http.StatusForbidden)
			return
		}
		ctx := withInstanceAuthorization(r.Context(), instanceAuthorization{
			OperationID: operationID,
			PrincipalID: principal.ID,
			InstanceID:  instanceID,
			Action:      requirement.Action,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	}))
}
