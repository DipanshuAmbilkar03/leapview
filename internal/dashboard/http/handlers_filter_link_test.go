package http

import (
	"html"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dashboardfilter "github.com/flidai/leapview/internal/dashboard/filter"
	dashboardresolver "github.com/flidai/leapview/internal/dashboard/resolver"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// filterLinkMetrics resolves "dash" to a definition with one URL-bound category filter.
type filterLinkMetrics struct{ fakeMetrics }

func (filterLinkMetrics) Resolver() dashboardresolver.Resolver { return filterLinkResolver{} }

type filterLinkResolver struct{}

func (filterLinkResolver) Resolve(dashboardID projectgraph.ResourceID) (dashboardresolver.Resolved, error) {
	resolved, err := (fakeDashboardResolver{}).Resolve(dashboardID)
	if err != nil {
		return resolved, err
	}
	resolved.Definition.FilterDefinitions = map[string]dashboardfilter.Definition{
		"category": {ValueKind: dashboardfilter.ValueString, Predicates: []dashboardfilter.PredicatePolicy{{Kind: dashboardfilter.ExpressionSet, Operators: []dashboardfilter.Operator{dashboardfilter.OperatorIn}}}},
	}
	resolved.Definition.FilterApplication = dashboardfilter.ApplicationPolicy{Mode: dashboardfilter.ApplicationImmediate}
	resolved.Definition.FilterBindings = map[string]dashboardfilter.Binding{
		"report_category": {
			Key: "fb_category", ID: "report_category", Filter: "category", Scope: dashboardfilter.ScopeReport,
			Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered},
			URL:     dashboardfilter.URLPolicy{Param: "category", Encoding: dashboardfilter.URLEncodingTypedV1},
		},
	}
	return resolved, nil
}

func TestPageWithUndecodableFilterParameterOpensWithABanner(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodGet, "/dashboards/dash/pages/overview?category=auto", nil)
	req.Header.Set("Accept", "text/html")

	testRouter(Handler{Metrics: filterLinkMetrics{}}).ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want the dashboard to open; body = %q", rec.Code, rec.Body.String())
	}
	body := html.UnescapeString(rec.Body.String())
	if !strings.Contains(body, `<lv-dashboard-page`) || !strings.Contains(body, `filter-notice="The “category” filter value in this link isn’t valid, so it was skipped. The other filters still apply."`) {
		t.Fatalf("page did not render with a filter notice:\n%s", body)
	}
	if strings.Contains(body, "category=auto") {
		t.Fatalf("the stream URL still carries the undecodable parameter:\n%s", body)
	}

	clean := httptest.NewRecorder()
	testRouter(Handler{Metrics: filterLinkMetrics{}}).ServeHTTP(clean, httptest.NewRequest(nethttp.MethodGet, "/dashboards/dash/pages/overview", nil))
	if strings.Contains(clean.Body.String(), "filter-notice") {
		t.Fatalf("a link without filters must not show a notice:\n%s", clean.Body.String())
	}
}
