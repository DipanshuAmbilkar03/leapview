package publication

import (
	"context"
	"sync"

	"github.com/flidai/leapview/internal/dashboard"
	"github.com/flidai/leapview/internal/dashboard/command"
)

type StreamVersion struct {
	PublicID       string
	ServingStateID string
}

type StreamRegistry interface {
	Register(context.Context, string, string, StreamVersion, ...dashboard.Filters) (context.Context, func(), error)
	PrepareCommand(context.Context, string, string, StreamVersion, func(dashboard.Filters) (command.PreparedRefresh, error)) (command.PreparedRefresh, uint64, error)
	Active(string, string, StreamVersion) bool
	Reconcile(context.Context, map[string]StreamVersion)
	ClosePublication(string)
}

type memoryStreamRegistry struct {
	mu      sync.Mutex
	streams map[string]map[string]*memoryStream
	limits  StreamLimits
}

// StreamRegistryOptions configures a stream registry.
type StreamRegistryOptions struct {
	// Limits caps concurrent streams per process; see StreamLimits.
	Limits StreamLimits
}

// StreamRegistryOption mutates StreamRegistryOptions.
type StreamRegistryOption func(*StreamRegistryOptions)

// WithStreamLimits sets the per-process stream caps.
func WithStreamLimits(limits StreamLimits) StreamRegistryOption {
	return func(o *StreamRegistryOptions) { o.Limits = limits }
}

// ResolveStreamRegistryOptions applies options over the defaults.
func ResolveStreamRegistryOptions(options ...StreamRegistryOption) StreamRegistryOptions {
	var resolved StreamRegistryOptions
	for _, option := range options {
		if option != nil {
			option(&resolved)
		}
	}
	resolved.Limits = resolved.Limits.Resolved()
	return resolved
}

type memoryStream struct {
	cancel  context.CancelFunc
	version StreamVersion
}

func NewMemoryStreamRegistry(options ...StreamRegistryOption) StreamRegistry {
	return &memoryStreamRegistry{streams: map[string]map[string]*memoryStream{}, limits: ResolveStreamRegistryOptions(options...).Limits}
}

func (r *memoryStreamRegistry) Register(parent context.Context, publicationID, streamID string, version StreamVersion, _ ...dashboard.Filters) (context.Context, func(), error) {
	r.mu.Lock()
	// Replacing a stream (a reconnect with the same identity) never consumes an
	// additional slot. Counts are derived from the map every removal path
	// mutates, so a slot cannot leak or be released twice.
	if r.streams[publicationID][streamID] == nil {
		total := 0
		for _, streams := range r.streams {
			total += len(streams)
		}
		if err := r.limits.Admit(len(r.streams[publicationID]), total); err != nil {
			r.mu.Unlock()
			return parent, func() {}, err
		}
	}
	ctx, cancel := context.WithCancel(parent)
	if r.streams[publicationID] == nil {
		r.streams[publicationID] = map[string]*memoryStream{}
	}
	if previous := r.streams[publicationID][streamID]; previous != nil {
		previous.cancel()
	}
	registration := &memoryStream{cancel: cancel, version: version}
	r.streams[publicationID][streamID] = registration
	r.mu.Unlock()
	return ctx, func() {
		r.mu.Lock()
		if current := r.streams[publicationID][streamID]; current == registration {
			delete(r.streams[publicationID], streamID)
			if len(r.streams[publicationID]) == 0 {
				delete(r.streams, publicationID)
			}
		}
		r.mu.Unlock()
		cancel()
	}, nil
}

func (r *memoryStreamRegistry) PrepareCommand(context.Context, string, string, StreamVersion, func(dashboard.Filters) (command.PreparedRefresh, error)) (command.PreparedRefresh, uint64, error) {
	return command.PreparedRefresh{}, 0, ErrStreamStateUnavailable
}

func (r *memoryStreamRegistry) Active(publicationID, streamID string, version StreamVersion) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	stream := r.streams[publicationID][streamID]
	return stream != nil && stream.version == version
}

func (r *memoryStreamRegistry) Reconcile(_ context.Context, active map[string]StreamVersion) {
	r.mu.Lock()
	stale := []context.CancelFunc{}
	for publicationID, streams := range r.streams {
		current, ok := active[publicationID]
		for streamID, stream := range streams {
			if ok && stream.version == current {
				continue
			}
			stale = append(stale, stream.cancel)
			delete(streams, streamID)
		}
		if len(streams) == 0 {
			delete(r.streams, publicationID)
		}
	}
	r.mu.Unlock()
	for _, cancel := range stale {
		cancel()
	}
}

func (r *memoryStreamRegistry) ClosePublication(publicationID string) {
	r.mu.Lock()
	streams := r.streams[publicationID]
	delete(r.streams, publicationID)
	r.mu.Unlock()
	for _, stream := range streams {
		stream.cancel()
	}
}

// StreamLimitConfigurer is implemented by registries whose per-process stream
// caps can be set after construction (for example by application composition).
type StreamLimitConfigurer interface {
	SetStreamLimits(StreamLimits)
}

// SetStreamLimits replaces the per-process caps. Streams that are already
// registered are never evicted by lowering a cap; the new cap applies to
// subsequent registrations.
func (r *memoryStreamRegistry) SetStreamLimits(limits StreamLimits) {
	r.mu.Lock()
	r.limits = limits.Resolved()
	r.mu.Unlock()
}
