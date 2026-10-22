package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access"
	accessmodule "github.com/flidai/leapview/internal/access/module"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// adminAwareAccess adds the platform-administrator probe the production access module exposes.
type adminAwareAccess struct {
	tusAccess
	admin bool
}

func (a adminAwareAccess) RequestPlatformAdmin(context.Context, *http.Request, string) (bool, error) {
	return a.admin, nil
}

func TestProtectedResourceRoutesAnswer404OnlyWhereExistenceIsAlreadyVisible(t *testing.T) {
	identity := projectgraph.ServingIdentity{ProjectID: "project_demo", Environment: "production", GenerationID: "generation_1"}
	graph, err := projectgraph.NewProjectGraph([]projectgraph.Resource{
		{ID: "dashboard_sales", Kind: projectgraph.KindDashboard, Name: "sales"},
		{ID: "dashboard_secret", Kind: projectgraph.KindDashboard, Name: "secret"},
		{ID: "source_orders", Kind: projectgraph.KindSource, Name: "orders"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reader := access.SubjectRef{Kind: access.SubjectKindPrincipal, ID: "reader"}
	salesDashboard, err := access.NewResourceRef("dashboard_sales", projectgraph.KindDashboard)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := access.NewExactPermissionPair(access.ActionDashboardRead, identity.ProjectID, salesDashboard)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accesssnapshot.NewTypedGrant("grant", "grant", reader, []access.PermissionPair{pair})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := accesssnapshot.NewAuthorizationSnapshot(identity, graph, []accesssnapshot.Grant{grant}, nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime := tusRuntime{project: identity.ProjectID, lease: tusLease{identity: identity, snapshot: snapshot}}
	resolve := func(r *http.Request, _ projectgraph.ResourceID) []access.ResourceRef {
		slug := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		resource, err := access.NewResourceRef(projectgraph.ResourceID(slug), projectgraph.KindDashboard)
		if err != nil {
			return nil
		}
		return []access.ResourceRef{resource}
	}
	serve := func(principal string, admin bool, path, accept string) *httptest.ResponseRecorder {
		subjects := []access.SubjectRef{{Kind: access.SubjectKindPrincipal, ID: principal}}
		module := adminAwareAccess{tusAccess: tusAccess{principal: accessmodule.Principal{ID: principal}, ok: true, subjects: subjects}, admin: admin}
		handler := protectProjectResourcesWithTypedAction(module, runtime, access.ActionDashboardRead, resolve, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept", accept)
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		return recorder
	}

	tests := []struct {
		name, principal, path, accept string
		admin                         bool
		status                        int
		body                          string
	}{
		{"reader opens a granted dashboard", "reader", "/dashboards/dashboard_sales", "text/html", false, http.StatusOK, ""},
		{"reader gets branded 404 for an unknown dashboard", "reader", "/dashboards/dashboard_nope", "text/html", false, http.StatusNotFound, "We couldn't find this dashboard"},
		{"platform admin gets branded 404 for an unknown dashboard", "admin", "/dashboards/dashboard_nope", "text/html", true, http.StatusNotFound, "We couldn't find this dashboard"},
		{"reader keeps 403 for an existing dashboard it cannot read", "reader", "/dashboards/dashboard_secret", "text/html", false, http.StatusForbidden, "You don't have access"},
		{"stranger keeps 403 for an unknown dashboard", "stranger", "/dashboards/dashboard_nope", "text/html", false, http.StatusForbidden, "You don't have access"},
		{"stranger sees the same 403 for an existing dashboard", "stranger", "/dashboards/dashboard_secret", "text/html", false, http.StatusForbidden, "You don't have access"},
		{"commands keep a plain 404", "reader", "/dashboards/dashboard_nope", "application/json", false, http.StatusNotFound, "404 page not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := serve(test.principal, test.admin, test.path, test.accept)
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("%s %s = %d %q, want %d containing %q", test.principal, test.path, recorder.Code, recorder.Body.String(), test.status, test.body)
			}
		})
	}
}
