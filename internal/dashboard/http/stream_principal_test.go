package http

import (
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dashboardstream "github.com/flidai/leapview/internal/dashboard/stream"
)

func principalFromHeader(r *nethttp.Request) string { return r.Header.Get("X-Test-Principal") }

func openUpdates(t *testing.T, handler Handler, principal, query string) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequestWithContext(ctx, nethttp.MethodGet, "/updates?"+query, nil)
	request.Header.Set("X-Test-Principal", principal)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Updates(httptest.NewRecorder(), request)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("updates handler did not stop")
		}
	}
}

func waitForStreams(t *testing.T, registry *dashboardstream.Registry, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for registry.Len() != want {
		if time.Now().After(deadline) {
			t.Fatalf("registry streams = %d, want %d", registry.Len(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUpdatesStreamsAreBoundToThePrincipal(t *testing.T) {
	registry := dashboardstream.NewRegistry()
	defer registry.Close()
	handler := Handler{
		Metrics: fakeMetrics{}, ProjectID: "workspace", Coordinators: registry,
		CurrentPrincipalID: principalFromHeader,
	}
	const query = "dashboard=dash&page=overview&clientId=victimclient&streamInstance=instance"

	stopA := openUpdates(t, handler, "principal-a", query)
	defer stopA()
	waitForStreams(t, registry, 1)

	stopB := openUpdates(t, handler, "principal-b", query)
	defer stopB()
	waitForStreams(t, registry, 2)

	streamA := handler.scopedStreamID(requestAs("principal-a"), "victimclient:dash:view:instance")
	streamB := handler.scopedStreamID(requestAs("principal-b"), "victimclient:dash:view:instance")
	if streamA == streamB {
		t.Fatalf("principals share stream id %q", streamA)
	}
	if _, ok := registry.Get(streamA); !ok {
		t.Fatalf("principal A stream %q was replaced by principal B", streamA)
	}
	if _, ok := registry.Get(streamB); !ok {
		t.Fatalf("principal B stream %q is missing", streamB)
	}
	if handler.privateStreamActive(handler.scopedStreamID(requestAs("principal-c"), "victimclient:dash:view:instance")) {
		t.Fatal("principal C commands reached another principal's stream")
	}
}

func TestScopedStreamIDHidesAndSeparatesPrincipals(t *testing.T) {
	handler := Handler{CurrentPrincipalID: principalFromHeader, StreamNamespace: "candidate:one"}
	const base = "client:dash:view:instance"
	a := handler.scopedStreamID(requestAs("user:a"), base)
	b := handler.scopedStreamID(requestAs("user:b"), base)
	if a == b {
		t.Fatal("distinct principals produced the same stream id")
	}
	if again := handler.scopedStreamID(requestAs("user:a"), base); again != a {
		t.Fatalf("stream id is not stable: %q != %q", again, a)
	}
	if want := "candidate:one:"; a[:len(want)] != want {
		t.Fatalf("stream id %q lost its namespace", a)
	}
	for _, id := range []string{a, b} {
		if strings.Contains(id, "user:a") || strings.Contains(id, "user:b") {
			t.Fatalf("stream id %q leaks the principal identifier", id)
		}
	}
	anonymous := Handler{}
	if got := anonymous.scopedStreamID(requestAs(""), base); got != base {
		t.Fatalf("principal-less stream id = %q, want %q", got, base)
	}
}

func requestAs(principal string) *nethttp.Request {
	request := httptest.NewRequest(nethttp.MethodGet, "/", nil)
	request.Header.Set("X-Test-Principal", principal)
	return request
}
