package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/flidai/leapview/internal/access"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// dashboardDeletionAllowed projects the delete route's permission decision
// into the catalog. Its result is an affordance, never mutation authority.
func dashboardDeletionAllowed(r *http.Request, accessModule canonicalAccessModule, runtimeHost canonicalRuntimeHost, authorizer repositoryDashboardAuthorizer, projectID projectgraph.ResourceID, dashboardID string) (bool, error) {
	err := authorizeDashboardDeletion(r, accessModule, runtimeHost, authorizer, projectID, dashboardID)
	if errors.Is(err, access.ErrForbidden) || errors.Is(err, authoring.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

type repositoryDashboardDeleteAuthorizer interface {
	AuthorizeDashboardDelete(context.Context, projectgraph.ResourceID, string, authoring.DashboardID) error
}

// authorizeDashboardDeletion is shared by catalog affordances and the delete
// route. The canonical application proves the durable owner/lifecycle before
// applying the private draft policy. Older authorizers retain exact delete checks.
func authorizeDashboardDeletion(r *http.Request, accessModule canonicalAccessModule, runtimeHost canonicalRuntimeHost, authorizer repositoryDashboardAuthorizer, projectID projectgraph.ResourceID, dashboardID string) error {
	if r == nil || accessModule == nil || runtimeHost == nil || authorizer == nil {
		return access.ErrForbidden
	}
	principal, ok := accessModule.CurrentPrincipal(r)
	if !ok {
		return access.ErrForbidden
	}
	resource, err := access.NewResourceRef(projectgraph.ResourceID(dashboardID), projectgraph.KindDashboard)
	if err != nil {
		return err
	}
	if principal.DevBypass {
		return nil
	}
	if deletion, ok := authorizer.(repositoryDashboardDeleteAuthorizer); ok {
		return deletion.AuthorizeDashboardDelete(r.Context(), projectID, principal.ID, authoring.DashboardID(dashboardID))
	}
	typed, allowed, err := authorizeTypedResourceActionWithDraft(r.Context(), accessModule, runtimeHost, principal.ID, projectID, []access.ResourceRef{resource}, access.ActionDashboardDelete, true)
	if err != nil {
		return err
	}
	if !typed || !allowed {
		return access.ErrForbidden
	}
	return authorizer.AuthorizeDashboardManage(r.Context(), projectID, principal.ID, authoring.DashboardID(dashboardID))
}
