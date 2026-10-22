package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/flidai/leapview/internal/access"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// missingResourceVisibleToPrincipal decides whether a denied request for resources that do not exist
// in the active serving generation may be answered 404 instead of 403.
//
// A 403 for a slug that exists must not become distinguishable from a 404, so existence is only
// disclosed to a viewer who could already enumerate that resource type: a platform administrator, or
// a principal holding the same action on at least one existing resource of the requested kind.
// Everyone else keeps the 403, which does not reveal whether the slug exists. Any failure to decide
// keeps the 403.
func missingResourceVisibleToPrincipal(
	ctx context.Context,
	r *http.Request,
	accessModule canonicalAccessModule,
	runtimeHost canonicalRuntimeHost,
	principalID string,
	projectID projectgraph.ResourceID,
	resources []access.ResourceRef,
	action access.Action,
) (bool, error) {
	if len(resources) == 0 {
		return false, nil
	}
	lease, err := runtimeHost.Acquire(ctx)
	if err != nil {
		return false, err
	}
	if lease == nil {
		return false, fmt.Errorf("runtime host returned a nil lease")
	}
	defer lease.Release()
	if lease.Identity().ProjectID != projectID {
		return false, fmt.Errorf("runtime project %q does not match requested project %q", lease.Identity().ProjectID, projectID)
	}
	authorizedLease, ok := lease.(interface {
		AuthorizationSnapshot() accesssnapshot.AuthorizationSnapshot
	})
	if !ok {
		return false, fmt.Errorf("active runtime lease does not expose authorization snapshot")
	}
	snapshot := authorizedLease.AuthorizationSnapshot()
	if snapshot.Identity() != lease.Identity() {
		return false, fmt.Errorf("authorization snapshot identity does not match leased serving generation")
	}
	if err := snapshot.ValidateBound(); err != nil {
		return false, err
	}
	for _, resource := range resources {
		if resource.Kind() == projectgraph.KindProjectNamespace {
			return false, nil
		}
	}
	for _, resource := range resources {
		if _, exists := snapshot.Project().Resource(resource.ID()); exists {
			return false, nil
		}
	}
	if platformAdmin, ok := accessModule.(interface {
		RequestPlatformAdmin(context.Context, *http.Request, string) (bool, error)
	}); ok {
		admin, err := platformAdmin.RequestPlatformAdmin(ctx, r, principalID)
		if err != nil {
			return false, err
		}
		if admin {
			return true, nil
		}
	}
	subjects, err := accessModule.AuthorizationSubjects(ctx, principalID)
	if err != nil {
		return false, err
	}
	kind := resources[0].Kind()
	for _, existing := range snapshot.Project().Resources() {
		if existing.Kind != kind {
			continue
		}
		candidate, err := access.NewResourceRef(existing.ID, existing.Kind)
		if err != nil {
			continue
		}
		typed, allowed, err := typedPermissionDecisionForSnapshot(ctx, principalID, projectID, []access.ResourceRef{candidate}, func(access.ResourceRef) (access.Action, bool) {
			return action, true
		}, snapshot, subjects)
		if err != nil {
			return false, err
		}
		if typed && allowed {
			return true, nil
		}
	}
	return false, nil
}
