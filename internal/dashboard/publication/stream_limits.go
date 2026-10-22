package publication

import (
	"errors"
	"fmt"
)

const (
	// DefaultMaxStreamsPerPublication bounds concurrent anonymous SSE streams for
	// one publication on one LeapView process.
	DefaultMaxStreamsPerPublication = 256
	// DefaultMaxStreamsTotal bounds concurrent anonymous SSE streams across all
	// publications on one process. It deliberately stays below the dashboard
	// stream coordinator registry capacity (stream.DefaultRegistryMaxEntries) so
	// anonymous viewers cannot consume the slots that authenticated dashboards
	// and command coordinators need.
	DefaultMaxStreamsTotal = 768
	// StreamCapacityRetryAfterSeconds is advertised on capacity rejections. SSE
	// clients reconnect automatically, so the hint also spreads reconnect storms.
	StreamCapacityRetryAfterSeconds = 5
)

// StreamLimits caps concurrent public dashboard streams.
//
// Caps are enforced per process, against the streams registered locally, not
// against the cluster-wide durable view. A stream is a long-lived HTTP response
// plus a coordinator and the query work it triggers, all of which live in the
// serving process, so the process is the resource being protected. Counting
// locally is exact (the count is derived from the same map every close path
// mutates), needs no database round trip on the anonymous hot path, and cannot
// be skewed by a lagging durable lease. The cluster-wide ceiling is therefore
// the cap multiplied by the number of serving processes.
//
// A zero field selects the default. A negative field disables that cap.
type StreamLimits struct {
	PerPublication int
	Total          int
}

// Resolved replaces zero fields with the defaults.
func (l StreamLimits) Resolved() StreamLimits {
	if l.PerPublication == 0 {
		l.PerPublication = DefaultMaxStreamsPerPublication
	}
	if l.Total == 0 {
		l.Total = DefaultMaxStreamsTotal
	}
	return l
}

// StreamCapacityScope names which cap rejected a registration.
type StreamCapacityScope string

const (
	StreamCapacityPublication StreamCapacityScope = "publication"
	StreamCapacityGlobal      StreamCapacityScope = "global"
)

// ErrStreamCapacity matches every StreamCapacityError.
var ErrStreamCapacity = errors.New("public dashboard stream capacity reached")

// StreamCapacityError reports a rejected registration and which cap applied.
type StreamCapacityError struct {
	Scope StreamCapacityScope
	Limit int
}

func (e *StreamCapacityError) Error() string {
	return fmt.Sprintf("%s: %s limit of %d streams", ErrStreamCapacity, e.Scope, e.Limit)
}

func (e *StreamCapacityError) Unwrap() error { return ErrStreamCapacity }

// Admit reports whether one more stream fits given the counts of streams that
// are already registered (or reserved) for the publication and in total. The
// receiver must already be Resolved.
func (l StreamLimits) Admit(publicationStreams, totalStreams int) error {
	if l.PerPublication > 0 && publicationStreams >= l.PerPublication {
		return &StreamCapacityError{Scope: StreamCapacityPublication, Limit: l.PerPublication}
	}
	if l.Total > 0 && totalStreams >= l.Total {
		return &StreamCapacityError{Scope: StreamCapacityGlobal, Limit: l.Total}
	}
	return nil
}
