package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationAndAuthenticatedBrowserResponsesArePrivate(t *testing.T) {
	server := assembleRuntime(fakeMetrics{}, assemblyConfig{})
	handler := server.Routes()

	for _, path := range []string{"/login", "/", "/auth/azureadv2"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
			if got := response.Header().Get("Pragma"); got != "no-cache" {
				t.Fatalf("Pragma = %q, want no-cache", got)
			}
		})
	}
}

// HEAD on the two public navigation entry points must mirror GET (same status
// and headers, no body) so link checkers, uptime probes and crawlers do not
// see a 405. Streams and other GET routes stay GET-only.
func TestEntryPointsAnswerHeadLikeGet(t *testing.T) {
	server := assembleRuntime(fakeMetrics{}, assemblyConfig{})
	handler := server.Routes()

	for _, path := range []string{"/login", "/"} {
		t.Run(path, func(t *testing.T) {
			get := httptest.NewRecorder()
			handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, path, nil))
			head := httptest.NewRecorder()
			handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, path, nil))

			if head.Code == http.StatusMethodNotAllowed {
				t.Fatalf("HEAD %s = 405, want GET status %d", path, get.Code)
			}
			if head.Code != get.Code {
				t.Fatalf("HEAD %s status = %d, GET status = %d", path, head.Code, get.Code)
			}
			if head.Body.Len() != 0 {
				t.Fatalf("HEAD %s returned a %d byte body", path, head.Body.Len())
			}
			for _, name := range []string{"Content-Type", "Cache-Control", "Pragma", "Location", "Content-Security-Policy"} {
				if head.Header().Get(name) != get.Header().Get(name) {
					t.Errorf("HEAD %s %s = %q, GET = %q", path, name, head.Header().Get(name), get.Header().Get(name))
				}
			}
		})
	}
}

func TestHeadDoesNotOpenUpdatesStream(t *testing.T) {
	server := assembleRuntime(fakeMetrics{}, assemblyConfig{})
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/updates?route=login", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD /updates = %d, want 405 so no stream is opened", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got == "text/event-stream" {
		t.Fatalf("HEAD /updates content type = %q", got)
	}
}
