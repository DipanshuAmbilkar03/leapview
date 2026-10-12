package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
	accessmodule "github.com/flidai/leapview/internal/access/module"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	"github.com/go-chi/chi/v5"
)

func TestCatalogDeletePermissionMatchesDraftDeleteRoute(t *testing.T) {
	identity, err := projectgraph.NewServingIdentity("project_demo", "prod", "generation_delete_menu")
	if err != nil {
		t.Fatal(err)
	}
	graph, err := projectgraph.NewProjectGraph([]projectgraph.Resource{{ID: "dashboard_draft", Kind: projectgraph.KindDashboard, Name: "draft"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	subject := access.SubjectRef{Kind: access.SubjectKindPrincipal, ID: "owner"}
	for _, tc := range []struct {
		name    string
		action  access.Action
		role    access.PermissionRole
		ceiling bool
		want    bool
	}{
		{name: "owner without delete permission"},
		{name: "editor", role: access.PermissionRoleEditor},
		{name: "project admin", role: access.PermissionRoleProjectAdmin},
		{name: "update does not grant delete", action: access.ActionDashboardUpdate},
		{name: "exact delete authority", action: access.ActionDashboardDelete, want: true},
		{name: "delete outside credential ceiling", action: access.ActionDashboardDelete, ceiling: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var grants []accesssnapshot.Grant
			var roles []accesssnapshot.RoleBinding
			if tc.action != "" {
				resource, err := access.NewResourceRef("dashboard_draft", projectgraph.KindDashboard)
				if err != nil {
					t.Fatal(err)
				}
				pair, err := access.NewExactPermissionPair(tc.action, identity.ProjectID, resource)
				if err != nil {
					t.Fatal(err)
				}
				grant, err := accesssnapshot.NewTypedGrant("dashboard-grant", "dashboard grant", subject, []access.PermissionPair{pair})
				if err != nil {
					t.Fatal(err)
				}
				grants = append(grants, grant)
			}
			if tc.role != "" {
				role, err := access.NewTypedRoleBinding("role", "role", subject, tc.role, identity.ProjectID)
				if err != nil {
					t.Fatal(err)
				}
				roles = append(roles, role)
			}
			snapshot, err := accesssnapshot.NewAuthorizationSnapshotWithRoleBindings(identity, graph, roles, grants, nil)
			if err != nil {
				t.Fatal(err)
			}
			accessModule := tusAccess{principal: accessmodule.Principal{ID: subject.ID}, ok: true, subjects: []access.SubjectRef{subject}}
			host := tusRuntime{project: identity.ProjectID, lease: tusLease{identity: identity, snapshot: snapshot}}
			request := httptest.NewRequest(http.MethodPost, "/dashboards/dashboard_draft/delete", nil)
			if tc.ceiling {
				pair, err := access.NewFutureProjectPermissionPair(access.ActionDashboardUpdate, identity.ProjectID, projectgraph.KindDashboard)
				if err != nil {
					t.Fatal(err)
				}
				credential := access.APICredential{Principal: access.Principal{ID: subject.ID}, Token: access.APIToken{ID: "limited", PrincipalID: subject.ID, PermissionProfile: access.PermissionCatalogProfile, Permissions: []access.PermissionPair{pair}}}
				request = request.WithContext(accessmodule.WithAPICredential(request.Context(), credential))
			}
			authorizer := &repositoryDashboardAuthorizerFake{}
			allowed, err := dashboardDeletionAllowed(request, accessModule, host, authorizer, identity.ProjectID, "dashboard_draft")
			if err != nil || allowed != tc.want {
				t.Fatalf("menu permission = %t, %v; want %t", allowed, err, tc.want)
			}
			wantCalls := 0
			if tc.want {
				wantCalls = 1
			}
			if authorizer.manageCalls != wantCalls {
				t.Fatalf("repository calls = %d, want %d", authorizer.manageCalls, wantCalls)
			}
			router := chi.NewRouter()
			router.Post("/dashboards/{dashboard}/delete", protectProjectAuthoringResourceWithTypedAction(accessModule, host, authorizer, access.ActionDashboardDelete, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if (response.Code == http.StatusNoContent) != allowed {
				t.Fatalf("route status = %d disagrees with menu permission %t", response.Code, allowed)
			}
		})
	}
}
