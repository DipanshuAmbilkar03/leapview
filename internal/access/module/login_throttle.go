package module

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var errTooManyLoginAttempts = errors.New("too many sign-in attempts; try again shortly")

// rejectThrottledLogin rejects a password attempt for an account that is inside
// its temporary per-account delay. The password is not verified.
func (a *Auth) rejectThrottledLogin(w http.ResponseWriter, r *http.Request, email string) bool {
	if email == "" {
		return false
	}
	retryAfter := a.loginThrottle.begin(email)
	if retryAfter <= 0 {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	if wantsJSON(r) {
		writeJSONError(w, errTooManyLoginAttempts, http.StatusTooManyRequests)
		return true
	}
	http.Redirect(w, r, "/login?error=too_many_attempts", http.StatusSeeOther)
	return true
}

// Per-account login throttling closes the gap left by per-address rate limits:
// an attacker rotating source addresses can otherwise guess one account's
// password without bound.
//
// The throttle is a temporary, capped delay rather than a lockout. After
// loginThrottleFreeAttempts attempts the account must wait a delay that doubles
// from loginThrottleBaseDelay up to loginThrottleMaxDelay between attempts, so
// a password guesser is held to roughly 30 guesses an hour per account while a
// deliberate attacker can keep the legitimate user waiting at most
// loginThrottleMaxDelay at a time. Attempts rejected during a delay are not
// verified and do not extend it. Every verified attempt is counted before the
// password is checked so concurrent guesses cannot all slip through one open
// window; a successful sign-in clears the account.
//
// State is in memory, per process, keyed by a hash of the normalized account,
// and bounded by loginThrottleMaxAccounts. Unknown accounts are tracked like
// known ones so the response does not reveal which accounts exist.
const (
	loginThrottleFreeAttempts   = 5
	loginThrottleBaseDelay      = 2 * time.Second
	loginThrottleMaxDelay       = 2 * time.Minute
	loginThrottleResetAfter     = 15 * time.Minute
	loginThrottleMaxAccounts    = 10_000
	loginThrottleEvictionSample = 16
)

type loginThrottle struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[[sha256.Size]byte]*loginThrottleEntry
}

type loginThrottleEntry struct {
	attempts     int
	lastAttempt  time.Time
	blockedUntil time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{now: time.Now, entries: map[[sha256.Size]byte]*loginThrottleEntry{}}
}

// begin reserves one password attempt for the normalized account. It returns
// zero when the attempt may proceed, or how long the caller must wait. A nil
// throttle never delays.
func (t *loginThrottle) begin(account string) time.Duration {
	if t == nil {
		return 0
	}
	key := sha256.Sum256([]byte(account))
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	entry := t.entries[key]
	if entry != nil && now.Sub(entry.lastAttempt) >= loginThrottleResetAfter {
		delete(t.entries, key)
		entry = nil
	}
	if entry != nil && now.Before(entry.blockedUntil) {
		return entry.blockedUntil.Sub(now)
	}
	if entry == nil {
		t.makeRoom(now)
		entry = &loginThrottleEntry{}
		t.entries[key] = entry
	}
	entry.attempts++
	entry.lastAttempt = now
	if entry.attempts >= loginThrottleFreeAttempts {
		entry.blockedUntil = now.Add(loginThrottleDelay(entry.attempts - loginThrottleFreeAttempts))
	}
	return 0
}

// succeeded clears the account after a successful sign-in.
func (t *loginThrottle) succeeded(account string) {
	if t == nil {
		return
	}
	key := sha256.Sum256([]byte(account))
	t.mu.Lock()
	delete(t.entries, key)
	t.mu.Unlock()
}

func (t *loginThrottle) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// makeRoom keeps memory bounded in constant time. It samples a few accounts
// (map iteration order is random), drops one that has been quiet long enough to
// reset, and otherwise drops the least-attempted sampled account, so a flood of
// one-off accounts evicts other one-off accounts rather than a heavily attacked
// one.
func (t *loginThrottle) makeRoom(now time.Time) {
	if len(t.entries) < loginThrottleMaxAccounts {
		return
	}
	var weakest [sha256.Size]byte
	weakestAttempts := -1
	sampled := 0
	for key, entry := range t.entries {
		if now.Sub(entry.lastAttempt) >= loginThrottleResetAfter {
			delete(t.entries, key)
			return
		}
		if weakestAttempts < 0 || entry.attempts < weakestAttempts {
			weakest, weakestAttempts = key, entry.attempts
		}
		if sampled++; sampled == loginThrottleEvictionSample {
			break
		}
	}
	delete(t.entries, weakest)
}

// loginThrottleDelay returns the delay after step prior throttled attempts:
// base, 2*base, 4*base, ... capped at the maximum.
func loginThrottleDelay(step int) time.Duration {
	if step >= 10 {
		return loginThrottleMaxDelay
	}
	return min(loginThrottleBaseDelay<<step, loginThrottleMaxDelay)
}
