package module

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/flidai/leapview/internal/dashboard/publication"
)

func TestWritePublicStreamRegistrationFailure(t *testing.T) {
	retryAfter := strconv.Itoa(publication.StreamCapacityRetryAfterSeconds)
	tests := []struct {
		name       string
		err        error
		status     int
		code       string
		retryAfter string
		problem    bool
	}{
		{
			name:   "publication cap is rate limited",
			err:    fmt.Errorf("register: %w", &publication.StreamCapacityError{Scope: publication.StreamCapacityPublication, Limit: 256}),
			status: http.StatusTooManyRequests, code: "PUBLIC_DASHBOARD_STREAM_LIMIT", retryAfter: retryAfter, problem: true,
		},
		{
			name:   "process cap is unavailable",
			err:    &publication.StreamCapacityError{Scope: publication.StreamCapacityGlobal, Limit: 768},
			status: http.StatusServiceUnavailable, code: "PUBLIC_DASHBOARD_STREAM_CAPACITY", retryAfter: retryAfter, problem: true,
		},
		{
			name:   "other failures stay generic and retryable",
			err:    errors.New("database unavailable"),
			status: http.StatusServiceUnavailable, retryAfter: "1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/p/public/updates", nil)
			recorder := httptest.NewRecorder()
			writePublicStreamRegistrationFailure(recorder, request, test.err)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			if got := recorder.Header().Get("Retry-After"); got != test.retryAfter {
				t.Fatalf("Retry-After = %q, want %q", got, test.retryAfter)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
			if !test.problem {
				return
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Fatalf("Content-Type = %q, want application/problem+json", got)
			}
			var body struct {
				Status int    `json:"status"`
				Code   string `json:"code"`
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != test.status || body.Code != test.code || body.Detail == "" {
				t.Fatalf("problem = %+v", body)
			}
		})
	}
}
