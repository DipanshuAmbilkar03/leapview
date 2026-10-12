package app

import (
	"errors"
	"net/http"

	"github.com/flidai/leapview/internal/access"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// dashboardDeletionAllowed projects the delete route's permission decision
// into the catalog. Its result is an affordance, never mutation authority.
func dashboardDeletionAllowed(r *http.Request, accessModule canonicalAccessModule, runtimeHost canonicalRuntimeHost, authorizer repositoryDashboardAuthorizer, projectID projectgraph.ResourceID, dashboardID string) (bool, error) {
	if r == nil || accessModule == nil || runtimeHost == nil || authorizer == nil {
		return false, nil
	}
	principal, ok := accessModule.CurrentPrincipal(r)
	if !ok {
		return false, nil
	}
	resource, err := access.NewResourceRef(projectgraph.ResourceID(dashboardID), projectgraph.KindDashboard)
	if err != nil {
		return false, err
	}
	if principal.DevBypass {
		return true, nil
	}
	typed, allowed, err := authorizeTypedResourceActionWithDraft(r.Context(), accessModule, runtimeHost, principal.ID, projectID, []access.ResourceRef{resource}, access.ActionDashboardDelete, true)
	if err != nil || !typed || !allowed {
		return false, err
	}
	err = authorizer.AuthorizeDashboardManage(r.Context(), projectID, principal.ID, authoring.DashboardID(dashboardID))
	if errors.Is(err, access.ErrForbidden) || errors.Is(err, authoring.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
