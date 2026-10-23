package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/dashboard/publication"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// These tests exercise StreamRegistry's locking, admission, and reconciliation
// against an in-memory stand-in for the durable stream table. They need no
// PostgreSQL server; the database-backed behavior is covered by the
// PostgreSQL18 repository tests.

type fakeStreamRow struct {
	registrationID string
	expired        bool
}

// fakeStreamDB models dashboard.publication_streams closely enough for the
// registry: upsert, registration-scoped expiry, publication-wide expiry, the
// publication-filtered active listing, and the activity check.
type fakeStreamDB struct {
	mu   sync.Mutex
	rows map[streamKey]fakeStreamRow
	// hook runs before an operation completes (after a list result has been
	// read, simulating a late response). It may block or fail the operation.
	hook func(ctx context.Context, op string, args []any) error

	listCalls     atomic.Int32
	listArgs      [][]string
	upsertsActive atomic.Int32
}

func newFakeStreamDB() *fakeStreamDB { return &fakeStreamDB{rows: map[streamKey]fakeStreamRow{}} }

func uuidString(value any) string {
	if id, ok := value.(pgtype.UUID); ok {
		return uuid.UUID(id.Bytes).String()
	}
	return fmt.Sprint(value)
}

func (f *fakeStreamDB) runHook(ctx context.Context, op string, args []any) error {
	if f.hook == nil {
		return nil
	}
	return f.hook(ctx, op, args)
}

func (f *fakeStreamDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !strings.Contains(sql, "upsert_publication_stream") {
		return pgconn.CommandTag{}, fmt.Errorf("unexpected exec %q", sql)
	}
	f.upsertsActive.Add(1)
	defer f.upsertsActive.Add(-1)
	if err := f.runHook(ctx, "upsert", args); err != nil {
		return pgconn.CommandTag{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[streamKey{publicationID: uuidString(args[0]), streamID: args[1].(string)}] = fakeStreamRow{registrationID: uuidString(args[4])}
	return pgconn.CommandTag{}, nil
}

func (f *fakeStreamDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "ANY($1::uuid[])") {
		return nil, fmt.Errorf("unexpected query %q", sql)
	}
	f.listCalls.Add(1)
	ids := map[string]bool{}
	recorded := []string{}
	for _, id := range args[0].([]pgtype.UUID) {
		ids[uuidString(id)] = true
		recorded = append(recorded, uuidString(id))
	}
	f.mu.Lock()
	f.listArgs = append(f.listArgs, recorded)
	var data [][]string
	for key, row := range f.rows {
		if !row.expired && ids[key.publicationID] {
			data = append(data, []string{key.publicationID, key.streamID, row.registrationID})
		}
	}
	f.mu.Unlock()
	// The result was read; now it is delivered late (or not at all).
	if err := f.runHook(ctx, "list", args); err != nil {
		return nil, err
	}
	return &fakeRows{data: data}, nil
}

func (f *fakeStreamDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "expire_stream_registration"):
		if err := f.runHook(ctx, "expire_registration", args); err != nil {
			return fakeRow{err: err}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		key := streamKey{publicationID: uuidString(args[0]), streamID: args[1].(string)}
		if row, ok := f.rows[key]; ok && row.registrationID == uuidString(args[2]) {
			row.expired = true
			f.rows[key] = row
			return fakeRow{value: int64(1)}
		}
		return fakeRow{value: int64(0)}
	case strings.Contains(sql, "expire_publication_streams"):
		if err := f.runHook(ctx, "expire_publication", args); err != nil {
			return fakeRow{err: err}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var n int64
		for key, row := range f.rows {
			if key.publicationID == uuidString(args[0]) && !row.expired {
				row.expired = true
				f.rows[key] = row
				n++
			}
		}
		return fakeRow{value: n}
	case strings.Contains(sql, "exists(SELECT 1"):
		if err := f.runHook(ctx, "active", args); err != nil {
			return fakeRow{err: err}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		row, ok := f.rows[streamKey{publicationID: uuidString(args[0]), streamID: args[1].(string)}]
		return fakeRow{value: ok && !row.expired && row.registrationID == uuidString(args[4])}
	case strings.Contains(sql, "extend_stream"):
		return fakeRow{value: int64(1)}
	}
	return fakeRow{err: fmt.Errorf("unexpected query row %q", sql)}
}

func (f *fakeStreamDB) activeRegistration(publicationID, streamID string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[streamKey{publicationID: publicationID, streamID: streamID}]
	return row.registrationID, ok && !row.expired
}

type fakeRow struct {
	value any
	err   error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch target := dest[0].(type) {
	case *int64:
		*target = r.value.(int64)
	case *bool:
		*target = r.value.(bool)
	default:
		return fmt.Errorf("unsupported scan target %T", dest[0])
	}
	return nil
}

type fakeRows struct {
	data [][]string
	i    int
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool                                   { r.i++; return r.i <= len(r.data) }
func (r *fakeRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }
func (r *fakeRows) Scan(dest ...any) error {
	for i, value := range r.data[r.i-1] {
		*(dest[i].(*string)) = value
	}
	return nil
}

var (
	pubOne   = uuid.MustParse("00000000-0000-7000-8000-000000000001").String()
	pubTwo   = uuid.MustParse("00000000-0000-7000-8000-000000000002").String()
	testVers = publication.StreamVersion{PublicID: "public", ServingStateID: "state"}
)

func newTestRegistry(t *testing.T, db *fakeStreamDB, limits publication.StreamLimits) *StreamRegistry {
	t.Helper()
	registry := NewStreamRegistry(db, publication.WithStreamLimits(limits))
	registry.dbTimeout = 2 * time.Second
	return registry
}

func mustRegister(t *testing.T, registry *StreamRegistry, publicationID, streamID string) (context.Context, func()) {
	t.Helper()
	ctx, unregister, err := registry.Register(context.Background(), publicationID, streamID, testVers)
	if err != nil {
		t.Fatalf("register %s/%s: %v", publicationID, streamID, err)
	}
	t.Cleanup(unregister)
	return ctx, unregister
}

func within(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not complete: another operation holds the registry lock across database I/O", name)
	}
}

// gate blocks hooked operations until released (or the operation's context ends).
type gate struct {
	entered chan struct{}
	once    sync.Once
	open    chan struct{}
}

func newGate(t *testing.T) *gate {
	g := &gate{entered: make(chan struct{}, 64), open: make(chan struct{})}
	t.Cleanup(g.release)
	return g
}
func (g *gate) release() { g.once.Do(func() { close(g.open) }) }
func (g *gate) block(ctx context.Context) error {
	g.entered <- struct{}{}
	select {
	case <-g.open:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (g *gate) await(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked operation was never reached")
	}
}

func TestRegisterDoesNotHoldRegistryLockAcrossUpsert(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: -1, Total: -1})
	mustRegister(t, registry, pubOne, "healthy")

	slow := newGate(t)
	db.hook = func(ctx context.Context, op string, args []any) error {
		if op == "upsert" && args[1].(string) == "slow" {
			return slow.block(ctx)
		}
		return nil
	}
	registered := make(chan error, 1)
	go func() {
		_, unregister, err := registry.Register(context.Background(), pubOne, "slow", testVers)
		if err == nil {
			defer unregister()
		}
		registered <- err
	}()
	slow.await(t)

	// Every public command takes the registry lock; none may queue behind the
	// slow registration's database round trip.
	within(t, "Active()", func() {
		if !registry.Active(pubOne, "healthy", testVers) {
			t.Error("healthy stream became inactive")
		}
	})
	within(t, "Register() of an unrelated stream", func() {
		_, unregister, err := registry.Register(context.Background(), pubTwo, "other", testVers)
		if err != nil {
			t.Error(err)
			return
		}
		unregister()
	})
	within(t, "Reconcile()", func() {
		registry.Reconcile(context.Background(), map[string]publication.StreamVersion{pubOne: testVers})
	})
	within(t, "ClosePublication() of an unrelated publication", func() { registry.ClosePublication(pubTwo) })

	slow.release()
	if err := <-registered; err != nil {
		t.Fatal(err)
	}
}

func TestReconcileSkipsDurableReadWithoutLocalStreams(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{})
	registry.Reconcile(context.Background(), map[string]publication.StreamVersion{pubOne: testVers})
	if got := db.listCalls.Load(); got != 0 {
		t.Fatalf("empty registry issued %d cluster-wide ownership queries per reconcile tick", got)
	}
}

func TestReconcileQueriesOnlyLocallyHeldPublications(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{})
	mustRegister(t, registry, pubOne, "a")
	mustRegister(t, registry, pubOne, "b")
	// A stream owned by some other instance for a publication we do not serve.
	db.rows[streamKey{publicationID: pubTwo, streamID: "remote"}] = fakeStreamRow{registrationID: uuid.NewString()}

	registry.Reconcile(context.Background(), map[string]publication.StreamVersion{pubOne: testVers, pubTwo: testVers})
	if len(db.listArgs) != 1 || len(db.listArgs[0]) != 1 || db.listArgs[0][0] != pubOne {
		t.Fatalf("durable read publication filter = %v, want exactly [%s]", db.listArgs, pubOne)
	}
}

func TestReconcileDoesNotHoldRegistryLockAcrossDurableRead(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: -1, Total: -1})
	mustRegister(t, registry, pubOne, "a")
	slow := newGate(t)
	db.hook = func(ctx context.Context, op string, _ []any) error {
		if op == "list" {
			return slow.block(ctx)
		}
		return nil
	}
	done := make(chan struct{})
	go func() {
		registry.Reconcile(context.Background(), map[string]publication.StreamVersion{pubOne: testVers})
		close(done)
	}()
	slow.await(t)

	within(t, "Active()", func() {
		if !registry.Active(pubOne, "a", testVers) {
			t.Error("stream inactive")
		}
	})
	within(t, "Register()", func() {
		_, unregister, err := registry.Register(context.Background(), pubOne, "b", testVers)
		if err != nil {
			t.Error(err)
			return
		}
		unregister()
	})
	slow.release()
	<-done
}

func TestReconcileNeverCancelsStreamRegisteredOrReplacedDuringTheRead(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: -1, Total: -1})
	original, _ := mustRegister(t, registry, pubOne, "a")
	// Another instance already took over stream a, so the ownership data the
	// reconcile reads is correct for the original registration but stale for
	// whatever replaces it.
	db.mu.Lock()
	db.rows[streamKey{publicationID: pubOne, streamID: "a"}] = fakeStreamRow{registrationID: uuid.NewString()}
	db.mu.Unlock()
	slow := newGate(t)
	db.hook = func(ctx context.Context, op string, _ []any) error {
		if op == "list" {
			return slow.block(ctx)
		}
		return nil
	}
	done := make(chan struct{})
	go func() {
		registry.Reconcile(context.Background(), map[string]publication.StreamVersion{pubOne: testVers})
		close(done)
	}()
	slow.await(t)

	// The durable result being read predates both of these registrations.
	fresh, _ := mustRegister(t, registry, pubOne, "fresh")
	replacement, _ := mustRegister(t, registry, pubOne, "a")
	slow.release()
	<-done

	if original.Err() == nil {
		t.Fatal("superseded registration was not canceled by its replacement")
	}
	if fresh.Err() != nil {
		t.Fatal("stream registered while the durable read was in flight was canceled by stale ownership data")
	}
	if replacement.Err() != nil {
		t.Fatal("replacement stream was canceled because the stale read still showed the old registration")
	}
	for _, id := range []string{"a", "fresh"} {
		if !registry.Active(pubOne, id, testVers) {
			t.Fatalf("stream %q was dropped from the registry by a decision based on stale ownership data", id)
		}
	}
}

func TestReconcileToleratesTransientDurableReadFailureButFailsClosedAfterGrace(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: -1, Total: -1})
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return now }
	ctx, _ := mustRegister(t, registry, pubOne, "a")
	active := map[string]publication.StreamVersion{pubOne: testVers}

	var failing atomic.Bool
	failing.Store(true)
	db.hook = func(_ context.Context, op string, _ []any) error {
		if op == "list" && failing.Load() {
			return errors.New("connection reset")
		}
		return nil
	}

	registry.Reconcile(context.Background(), active)
	if ctx.Err() != nil {
		t.Fatal("one failed ownership read tore down the stream")
	}
	now = now.Add(reconcileFailureGrace / 2)
	registry.Reconcile(context.Background(), active)
	if ctx.Err() != nil {
		t.Fatal("stream was canceled inside the grace window")
	}

	// A successful read resets the clock.
	failing.Store(false)
	registry.Reconcile(context.Background(), active)
	failing.Store(true)
	now = now.Add(reconcileFailureGrace)
	registry.Reconcile(context.Background(), active) // first failure after recovery
	if ctx.Err() != nil {
		t.Fatal("grace window was not reset by the successful read")
	}

	// Sustained unverifiability fails closed.
	now = now.Add(reconcileFailureGrace)
	registry.Reconcile(context.Background(), active)
	if ctx.Err() == nil {
		t.Fatal("stream with unverifiable ownership was retained beyond the grace window")
	}
}

func TestReconcileStillCancelsSupersededRegistrationAndStaleVersion(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: -1, Total: -1})
	superseded, _ := mustRegister(t, registry, pubOne, "a")
	current, _ := mustRegister(t, registry, pubOne, "b")
	staleVersion, _ := mustRegister(t, registry, pubTwo, "c")

	// Another instance takes over stream a.
	db.mu.Lock()
	db.rows[streamKey{publicationID: pubOne, streamID: "a"}] = fakeStreamRow{registrationID: uuid.NewString()}
	db.mu.Unlock()

	registry.Reconcile(context.Background(), map[string]publication.StreamVersion{
		pubOne: testVers,
		pubTwo: {PublicID: "rotated", ServingStateID: "state"},
	})
	if superseded.Err() == nil {
		t.Fatal("stream superseded by another instance was not canceled")
	}
	if current.Err() != nil {
		t.Fatal("current stream was canceled")
	}
	if staleVersion.Err() == nil {
		t.Fatal("stream of a rotated public id was not canceled")
	}
}

func TestClosePublicationDoesNotHoldRegistryLockAcrossExpiry(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: -1, Total: -1})
	closing, _ := mustRegister(t, registry, pubOne, "a")
	mustRegister(t, registry, pubTwo, "b")
	slow := newGate(t)
	db.hook = func(ctx context.Context, op string, _ []any) error {
		if op == "expire_publication" {
			return slow.block(ctx)
		}
		return nil
	}
	done := make(chan struct{})
	go func() { registry.ClosePublication(pubOne); close(done) }()
	slow.await(t)

	// Local streams are cut immediately; the database call is still in flight.
	if closing.Err() == nil {
		t.Fatal("closed publication's stream is still running while expiry is pending")
	}
	within(t, "Active() of another publication", func() {
		if !registry.Active(pubTwo, "b", testVers) {
			t.Error("unrelated stream became inactive")
		}
	})
	within(t, "Register() of another publication", func() {
		_, unregister, err := registry.Register(context.Background(), pubTwo, "c", testVers)
		if err != nil {
			t.Error(err)
			return
		}
		unregister()
	})
	// New registrations of the closing publication are refused until the
	// durable expiry finishes, so they cannot be expired out from under the
	// viewer.
	_, _, err := registry.Register(context.Background(), pubOne, "late", testVers)
	if !errors.Is(err, publication.ErrStreamStateUnavailable) {
		t.Fatalf("register during close err = %v, want ErrStreamStateUnavailable", err)
	}
	slow.release()
	<-done

	ctx, unregister, err := registry.Register(context.Background(), pubOne, "after", testVers)
	if err != nil {
		t.Fatalf("register after close completed: %v", err)
	}
	defer unregister()
	if _, active := db.activeRegistration(pubOne, "after"); !active || ctx.Err() != nil {
		t.Fatal("registration made wholly after the close must stay active")
	}
}

func TestRegisterRacingWithCloseBacksOutAndLeavesNoDurableOrphan(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: 1, Total: 1})
	slow := newGate(t)
	db.hook = func(ctx context.Context, op string, _ []any) error {
		if op == "upsert" {
			return slow.block(ctx)
		}
		return nil
	}
	result := make(chan error, 1)
	go func() {
		_, unregister, err := registry.Register(context.Background(), pubOne, "racing", testVers)
		if err == nil {
			unregister()
		}
		result <- err
	}()
	slow.await(t)

	// The publication is closed while the registration's upsert is in flight
	// (and, here, commits after the close's expiry ran).
	db.hook = nil
	registry.ClosePublication(pubOne)
	slow.release()

	if err := <-result; !errors.Is(err, publication.ErrStreamStateUnavailable) {
		t.Fatalf("racing register err = %v, want ErrStreamStateUnavailable", err)
	}
	if _, active := db.activeRegistration(pubOne, "racing"); active {
		t.Fatal("registration that lost the race against ClosePublication left an active durable row")
	}
	// Its reserved slot was released.
	ctx, unregister, err := registry.Register(context.Background(), pubOne, "next", testVers)
	if err != nil {
		t.Fatalf("slot leaked by the backed-out registration: %v", err)
	}
	defer unregister()
	_ = ctx
}

func TestRegisterFailureReleasesReservedSlotAndExpiresAmbiguousRow(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: 1, Total: 1})
	var fail atomic.Bool
	fail.Store(true)
	db.hook = func(_ context.Context, op string, args []any) error {
		if op == "upsert" && fail.Load() {
			// The write commits but the client sees an error (timeout after commit).
			db.mu.Lock()
			db.rows[streamKey{publicationID: uuidString(args[0]), streamID: args[1].(string)}] = fakeStreamRow{registrationID: uuidString(args[4])}
			db.mu.Unlock()
			return errors.New("timeout after commit")
		}
		return nil
	}
	if _, _, err := registry.Register(context.Background(), pubOne, "a", testVers); err == nil {
		t.Fatal("register succeeded despite upsert error")
	}
	if _, active := db.activeRegistration(pubOne, "a"); active {
		t.Fatal("ambiguous upsert left an active durable registration without a local owner")
	}
	fail.Store(false)
	mustRegister(t, registry, pubOne, "a")
}

func TestRegisterBoundsDatabaseCallsWithTimeout(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: 1, Total: 1})
	registry.dbTimeout = 50 * time.Millisecond
	db.hook = func(ctx context.Context, op string, _ []any) error {
		if op == "upsert" {
			<-ctx.Done() // never answers
			return ctx.Err()
		}
		return nil
	}
	started := time.Now()
	_, _, err := registry.Register(context.Background(), pubOne, "a", testVers)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("register was not bounded by the database timeout")
	}
	db.hook = nil
	mustRegister(t, registry, pubOne, "a") // slot was released
}

func TestRegisterCapsCountInFlightReservations(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: 2, Total: -1})
	slow := newGate(t)
	db.hook = func(ctx context.Context, op string, _ []any) error {
		if op == "upsert" {
			return slow.block(ctx)
		}
		return nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, unregister, err := registry.Register(context.Background(), pubOne, fmt.Sprintf("s%d", i), testVers)
			if err != nil {
				t.Error(err)
				return
			}
			t.Cleanup(unregister)
		}()
	}
	slow.await(t)
	slow.await(t)

	// Both slots are promised to registrations that have not installed yet.
	_, _, err := registry.Register(context.Background(), pubOne, "third", testVers)
	var capacity *publication.StreamCapacityError
	if !errors.As(err, &capacity) || capacity.Scope != publication.StreamCapacityPublication {
		t.Fatalf("third concurrent registration err = %v, want per-publication capacity error", err)
	}
	slow.release()
	wg.Wait()
}

func TestRegisterCapsAndReleasesOnEveryClosePath(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: 1, Total: 2})

	// Replacing a stream with the same identity does not need a second slot.
	first, unregisterFirst := mustRegister(t, registry, pubOne, "a")
	second, unregisterSecond := mustRegister(t, registry, pubOne, "a")
	if first.Err() == nil || second.Err() != nil {
		t.Fatal("replacement did not supersede the first registration")
	}
	if _, _, err := registry.Register(context.Background(), pubOne, "b", testVers); !errors.Is(err, publication.ErrStreamCapacity) {
		t.Fatalf("second identity err = %v, want per-publication capacity", err)
	}
	// The superseded registration's cleanup must not free the replacement's slot.
	unregisterFirst()
	unregisterFirst()
	if _, _, err := registry.Register(context.Background(), pubOne, "b", testVers); !errors.Is(err, publication.ErrStreamCapacity) {
		t.Fatalf("late cleanup of a replaced registration freed a live slot: %v", err)
	}
	// Global cap spans publications.
	mustRegister(t, registry, pubTwo, "x")
	var capacity *publication.StreamCapacityError
	_, _, err := registry.Register(context.Background(), "00000000-0000-7000-8000-000000000003", "y", testVers)
	if !errors.As(err, &capacity) || capacity.Scope != publication.StreamCapacityGlobal {
		t.Fatalf("err = %v, want global capacity error", err)
	}

	// Unregister frees the slot.
	unregisterSecond()
	third, _ := mustRegister(t, registry, pubOne, "c")
	// Reconcile frees it.
	registry.Reconcile(context.Background(), map[string]publication.StreamVersion{pubTwo: testVers})
	if third.Err() == nil {
		t.Fatal("stale stream survived reconcile")
	}
	fourth, _ := mustRegister(t, registry, pubOne, "d")
	// ClosePublication frees it.
	registry.ClosePublication(pubOne)
	if fourth.Err() == nil {
		t.Fatal("ClosePublication left the stream running")
	}
	mustRegister(t, registry, pubOne, "e")
}

func TestRegisterSerializesSameStreamIdentityButNotOthers(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: -1, Total: -1})
	slow := newGate(t)
	var concurrentSameKey, maxSameKey atomic.Int32
	db.hook = func(ctx context.Context, op string, args []any) error {
		if op != "upsert" || args[1].(string) != "dup" {
			return nil
		}
		n := concurrentSameKey.Add(1)
		defer concurrentSameKey.Add(-1)
		for {
			m := maxSameKey.Load()
			if n <= m || maxSameKey.CompareAndSwap(m, n) {
				break
			}
		}
		return slow.block(ctx)
	}
	type result struct {
		ctx    context.Context
		cancel func()
		err    error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			ctx, unregister, err := registry.Register(context.Background(), pubOne, "dup", testVers)
			results <- result{ctx: ctx, cancel: unregister, err: err}
		}()
	}
	slow.await(t)
	// An unrelated identity is not queued behind the pair.
	within(t, "Register() of a different stream", func() {
		_, unregister, err := registry.Register(context.Background(), pubOne, "other", testVers)
		if err != nil {
			t.Error(err)
			return
		}
		unregister()
	})
	time.Sleep(50 * time.Millisecond)
	slow.release()
	var winners []result
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		t.Cleanup(r.cancel)
		winners = append(winners, r)
	}
	if got := maxSameKey.Load(); got != 1 {
		t.Fatalf("%d concurrent upserts for one stream identity; they must be serialized so the database and local winner agree", got)
	}
	live := 0
	for _, r := range winners {
		if r.ctx.Err() == nil {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("%d live registrations for one identity, want exactly 1", live)
	}
	registry.mu.Lock()
	localRegistration := registry.streams[pubOne]["dup"].registrationID
	registry.mu.Unlock()
	if durable, ok := db.activeRegistration(pubOne, "dup"); !ok || durable != localRegistration {
		t.Fatalf("durable registration %q != local registration %q", durable, localRegistration)
	}
	registry.mu.Lock()
	leaked := len(registry.keyLocks)
	registry.mu.Unlock()
	if leaked != 0 {
		t.Fatalf("%d per-stream locks leaked", leaked)
	}
}

func TestRegisterWaitingOnStreamIdentityHonorsContext(t *testing.T) {
	db := newFakeStreamDB()
	registry := newTestRegistry(t, db, publication.StreamLimits{PerPublication: 1, Total: 1})
	slow := newGate(t)
	db.hook = func(ctx context.Context, op string, _ []any) error {
		if op == "upsert" {
			return slow.block(ctx)
		}
		return nil
	}
	go func() {
		_, unregister, err := registry.Register(context.Background(), pubOne, "a", testVers)
		if err == nil {
			unregister()
		}
	}()
	slow.await(t)

	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, _, err := registry.Register(ctx, pubOne, "a", testVers)
		errs <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errs:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registration queued behind another registration ignored its canceled context")
	}
	slow.release()
}
