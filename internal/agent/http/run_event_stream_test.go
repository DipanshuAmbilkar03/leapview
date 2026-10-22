package http

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/agent"
)

// scriptedRunEvents models a run whose status and event log change between
// the calls the stream makes.
type scriptedRunEvents struct {
	mu        sync.Mutex
	statuses  []string // returned by successive GetRun calls; the last repeats
	events    []agent.Event
	appendOn  map[int][]agent.Event // events appended after the Nth ListRunEventsPage call
	listCalls int
	getCalls  int
	listErr   error
	listErrAt int
}

func (s *scriptedRunEvents) GetRun(context.Context, agent.Scope, string, string) (agent.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.statuses[min(s.getCalls, len(s.statuses)-1)]
	s.getCalls++
	return agent.Run{Status: status}, nil
}

func (s *scriptedRunEvents) ListRunEventsPage(_ context.Context, _ agent.Scope, _, _ string, page agent.Page) ([]agent.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	if s.listErr != nil && s.listCalls >= s.listErrAt {
		return nil, s.listErr
	}
	var out []agent.Event
	for _, event := range s.events {
		if event.ID > page.After {
			out = append(out, event)
		}
	}
	s.events = append(s.events, s.appendOn[s.listCalls]...)
	return out, nil
}

func runEvent(seq int64, eventType string) agent.Event {
	return agent.Event{ID: fmt.Sprintf("%020d", seq), Seq: seq, EventType: eventType, RunID: "run-1"}
}

func pumpForTest(t *testing.T, source runEventReader) string {
	t.Helper()
	var out strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		pumpRunEvents(t.Context(), &out, nil, source, agent.Scope{}, "conv-1", "run-1", "", runEventStreamTiming{
			heartbeat: time.Hour, poll: time.Millisecond, reauthorize: time.Hour,
		})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not finish")
	}
	return out.String()
}

func TestPumpRunEventsSendsTerminalEventAppendedAfterDrain(t *testing.T) {
	// The run is already terminal when status is read, and its final event is
	// appended after the first drain completed: it must still be sent.
	source := &scriptedRunEvents{
		statuses: []string{agent.RunStatusRunning, agent.RunStatusCompleted},
		events:   []agent.Event{runEvent(1, "message")},
		appendOn: map[int][]agent.Event{1: {runEvent(2, "run.completed")}},
	}
	body := pumpForTest(t, source)
	if !strings.Contains(body, "event: run.completed") {
		t.Fatalf("terminal event was not streamed:\n%s", body)
	}
	if strings.Index(body, "event: message") > strings.Index(body, "event: run.completed") {
		t.Fatalf("events out of order:\n%s", body)
	}
}

func TestPumpRunEventsDrainsAfterTerminalStatus(t *testing.T) {
	source := &scriptedRunEvents{
		statuses: []string{agent.RunStatusCompleted},
		events:   []agent.Event{runEvent(1, "message"), runEvent(2, "run.completed")},
	}
	body := pumpForTest(t, source)
	if strings.Count(body, "event: ") != 2 || source.getCalls != 1 {
		t.Fatalf("body=%s getCalls=%d", body, source.getCalls)
	}
}

func TestPumpRunEventsReportsMidStreamFailure(t *testing.T) {
	source := &scriptedRunEvents{
		statuses:  []string{agent.RunStatusRunning},
		events:    []agent.Event{runEvent(1, "message")},
		listErr:   errors.New("database unavailable"),
		listErrAt: 2,
	}
	body := pumpForTest(t, source)
	if !strings.Contains(body, "event: message") || !strings.Contains(body, "event: error\ndata: ") {
		t.Fatalf("expected the delivered event then an error event:\n%s", body)
	}
	if strings.Contains(body, "database unavailable") {
		t.Fatalf("error event leaked the internal cause:\n%s", body)
	}
}
