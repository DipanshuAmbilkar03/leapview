package httpmiddleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func authLimiter(useRealIP bool, limit int) http.Handler {
	config := RateLimitConfig{Enabled: true, UseRealIP: useRealIP, AuthLimit: limit, AuthWindow: time.Minute}
	return config.Auth()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
}

func loginRequest(remoteAddr string, headers map[string]string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/auth/local/login", nil)
	request.RemoteAddr = remoteAddr
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return request
}

func serve(handler http.Handler, request *http.Request) int {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response.Code
}

func TestRateLimitIgnoresClientSuppliedAddressHeadersWhenProxyNotTrusted(t *testing.T) {
	handler := authLimiter(false, 3)
	var last int
	for i := range 10 {
		last = serve(handler, loginRequest("203.0.113.9:4000", map[string]string{
			"True-Client-IP":  fmt.Sprintf("198.51.100.%d", i),
			"X-Real-IP":       fmt.Sprintf("198.51.100.%d", i),
			"X-Forwarded-For": fmt.Sprintf("198.51.100.%d", i),
		}))
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status after rotating spoofed headers = %d, want %d", last, http.StatusTooManyRequests)
	}
}

func TestRateLimitTrustedProxyIgnoresRotatingTrueClientIPAndRealIP(t *testing.T) {
	handler := authLimiter(true, 3)
	var last int
	for i := range 10 {
		last = serve(handler, loginRequest("172.18.0.5:4000", map[string]string{
			"True-Client-IP":  fmt.Sprintf("198.51.100.%d", i),
			"X-Real-IP":       fmt.Sprintf("198.51.100.%d", i),
			"X-Forwarded-For": "203.0.113.9",
		}))
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status after rotating True-Client-IP/X-Real-IP = %d, want %d", last, http.StatusTooManyRequests)
	}
}

func TestRateLimitTrustedProxyIgnoresClientSuppliedForwardedForPrefix(t *testing.T) {
	handler := authLimiter(true, 3)
	var last int
	for i := range 10 {
		last = serve(handler, loginRequest("172.18.0.5:4000", map[string]string{
			"X-Forwarded-For": fmt.Sprintf("198.51.100.%d, 203.0.113.9", i),
		}))
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status after rotating leftmost X-Forwarded-For = %d, want %d", last, http.StatusTooManyRequests)
	}
}

func TestRateLimitTrustedProxyKeysClientsByProxyAppendedAddress(t *testing.T) {
	handler := authLimiter(true, 2)
	for _, client := range []string{"203.0.113.1", "203.0.113.2", "2001:db8::1"} {
		for attempt := 1; attempt <= 2; attempt++ {
			if got := serve(handler, loginRequest("172.18.0.5:4000", map[string]string{"X-Forwarded-For": client})); got != http.StatusNoContent {
				t.Fatalf("client %s attempt %d = %d, want %d", client, attempt, got, http.StatusNoContent)
			}
		}
		if got := serve(handler, loginRequest("172.18.0.5:4000", map[string]string{"X-Forwarded-For": client})); got != http.StatusTooManyRequests {
			t.Fatalf("client %s over budget = %d, want %d", client, got, http.StatusTooManyRequests)
		}
	}
}

func TestRateLimitTrustedProxyFallsBackToPeerWithoutUsableForwardedFor(t *testing.T) {
	handler := authLimiter(true, 2)
	var last int
	for _, forwarded := range []string{"", "not-an-ip", "unknown", ""} {
		headers := map[string]string{"True-Client-IP": "198.51.100.77"}
		if forwarded != "" {
			headers["X-Forwarded-For"] = forwarded
		}
		last = serve(handler, loginRequest("172.18.0.5:4000", headers))
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status without usable X-Forwarded-For = %d, want %d", last, http.StatusTooManyRequests)
	}
}
