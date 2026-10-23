package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	stdhttp "net/http"
	"time"

	"github.com/flidai/leapview/internal/agent"
)

// runEventReader is the slice of the agent service the run event stream needs.
type runEventReader interface {
	GetRun(ctx context.Context, scope agent.Scope, conversationID, runID string) (agent.Run, error)
	ListRunEventsPage(ctx context.Context, scope agent.Scope, conversationID, runID string, page agent.Page) ([]agent.Event, error)
}

type runEventStreamTiming struct {
	heartbeat, poll, reauthorize time.Duration
}

// pumpRunEvents sends a run's events until the run is terminal and fully
// drained. Each round reads the run status before draining, so a terminal
// event appended after the drain is picked up by the next round instead of
// being lost when the run turns terminal in between. Failures after the
// response headers were sent end the stream with an error event rather than
// a silent 200.
func pumpRunEvents(ctx context.Context, w io.Writer, flusher stdhttp.Flusher, source runEventReader, scope agent.Scope, conversationID, runID, lastID string, timing runEventStreamTiming) {
	heartbeat := time.NewTicker(timing.heartbeat)
	poll := time.NewTicker(timing.poll)
	reauthorize := time.NewTimer(timing.reauthorize)
	defer heartbeat.Stop()
	defer poll.Stop()
	defer reauthorize.Stop()
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	fail := func(err error) {
		if ctx.Err() != nil {
			return
		}
		slog.Warn("agent run event stream failed", "run_id", runID, "conversation_id", conversationID, "error", err)
		payload, _ := json.Marshal(map[string]string{"code": "stream_failed", "message": "The event stream ended unexpectedly; reconnect with Last-Event-ID to resume."})
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
		flush()
	}

	for {
		run, err := source.GetRun(ctx, scope, conversationID, runID)
		if err != nil {
			fail(err)
			return
		}
		terminal := agentRunTerminal(run.Status)
		for {
			page, pageErr := source.ListRunEventsPage(ctx, scope, conversationID, runID, agent.Page{Limit: 100, After: lastID})
			if pageErr != nil {
				fail(pageErr)
				return
			}
			for _, event := range page {
				payload, _ := json.Marshal(agentEventDTO(event))
				_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.EventType, payload)
				lastID = event.ID
			}
			if len(page) < 100 {
				break
			}
		}
		flush()
		if terminal {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-reauthorize.C:
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flush()
		case <-poll.C:
		}
	}
}
