package publication

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

var testVersion = StreamVersion{PublicID: "public", ServingStateID: "state"}

func register(t *testing.T, registry StreamRegistry, publicationID, streamID string) (context.Context, func(), error) {
	t.Helper()
	return registry.Register(context.Background(), publicationID, streamID, testVersion)
}

func TestMemoryStreamRegistryCapsStreamsPerPublication(t *testing.T) {
	registry := NewMemoryStreamRegistry(WithStreamLimits(StreamLimits{PerPublication: 2, Total: -1}))
	_, closeA, err := register(t, registry, "pub", "a")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = register(t, registry, "pub", "b")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = register(t, registry, "pub", "c")
	var capacity *StreamCapacityError
	if !errors.Is(err, ErrStreamCapacity) || !errors.As(err, &capacity) || capacity.Scope != StreamCapacityPublication || capacity.Limit != 2 {
		t.Fatalf("third stream err = %v, want per-publication capacity error", err)
	}
	// Another publication has its own budget.
	if _, closeOther, err := register(t, registry, "other", "a"); err != nil {
		t.Fatalf("other publication was rejected by an unrelated cap: %v", err)
	} else {
		defer closeOther()
	}
	// Closing a stream releases its slot.
	closeA()
	if _, closeC, err := register(t, registry, "pub", "c"); err != nil {
		t.Fatalf("slot was not released on unregister: %v", err)
	} else {
		defer closeC()
	}
}

func TestMemoryStreamRegistryReplacingAStreamDoesNotConsumeASlot(t *testing.T) {
	registry := NewMemoryStreamRegistry(WithStreamLimits(StreamLimits{PerPublication: 1, Total: 1}))
	first, _, err := register(t, registry, "pub", "a")
	if err != nil {
		t.Fatal(err)
	}
	second, closeSecond, err := register(t, registry, "pub", "a")
	if err != nil {
		t.Fatalf("reconnect with the same stream identity must replace, not exceed the cap: %v", err)
	}
	defer closeSecond()
	select {
	case <-first.Done():
	default:
		t.Fatal("replaced stream was not canceled")
	}
	if second.Err() != nil {
		t.Fatal("replacement stream is canceled")
	}
}

func TestMemoryStreamRegistryCapsTotalStreams(t *testing.T) {
	registry := NewMemoryStreamRegistry(WithStreamLimits(StreamLimits{PerPublication: -1, Total: 3}))
	closers := []func(){}
	for i := 0; i < 3; i++ {
		_, closeStream, err := register(t, registry, fmt.Sprintf("pub-%d", i), "s")
		if err != nil {
			t.Fatal(err)
		}
		closers = append(closers, closeStream)
	}
	_, _, err := register(t, registry, "pub-9", "s")
	var capacity *StreamCapacityError
	if !errors.As(err, &capacity) || capacity.Scope != StreamCapacityGlobal {
		t.Fatalf("err = %v, want global capacity error", err)
	}
	closers[0]()
	if _, closeStream, err := register(t, registry, "pub-9", "s"); err != nil {
		t.Fatal(err)
	} else {
		closeStream()
	}
}

func TestMemoryStreamRegistryReleasesSlotsOnEveryClosePath(t *testing.T) {
	registry := NewMemoryStreamRegistry(WithStreamLimits(StreamLimits{PerPublication: 1, Total: 1}))
	ctx, unregister, err := register(t, registry, "pub", "a")
	if err != nil {
		t.Fatal(err)
	}
	// Reconcile (stale version) frees the slot even though unregister was never called.
	registry.Reconcile(context.Background(), map[string]StreamVersion{"pub": {PublicID: "new", ServingStateID: "new"}})
	if ctx.Err() == nil {
		t.Fatal("stale stream not canceled")
	}
	_, unregister2, err := register(t, registry, "pub", "b")
	if err != nil {
		t.Fatalf("slot leaked after reconcile: %v", err)
	}
	// ClosePublication frees it too.
	registry.ClosePublication("pub")
	_, unregister3, err := register(t, registry, "pub", "c")
	if err != nil {
		t.Fatalf("slot leaked after ClosePublication: %v", err)
	}
	// Late, duplicate unregister of already-removed streams must not corrupt accounting.
	unregister()
	unregister()
	unregister2()
	if _, _, err := register(t, registry, "pub", "d"); !errors.Is(err, ErrStreamCapacity) {
		t.Fatalf("late unregister of a removed stream freed someone else's slot: err = %v", err)
	}
	unregister3()
	if _, closeD, err := register(t, registry, "pub", "d"); err != nil {
		t.Fatal(err)
	} else {
		closeD()
	}
}

func TestMemoryStreamRegistryCapIsExactUnderConcurrency(t *testing.T) {
	const limit = 10
	registry := NewMemoryStreamRegistry(WithStreamLimits(StreamLimits{PerPublication: limit, Total: -1}))
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, rejected := 0, 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := register(t, registry, "pub", fmt.Sprintf("s-%d", i))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				accepted++
			} else if errors.Is(err, ErrStreamCapacity) {
				rejected++
			} else {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted != limit || rejected != 200-limit {
		t.Fatalf("accepted=%d rejected=%d, want %d/%d", accepted, rejected, limit, 200-limit)
	}
}

func TestStreamLimitsResolveDefaultsAndUnlimited(t *testing.T) {
	got := StreamLimits{}.Resolved()
	if got.PerPublication != DefaultMaxStreamsPerPublication || got.Total != DefaultMaxStreamsTotal {
		t.Fatalf("defaults = %#v", got)
	}
	if err := (StreamLimits{PerPublication: -1, Total: -1}).Resolved().Admit(1<<30, 1<<30); err != nil {
		t.Fatalf("negative limits must disable the caps: %v", err)
	}
}
