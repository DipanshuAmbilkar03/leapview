package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/app/config"
	apihttpmiddleware "github.com/flidai/leapview/internal/platform/http/middleware"
)

func TestDevelopmentAllowedHostsRejectRebindingHostsAndKeepLoopbackForms(t *testing.T) {
	hosts := developmentAllowedHosts(config.Config{Addr: "127.0.0.1:8080"})
	handler := apihttpmiddleware.AllowedHosts(hosts)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for host, want := range map[string]int{
		"localhost":            http.StatusNoContent,
		"localhost:8080":       http.StatusNoContent,
		"127.0.0.1:8080":       http.StatusNoContent,
		"[::1]:8080":           http.StatusNoContent,
		"evil.example":         http.StatusMisdirectedRequest,
		"evil.example:8080":    http.StatusMisdirectedRequest,
		"localhost.evil.test":  http.StatusMisdirectedRequest,
		"10.0.0.5:8080":        http.StatusMisdirectedRequest,
		"127.0.0.1.evil.test.": http.StatusMisdirectedRequest,
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = host
		request.RemoteAddr = "127.0.0.1:50000"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Errorf("Host %q = %d, want %d", host, recorder.Code, want)
		}
	}
}

func TestDevelopmentAllowedHostsIncludePublicURLAndExplicitListenHost(t *testing.T) {
	hosts := developmentAllowedHosts(config.Config{Addr: "192.168.1.20:8080", PublicURL: "https://dev.example.test:8443"})
	for _, want := range []string{"localhost", "127.0.0.1", "::1", "dev.example.test", "192.168.1.20"} {
		found := false
		for _, host := range hosts {
			found = found || host == want
		}
		if !found {
			t.Errorf("developmentAllowedHosts() = %v, missing %q", hosts, want)
		}
	}
	for _, host := range developmentAllowedHosts(config.Config{Addr: "0.0.0.0:8080"}) {
		if host == "0.0.0.0" {
			t.Fatal("wildcard listen address must not become an allowed Host")
		}
	}
}
