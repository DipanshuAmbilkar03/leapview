package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	dashboardcatalog "github.com/flidai/leapview/internal/dashboard/authoring/catalog"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

func TestDashboardCatalogDeleteAffordanceUsesExplicitPermission(t *testing.T) {
	for _, tc := range []struct {
		name     string
		callback bool
		allowed  bool
		err      error
	}{
		{name: "missing authorizer"},
		{name: "denied", callback: true},
		{name: "allowed", callback: true, allowed: true},
		{name: "unavailable", callback: true, allowed: true, err: errors.New("unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &browserDashboardCatalogStub{result: dashboardcatalog.ListResult{Items: []dashboardcatalog.Dashboard{
				{ID: "dashboard:mine", StableID: "mine", Source: dashboardcatalog.SourceInstance, Owner: "owner", DraftID: "draft"},
				{ID: "dashboard:other", StableID: "other", Source: dashboardcatalog.SourceInstance, Owner: "other", DraftID: "other-draft"},
				{ID: "dashboard:managed", StableID: "managed", Source: dashboardcatalog.SourceProject},
			}}}
			handler := &BrowserHandler{DashboardCatalog: reader, ResolveProjectID: func(context.Context) (projectgraph.ResourceID, error) { return "project:demo", nil }, CurrentUser: func(*http.Request) (Principal, bool) { return Principal{ID: "owner"}, true }}
			calls := 0
			if tc.callback {
				handler.AuthorizeDeleteDashboard = func(_ *http.Request, project projectgraph.ResourceID, dashboard string) (bool, error) {
					calls++
					if project != "project:demo" || dashboard != "dashboard:mine" {
						t.Fatalf("unexpected permission target %s/%s", project, dashboard)
					}
					return tc.allowed, tc.err
				}
			}
			_, options, err := handler.dashboardCatalogPage(httptest.NewRequest(http.MethodGet, "/", nil), "")
			if err != nil {
				t.Fatal(err)
			}
			want := tc.callback && tc.allowed && tc.err == nil
			if len(options.Dashboards) != 3 || options.Dashboards[0].CanDelete != want || options.Dashboards[1].CanDelete || options.Dashboards[2].CanDelete {
				t.Fatalf("delete affordances = %#v", options.Dashboards)
			}
			if tc.callback && calls != 1 {
				t.Fatalf("permission checks = %d, want 1", calls)
			}
		})
	}
}
