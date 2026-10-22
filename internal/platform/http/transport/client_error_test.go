package transport

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sqlStateError struct{ code string }

func (e sqlStateError) Error() string    { return "ERROR: boom (SQLSTATE " + e.code + ")" }
func (e sqlStateError) SQLState() string { return e.code }

func TestClientMessageHidesInternalErrorsAndLogsCause(t *testing.T) {
	var logged strings.Builder
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
	request.Header.Set("X-Request-ID", "req-123")
	cases := []struct {
		name   string
		status int
		err    error
		safe   bool
	}{
		{"unclassified 500", 500, errors.New("dial tcp 10.0.0.5:5432: connect refused"), false},
		{"unclassified 503", 503, errors.New("open /var/lib/leapview/state.db: denied"), false},
		{"database error on 400", 400, fmt.Errorf("create user: %w", sqlStateError{"23514"}), false},
		{"wrapped path error on 400", 400, fmt.Errorf("read: %w", &fs.PathError{Op: "open", Path: "/etc/secret", Err: errors.New("denied")}), false},
		{"sqlstate text only", 400, errors.New("violates check constraint (SQLSTATE 23514)"), false},
		{"validation 400", 400, errors.New("limit must be at least 1"), true},
		{"not found 404", 404, errors.New(`dashboard "x" not found`), true},
		{"public 503", 503, Public(errors.New("serving snapshot is unavailable")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			message := ClientMessage(request, tc.status, tc.err)
			if tc.safe {
				if message != tc.err.Error() {
					t.Fatalf("message = %q, want %q", message, tc.err.Error())
				}
				return
			}
			if strings.Contains(message, tc.err.Error()) || strings.Contains(message, "SQLSTATE") || strings.Contains(message, "/etc") || strings.Contains(message, "10.0.0.5") {
				t.Fatalf("message leaks cause: %q", message)
			}
			if !strings.Contains(message, "req-123") {
				t.Fatalf("message %q does not carry the request id", message)
			}
		})
	}
	if !strings.Contains(logged.String(), "10.0.0.5") || !strings.Contains(logged.String(), "req-123") {
		t.Fatalf("cause and request id were not logged: %s", logged.String())
	}
}

func TestErrorWritesSanitizedPlaintext(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/x", nil)
	recorder := httptest.NewRecorder()
	Error(recorder, request, http.StatusInternalServerError, errors.New("pq: password authentication failed"))
	if recorder.Code != 500 || strings.Contains(recorder.Body.String(), "password") {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected a request id header")
	}
}
