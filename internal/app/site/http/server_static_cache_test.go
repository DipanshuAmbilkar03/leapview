package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSiteStaticAssetsRevalidateWithContentETag(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	for _, path := range []string{"/static/site.css", "/static/favicon.svg", "/static/product-dashboard-light.png"} {
		get := func(headers map[string]string) *http.Response {
			t.Helper()
			request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
			if err != nil {
				t.Fatalf("create %s request: %v", path, err)
			}
			for name, value := range headers {
				request.Header.Set(name, value)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatalf("get %s: %v", path, err)
			}
			t.Cleanup(func() { response.Body.Close() })
			return response
		}

		first := get(map[string]string{"Accept-Encoding": "gzip"})
		etag := first.Header.Get("ETag")
		if etag == "" {
			t.Fatalf("%s has no ETag; browsers cannot revalidate it with a 304", path)
		}
		if got := first.Header.Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
			t.Errorf("%s cache control = %q, want revalidation", path, got)
		}
		if again := get(map[string]string{"Accept-Encoding": "gzip"}).Header.Get("ETag"); again != etag {
			t.Errorf("%s ETag is not stable: %q then %q", path, etag, again)
		}
		if strings.HasSuffix(path, ".png") && first.Header.Get("Content-Encoding") != "" {
			t.Errorf("%s is already compressed but was served with Content-Encoding %q", path, first.Header.Get("Content-Encoding"))
		}

		revalidated := get(map[string]string{"Accept-Encoding": "gzip", "If-None-Match": etag})
		body := readBody(t, revalidated)
		if revalidated.StatusCode != http.StatusNotModified {
			t.Fatalf("%s conditional status = %d, want 304", path, revalidated.StatusCode)
		}
		if body != "" {
			t.Errorf("%s 304 response carried %d body bytes", path, len(body))
		}
		if revalidated.Header.Get("Content-Encoding") != "" {
			t.Errorf("%s 304 response advertises Content-Encoding %q", path, revalidated.Header.Get("Content-Encoding"))
		}
		if revalidated.Header.Get("Cache-Control") == "" {
			t.Errorf("%s 304 response lost its Cache-Control header", path)
		}
	}
}
