package app

import (
	"testing"

	"github.com/flidai/leapview/internal/access"
	accessmodule "github.com/flidai/leapview/internal/access/module"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// Exercise every project/resource action, including destructive permissions,
// through the production decision path. Domain mutations are separately tested
// with disposable repositories; this matrix proves positive and denial gates.
func TestPermissionCatalogEveryProjectActionHonorsScopeAndCredential(t *testing.T) {
	identity := projectgraph.ServingIdentity{ProjectID: "project_matrix", Environment: "production", GenerationID: "generation_matrix"}
	subject := access.SubjectRef{Kind: access.SubjectKindPrincipal, ID: "matrix-user"}
	for _, definition := range access.PermissionCatalog() {
		if definition.Scope == access.PermissionScopeInstance {
			continue
		}
		kinds := definition.CheckKinds
		for _, kind := range kinds {
			t.Run(string(definition.Action)+"/"+string(kind), func(t *testing.T) {
				id := projectgraph.ResourceID("resource_main")
				otherID := projectgraph.ResourceID("resource_other")
				if kind == projectgraph.KindProjectNamespace {
					id, otherID = identity.ProjectID, "project_other"
				}
				resource, err := access.NewResourceRef(id, kind)
				if err != nil {
					t.Fatal(err)
				}
				var resources []projectgraph.Resource
				if kind != projectgraph.KindProjectNamespace {
					resources = []projectgraph.Resource{{ID: id, Kind: kind, Name: "main"}, {ID: otherID, Kind: kind, Name: "other"}}
				}
				graph, err := projectgraph.NewProjectGraph(resources, nil)
				if err != nil {
					t.Fatal(err)
				}
				pair, err := access.NewExactPermissionPair(definition.Action, identity.ProjectID, resource)
				if kind == projectgraph.KindProjectNamespace {
					pair, err = access.NewProjectPermissionPair(definition.Action, identity.ProjectID)
				}
				if err != nil {
					t.Fatal(err)
				}
				required, err := access.RequiredPermissionPairs(pair)
				if err != nil {
					t.Fatal(err)
				}
				grant, err := accesssnapshot.NewTypedGrant("grant", "grant", subject, required)
				if err != nil {
					t.Fatal(err)
				}
				snapshot, err := accesssnapshot.NewAuthorizationSnapshot(identity, graph, []accesssnapshot.Grant{grant}, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, scenario := range []string{"authorized session", "authorized token", "empty token", "other subject", "other target", "no grant"} {
					t.Run(scenario, func(t *testing.T) {
						ctx := t.Context()
						current := snapshot
						subjects := []access.SubjectRef{subject}
						target := resource
						if scenario == "authorized token" || scenario == "empty token" {
							ceiling := access.ClonePermissionPairs(required)
							if scenario == "empty token" {
								ceiling = []access.PermissionPair{}
							}
							ctx = accessmodule.WithAPICredential(ctx, access.APICredential{Principal: access.Principal{ID: subject.ID}, Token: access.APIToken{ID: "token", PrincipalID: subject.ID, PermissionProfile: access.PermissionCatalogProfile, Permissions: ceiling}})
						}
						if scenario == "other subject" {
							subjects = []access.SubjectRef{{Kind: access.SubjectKindPrincipal, ID: "other-user"}}
						}
						if scenario == "other target" {
							target, err = access.NewResourceRef(otherID, kind)
							if err != nil {
								t.Fatal(err)
							}
						}
						if scenario == "no grant" {
							current, err = accesssnapshot.NewAuthorizationSnapshot(identity, graph, nil, nil)
							if err != nil {
								t.Fatal(err)
							}
						}
						typed, allowed, err := typedPermissionDecisionForSnapshot(ctx, subject.ID, identity.ProjectID, []access.ResourceRef{target}, func(access.ResourceRef) (access.Action, bool) { return definition.Action, true }, current, subjects)
						want := scenario == "authorized session" || scenario == "authorized token"
						if !typed || allowed != want || (want && err != nil) {
							t.Fatalf("typed=%t allowed=%t error=%v; want %t", typed, allowed, err, want)
						}
					})
				}
				if len(required) > 1 {
					withoutPrerequisite, err := accesssnapshot.NewTypedGrant("primary", "primary", subject, []access.PermissionPair{pair})
					if err != nil {
						t.Fatal(err)
					}
					current, err := accesssnapshot.NewAuthorizationSnapshot(identity, graph, []accesssnapshot.Grant{withoutPrerequisite}, nil)
					if err != nil {
						t.Fatal(err)
					}
					_, allowed, err := typedPermissionDecisionForSnapshot(t.Context(), subject.ID, identity.ProjectID, []access.ResourceRef{resource}, func(access.ResourceRef) (access.Action, bool) { return definition.Action, true }, current, []access.SubjectRef{subject})
					if err != nil || allowed {
						t.Fatalf("missing prerequisite allowed=%t error=%v", allowed, err)
					}
				}
			})
		}
	}
}
