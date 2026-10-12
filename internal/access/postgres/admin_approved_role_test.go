package postgres

import (
	"testing"

	"github.com/flidai/leapview/internal/access"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	"github.com/stretchr/testify/require"
)

func TestAdminApprovedRolePostgreSQLRetainsApprovalAndImmutableHistory(t *testing.T) {
	db := newStandaloneAccessDatabase(t)
	repository := &Repository{db: db.runtime}
	principal, err := repository.CreateLocalUser(t.Context(), access.LocalUserInput{Email: "approval@example.test"})
	require.NoError(t, err)
	scope := access.AuthorizationPolicyScope{TargetID: "target", ProjectID: "project", Environment: "production"}
	binding, err := access.NewTypedRoleBinding("viewer", "Viewer", access.SubjectRef{Kind: access.SubjectKindPrincipal, ID: principal.Principal.ID}, access.PermissionRoleViewer, projectgraph.ResourceID(scope.ProjectID))
	require.NoError(t, err)
	staged, err := repository.UpsertAuthorizationRoleBinding(t.Context(), access.AuthorizationRoleBindingInput{Scope: scope, Binding: binding, IdempotencyKey: "stage"})
	require.NoError(t, err)
	require.False(t, staged.RoleBindings[0].AdminApproved)
	binding.AdminApproved = true
	input := access.AuthorizationRoleBindingInput{Scope: scope, Binding: binding, ExpectedRevision: staged.Revision, IdempotencyKey: "approve"}
	approved, err := repository.UpsertAuthorizationRoleBinding(t.Context(), input)
	require.NoError(t, err)
	require.True(t, approved.RoleBindings[0].AdminApproved)
	require.NotEqual(t, staged.Digest, approved.Digest)
	replay, err := repository.UpsertAuthorizationRoleBinding(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, approved, replay)
	old, err := repository.AuthorizationPolicyRevision(t.Context(), scope, staged.Revision)
	require.NoError(t, err)
	require.Equal(t, staged, old)
	_, err = db.admin.Exec(t.Context(), `UPDATE access.authorization_policy_role_binding SET admin_approved=false WHERE target_id=$1 AND revision=$2`, scope.TargetID, approved.Revision)
	require.ErrorContains(t, err, "history is immutable")
	removed, err := repository.RemoveAuthorizationRoleBinding(t.Context(), access.AuthorizationRoleBindingDeleteInput{Scope: scope, BindingID: binding.ID, ExpectedRevision: approved.Revision, IdempotencyKey: "revoke"})
	require.NoError(t, err)
	require.Empty(t, removed.RoleBindings)
	historic, err := repository.AuthorizationPolicyRevision(t.Context(), scope, approved.Revision)
	require.NoError(t, err)
	require.Equal(t, approved, historic)
}
