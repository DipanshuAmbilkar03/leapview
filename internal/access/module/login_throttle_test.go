package module

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/access"
)

type throttleClock struct{ now time.Time }

func (c *throttleClock) Now() time.Time          { return c.now }
func (c *throttleClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestLoginThrottle() (*loginThrottle, *throttleClock) {
	clock := &throttleClock{now: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}
	throttle := newLoginThrottle()
	throttle.now = clock.Now
	return throttle, clock
}

func TestLoginThrottleBacksOffPerAccountAndCapsDelay(t *testing.T) {
	throttle, clock := newTestLoginThrottle()
	for attempt := 1; attempt <= loginThrottleFreeAttempts; attempt++ {
		if retry := throttle.begin("victim@example.com"); retry != 0 {
			t.Fatalf("attempt %d delayed by %s, want free", attempt, retry)
		}
	}
	retry := throttle.begin("victim@example.com")
	if retry != loginThrottleBaseDelay {
		t.Fatalf("first throttled retry = %s, want %s", retry, loginThrottleBaseDelay)
	}
	if again := throttle.begin("victim@example.com"); again <= 0 || again > retry {
		t.Fatalf("attempt inside the delay = %s, want a remaining delay up to %s", again, retry)
	}
	if other := throttle.begin("bystander@example.com"); other != 0 {
		t.Fatalf("unrelated account delayed by %s", other)
	}

	previous := loginThrottleBaseDelay
	for range 20 {
		clock.advance(previous)
		next := throttle.begin("victim@example.com")
		if next != 0 {
			t.Fatalf("attempt after delay expired still delayed by %s", next)
		}
		// The delayed attempt itself counted; the next one reports the new delay.
		previous = throttle.begin("victim@example.com")
		if previous <= 0 || previous > loginThrottleMaxDelay {
			t.Fatalf("delay = %s, want within (0, %s]", previous, loginThrottleMaxDelay)
		}
	}
	if previous != loginThrottleMaxDelay {
		t.Fatalf("delay did not cap: %s, want %s", previous, loginThrottleMaxDelay)
	}
}

func TestLoginThrottleDelayIsTemporaryNotALockout(t *testing.T) {
	throttle, clock := newTestLoginThrottle()
	for range loginThrottleFreeAttempts + 10 {
		throttle.begin("victim@example.com")
	}
	if throttle.begin("victim@example.com") == 0 {
		t.Fatal("account not delayed after repeated attempts")
	}
	clock.advance(loginThrottleMaxDelay)
	if retry := throttle.begin("victim@example.com"); retry != 0 {
		t.Fatalf("account still delayed %s after the maximum delay elapsed", retry)
	}
	clock.advance(loginThrottleResetAfter)
	for attempt := 1; attempt <= loginThrottleFreeAttempts; attempt++ {
		if retry := throttle.begin("victim@example.com"); retry != 0 {
			t.Fatalf("attempt %d after quiet period delayed by %s, want free budget restored", attempt, retry)
		}
	}
}

func TestLoginThrottleSuccessClearsAccount(t *testing.T) {
	throttle, _ := newTestLoginThrottle()
	for range loginThrottleFreeAttempts {
		throttle.begin("user@example.com")
	}
	throttle.succeeded("user@example.com")
	for attempt := 1; attempt <= loginThrottleFreeAttempts; attempt++ {
		if retry := throttle.begin("user@example.com"); retry != 0 {
			t.Fatalf("attempt %d after success delayed by %s", attempt, retry)
		}
	}
}

func TestLoginThrottleMemoryIsBoundedAndKeepsHeavilyAttackedAccounts(t *testing.T) {
	throttle, _ := newTestLoginThrottle()
	for range loginThrottleFreeAttempts + 3 {
		throttle.begin("victim@example.com")
	}
	for i := range loginThrottleMaxAccounts * 3 {
		throttle.begin(fmt.Sprintf("random-%d@example.com", i))
	}
	if size := throttle.size(); size > loginThrottleMaxAccounts {
		t.Fatalf("tracked accounts = %d, want at most %d", size, loginThrottleMaxAccounts)
	}
	if throttle.begin("victim@example.com") == 0 {
		t.Fatal("attacked account evicted by a flood of one-off accounts")
	}
}

func TestLoginThrottleNilIsDisabled(t *testing.T) {
	var throttle *loginThrottle
	if retry := throttle.begin("user@example.com"); retry != 0 {
		t.Fatalf("nil throttle delayed by %s", retry)
	}
	throttle.succeeded("user@example.com")
}

type throttleLoginRepository struct {
	access.Repository
	verifications int
}

func (r *throttleLoginRepository) VerifyLocalPassword(context.Context, string, string) (access.Principal, access.LocalCredential, error) {
	r.verifications++
	return access.Principal{}, access.LocalCredential{}, errUnauthorized
}

func (r *throttleLoginRepository) RecordAuditEvent(context.Context, access.AuditEventInput) error {
	return nil
}

func postThrottledLogin(auth *Auth, username, remoteAddr, accept string) *httptest.ResponseRecorder {
	form := url.Values{"email": {username}, "password": {"wrong"}}
	request := httptest.NewRequest(http.MethodPost, "/auth/local/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	request.RemoteAddr = remoteAddr
	response := httptest.NewRecorder()
	auth.LocalLogin(response, request)
	return response
}

func TestLocalLoginThrottlesAccountAcrossRotatingAddresses(t *testing.T) {
	repository := &throttleLoginRepository{}
	auth := mustNewAuth(t, repository, AuthConfig{LocalAuth: true})
	for i := range loginThrottleFreeAttempts {
		response := postThrottledLogin(auth, "Victim@Example.com", fmt.Sprintf("198.51.100.%d:5000", i), "")
		if response.Header().Get("Location") != "/login?error=invalid_credentials" {
			t.Fatalf("attempt %d location = %q", i+1, response.Header().Get("Location"))
		}
	}
	// Account name normalization must not let case or spacing variants evade it.
	response := postThrottledLogin(auth, "  victim@example.com ", "203.0.113.200:5000", "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?error=too_many_attempts" {
		t.Fatalf("throttled browser response = %d %q", response.Code, response.Header().Get("Location"))
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("throttled response is missing Retry-After")
	}
	if repository.verifications != loginThrottleFreeAttempts {
		t.Fatalf("password verifications = %d, want %d (throttled attempts must not reach verification)", repository.verifications, loginThrottleFreeAttempts)
	}

	jsonResponse := postThrottledLogin(auth, "victim@example.com", "203.0.113.201:5000", "application/json")
	if jsonResponse.Code != http.StatusTooManyRequests || jsonResponse.Header().Get("Retry-After") == "" {
		t.Fatalf("throttled JSON response = %d retry-after=%q", jsonResponse.Code, jsonResponse.Header().Get("Retry-After"))
	}

	other := postThrottledLogin(auth, "someone-else@example.com", "203.0.113.202:5000", "")
	if other.Header().Get("Location") != "/login?error=invalid_credentials" {
		t.Fatalf("unrelated account location = %q, want normal credential failure", other.Header().Get("Location"))
	}
}
