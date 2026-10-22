package protocol

import (
	"errors"
	"sync/atomic"
	"time"
)

// leaseFailureWindow bounds how long a lease failure keeps readiness unhealthy.
// Repeated failures keep refreshing it; a transient blip ages out on its own.
const leaseFailureWindow = 2 * IdempotencyLease

// leaseHealth reports an unhealthy idempotency lease while a lease failure is
// recent, so a transient store outage does not leave readiness failed until
// restart while continuing failures keep it unhealthy.
type leaseHealth struct {
	// failedAt is the Unix-nano time of the latest lease failure, or zero.
	failedAt atomic.Int64
	now      func() time.Time
}

func (h *leaseHealth) record() {
	h.failedAt.Store(h.clock().UnixNano())
}

func (h *leaseHealth) err() error {
	failedAt := h.failedAt.Load()
	if failedAt != 0 && h.clock().Sub(time.Unix(0, failedAt)) < leaseFailureWindow {
		return errors.New("idempotency lease renewal is unhealthy")
	}
	return nil
}

func (h *leaseHealth) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}
