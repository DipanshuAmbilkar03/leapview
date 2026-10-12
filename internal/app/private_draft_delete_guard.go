package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/flidai/leapview/internal/access"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	uitransport "github.com/flidai/leapview/internal/platform/web/transport"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	"github.com/go-chi/chi/v5"
)

// protectProjectDraftDeletion enables the owner exception only on the permanent
// private draft delete route. Archive keeps its exact dashboard.delete guard.
func protectProjectDraftDeletion(accessModule canonicalAccessModule, runtimeHost canonicalRuntimeHost, authorizer repositoryDashboardAuthorizer, next http.HandlerFunc) http.HandlerFunc {
	return protectProjectAuthoringResourceWithDeletionPolicy(accessModule, runtimeHost, authorizer, access.ActionDashboardDelete, func(r *http.Request) string { return chi.URLParam(r, "dashboard") }, true, next)
}

func protectProjectAuthoringResourceWithDeletionPolicy(
	accessModule canonicalAccessModule,
	runtimeHost canonicalRuntimeHost,
	authorizer repositoryDashboardAuthorizer,
	action access.Action,
	dashboardIDForRequest func(*http.Request) string,
	allowOwnerDraftDelete bool,
	next http.HandlerFunc,
) http.HandlerFunc {
	if accessModule == nil || runtimeHost == nil || dashboardIDForRequest == nil || action == "" || next == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		}
	}
	if action != access.ActionDashboardRead && action != access.ActionDashboardUpdate && action != access.ActionDashboardDelete {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		}
	}
	if action != access.ActionDashboardRead && authorizer == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		}
	}
	return accessModule.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := accessModule.CurrentPrincipal(r)
		if !ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		projectID := runtimeHost.ProjectID()
		if err := projectID.Validate(); err != nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		dashboardID := strings.TrimSpace(dashboardIDForRequest(r))
		if err := authoring.ValidateDashboardID(authoring.DashboardID(dashboardID)); err != nil {
			http.NotFound(w, r)
			return
		}
		if principal.DevBypass {
			next(w, r)
			return
		}
		if action == access.ActionDashboardDelete && allowOwnerDraftDelete {
			err := authorizeDashboardDeletion(r, accessModule, runtimeHost, authorizer, projectID, dashboardID)
			if errors.Is(err, access.ErrForbidden) {
				uitransport.WriteBrowserAuthorizationError(w, r, http.StatusForbidden)
				return
			}
			if errors.Is(err, authoring.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
				return
			}
			next(w, r)
			return
		}
		resource, resourceErr := access.NewResourceRef(projectgraph.ResourceID(dashboardID), projectgraph.KindDashboard)
		if resourceErr != nil {
			http.NotFound(w, r)
			return
		}
		// The exact action is checked against principal/group assignments and,
		// for bearer credentials, the token's typed permission ceiling.
		// Private drafts can be absent from the published graph. Only this
		// dashboard-authoring guard admits that absence; the repository check
		// below must still prove the principal may edit or manage this draft.
		typed, allowed, typedErr := authorizeTypedResourceActionWithDraft(r.Context(), accessModule, runtimeHost, principal.ID, projectID, []access.ResourceRef{resource}, action, action == access.ActionDashboardUpdate || action == access.ActionDashboardDelete)
		if typedErr != nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		if !typed || !allowed {
			uitransport.WriteBrowserAuthorizationError(w, r, http.StatusForbidden)
			return
		}
		var err error
		switch action {
		case access.ActionDashboardUpdate:
			err = authorizer.AuthorizeDashboardEdit(r.Context(), projectID, principal.ID, authoring.DashboardID(dashboardID))
		case access.ActionDashboardDelete:
			err = authorizer.AuthorizeDashboardManage(r.Context(), projectID, principal.ID, authoring.DashboardID(dashboardID))
		}
		if err != nil {
			if errors.Is(err, access.ErrForbidden) {
				uitransport.WriteBrowserAuthorizationError(w, r, http.StatusForbidden)
				return
			}
			if errors.Is(err, authoring.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	})).ServeHTTP
}
