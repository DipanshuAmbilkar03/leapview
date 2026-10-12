package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/flidai/leapview/internal/access"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// currentTargetAuthorizationFilter applies the target-owned policy at every
// active lease acquisition. Administrator-approved typed role assignments are
// live; other additions await generation activation. Revocations are immediate.
// The serving graph, data policies, and staged resource grants remain captured.
func currentTargetAuthorizationFilter(reader access.AuthorizationPolicyReader, targetID string, projectID projectgraph.ResourceID, environment string) func(context.Context, accesssnapshot.AuthorizationSnapshot) (accesssnapshot.AuthorizationSnapshot, error) {
	return func(ctx context.Context, captured accesssnapshot.AuthorizationSnapshot) (accesssnapshot.AuthorizationSnapshot, error) {
		if reader == nil {
			return accesssnapshot.AuthorizationSnapshot{}, errors.New("current authorization policy reader is unavailable")
		}
		identity := captured.Identity()
		// A fresh target has no claimed Project when composition runs. The
		// runtime host binds it at first activation; the lease supplies that
		// validated identity. A pre-existing claim is checked here as well.
		if (projectID != "" && identity.ProjectID != projectID) || identity.Environment != environment {
			return accesssnapshot.AuthorizationSnapshot{}, errors.New("active authorization snapshot is outside the bound project/environment")
		}
		scope := access.AuthorizationPolicyScope{TargetID: targetID, ProjectID: identity.ProjectID.String(), Environment: identity.Environment}
		current, err := reader.AuthorizationPolicy(ctx, scope)
		if err != nil {
			return accesssnapshot.AuthorizationSnapshot{}, fmt.Errorf("read current target authorization policy: %w", err)
		}
		if current.Scope != scope || current.Revision <= 0 {
			return accesssnapshot.AuthorizationSnapshot{}, errors.New("current authorization policy identity is invalid")
		}
		digest, err := access.AuthorizationPolicyDigest(scope, current.RoleBindings, current.Grants...)
		if err != nil || digest != current.Digest {
			return accesssnapshot.AuthorizationSnapshot{}, errors.New("current authorization policy digest is invalid")
		}
		rolesRestricted, err := captured.WithAdminApprovedRoleBindings(current.RoleBindings)
		if err != nil {
			return accesssnapshot.AuthorizationSnapshot{}, err
		}
		return rolesRestricted.RestrictToCurrentAuthorizationGrants(current.Grants)
	}
}
