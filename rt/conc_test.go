package rt

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type concTestResult struct {
	value any
	err   error
}

func runConcTest(action any) <-chan concTestResult {
	result := make(chan concTestResult, 1)
	go func() {
		value, err := action.(IO)()
		result <- concTestResult{value, err}
	}()
	return result
}

func receiveConcTest[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent action did not complete")
		var zero T
		return zero
	}
}

func concTestGate(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	return gate, release
}

func TestForkAwaitLazyReusableAndConcurrent(t *testing.T) {
	gate, release := concTestGate(t)
	started := make(chan struct{}, 2)
	var calls atomic.Int32
	want := &struct{ value int }{42}
	fork := Fork(IO(func() (any, error) {
		calls.Add(1)
		started <- struct{}{}
		<-gate
		return want, nil
	})).(IO)
	if calls.Load() != 0 {
		t.Fatal("Fork ran before its IO was executed")
	}
	fiber, err := fork()
	if err != nil {
		t.Fatal(err)
	}
	receiveConcTest(t, started)
	await := Await(fiber)
	const count = 16
	waiters := make([]<-chan concTestResult, count)
	for i := range waiters {
		waiters[i] = runConcTest(await)
	}
	release()
	for _, waiter := range waiters {
		got := receiveConcTest(t, waiter)
		if got.value != want || got.err != nil {
			t.Fatalf("Await = (%v, %v), want (%v, nil)", got.value, got.err, want)
		}
	}
	if value, err := await.(IO)(); value != want || err != nil || calls.Load() != 1 {
		t.Fatalf("repeated Await = (%v, %v), calls = %d", value, err, calls.Load())
	}
	other, err := fork()
	if err != nil || other == fiber {
		t.Fatalf("repeated Fork = (%v, %v), want a fresh fiber", other, err)
	}
	if value, err := Await(other).(IO)(); value != want || err != nil || calls.Load() != 2 {
		t.Fatalf("second fiber = (%v, %v), calls = %d", value, err, calls.Load())
	}
}

func TestAwaitFailureAndPanic(t *testing.T) {
	wantErr := errors.New("fiber failed")
	for _, tt := range []struct {
		name string
		io   IO
		want string
	}{
		{"failure", IO(func() (any, error) { return nil, wantErr }), "fiber failed"},
		{"panic string", IO(func() (any, error) { panic("broken task") }), "panic: broken task"},
		{"panic error", IO(func() (any, error) { panic(wantErr) }), "panic: fiber failed"},
		{"panic nil", IO(func() (any, error) { panic(nil) }), "panic: panic called with nil argument"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fiber, err := Fork(tt.io).(IO)()
			if err != nil {
				t.Fatalf("Fork failed rather than capturing the outcome: %v", err)
			}
			for range 2 {
				got := receiveConcTest(t, runConcTest(Await(fiber)))
				if got.value != nil || got.err == nil || got.err.Error() != tt.want {
					t.Fatalf("Await = (%v, %v), want (nil, %q)", got.value, got.err, tt.want)
				}
				if tt.name == "failure" && got.err != wantErr {
					t.Fatal("Await did not preserve the original error")
				}
			}
		})
	}
	fiber, _ := Fork(Fail("stop")).(IO)()
	called := false
	_, err := Bind(Await(fiber), func(any) any {
		called = true
		return Pure(Unit{})
	}).(IO)()
	if called || err == nil || err.Error() != "stop" {
		t.Fatalf("awaited failure did not short-circuit Bind: called=%v, error=%v", called, err)
	}
}

func TestParMapIOParallelAndInputOrder(t *testing.T) {
	installTestShapes(t)
	values := []string{"first", "second", "third"}
	gates := make(map[string]<-chan struct{}, len(values))
	releases := make([]func(), len(values))
	for i, value := range values {
		gates[value], releases[i] = concTestGate(t)
	}
	started := make(chan string, len(values))
	finished := make(chan string, len(values))
	var called atomic.Int32
	action := ParMapIO(ListFromStrings(values), func(value any) any {
		called.Add(1)
		return IO(func() (any, error) {
			name := value.(string)
			started <- name
			<-gates[name]
			finished <- name
			return strings.ToUpper(name), nil
		})
	})
	if called.Load() != 0 {
		t.Fatal("ParMapIO evaluated its mapper before execution")
	}
	result := runConcTest(action)
	seen := make(map[string]bool)
	for range values {
		seen[receiveConcTest(t, started)] = true
	}
	if len(seen) != len(values) {
		t.Fatalf("started tasks = %v, want every input", seen)
	}
	for i := len(releases) - 1; i >= 0; i-- {
		releases[i]()
		if got := receiveConcTest(t, finished); got != values[i] {
			t.Fatalf("completed task = %q, want %q", got, values[i])
		}
	}
	got := receiveConcTest(t, result)
	if got.err != nil {
		t.Fatal(got.err)
	}
	if list := ListToStrings(got.value); !slices.Equal(list, []string{"FIRST", "SECOND", "THIRD"}) {
		t.Fatalf("result list = %v, want input order", list)
	}
}

func TestParMapIOEmptyAndFailures(t *testing.T) {
	installTestShapes(t)
	value, err := ParMapIO(Shapes.Nil(), func(any) any {
		t.Fatal("mapper called for empty list")
		return nil
	}).(IO)()
	if value != (testNil{}) || err != nil {
		t.Fatalf("empty result = (%v, %v)", value, err)
	}

	for _, mapperPanic := range []bool{false, true} {
		gate, release := concTestGate(t)
		failed := make(chan struct{})
		finished := make(chan struct{})
		result := runConcTest(ParMapIO(ListFromStrings([]string{"blocked", "failure"}), func(a any) any {
			if a == "failure" && mapperPanic {
				close(failed)
				panic("mapper failed")
			}
			return IO(func() (any, error) {
				if a == "failure" {
					close(failed)
					return nil, errors.New("task failed")
				}
				<-gate
				close(finished)
				return a, nil
			})
		}))
		receiveConcTest(t, failed)
		select {
		case got := <-result:
			t.Fatalf("ParMapIO returned before every task completed: %+v", got)
		default:
		}
		release()
		got := receiveConcTest(t, result)
		receiveConcTest(t, finished)
		want := "task failed"
		if mapperPanic {
			want = "panic: mapper failed"
		}
		if got.value != nil || got.err == nil || got.err.Error() != want {
			t.Fatalf("ParMapIO = (%v, %v), want (nil, %q)", got.value, got.err, want)
		}
	}
}

func TestRaceFirstCompletionAndAbandonedPanic(t *testing.T) {
	for _, winnerOnLeft := range []bool{true, false} {
		for _, failure := range []bool{false, true} {
			gate, release := concTestGate(t)
			started, finished := make(chan struct{}), make(chan struct{})
			slow := IO(func() (any, error) {
				close(started)
				<-gate
				defer close(finished)
				panic("loser failed after race returned")
			})
			wantErr := errors.New("winner failed")
			fast := IO(func() (any, error) {
				<-started
				if failure {
					return nil, wantErr
				}
				return "winner", nil
			})
			left, right := fast, slow
			if !winnerOnLeft {
				left, right = right, left
			}
			action := Race(left, right)
			select {
			case <-started:
				t.Fatal("Race started an effect before execution")
			default:
			}
			got := receiveConcTest(t, runConcTest(action))
			if failure {
				if got.value != nil || got.err != wantErr {
					t.Fatalf("Race = (%v, %v), want winning failure", got.value, got.err)
				}
			} else if got.value != "winner" || got.err != nil {
				t.Fatalf("Race = (%v, %v), want (winner, nil)", got.value, got.err)
			}
			release()
			receiveConcTest(t, finished)
		}
	}
}

func TestParBothTupleAndFailure(t *testing.T) {
	installTestShapes(t)
	Shapes.Tuple2 = func(a, b any) any { return [2]any{a, b} }
	for _, failure := range []bool{false, true} {
		gate, release := concTestGate(t)
		started := make(chan struct{}, 2)
		left := IO(func() (any, error) {
			started <- struct{}{}
			<-gate
			return int64(7), nil
		})
		right := IO(func() (any, error) {
			started <- struct{}{}
			if failure {
				panic("right failed")
			}
			return "right", nil
		})
		result := runConcTest(ParBoth(left, right))
		receiveConcTest(t, started)
		receiveConcTest(t, started)
		select {
		case got := <-result:
			t.Fatalf("ParBoth returned before both tasks completed: %+v", got)
		default:
		}
		release()
		got := receiveConcTest(t, result)
		if failure {
			if got.value != nil || got.err == nil || got.err.Error() != "panic: right failed" {
				t.Fatalf("ParBoth = (%v, %v), want right panic failure", got.value, got.err)
			}
		} else if !reflect.DeepEqual(got.value, [2]any{int64(7), "right"}) || got.err != nil {
			t.Fatalf("ParBoth = (%v, %v), want (7, right) tuple", got.value, got.err)
		}
	}
}

func TestTimeoutSomeNoneAndFailures(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		name string
		io   any
		want any
		err  string
	}{
		{"success", Pure(int64(8)), testSome{int64(8)}, ""},
		{"nil success", Pure(nil), testSome{nil}, ""},
		{"failure", Fail("timed task failed"), nil, "timed task failed"},
		{"panic", IO(func() (any, error) { panic("timed panic") }), nil, "panic: timed panic"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := receiveConcTest(t, runConcTest(Timeout(int64(1000), tt.io)))
			if !reflect.DeepEqual(got.value, tt.want) {
				t.Fatalf("value = %#v, want %#v", got.value, tt.want)
			}
			if tt.err == "" && got.err != nil || tt.err != "" && (got.err == nil || got.err.Error() != tt.err) {
				t.Fatalf("error = %v, want %q", got.err, tt.err)
			}
		})
	}
	gate, release := concTestGate(t)
	started, finished := make(chan struct{}), make(chan struct{})
	action := Timeout(int64(5), IO(func() (any, error) {
		close(started)
		<-gate
		defer close(finished)
		panic("late panic is captured")
	}))
	select {
	case <-started:
		t.Fatal("Timeout ran before its IO was executed")
	default:
	}
	result := runConcTest(action)
	receiveConcTest(t, started)
	got := receiveConcTest(t, result)
	if got.value != (testNone{}) || got.err != nil {
		t.Fatalf("expired timeout = (%v, %v), want (None, nil)", got.value, got.err)
	}
	release()
	receiveConcTest(t, finished)
}

func TestTimeoutDurationBoundaries(t *testing.T) {
	installTestShapes(t)
	for _, millis := range []int64{math.MinInt64, -1, 0, math.MaxInt64} {
		var called atomic.Bool
		got := receiveConcTest(t, runConcTest(Timeout(millis, IO(func() (any, error) {
			called.Store(true)
			return int64(1), nil
		}))))
		if called.Load() {
			t.Fatalf("Timeout(%d) started an action with no valid time budget", millis)
		}
		if millis <= 0 {
			if got.value != (testNone{}) || got.err != nil {
				t.Fatalf("Timeout(%d) = (%v, %v), want (None, nil)", millis, got.value, got.err)
			}
		} else if got.value != nil || got.err == nil || got.err.Error() != "timeout: duration out of range" {
			t.Fatalf("Timeout(%d) = (%v, %v), want range failure", millis, got.value, got.err)
		}
	}
}
