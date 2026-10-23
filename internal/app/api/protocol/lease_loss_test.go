package protocol

import (
	"context"
	"errors"
	"testing"
	"time"

	apiidempotency "github.com/flidai/leapview/internal/platform/http/idempotency"
)

func TestLeaseLossPublishedBeforeHandlerCancellation(t *testing.T) {
	p := &Protocol{}
	leaseLost := make(chan error, 1)
	handlerCtx, cancelHandler := context.WithCancelCause(t.Context())
	defer cancelHandler(nil)
	cancelled := false
	p.notifyIdempotencyLeaseLost(apiidempotency.ErrLeaseLost, leaseLost, func(cause error) {
		cancelHandler(cause)
		cancelled = true
		// The handler can return as soon as cancellation wakes it. Its caller
		// must already see lease loss rather than commit the captured response.
		select {
		case err := <-leaseLost:
			if err != apiidempotency.ErrLeaseLost {
				t.Fatalf("lease loss = %v", err)
			}
		default:
			t.Fatal("handler cancellation preceded lease-loss publication")
		}
		if p.LeaseRenewalError() == nil {
			t.Fatal("handler cancellation preceded unhealthy readiness")
		}
	})
	if !cancelled || context.Cause(handlerCtx) != apiidempotency.ErrLeaseLost {
		t.Fatalf("handler cancelled=%t cause=%v", cancelled, context.Cause(handlerCtx))
	}
}

type leaseHealthClock struct{ now time.Time }

func (c *leaseHealthClock) Now() time.Time { return c.now }

func TestLeaseFailureReadinessRecoversAfterWindow(t *testing.T) {
	clock := &leaseHealthClock{now: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	p := &Protocol{leaseHealth: leaseHealth{now: clock.Now}}
	if err := p.LeaseRenewalError(); err != nil {
		t.Fatalf("fresh protocol unhealthy: %v", err)
	}

	p.notifyIdempotencyLeaseLost(apiidempotency.ErrLeaseLost, make(chan error, 1), func(error) {})
	if p.LeaseRenewalError() == nil {
		t.Fatal("lease loss did not fail readiness")
	}

	clock.now = clock.now.Add(leaseFailureWindow - time.Second)
	if p.LeaseRenewalError() == nil {
		t.Fatal("readiness recovered inside the failure window")
	}

	clock.now = clock.now.Add(2 * time.Second)
	if err := p.LeaseRenewalError(); err != nil {
		t.Fatalf("readiness did not recover after the failure window: %v", err)
	}
}

func TestLeaseFailureReadinessStaysUnhealthyWhileFailuresContinue(t *testing.T) {
	clock := &leaseHealthClock{now: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	p := &Protocol{leaseHealth: leaseHealth{now: clock.Now}}
	for range 4 {
		p.notifyIdempotencyLeaseLost(errors.New("database unavailable"), make(chan error, 1), func(error) {})
		clock.now = clock.now.Add(leaseFailureWindow / 2)
		if p.LeaseRenewalError() == nil {
			t.Fatal("ongoing lease failures reported healthy")
		}
	}
	clock.now = clock.now.Add(leaseFailureWindow)
	if err := p.LeaseRenewalError(); err != nil {
		t.Fatalf("readiness did not recover once failures stopped: %v", err)
	}
}
