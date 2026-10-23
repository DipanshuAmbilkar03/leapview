package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/dashboard"
)

// prepareGate simulates a command prepare callback performing database I/O: the
// callback reports that it was entered and then blocks until released. Release
// is idempotent and also runs at cleanup, so a failing assertion cannot leave
// the callback (and the coordinator it pins) blocked.
type prepareGate struct {
	entered chan struct{}
	release func()
	wait    chan struct{}
}

func newPrepareGate(t *testing.T) *prepareGate {
	t.Helper()
	gate := &prepareGate{entered: make(chan struct{}, 8), wait: make(chan struct{})}
	var once sync.Once
	gate.release = func() { once.Do(func() { close(gate.wait) }) }
	t.Cleanup(gate.release)
	return gate
}

// prepare returns a RefreshPrepare that blocks on the gate.
func (g *prepareGate) prepare(mutate func(dashboard.Filters) dashboard.Filters) RefreshPrepare {
	return func(current dashboard.Filters) (RefreshPreparation, error) {
		g.entered <- struct{}{}
		<-g.wait
		if mutate != nil {
			current = mutate(current)
		}
		return RefreshPreparation{Filters: current, Command: "filter"}, nil
	}
}

func (g *prepareGate) awaitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("prepare callback was not entered")
	}
}

func within(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s blocked while a command prepare callback was performing I/O", name)
	}
}

func TestCoordinatorPrepareIODoesNotBlockEventDeliveryOrReads(t *testing.T) {
	events := make(chan RefreshEvent, 16)
	coordinator := NewCoordinator(context.Background(), func(event RefreshEvent) { events <- event })
	t.Cleanup(coordinator.Close)
	gate := newPrepareGate(t)

	// Generation 1 is running and will publish after the second command has
	// entered its (blocking) prepare callback.
	publishLate := make(chan struct{})
	published := make(chan bool, 1)
	if _, err := coordinator.Begin(nil, func(ctx context.Context, publish RefreshPublisher) {
		select {
		case <-publishLate:
			published <- publish(RefreshEvent{Type: RefreshEventProgress})
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := coordinator.BeginPrepared(gate.prepare(nil), nil)
		result <- err
	}()
	gate.awaitEntered(t)

	within(t, "Filters()", func() { _ = coordinator.Filters() })
	close(publishLate)
	select {
	case ok := <-published:
		if !ok {
			t.Fatal("current generation could not publish while the next command was preparing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event delivery blocked while a command prepare callback was performing I/O")
	}

	gate.release()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if got := coordinator.generationForTest(); got != 2 {
		t.Fatalf("generation = %d, want 2", got)
	}
}

func TestCoordinatorCloseDuringPrepareDiscardsCommandWithoutStartingWork(t *testing.T) {
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	t.Cleanup(coordinator.Close)
	gate := newPrepareGate(t)
	result := make(chan error, 1)
	workBuilt := make(chan struct{}, 1)
	go func() {
		_, err := coordinator.BeginPrepared(gate.prepare(nil), func(RefreshPreparation) RefreshWork {
			workBuilt <- struct{}{}
			return nil
		})
		result <- err
	}()
	gate.awaitEntered(t)

	// Disconnect cleanup must not wait for the in-flight database round trips.
	within(t, "CloseWithReason()", func() { coordinator.CloseWithReason("disconnect") })

	gate.release()
	select {
	case err := <-result:
		if !errors.Is(err, ErrCoordinatorClosed) {
			t.Fatalf("BeginPrepared after close err = %v, want ErrCoordinatorClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BeginPrepared did not return after close")
	}
	select {
	case <-workBuilt:
		t.Fatal("work was started for a coordinator closed during prepare")
	default:
	}
	if got := coordinator.generationForTest(); got != 0 {
		t.Fatalf("generation = %d, want 0 (closed coordinator must not advance)", got)
	}
}

func TestCoordinatorQueuedCommandIsReleasedByClose(t *testing.T) {
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	t.Cleanup(coordinator.Close)
	gate := newPrepareGate(t)
	first := make(chan error, 1)
	go func() {
		_, err := coordinator.BeginPrepared(gate.prepare(nil), nil)
		first <- err
	}()
	gate.awaitEntered(t)

	secondPrepared := make(chan struct{}, 1)
	second := make(chan error, 1)
	go func() {
		_, err := coordinator.BeginPrepared(func(current dashboard.Filters) (RefreshPreparation, error) {
			secondPrepared <- struct{}{}
			return RefreshPreparation{Filters: current}, nil
		}, nil)
		second <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the second command queue behind the first
	within(t, "Close()", coordinator.Close)
	select {
	case err := <-second:
		if !errors.Is(err, ErrCoordinatorClosed) {
			t.Fatalf("queued BeginPrepared err = %v, want ErrCoordinatorClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued command stayed blocked behind a prepare after close (goroutine leak)")
	}
	select {
	case <-secondPrepared:
		t.Fatal("queued command ran its prepare callback after close")
	default:
	}
	gate.release()
	if err := <-first; !errors.Is(err, ErrCoordinatorClosed) {
		t.Fatalf("first err = %v", err)
	}
}

func TestCoordinatorCommandsStillPrepareSeriallyAgainstLatestState(t *testing.T) {
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	t.Cleanup(coordinator.Close)

	var mu sync.Mutex
	var seen []string
	const commands = 8
	var wg sync.WaitGroup
	for i := 0; i < commands; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := coordinator.BeginPrepared(func(current dashboard.Filters) (RefreshPreparation, error) {
				mu.Lock()
				seen = append(seen, current.ServingStateID)
				mu.Unlock()
				time.Sleep(2 * time.Millisecond) // widen the unlocked window
				current.ServingStateID += "x"
				return RefreshPreparation{Filters: current}, nil
			}, nil)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	// Every prepare observed the filters committed by the previous one, so the
	// accumulated suffix is strictly one character longer each time.
	for i, value := range seen {
		if len(value) != i {
			t.Fatalf("prepare %d saw %q; commands must prepare serially against the latest committed state", i, value)
		}
	}
	if got := coordinator.generationForTest(); got != commands {
		t.Fatalf("generation = %d, want %d", got, commands)
	}
}

func TestCoordinatorKeepsDataRevisionsPublishedDuringPrepare(t *testing.T) {
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	t.Cleanup(coordinator.Close)
	gate := newPrepareGate(t)

	published := make(chan struct{})
	proceed := make(chan struct{})
	if _, err := coordinator.Begin(nil, func(ctx context.Context, publish RefreshPublisher) {
		<-proceed
		publish(testVisualizationEvent(RefreshEventVisual, "orders"))
		close(published)
		<-ctx.Done()
	}); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		// The callback keeps the (stale) snapshot's DataRevisions, as the shared
		// command path does (shared.DataRevisions = current.DataRevisions).
		_, err := coordinator.BeginPrepared(gate.prepare(func(f dashboard.Filters) dashboard.Filters { return f }), nil)
		result <- err
	}()
	gate.awaitEntered(t)
	close(proceed)
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("event delivery blocked while a command prepare callback was performing I/O")
	}
	gate.release()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	// The visual published during the prepare window advanced the revision. The
	// snapshot handed to prepare predates it, so the commit must merge the
	// advanced revision forward instead of rolling the canonical state back.
	if rev := coordinator.Filters().DataRevisions["orders"]; rev != 1 {
		t.Fatalf("canonical DataRevisions[orders] = %d, want 1", rev)
	}
}

func (c *Coordinator) generationForTest() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}
