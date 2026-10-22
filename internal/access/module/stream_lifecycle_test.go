package module

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type streamClock struct{ now time.Time }

func (c *streamClock) Now() time.Time { return c.now }

func newTestGuard(check func(context.Context) bool, clock *streamClock) (*streamResponseWriter, *httptest.ResponseRecorder) {
	ctx, cancel := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()
	guard := newStreamResponseWriter(recorder, ctx, cancel, check)
	guard.now = clock.Now
	return guard, recorder
}

func TestStreamWritesReuseRecentCredentialCheck(t *testing.T) {
	clock := &streamClock{now: time.Unix(1_000, 0)}
	checks := 0
	guard, recorder := newTestGuard(func(context.Context) bool { checks++; return true }, clock)
	for range 20 {
		if _, err := guard.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		clock.now = clock.now.Add(10 * time.Millisecond)
	}
	if checks != 1 {
		t.Fatalf("credential checks = %d, want 1 within the validity window", checks)
	}
	if recorder.Body.Len() != 20 {
		t.Fatalf("body length = %d", recorder.Body.Len())
	}
	clock.now = clock.now.Add(streamValidityTTL)
	if _, err := guard.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if checks != 2 {
		t.Fatalf("credential checks after window = %d, want 2", checks)
	}
}

func TestStreamRevocationTakesEffectAfterValidityWindow(t *testing.T) {
	clock := &streamClock{now: time.Unix(1_000, 0)}
	valid := true
	guard, recorder := newTestGuard(func(context.Context) bool { return valid }, clock)
	if _, err := guard.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	valid = false
	// Inside the window the cached result is still honored.
	clock.now = clock.now.Add(streamValidityTTL / 2)
	if _, err := guard.Write([]byte("b")); err != nil {
		t.Fatalf("write inside window = %v", err)
	}
	clock.now = clock.now.Add(streamValidityTTL)
	if _, err := guard.Write([]byte("c")); err == nil {
		t.Fatal("write after the window delivered data to a revoked stream")
	}
	if guard.ctx.Err() == nil {
		t.Fatal("revoked stream did not cancel its request context")
	}
	if got := recorder.Body.String(); got != "ab" {
		t.Fatalf("body = %q, want only pre-revocation writes", got)
	}
	// Failure is sticky even if the check would succeed again.
	valid = true
	if _, err := guard.Write([]byte("d")); err == nil {
		t.Fatal("revoked stream resumed writing")
	}
}

func TestStreamTickerRevalidationBypassesCache(t *testing.T) {
	clock := &streamClock{now: time.Unix(1_000, 0)}
	valid := true
	guard, _ := newTestGuard(func(context.Context) bool { return valid }, clock)
	if !guard.valid() {
		t.Fatal("initial check failed")
	}
	valid = false
	// No time has passed, so the cache would still say valid; the ticker must not trust it.
	if guard.revalidate() {
		t.Fatal("revalidate trusted the cached result")
	}
	if guard.ctx.Err() == nil {
		t.Fatal("revalidate failure did not cancel the stream")
	}
}

var _ http.ResponseWriter = (*streamResponseWriter)(nil)
