package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
	accessmodule "github.com/flidai/leapview/internal/access/module"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	"github.com/flidai/leapview/internal/dashboard/authoring/accessadapter"
	authoringservice "github.com/flidai/leapview/internal/dashboard/authoring/service"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	"github.com/go-chi/chi/v5"
)

// This repository authorization port fixture keeps the canonical adapter and
// active typed snapshot real while varying owner and publication evidence.
type privateDraftDeleteAuthorizer struct {
	repositoryDashboardAuthorizerFake
	adapter   *accessadapter.Adapter
	lifecycle authoring.DashboardLifecycle
}

func (a *privateDraftDeleteAuthorizer) AuthorizeDashboardDelete(ctx context.Context, project projectgraph.ResourceID, actor string, dashboard authoring.DashboardID) error {
	return a.adapter.Authorize(ctx, authoringservice.AuthorizationRequest{ActorID: actor, ProjectID: project, DashboardID: dashboard, OwnerPrincipalID: a.lifecycle.OwnerPrincipalID, PrivateDraft: a.lifecycle.IsPrivateDraft(), Visibility: a.lifecycle.Visibility, Target: authoringservice.AuthorizationTargetAuthoredDashboard, Action: authoring.AuthorizationActionDelete})
}

func TestOwnerPrivateDraftDeleteCatalogAndRouteRespectTypedAuthority(t *testing.T) {
	identity, err := projectgraph.NewServingIdentity("project_demo", "prod", "generation_owner_delete")
	if err != nil {
		t.Fatal(err)
	}
	// New private drafts intentionally do not appear in the published graph.
	graph, err := projectgraph.NewProjectGraph([]projectgraph.Resource{{ID: "existing_dashboard", Kind: projectgraph.KindDashboard, Name: "existing"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	subject := access.SubjectRef{Kind: access.SubjectKindPrincipal, ID: "owner"}
	for _, tc := range []struct {
		name                                            string
		role                                            access.PermissionRole
		actor                                           string
		token, explicitDelete, published, updateCeiling bool
		want                                            bool
	}{
		{name: "editor owns unpublished draft", role: access.PermissionRoleEditor, want: true},
		{name: "ownership without edit"},
		{name: "project admin alone", role: access.PermissionRoleProjectAdmin},
		{name: "editor cannot delete another owner's draft", role: access.PermissionRoleEditor, actor: "another-editor"},
		{name: "retained published revision", role: access.PermissionRoleEditor, published: true},
		{name: "update token does not inherit owner exception", role: access.PermissionRoleEditor, token: true},
		{name: "explicit delete assignment outside token ceiling", role: access.PermissionRoleEditor, token: true, explicitDelete: true, updateCeiling: true},
		{name: "explicit delete token remains allowed", role: access.PermissionRoleEditor, token: true, explicitDelete: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := tc.actor
			if actor == "" {
				actor = "owner"
			}
			current := subject
			current.ID = actor
			var roles []accesssnapshot.RoleBinding
			if tc.role != "" {
				role, err := access.NewTypedRoleBinding("role", "role", current, tc.role, identity.ProjectID)
				if err != nil {
					t.Fatal(err)
				}
				roles = append(roles, role)
			}
			var grants []accesssnapshot.Grant
			resource, err := access.NewResourceRef("dashboard_draft", projectgraph.KindDashboard)
			if err != nil {
				t.Fatal(err)
			}
			pair, err := access.NewExactPermissionPair(access.ActionDashboardDelete, identity.ProjectID, resource)
			if err != nil {
				t.Fatal(err)
			}
			if tc.explicitDelete {
				grant, err := accesssnapshot.NewTypedGrant("delete", "delete", current, []access.PermissionPair{pair})
				if err != nil {
					t.Fatal(err)
				}
				grants = append(grants, grant)
			}
			permissionGraph := graph
			if tc.explicitDelete {
				permissionGraph, err = projectgraph.NewProjectGraph([]projectgraph.Resource{{ID: "dashboard_draft", Kind: projectgraph.KindDashboard, Name: "draft"}}, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := accesssnapshot.NewAuthorizationSnapshotWithRoleBindings(identity, permissionGraph, roles, grants, nil)
			if err != nil {
				t.Fatal(err)
			}
			accessModule := tusAccess{principal: accessmodule.Principal{ID: actor}, ok: true, subjects: []access.SubjectRef{current}}
			host := tusRuntime{project: identity.ProjectID, lease: tusLease{identity: identity, snapshot: snapshot}}
			adapter, err := accessadapter.New(accessadapter.Options{
				AuthorizeTypedResource: func(ctx context.Context, actor string, project projectgraph.ResourceID, ref access.ResourceRef, action access.Action) (bool, bool, error) {
					return authorizeTypedResourceActionWithDraft(ctx, accessModule, host, actor, project, []access.ResourceRef{ref}, action, true)
				},
				AuthorizeTypedProject: func(context.Context, string, projectgraph.ResourceID, access.Action) (bool, bool, error) {
					return true, false, nil
				},
				AuthorizeOwnedDraftDelete: func(ctx context.Context, actor string, project projectgraph.ResourceID, ref access.ResourceRef) (bool, bool, error) {
					return authorizeOwnedDraftDelete(ctx, accessModule, host, actor, project, ref)
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			lifecycle := authoring.DashboardLifecycle{OwnerPrincipalID: "owner", Visibility: authoring.VisibilityPrivate, Status: authoring.LifecycleStatusDraft, Draft: &authoring.Draft{}}
			if tc.published {
				lifecycle.Published = &authoring.Published{}
			}
			authorizer := &privateDraftDeleteAuthorizer{adapter: adapter, lifecycle: lifecycle}
			request := httptest.NewRequest(http.MethodPost, "/dashboards/dashboard_draft/delete", nil)
			if tc.token {
				ceiling := pair
				if !tc.explicitDelete || tc.updateCeiling {
					ceiling, err = access.NewFutureProjectPermissionPair(access.ActionDashboardUpdate, identity.ProjectID, projectgraph.KindDashboard)
					if err != nil {
						t.Fatal(err)
					}
				}
				credential := access.APICredential{Principal: access.Principal{ID: actor}, Token: access.APIToken{ID: "limited", PrincipalID: actor, PermissionProfile: access.PermissionCatalogProfile, Permissions: []access.PermissionPair{ceiling}}}
				request = request.WithContext(accessmodule.WithAPICredential(request.Context(), credential))
			}
			allowed, err := dashboardDeletionAllowed(request, accessModule, host, authorizer, identity.ProjectID, "dashboard_draft")
			if err != nil || allowed != tc.want {
				t.Fatalf("catalog permission = %t, %v; want %t", allowed, err, tc.want)
			}
			router := chi.NewRouter()
			router.Post("/dashboards/{dashboard}/delete", protectProjectDraftDeletion(accessModule, host, authorizer, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if (response.Code == http.StatusNoContent) != allowed {
				t.Fatalf("route status = %d disagrees with catalog %t", response.Code, allowed)
			}
			if tc.want && !tc.explicitDelete {
				archive := chi.NewRouter()
				archive.Post("/dashboards/{dashboard}/archive", protectProjectAuthoringResourceWithTypedAction(accessModule, host, authorizer, access.ActionDashboardDelete, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
				archiveResponse := httptest.NewRecorder()
				archive.ServeHTTP(archiveResponse, httptest.NewRequest(http.MethodPost, "/dashboards/dashboard_draft/archive", nil))
				if archiveResponse.Code != http.StatusForbidden {
					t.Fatalf("owner-only archive status = %d", archiveResponse.Code)
				}
			}

		})
	}
}
