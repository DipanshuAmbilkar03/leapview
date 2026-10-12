package app

import (
	"testing"

	"github.com/flidai/leapview/internal/access"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

func TestCurrentPolicyUsesOnlyAdminApprovedAdditionsImmediately(t *testing.T) {
	identity := projectgraph.ServingIdentity{ProjectID: "project_demo", Environment: "production", GenerationID: "generation_1"}
	graph, err := projectgraph.NewProjectGraph([]projectgraph.Resource{{ID: "dashboard_main", Kind: projectgraph.KindDashboard, Name: "main"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := accesssnapshot.NewAuthorizationSnapshot(identity, graph, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	subject := access.SubjectRef{Kind: access.SubjectKindPrincipal, ID: "recipient"}
	binding, err := access.NewTypedRoleBinding("new", "Viewer", subject, access.PermissionRoleViewer, identity.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := access.NewResourceRef("dashboard_main", projectgraph.KindDashboard)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := access.NewExactPermissionPair(access.ActionDashboardRead, identity.ProjectID, ref)
	if err != nil {
		t.Fatal(err)
	}
	scope := access.AuthorizationPolicyScope{TargetID: "target_demo", ProjectID: identity.ProjectID.String(), Environment: identity.Environment}
	reader := &currentAuthorizationPolicyReader{}
	filter := currentTargetAuthorizationFilter(reader, scope.TargetID, identity.ProjectID, identity.Environment)
	for _, approved := range []bool{false, true} {
		binding.AdminApproved = approved
		reader.policy = access.AuthorizationPolicy{Scope: scope, Revision: 1, RoleBindings: []access.RoleBinding{binding}}
		reader.policy.Digest, err = access.AuthorizationPolicyDigest(scope, reader.policy.RoleBindings)
		if err != nil {
			t.Fatal(err)
		}
		active, err := filter(t.Context(), captured)
		if err != nil {
			t.Fatal(err)
		}
		allowed, err := active.AllowsTyped(subject, pair)
		if err != nil || allowed != approved {
			t.Fatalf("approved=%t: allowed=%t error=%v", approved, allowed, err)
		}
		if len(captured.RoleBindings()) != 0 {
			t.Fatal("captured generation mutated")
		}
	}
	reader.policy.Revision++
	reader.policy.RoleBindings = nil
	reader.policy.Digest, err = access.AuthorizationPolicyDigest(scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	active, err := filter(t.Context(), captured)
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := active.AllowsTyped(subject, pair); err != nil || allowed {
		t.Fatalf("revoked approval: allowed=%t error=%v", allowed, err)
	}
}
