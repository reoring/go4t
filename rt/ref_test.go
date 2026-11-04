package rt

import (
	"context"
	"errors"
	"testing"
)

func newTestRef(t *testing.T, value any) any {
	t.Helper()
	r, err := RefNew(value).(IO)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func assertRefValue(t *testing.T, r, want any) {
	t.Helper()
	got, err := RefGet(r).(IO)(context.Background())
	if err != nil || got != want {
		t.Fatalf("RefGet = (%v, %v), want (%v, nil)", got, err, want)
	}
}

func TestRefSetGetAndFreshAllocation(t *testing.T) {
	create := RefNew(int64(1)).(IO)
	first, err := create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := create(context.Background())
	if err != nil || first == second {
		t.Fatalf("repeated RefNew = (%v, %v), want a distinct cell", second, err)
	}
	set := RefSet(first, int64(42))
	get := RefGet(first)
	assertRefValue(t, first, int64(1))
	assertIOUnit(t, set)
	got, err := get.(IO)(context.Background())
	if err != nil || got != int64(42) {
		t.Fatalf("deferred RefGet = (%v, %v), want (42, nil)", got, err)
	}
	assertRefValue(t, second, int64(1))
	assertIOUnit(t, RefSet(first, int64(7)))
	assertIOUnit(t, set)
	assertRefValue(t, first, int64(42))
}

func TestRefModifyConcurrentFibers(t *testing.T) {
	r := newTestRef(t, int64(0))
	gate, release := concTestGate(t)
	const count = 100
	fibers := make([]any, count)
	increment := RefModify(r, func(value any) any { return value.(int64) + 1 })
	for i := range fibers {
		fiber, err := Fork(IO(func(ctx context.Context) (any, error) {
			<-gate
			return increment.(IO)(ctx)
		})).(IO)(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		fibers[i] = fiber
	}
	assertRefValue(t, r, int64(0))
	release()
	for _, fiber := range fibers {
		got := receiveConcTest(t, runConcTest(Await(fiber)))
		if got.err != nil || got.value != (Unit{}) {
			t.Fatalf("Await = (%v, %v), want (Unit, nil)", got.value, got.err)
		}
	}
	assertRefValue(t, r, int64(count))
}

func TestRefUpdateResultAndAtomicity(t *testing.T) {
	installTestShapes(t)
	r := newTestRef(t, int64(10))
	calls := 0
	action := RefUpdate(r, func(value any) any {
		calls++
		return Tuple2Of(value.(int64)+1, "previous="+ShowInt(value).(string))
	})
	if calls != 0 {
		t.Fatal("RefUpdate evaluated its callback before execution")
	}
	for _, want := range []string{"previous=10", "previous=11"} {
		got, err := action.(IO)(context.Background())
		if err != nil || got != want {
			t.Fatalf("RefUpdate = (%v, %v), want (%q, nil)", got, err, want)
		}
	}
	if calls != 2 {
		t.Fatalf("callback calls = %d, want 2", calls)
	}
	assertRefValue(t, r, int64(12))

	assertIOUnit(t, RefSet(r, int64(0)))
	gate, release := concTestGate(t)
	const count = 100
	fibers := make([]any, count)
	increment := RefUpdate(r, func(value any) any { return Tuple2Of(value.(int64)+1, value) })
	for i := range fibers {
		fiber, err := Fork(IO(func(ctx context.Context) (any, error) {
			<-gate
			return increment.(IO)(ctx)
		})).(IO)(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		fibers[i] = fiber
	}
	release()
	seen := make(map[int64]bool, count)
	for _, fiber := range fibers {
		got := receiveConcTest(t, runConcTest(Await(fiber)))
		if got.err != nil {
			t.Fatal(got.err)
		}
		n := got.value.(int64)
		if n < 0 || n >= count || seen[n] {
			t.Fatalf("RefUpdate returned duplicate or out-of-range result %d", n)
		}
		seen[n] = true
	}
	assertRefValue(t, r, int64(count))
}

func TestRefCancelledActionsLeaveStateUnchanged(t *testing.T) {
	installTestShapes(t)
	r := newTestRef(t, int64(5))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	callback := func(any) any {
		t.Fatal("cancelled action invoked its callback")
		return nil
	}
	for name, action := range map[string]any{
		"new":    RefNew(int64(99)),
		"get":    RefGet(r),
		"set":    RefSet(r, int64(99)),
		"modify": RefModify(r, callback),
		"update": RefUpdate(r, callback),
	} {
		t.Run(name, func(t *testing.T) {
			value, err := action.(IO)(ctx)
			var failure *Failure
			if value != nil || !errors.Is(err, context.Canceled) || !errors.As(err, &failure) || failure.Kind != Cancelled {
				t.Fatalf("cancelled action = (%v, %v), want classified cancellation", value, err)
			}
			assertRefValue(t, r, int64(5))
		})
	}
}

func TestRefCancellationDuringCallbackDoesNotCommit(t *testing.T) {
	installTestShapes(t)
	for _, update := range []bool{false, true} {
		r := newTestRef(t, int64(5))
		gate, release := concTestGate(t)
		started := make(chan struct{})
		callback := func(value any) any {
			close(started)
			<-gate
			if update {
				return Tuple2Of(int64(99), value)
			}
			return int64(99)
		}
		var action any
		if update {
			action = RefUpdate(r, callback)
		} else {
			action = RefModify(r, callback)
		}
		fiber, err := Fork(action).(IO)(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		receiveConcTest(t, started)
		assertIOUnit(t, Cancel(fiber))
		release()
		receiveConcTest(t, fiber.(*Fiber).done)
		got := receiveConcTest(t, runConcTest(Await(fiber)))
		if got.value != nil || !errors.Is(got.err, context.Canceled) {
			t.Fatalf("cancelled callback = (%v, %v)", got.value, got.err)
		}
		assertRefValue(t, r, int64(5))
		assertIOUnit(t, RefSet(r, int64(6)))
	}
}

func TestRefCallbackPanicReleasesLock(t *testing.T) {
	installTestShapes(t)
	for _, update := range []bool{false, true} {
		r := newTestRef(t, int64(5))
		callback := func(any) any { panic("ref callback failed") }
		var action any
		if update {
			action = RefUpdate(r, callback)
		} else {
			action = RefModify(r, callback)
		}
		got := receiveConcTest(t, runConcTest(TryIO(action)))
		if got.err != nil || got.value != (testErr{testIoError{"Panic", "ref callback failed"}}) {
			t.Fatalf("callback panic = (%v, %v)", got.value, got.err)
		}
		got = receiveConcTest(t, runConcTest(RefGet(r)))
		if got.err != nil || got.value != int64(5) {
			t.Fatalf("RefGet after panic = (%v, %v), want (5, nil)", got.value, got.err)
		}
		assertIOUnit(t, RefSet(r, int64(6)))
	}
}
