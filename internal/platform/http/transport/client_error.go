package transport

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// publicError marks an error whose message was written for API clients.
type publicError struct{ err error }

func (e publicError) Error() string { return e.err.Error() }
func (e publicError) Unwrap() error { return e.err }

// Public marks err's message as safe to show to clients even at a 5xx status,
// for domain conditions such as "serving snapshot is unavailable".
func Public(err error) error {
	if err == nil {
		return nil
	}
	return publicError{err: err}
}

// ClientMessage returns the text that may be shown to a client for err at the
// given response status. Messages of public errors and of ordinary 4xx
// failures (validation, not found, authorization) pass through. Server-side
// failures and infrastructure errors (database, filesystem, network) are
// replaced with a generic message carrying the request id, and the cause is
// logged with that id so operators can correlate it.
func ClientMessage(r *http.Request, status int, err error) string {
	if err == nil {
		return http.StatusText(status)
	}
	var public publicError
	if errors.As(err, &public) && !isInfrastructureError(err) {
		return err.Error()
	}
	if status < http.StatusInternalServerError && !isInfrastructureError(err) {
		return err.Error()
	}
	requestID := requestIDFor(nil, r)
	attrs := []any{"status", status, "requestId", requestID, "error", err}
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
		attrs = append(attrs, "method", r.Method, "path", r.URL.Path)
	}
	slog.ErrorContext(ctx, "request failed", attrs...)
	return genericMessage(status) + " (request id " + requestID + ")"
}

func genericMessage(status int) string {
	switch {
	case status == http.StatusServiceUnavailable:
		return "The service is temporarily unavailable"
	case status >= http.StatusInternalServerError:
		return "The server could not complete the request"
	default:
		return "The request could not be processed"
	}
}

// Error writes a plaintext error response with a sanitized message. It is the
// http.Error replacement for handlers that hold an error value.
func Error(w http.ResponseWriter, r *http.Request, status int, err error) {
	message := ClientMessage(r, status, err)
	w.Header().Set("X-Request-ID", requestIDFor(w, r))
	http.Error(w, message, status)
}

// RequestID returns the request id for r, assigning one when absent.
func RequestID(w http.ResponseWriter, r *http.Request) string { return requestIDFor(w, r) }

func requestIDFor(w http.ResponseWriter, r *http.Request) string {
	if r == nil {
		return NewRequestID()
	}
	if id := strings.TrimSpace(r.Header.Get("X-Request-ID")); id != "" {
		return id
	}
	if w != nil {
		if id := w.Header().Get("X-Request-ID"); id != "" {
			return id
		}
	}
	id := NewRequestID()
	r.Header.Set("X-Request-ID", id)
	return id
}

// isInfrastructureError reports errors whose text describes servers, drivers,
// files or sockets rather than the client's request.
func isInfrastructureError(err error) bool {
	var state interface{ SQLState() string }
	var pathErr *fs.PathError
	var syscallErr *os.SyscallError
	var netErr net.Error
	var urlErr *url.Error
	if errors.As(err, &state) || errors.As(err, &pathErr) || errors.As(err, &syscallErr) || errors.As(err, &netErr) || errors.As(err, &urlErr) {
		return true
	}
	return strings.Contains(err.Error(), "SQLSTATE")
}

