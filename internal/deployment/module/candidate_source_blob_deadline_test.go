package module

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type readDeadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
	err      error
}

func (r *readDeadlineRecorder) SetReadDeadline(deadline time.Time) error {
	r.deadline = deadline
	return r.err
}

// The server-wide 30 s ReadTimeout is too short for a slow link uploading a
// source blob, so the route extends its own read deadline.
func TestExtendCandidateSourceBlobReadDeadlineExtendsPastServerReadTimeout(t *testing.T) {
	recorder := &readDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	if err := extendCandidateSourceBlobReadDeadline(recorder, now); err != nil {
		t.Fatal(err)
	}
	if got, want := recorder.deadline, now.Add(candidateSourceBlobReadTimeout); !got.Equal(want) {
		t.Fatalf("read deadline = %s, want %s", got, want)
	}
	if candidateSourceBlobReadTimeout <= 30*time.Second {
		t.Fatalf("candidateSourceBlobReadTimeout = %s, must exceed the 30s server ReadTimeout", candidateSourceBlobReadTimeout)
	}
}

func TestExtendCandidateSourceBlobReadDeadlineToleratesUnsupportedWritersButNotFailures(t *testing.T) {
	if err := extendCandidateSourceBlobReadDeadline(httptest.NewRecorder(), time.Now()); err != nil {
		t.Fatalf("writer without deadline support error = %v, want nil", err)
	}
	failure := errors.New("deadline rejected")
	recorder := &readDeadlineRecorder{ResponseRecorder: httptest.NewRecorder(), err: failure}
	if err := extendCandidateSourceBlobReadDeadline(recorder, time.Now()); !errors.Is(err, failure) {
		t.Fatalf("deadline failure = %v, want %v", err, failure)
	}
	var _ http.ResponseWriter = recorder
}
