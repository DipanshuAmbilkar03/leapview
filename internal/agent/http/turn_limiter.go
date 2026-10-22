package http

import (
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"sync"
	"time"
)

// TurnLimits bounds how fast and how widely one principal can start model
// calls. Zero fields take the defaults below. The instance-wide provider
// admission still caps total concurrency; this keeps one principal from
// consuming it or the provider budget.
type TurnLimits struct {
	// Burst is the number of turns a principal may start back to back.
	Burst int
	// Interval is the time to earn one more turn once the burst is spent.
	Interval time.Duration
	// MaxInFlight caps turn requests a principal has open at once. Embedded
	// dashboard turns hold their request for the whole answer.
	MaxInFlight int
}

const (
	defaultTurnBurst       = 10
	defaultTurnInterval    = 3 * time.Second
	defaultTurnMaxInFlight = 4
	// turnLimiterPruneAt is the tracked-principal count that triggers pruning
	// of principals whose allowance has fully recovered.
	turnLimiterPruneAt = 1024
)

var errTurnRateLimited = errors.New("too many chat turns; wait a moment before sending another")

type turnLimiter struct {
	burst       float64
	interval    time.Duration
	maxInFlight int
	now         func() time.Time

	mu         sync.Mutex
	principals map[string]*turnAllowance
}

type turnAllowance struct {
	tokens   float64
	updated  time.Time
	inFlight int
}

func newTurnLimiter(limits TurnLimits, now func() time.Time) *turnLimiter {
	if limits.Burst <= 0 {
		limits.Burst = defaultTurnBurst
	}
	if limits.Interval <= 0 {
		limits.Interval = defaultTurnInterval
	}
	if limits.MaxInFlight <= 0 {
		limits.MaxInFlight = defaultTurnMaxInFlight
	}
	if now == nil {
		now = time.Now
	}
	return &turnLimiter{
		burst: float64(limits.Burst), interval: limits.Interval, maxInFlight: limits.MaxInFlight,
		now: now, principals: map[string]*turnAllowance{},
	}
}

// acquire admits one turn for the principal. On success the caller must call
// release when the request finishes. Otherwise retryAfter says when to retry.
func (l *turnLimiter) acquire(principalID string) (release func(), retryAfter time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	allowance := l.principals[principalID]
	if allowance == nil {
		if len(l.principals) >= turnLimiterPruneAt {
			l.pruneLocked(now)
		}
		allowance = &turnAllowance{tokens: l.burst, updated: now}
		l.principals[principalID] = allowance
	}
	l.refillLocked(allowance, now)
	if allowance.inFlight >= l.maxInFlight {
		return nil, time.Second, false
	}
	if allowance.tokens < 1 {
		missing := 1 - allowance.tokens
		return nil, time.Duration(missing * float64(l.interval)), false
	}
	allowance.tokens--
	allowance.inFlight++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			allowance.inFlight--
			l.mu.Unlock()
		})
	}, 0, true
}

func (l *turnLimiter) refillLocked(allowance *turnAllowance, now time.Time) {
	if elapsed := now.Sub(allowance.updated); elapsed > 0 {
		allowance.tokens = min(l.burst, allowance.tokens+float64(elapsed)/float64(l.interval))
		allowance.updated = now
	}
}

func (l *turnLimiter) pruneLocked(now time.Time) {
	for id, allowance := range l.principals {
		l.refillLocked(allowance, now)
		if allowance.inFlight == 0 && allowance.tokens >= l.burst {
			delete(l.principals, id)
		}
	}
}

// admitTurn applies the per-principal turn limits. When the principal is over
// them it sets Retry-After, writes a 429 with write, and returns false.
func (h *Handler) admitTurn(w stdhttp.ResponseWriter, principalID string, write func(error)) (release func(), ok bool) {
	release, retryAfter, ok := h.turns.acquire(principalID)
	if ok {
		return release, true
	}
	seconds := max(1, int((retryAfter+time.Second-1)/time.Second))
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	write(fmt.Errorf("%w (retry in %ds)", errTurnRateLimited, seconds))
	return nil, false
}
