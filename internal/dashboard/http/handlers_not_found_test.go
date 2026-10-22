package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnknownPageRendersBrandedNotFoundForNavigationOnly(t *testing.T) {
	for _, path := range []string{"/dashboards/missing/pages/overview", "/dashboards/dash/pages/missing"} {
		t.Run(path, func(t *testing.T) {
			page := httptest.NewRecorder()
			request := httptest.NewRequest(nethttp.MethodGet, path, nil)
			request.Header.Set("Accept", "text/html")
			testRouter(Handler{Metrics: fakeMetrics{}}).ServeHTTP(page, request)
			if page.Code != nethttp.StatusNotFound || !strings.Contains(page.Body.String(), "We couldn't find this dashboard") {
				t.Fatalf("navigation = %d %q", page.Code, page.Body.String())
			}
			api := httptest.NewRecorder()
			request = httptest.NewRequest(nethttp.MethodGet, path, nil)
			request.Header.Set("Accept", "application/json")
			testRouter(Handler{Metrics: fakeMetrics{}}).ServeHTTP(api, request)
			if api.Code != nethttp.StatusNotFound || strings.Contains(api.Body.String(), "<html") {
				t.Fatalf("JSON client = %d %q", api.Code, api.Body.String())
			}
		})
	}
}
