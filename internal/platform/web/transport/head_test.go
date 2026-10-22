package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeadFromGetServesGetHeadersWithoutBody(t *testing.T) {
	calls := 0
	var seenMethod string
	handler := HeadFromGet(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		seenMethod = r.Method
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusAccepted)
		if n, err := w.Write([]byte("<html>body</html>")); err != nil || n != len("<html>body</html>") {
			t.Fatalf("Write = %d, %v; want full length and no error", n, err)
		}
	}))

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/login", nil))
	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/login", nil))

	if seenMethod != http.MethodGet {
		t.Fatalf("handler saw method %q, want GET", seenMethod)
	}
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2", calls)
	}
	if head.Code != get.Code {
		t.Fatalf("HEAD status = %d, GET status = %d", head.Code, get.Code)
	}
	for _, name := range []string{"Content-Type", "Cache-Control"} {
		if head.Header().Get(name) != get.Header().Get(name) {
			t.Fatalf("HEAD %s = %q, GET %s = %q", name, head.Header().Get(name), name, get.Header().Get(name))
		}
	}
	if head.Body.Len() != 0 {
		t.Fatalf("HEAD body = %q, want empty", head.Body.String())
	}
}

func TestHeadFromGetLeavesOtherMethodsUntouched(t *testing.T) {
	var seen string
	handler := HeadFromGet(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method
		_, _ = w.Write([]byte("ok"))
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen != http.MethodGet || recorder.Body.String() != "ok" {
		t.Fatalf("GET seen=%q body=%q", seen, recorder.Body.String())
	}
}
