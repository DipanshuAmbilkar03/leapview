package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/flidai/leapview/internal/dashboard"
	"github.com/flidai/leapview/internal/dashboard/command"
	"github.com/flidai/leapview/internal/dashboard/publication"
	publicationdb "github.com/flidai/leapview/internal/dashboard/publication/postgres/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const streamLease = 90 * time.Second
const streamHeartbeat = 30 * time.Second

const (
	// streamDBTimeout bounds every individual registry round trip so a slow or
	// partitioned database cannot pin registrations, heartbeats, or command
	// requests indefinitely.
	streamDBTimeout = 5 * time.Second
	// reconcileFailureGrace is how long ownership may stay unverifiable before
	// local streams are canceled fail-closed. One failed (or timed out) read of
	// the durable registrations is routine under load or failover and must not
	// tear down every viewer at once. Commands remain fenced regardless: Active
	// and PrepareCommand verify the durable registration on every command, so
	// tolerating a stale ownership view only delays canceling a superseded
	// stream; it never lets a superseded node execute a command. The grace is
	// half the heartbeat interval and far below the lease.
	reconcileFailureGrace = streamHeartbeat / 2
)

// StreamRegistry tracks anonymous dashboard streams registered with this
// process and mirrors their ownership to PostgreSQL.
//
// Locking: mu guards only the in-memory maps and is never held across database
// I/O. Registrations for the same (publication, stream) identity are
// serialized by a per-key lock so the database winner and the local winner
// cannot diverge; unrelated streams, commands, and reconciliation proceed
// concurrently. ClosePublication fences in-flight registrations with an epoch
// instead of a lock (see inflightPublication).
type StreamRegistry struct {
	mu       sync.Mutex
	streams  map[string]map[string]*localStream
	inflight map[string]*inflightPublication
	closing  map[string]int
	keyLocks map[streamKey]*keyLock
	// reservedTotal is the sum of inflightPublication.reserved: slots promised
	// to registrations whose database upsert is still in flight.
	reservedTotal int
	limits        publication.StreamLimits
	// unverifiedSince is when durable ownership first became unreadable; zero
	// while the last reconciliation succeeded or there was nothing to verify.
	unverifiedSince time.Time
	now             func() time.Time
	dbTimeout       time.Duration
	db              DBTX
	q               *publicationdb.Queries
}

// inflightPublication tracks registrations of one publication between their
// admission and their local install. epoch is bumped by ClosePublication, so a
// registration whose upsert raced with a close observes the change and backs
// out instead of installing a stream the close already expired.
type inflightPublication struct {
	registers int
	reserved  int
	epoch     uint64
}

// keyLock is a context-aware mutex for one stream identity.
type keyLock struct {
	held chan struct{}
	refs int
}

// MaintenanceDBTX is the intentionally narrow database surface required by
// retention work. Keeping it separate from the runtime registry's DBTX makes
// the destructive capability explicit at construction and review boundaries.
type MaintenanceDBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Maintenance owns destructive retention work and must be constructed with a
// database role granted DELETE on stream state. Runtime stream registries do
// not receive that authority.
type Maintenance struct{ q *publicationdb.Queries }

func NewMaintenance(db MaintenanceDBTX) *Maintenance {
	if db == nil {
		return &Maintenance{}
	}
	return &Maintenance{q: publicationdb.New(db)}
}

func (m *Maintenance) PruneExpired(ctx context.Context, now, _ time.Time, batchLimit int32) error {
	if m == nil || m.q == nil {
		return fmt.Errorf("publication maintenance database is unavailable")
	}
	if batchLimit == 0 {
		batchLimit = 1000
	}
	if batchLimit < 1 || batchLimit > 1000 {
		return fmt.Errorf("publication maintenance batch limit must be between 1 and 1000")
	}
	if _, err := m.q.DeleteExpiredStreams(ctx, publicationdb.DeleteExpiredStreamsParams{Now: now, BatchLimit: batchLimit}); err != nil {
		return err
	}
	return nil
}

type localStream struct {
	cancel         context.CancelFunc
	version        publication.StreamVersion
	registrationID string
}

type streamKey struct {
	publicationID string
	streamID      string
}

// NewStreamRegistry creates a registry. Options may set the per-process stream
// caps; unset caps use publication.DefaultMaxStreamsPerPublication and
// publication.DefaultMaxStreamsTotal (see publication.StreamLimits for the
// per-process scope).
func NewStreamRegistry(db DBTX, options ...publication.StreamRegistryOption) *StreamRegistry {
	registry := &StreamRegistry{
		streams:   map[string]map[string]*localStream{},
		inflight:  map[string]*inflightPublication{},
		closing:   map[string]int{},
		keyLocks:  map[streamKey]*keyLock{},
		limits:    publication.ResolveStreamRegistryOptions(options...).Limits,
		now:       time.Now,
		dbTimeout: streamDBTimeout,
	}
	if db == nil {
		return registry
	}
	registry.db, registry.q = db, publicationdb.New(db)
	return registry
}

// IsNative marks a registry backed by a configured PostgreSQL capability.
func (r *StreamRegistry) IsNative() bool { return r != nil && r.db != nil && r.q != nil }

// SetStreamLimits replaces the per-process caps. Registered streams are never
// evicted by lowering a cap; it applies to subsequent registrations.
func (r *StreamRegistry) SetStreamLimits(limits publication.StreamLimits) {
	r.mu.Lock()
	r.limits = limits.Resolved()
	r.mu.Unlock()
}

// withTimeout runs fn with a context bounded by the registry database timeout.
func withTimeout[T any](r *StreamRegistry, parent context.Context, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(parent, r.dbTimeout)
	defer cancel()
	return fn(ctx)
}

func (r *StreamRegistry) lockKey(ctx context.Context, key streamKey) error {
	r.mu.Lock()
	lock := r.keyLocks[key]
	if lock == nil {
		lock = &keyLock{held: make(chan struct{}, 1)}
		r.keyLocks[key] = lock
	}
	lock.refs++
	r.mu.Unlock()
	select {
	case lock.held <- struct{}{}:
		return nil
	case <-ctx.Done():
		r.releaseKeyRef(key, lock)
		return ctx.Err()
	}
}

func (r *StreamRegistry) unlockKey(key streamKey) {
	r.mu.Lock()
	lock := r.keyLocks[key]
	<-lock.held
	r.releaseKeyRefLocked(key, lock)
	r.mu.Unlock()
}

func (r *StreamRegistry) releaseKeyRef(key streamKey, lock *keyLock) {
	r.mu.Lock()
	r.releaseKeyRefLocked(key, lock)
	r.mu.Unlock()
}

func (r *StreamRegistry) releaseKeyRefLocked(key streamKey, lock *keyLock) {
	lock.refs--
	if lock.refs == 0 && r.keyLocks[key] == lock {
		delete(r.keyLocks, key)
	}
}

// localTotalLocked counts installed streams. Counts are always derived from
// the maps every removal path mutates, so a slot can neither leak nor be
// released twice.
func (r *StreamRegistry) localTotalLocked() int {
	total := 0
	for _, streams := range r.streams {
		total += len(streams)
	}
	return total
}

func (r *StreamRegistry) admitLocked(publicationID string) error {
	reserved := 0
	if flight := r.inflight[publicationID]; flight != nil {
		reserved = flight.reserved
	}
	return r.limits.Admit(len(r.streams[publicationID])+reserved, r.localTotalLocked()+r.reservedTotal)
}

// expireRegistration ends one durable registration without relying on the
// caller's context, which may already be canceled.
func (r *StreamRegistry) expireRegistration(parent context.Context, publicationUUID pgtype.UUID, streamID string, registrationUUID pgtype.UUID) {
	_, _ = withTimeout(r, context.WithoutCancel(parent), func(ctx context.Context) (int64, error) {
		return r.q.ExpireStreamRegistration(ctx, publicationdb.ExpireStreamRegistrationParams{
			PublicationID: publicationUUID, StreamID: streamID, RegistrationID: registrationUUID,
		})
	})
}

func (r *StreamRegistry) Register(parent context.Context, publicationID, streamID string, version publication.StreamVersion, initialFilters ...dashboard.Filters) (context.Context, func(), error) {
	ctx, cancel := context.WithCancel(parent)
	fail := func(err error) (context.Context, func(), error) {
		cancel()
		return ctx, func() {}, err
	}
	registrationID, err := streamRegistrationID()
	if err != nil {
		return fail(err)
	}
	filters := dashboard.Filters{}.WithDefaults()
	if len(initialFilters) > 0 {
		filters = initialFilters[0].WithDefaults()
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return fail(err)
	}
	publicationUUID, err := nativeUUID(publicationID)
	if err != nil {
		return fail(err)
	}
	registrationUUID, err := nativeUUID(registrationID)
	if err != nil {
		return fail(err)
	}

	// Two concurrent registrations of one identity can commit in database order
	// but install locally in the opposite order, leaving the database and this
	// process disagreeing about the winner. The per-key lock serializes exactly
	// that pair; mu is never held across the upsert below.
	key := streamKey{publicationID: publicationID, streamID: streamID}
	if err := r.lockKey(parent, key); err != nil {
		return fail(err)
	}
	defer r.unlockKey(key)

	r.mu.Lock()
	if r.closing[publicationID] > 0 {
		r.mu.Unlock()
		return fail(fmt.Errorf("publication streams are closing: %w", publication.ErrStreamStateUnavailable))
	}
	// A reconnect that replaces a live stream of the same identity needs no
	// additional slot; anything else reserves one for the duration of the
	// upsert so concurrent registrations cannot overshoot the caps.
	reserve := r.streams[publicationID][streamID] == nil
	if reserve {
		if err := r.admitLocked(publicationID); err != nil {
			r.mu.Unlock()
			return fail(err)
		}
	}
	flight := r.inflight[publicationID]
	if flight == nil {
		flight = &inflightPublication{}
		r.inflight[publicationID] = flight
	}
	flight.registers++
	if reserve {
		flight.reserved++
		r.reservedTotal++
	}
	epoch := flight.epoch
	r.mu.Unlock()

	_, upsertErr := withTimeout(r, parent, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, r.q.UpsertStream(ctx, publicationdb.UpsertStreamParams{
			PublicationID: publicationUUID, StreamID: streamID,
			PublicID: version.PublicID, ServingStateID: version.ServingStateID,
			RegistrationID: registrationUUID, FiltersJson: filtersJSON, ExpiresAt: streamExpiry(),
		})
	})

	r.mu.Lock()
	closedDuringUpsert := flight.epoch != epoch
	flight.registers--
	if reserve {
		flight.reserved--
		r.reservedTotal--
	}
	if flight.registers == 0 {
		delete(r.inflight, publicationID)
	}
	var admitErr error
	if upsertErr == nil && !closedDuringUpsert && !reserve && r.streams[publicationID][streamID] == nil {
		// The stream we meant to replace vanished while the upsert was in
		// flight, so this registration now needs the slot it did not reserve.
		admitErr = r.admitLocked(publicationID)
	}
	if upsertErr != nil || closedDuringUpsert || admitErr != nil {
		r.mu.Unlock()
		// The upsert may have committed even when it reported an error (timeout
		// after commit) or when a concurrent close already expired the
		// publication. Expiring our own registration id is idempotent, scoped to
		// this registration, and keeps the durable view free of orphans.
		r.expireRegistration(parent, publicationUUID, streamID, registrationUUID)
		switch {
		case upsertErr != nil:
			return fail(upsertErr)
		case admitErr != nil:
			return fail(admitErr)
		default:
			return fail(fmt.Errorf("publication streams were closed during registration: %w", publication.ErrStreamStateUnavailable))
		}
	}
	if r.streams[publicationID] == nil {
		r.streams[publicationID] = map[string]*localStream{}
	}
	previous := r.streams[publicationID][streamID]
	registration := &localStream{cancel: cancel, version: version, registrationID: registrationID}
	r.streams[publicationID][streamID] = registration
	r.mu.Unlock()
	if previous != nil {
		previous.cancel()
	}
	go r.heartbeat(ctx, publicationID, streamID, version, registrationID)
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
		r.expireRegistration(parent, publicationUUID, streamID, registrationUUID)
	}, nil
}

func (r *StreamRegistry) PrepareCommand(ctx context.Context, publicationID, streamID string, version publication.StreamVersion, prepare func(dashboard.Filters) (command.PreparedRefresh, error)) (command.PreparedRefresh, uint64, error) {
	if prepare == nil {
		return command.PreparedRefresh{}, 0, fmt.Errorf("publication command preparation is required")
	}
	publicationUUID, err := nativeUUID(publicationID)
	if err != nil {
		return command.PreparedRefresh{}, 0, err
	}
	registrationID, ok := r.currentRegistration(publicationID, streamID, version)
	if !ok {
		return command.PreparedRefresh{}, 0, publication.ErrStreamStateUnavailable
	}
	registrationUUID, err := nativeUUID(registrationID)
	if err != nil {
		return command.PreparedRefresh{}, 0, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		loadCtx, cancelLoad := context.WithTimeout(ctx, r.dbTimeout)
		row, err := r.q.GetCommandState(loadCtx, publicationdb.GetCommandStateParams{
			PublicationID: publicationUUID, StreamID: streamID,
			PublicID: version.PublicID, ServingStateID: version.ServingStateID,
			RegistrationID: registrationUUID,
		})
		cancelLoad()
		if err != nil {
			return command.PreparedRefresh{}, 0, fmt.Errorf("load publication command state: %w", err)
		}
		var filters dashboard.Filters
		if err := json.Unmarshal([]byte(row.FiltersJson), &filters); err != nil {
			return command.PreparedRefresh{}, 0, fmt.Errorf("decode publication command state: %w", err)
		}
		prepared, err := prepare(filters.WithDefaults())
		if err != nil {
			return command.PreparedRefresh{}, 0, err
		}
		nextFilters, err := json.Marshal(prepared.Filters.WithDefaults())
		if err != nil {
			return command.PreparedRefresh{}, 0, err
		}
		nextGeneration := row.Generation + 1
		updateCtx, cancelUpdate := context.WithTimeout(ctx, r.dbTimeout)
		result, err := r.q.UpdateCommandState(updateCtx, publicationdb.UpdateCommandStateParams{
			FiltersJson: nextFilters, NextGeneration: nextGeneration, ExpiresAt: streamExpiry(),
			PublicationID: publicationUUID, StreamID: streamID,
			PublicID: version.PublicID, ServingStateID: version.ServingStateID,
			RegistrationID:    registrationUUID,
			CurrentGeneration: row.Generation,
		})
		cancelUpdate()
		if err != nil {
			return command.PreparedRefresh{}, 0, err
		}
		changed := result
		if changed == 1 {
			return prepared, uint64(nextGeneration), nil
		}
	}
	return command.PreparedRefresh{}, 0, fmt.Errorf("publication command state changed concurrently")
}

// currentRegistration binds command CAS operations to the local stream
// registration. A durable row with the right public/serving version is not
// sufficient: a newer registration for the same stream must fence this
// process's in-flight command.
func (r *StreamRegistry) currentRegistration(publicationID, streamID string, version publication.StreamVersion) (string, bool) {
	r.mu.Lock()
	stream := r.streams[publicationID][streamID]
	if stream == nil || stream.version != version {
		r.mu.Unlock()
		return "", false
	}
	registrationID := stream.registrationID
	r.mu.Unlock()
	return registrationID, true
}

func (r *StreamRegistry) Active(publicationID, streamID string, version publication.StreamVersion) bool {
	publicationUUID, err := nativeUUID(publicationID)
	if err != nil {
		return false
	}
	registrationID, ok := r.currentRegistration(publicationID, streamID, version)
	if !ok {
		return false
	}
	registrationUUID, err := nativeUUID(registrationID)
	if err != nil {
		return false
	}
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), r.dbTimeout)
	defer cancelCheck()
	exists, err := r.q.StreamActive(checkCtx, publicationdb.StreamActiveParams{
		PublicationID: publicationUUID, StreamID: streamID,
		PublicID: version.PublicID, ServingStateID: version.ServingStateID,
		RegistrationID: registrationUUID,
	})
	return err == nil && exists
}

type localSnapshot struct {
	publicationID string
	streamID      string
	stream        *localStream
}

// Reconcile cancels local streams whose publication version changed or whose
// durable registration is no longer ours.
//
// The durable read happens outside mu and only covers publications that have
// local streams; a process with no local streams issues no query at all. The
// decision is applied to the exact stream instances that were snapshotted: a
// stream registered or replaced while the read was in flight is untouched
// (its registration committed after the snapshot or replaced the instance), so
// stale durable data can never cancel a newer registration.
func (r *StreamRegistry) Reconcile(ctx context.Context, active map[string]publication.StreamVersion) {
	r.mu.Lock()
	snapshot := make([]localSnapshot, 0, r.localTotalLocked())
	publicationIDs := make([]string, 0, len(r.streams))
	for publicationID, streams := range r.streams {
		publicationIDs = append(publicationIDs, publicationID)
		for streamID, stream := range streams {
			snapshot = append(snapshot, localSnapshot{publicationID: publicationID, streamID: streamID, stream: stream})
		}
	}
	if len(snapshot) == 0 {
		r.unverifiedSince = time.Time{}
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	durable, verified := r.loadDurableRegistrations(ctx, publicationIDs)

	r.mu.Lock()
	failClosed := false
	switch {
	case verified:
		r.unverifiedSince = time.Time{}
	case r.unverifiedSince.IsZero():
		r.unverifiedSince = r.now()
	default:
		// Losing the durable ownership read is tolerated briefly and fail-closed
		// after reconcileFailureGrace: the client can reconnect and establish a
		// fresh registration once PostgreSQL recovers, while a longer-lived
		// unverifiable owner is not retained.
		failClosed = r.now().Sub(r.unverifiedSince) >= reconcileFailureGrace
	}
	stale := []context.CancelFunc{}
	for _, entry := range snapshot {
		if r.streams[entry.publicationID][entry.streamID] != entry.stream {
			continue // replaced or removed while the durable read was in flight
		}
		current, ok := active[entry.publicationID]
		registrationCurrent := !failClosed
		if verified {
			registrationCurrent = durable[streamKey{publicationID: entry.publicationID, streamID: entry.streamID}] == entry.stream.registrationID
		}
		if ok && entry.stream.version == current && registrationCurrent {
			continue
		}
		stale = append(stale, entry.stream.cancel)
		delete(r.streams[entry.publicationID], entry.streamID)
		if len(r.streams[entry.publicationID]) == 0 {
			delete(r.streams, entry.publicationID)
		}
	}
	r.mu.Unlock()
	for _, cancel := range stale {
		cancel()
	}
}

func (r *StreamRegistry) loadDurableRegistrations(ctx context.Context, publicationIDs []string) (map[streamKey]string, bool) {
	ids := make([]pgtype.UUID, 0, len(publicationIDs))
	for _, publicationID := range publicationIDs {
		id, err := nativeUUID(publicationID)
		if err != nil {
			return nil, false
		}
		ids = append(ids, id)
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.dbTimeout)
	defer cancel()
	rows, err := r.q.ListActiveStreamsForPublications(queryCtx, ids)
	if err != nil {
		return nil, false
	}
	registrations := make(map[streamKey]string, len(rows))
	for _, row := range rows {
		registrations[streamKey{publicationID: row.PublicationID, streamID: row.StreamID}] = row.RegistrationID
	}
	return registrations, true
}

// ClosePublication cancels every local stream of the publication and expires
// its durable registrations.
//
// The database call runs outside mu. Atomicity against concurrent Register
// calls comes from two fences taken while mu is held: closing rejects new
// registrations until the durable expiry has finished, and the in-flight
// epoch makes a registration whose upsert straddled the close back out (and
// expire its own row). A Register therefore lands wholly before this close
// (and is expired/canceled) or wholly after it (and stays active).
func (r *StreamRegistry) ClosePublication(publicationID string) {
	r.mu.Lock()
	streams := r.streams[publicationID]
	delete(r.streams, publicationID)
	r.closing[publicationID]++
	if flight := r.inflight[publicationID]; flight != nil {
		flight.epoch++
	}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		if r.closing[publicationID]--; r.closing[publicationID] <= 0 {
			delete(r.closing, publicationID)
		}
		r.mu.Unlock()
	}()

	for _, stream := range streams {
		stream.cancel()
	}
	if publicationUUID, err := nativeUUID(publicationID); err == nil {
		_, _ = withTimeout(r, context.Background(), func(ctx context.Context) (int64, error) {
			return r.q.ExpirePublicationStreams(ctx, publicationUUID)
		})
	}
}

func (r *StreamRegistry) heartbeat(ctx context.Context, publicationID, streamID string, version publication.StreamVersion, registrationID string) {
	publicationUUID, err := nativeUUID(publicationID)
	if err != nil {
		return
	}
	registrationUUID, err := nativeUUID(registrationID)
	if err != nil {
		return
	}
	ticker := time.NewTicker(streamHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			extendCtx, cancelExtend := context.WithTimeout(ctx, r.dbTimeout)
			result, err := r.q.ExtendStream(extendCtx, publicationdb.ExtendStreamParams{
				ExpiresAt: streamExpiry(), PublicationID: publicationUUID,
				StreamID: streamID, PublicID: version.PublicID,
				ServingStateID: version.ServingStateID, RegistrationID: registrationUUID,
			})
			cancelExtend()
			if err != nil {
				continue
			}
			if result == 0 {
				return
			}
		}
	}
}

func streamExpiry() time.Time {
	return time.Now().UTC().Add(streamLease)
}

func streamRegistrationID() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return value.String(), nil
}
