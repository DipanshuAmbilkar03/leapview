package http

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type limiterClock struct{ now time.Time }

func (c *limiterClock) Now() time.Time { return c.now }

func TestTurnLimiterEnforcesBurstThenRefills(t *testing.T) {
	clock := &limiterClock{now: time.Unix(1_000, 0)}
	limiter := newTurnLimiter(TurnLimits{Burst: 3, Interval: time.Second, MaxInFlight: 10}, clock.Now)
	for i := range 3 {
		release, _, ok := limiter.acquire("alice")
		if !ok {
			t.Fatalf("turn %d inside the burst was rejected", i)
		}
		release()
	}
	_, retryAfter, ok := limiter.acquire("alice")
	if ok || retryAfter <= 0 || retryAfter > time.Second {
		t.Fatalf("over-burst acquire ok=%v retryAfter=%v", ok, retryAfter)
	}
	// Another principal is unaffected.
	if release, _, ok := limiter.acquire("bob"); !ok {
		t.Fatal("a different principal was throttled")
	} else {
		release()
	}
	clock.now = clock.now.Add(time.Second)
	if release, _, ok := limiter.acquire("alice"); !ok {
		t.Fatal("turn after the refill interval was rejected")
	} else {
		release()
	}
}

func TestTurnLimiterCapsConcurrentTurnsPerPrincipal(t *testing.T) {
	clock := &limiterClock{now: time.Unix(1_000, 0)}
	limiter := newTurnLimiter(TurnLimits{Burst: 100, Interval: time.Millisecond, MaxInFlight: 2}, clock.Now)
	first, _, _ := limiter.acquire("alice")
	_, _, ok := limiter.acquire("alice")
	if !ok {
		t.Fatal("second concurrent turn rejected")
	}
	if _, _, ok := limiter.acquire("alice"); ok {
		t.Fatal("third concurrent turn admitted past the cap")
	}
	first()
	first() // release is idempotent
	if release, _, ok := limiter.acquire("alice"); !ok {
		t.Fatal("turn rejected after a slot was released")
	} else {
		release()
	}
	if _, _, ok := limiter.acquire("alice"); !ok {
		t.Fatal("double release freed more than one slot")
	}
	if _, _, ok := limiter.acquire("alice"); ok {
		t.Fatal("cap was exceeded after double release")
	}
}

func TestTurnLimiterPrunesRecoveredPrincipals(t *testing.T) {
	clock := &limiterClock{now: time.Unix(1_000, 0)}
	limiter := newTurnLimiter(TurnLimits{Burst: 1, Interval: time.Second, MaxInFlight: 1}, clock.Now)
	for i := range turnLimiterPruneAt {
		release, _, ok := limiter.acquire(fmt.Sprintf("user-%d", i))
		if !ok {
			t.Fatalf("principal %d rejected", i)
		}
		release()
	}
	clock.now = clock.now.Add(time.Hour)
	if release, _, ok := limiter.acquire("late"); !ok {
		t.Fatal("new principal rejected")
	} else {
		release()
	}
	if got := len(limiter.principals); got != 1 {
		t.Fatalf("tracked principals = %d, want recovered ones pruned", got)
	}
}

func TestAdmitTurnWritesRetryAfter(t *testing.T) {
	handler := NewHandler(Options{TurnLimits: TurnLimits{Burst: 1, Interval: 90 * time.Second, MaxInFlight: 5}})
	write := func(w *httptest.ResponseRecorder) func(error) {
		return func(err error) { w.WriteHeader(429); _, _ = w.WriteString(err.Error()) }
	}
	first := httptest.NewRecorder()
	release, ok := handler.admitTurn(first, "alice", write(first))
	if !ok {
		t.Fatal("first turn rejected")
	}
	release()
	second := httptest.NewRecorder()
	if _, ok := handler.admitTurn(second, "alice", write(second)); ok {
		t.Fatal("second turn admitted")
	}
	if second.Code != 429 || second.Header().Get("Retry-After") != "90" || !strings.Contains(second.Body.String(), "too many chat turns") {
		t.Fatalf("rejection = %d retry-after=%q body=%q", second.Code, second.Header().Get("Retry-After"), second.Body.String())
	}
}
