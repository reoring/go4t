package rt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestBindCancellationBoundaries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := false
	left := IO(func(got context.Context) (any, error) {
		if got != ctx {
			t.Fatal("Bind replaced its execution context")
		}
		cancel()
		return Unit{}, nil
	})
	_, err := Bind(left, func(any) any {
		called = true
		return Pure(Unit{})
	}).(IO)(ctx)
	if called || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Bind: continuation=%v, error=%v", called, err)
	}
	installTestShapes(t)
	for _, action := range []any{Pure(Unit{}), Fail("not reached"), Raise(testIoError{"NotFound", "not reached"}), Sleep(int64(0)), ReadFile("not reached"), Println("not reached")} {
		if _, err := action.(IO)(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled effect error = %v", err)
		}
		if _, err := action.(IO)(ctx); !reflect.DeepEqual(err, &Failure{Kind: Cancelled, Message: context.Canceled.Error()}) {
			t.Fatalf("cancelled effect was not classified: %#v", err)
		}
	}
	got, err := TryIO(Pure(Unit{})).(IO)(ctx)
	if err != nil || got != (testErr{testIoError{"Cancelled", context.Canceled.Error()}}) {
		t.Fatalf("TryIO cancellation = (%v, %v)", got, err)
	}
}

func TestCancelFiberAndParent(t *testing.T) {
	for _, parentCancel := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		started, finished := make(chan struct{}), make(chan struct{})
		action := IO(func(child context.Context) (any, error) {
			defer close(finished)
			if child == ctx {
				return nil, errors.New("fiber did not get a child context")
			}
			close(started)
			return Sleep(int64(10000)).(IO)(child)
		})
		value, err := Fork(action).(IO)(ctx)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		receiveConcTest(t, started)
		if parentCancel {
			cancel()
		} else {
			assertIOUnit(t, Cancel(value))
			assertIOUnit(t, Cancel(value))
		}
		for range 2 {
			got := receiveConcTest(t, runConcTest(Await(value)))
			if got.value != nil || !errors.Is(got.err, context.Canceled) {
				t.Fatalf("cancelled Await = (%v, %v)", got.value, got.err)
			}
		}
		receiveConcTest(t, finished)
		cancel()
	}
}

func TestCancelCompletedFiberKeepsOutcome(t *testing.T) {
	wantErr := &Failure{Kind: FailureKind, Message: "completed failure"}
	for _, failure := range []error{nil, wantErr} {
		value, err := Fork(IO(func(context.Context) (any, error) {
			if failure != nil {
				return nil, failure
			}
			return "completed", nil
		})).(IO)(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		fiber := value.(*Fiber)
		receiveConcTest(t, fiber.done)
		for range 2 {
			assertIOUnit(t, Cancel(fiber))
			got, err := Await(fiber).(IO)(context.Background())
			if err != failure || failure == nil && got != "completed" || failure != nil && got != nil {
				t.Fatalf("completed fiber changed after cancel: (%v, %v), original error=%v", got, err, failure)
			}
		}
	}
}

func TestCancelUncooperativeFiberDoesNotWait(t *testing.T) {
	for _, latePanic := range []bool{false, true} {
		gate, release := concTestGate(t)
		started := make(chan struct{})
		value, err := Fork(IO(func(context.Context) (any, error) {
			close(started)
			<-gate
			if latePanic {
				panic("late panic")
			}
			return "late success", nil
		})).(IO)(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		fiber := value.(*Fiber)
		receiveConcTest(t, started)
		start := time.Now()
		assertIOUnit(t, Cancel(fiber))
		got := receiveConcTest(t, runConcTest(Await(fiber)))
		if !errors.Is(got.err, context.Canceled) || time.Since(start) >= time.Second {
			t.Fatalf("cancelled uncooperative fiber: %v after %s", got.err, time.Since(start))
		}
		release()
		receiveConcTest(t, fiber.done)
		if _, err := Await(fiber).(IO)(context.Background()); !errors.Is(err, context.Canceled) {
			t.Fatalf("late outcome replaced cancellation: %v", err)
		}
	}
}

func TestTimeoutCancelsSleepBeforePrint(t *testing.T) {
	installTestShapes(t)
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = stdout
	t.Cleanup(func() { os.Stdout = previous; stdout.Close() })
	finished := make(chan struct{})
	action := IO(func(ctx context.Context) (any, error) {
		defer close(finished)
		return Bind(Sleep(int64(10000)), func(any) any {
			return Println("must not print")
		}).(IO)(ctx)
	})
	start := time.Now()
	got, err := Timeout(int64(10), action).(IO)(context.Background())
	if got != (testNone{}) || err != nil || time.Since(start) >= time.Second {
		t.Fatalf("Timeout = (%v, %v) after %s", got, err, time.Since(start))
	}
	select {
	case <-finished:
	default:
		t.Fatal("timeout returned before cooperative task finished")
	}
	info, err := stdout.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatalf("cancelled timeout printed: info=%v, error=%v", info, err)
	}
}

func TestRaceCancelsAndJoinsLoser(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	var sideEffect atomic.Bool
	loser := IO(func(ctx context.Context) (any, error) {
		defer close(finished)
		close(started)
		return Bind(Sleep(int64(10000)), func(any) any {
			sideEffect.Store(true)
			return Pure("loser")
		}).(IO)(ctx)
	})
	winner := IO(func(ctx context.Context) (any, error) {
		<-started
		return "winner", nil
	})
	got, err := Race(loser, winner).(IO)(context.Background())
	if got != "winner" || err != nil || sideEffect.Load() {
		t.Fatalf("Race = (%v, %v), loser effect=%v", got, err, sideEffect.Load())
	}
	select {
	case <-finished:
	default:
		t.Fatal("race returned before cooperative loser finished")
	}
}

func TestWithTimeoutBoundariesAndErrors(t *testing.T) {
	for _, millis := range []int64{math.MinInt64, -1, 0, math.MaxInt64} {
		var called atomic.Bool
		got, err := WithTimeout(millis, IO(func(context.Context) (any, error) {
			called.Store(true)
			return Unit{}, nil
		})).(IO)(context.Background())
		if got != nil || called.Load() {
			t.Fatalf("WithTimeout(%d) = %v, action called=%v", millis, got, called.Load())
		}
		if millis <= 0 {
			if !errors.Is(err, context.DeadlineExceeded) || err.Error() != context.DeadlineExceeded.Error() {
				t.Fatalf("WithTimeout(%d) error = %v", millis, err)
			}
		} else if err == nil || err.Error() != "withTimeout: duration out of range" {
			t.Fatalf("overflow error = %v", err)
		}
	}
	if got, err := WithTimeout(int64(1000), Pure("ready")).(IO)(context.Background()); got != "ready" || err != nil {
		t.Fatalf("successful WithTimeout = (%v, %v)", got, err)
	}
	if _, err := WithTimeout(int64(10), Sleep(int64(10000))).(IO)(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired WithTimeout = %v", err)
	}
	want := errors.New("action failure")
	for _, failure := range []error{want, context.DeadlineExceeded, context.Canceled, &Failure{Kind: TimeoutKind, Message: context.DeadlineExceeded.Error()}} {
		action := IO(func(context.Context) (any, error) { return nil, failure })
		for _, timed := range []any{Timeout(int64(1000), action), WithTimeout(int64(1000), action)} {
			if _, err := timed.(IO)(context.Background()); !reflect.DeepEqual(err, ClassifyError(failure)) {
				t.Fatalf("timely error = %#v, want classified %#v", err, failure)
			}
		}
	}
	parent, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := Timeout(int64(1000), Sleep(int64(10000))).(IO)(parent); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("parent deadline swallowed by Timeout: %v", err)
	}
}

func TestTimeoutCleanupIsBounded(t *testing.T) {
	gate, release := concTestGate(t)
	started, finished := make(chan struct{}), make(chan struct{})
	action := IO(func(ctx context.Context) (any, error) {
		defer close(finished)
		close(started)
		<-gate
		return Unit{}, nil
	})
	start := time.Now()
	result := runConcTest(WithTimeout(int64(10), action))
	receiveConcTest(t, started)
	got := receiveConcTest(t, result)
	elapsed := time.Since(start)
	if !errors.Is(got.err, context.DeadlineExceeded) || elapsed < 100*time.Millisecond || elapsed >= time.Second {
		t.Fatalf("bounded cleanup: error=%v duration=%s", got.err, elapsed)
	}
	release()
	receiveConcTest(t, finished)
}

func TestParMapIOCancelsSiblingsOnFailure(t *testing.T) {
	installTestShapes(t)
	started, finished := make(chan struct{}), make(chan struct{})
	want := &Failure{Kind: FailureKind, Message: "first failure"}
	action := ParMapIO(ListFromStrings([]string{"slow", "fail"}), func(value any) any {
		return IO(func(ctx context.Context) (any, error) {
			if value == "fail" {
				<-started
				return nil, want
			}
			defer close(finished)
			close(started)
			return Sleep(int64(10000)).(IO)(ctx)
		})
	})
	got := receiveConcTest(t, runConcTest(action))
	if got.err != want {
		t.Fatalf("ParMapIO error = %v, want first failure", got.err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("ParMapIO did not cancel and join its sibling")
	}
}

type cancelledInput struct {
	started chan struct{}
	finish  chan struct{}
}

func (r cancelledInput) Read([]byte) (int, error) {
	close(r.started)
	<-r.finish
	return 0, io.EOF
}

func TestStdinCancellation(t *testing.T) {
	for _, action := range []func() any{ReadLine, ReadAll} {
		input := cancelledInput{make(chan struct{}), make(chan struct{})}
		setTestInput(t, input)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := action().(IO)(ctx)
			result <- err
		}()
		receiveConcTest(t, input.started)
		cancel()
		if err := receiveConcTest(t, result); !errors.Is(err, context.Canceled) {
			t.Fatalf("stdin cancellation = %v", err)
		}
		// A second reader must be able to cancel while waiting for the buffer.
		if _, err := ReadLine().(IO)(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("queued stdin cancellation = %v", err)
		}
		close(input.finish)
		stdinGate <- struct{}{}
		<-stdinGate
	}
}

func TestRunMainSignalCancellation(t *testing.T) {
	if os.Getenv("GO4T_RT_SIGNAL_TEST") == "1" {
		RunMain(Bind(Println("ready"), func(any) any { return Sleep(int64(10000)) }))
		os.Exit(0)
	}
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunMainSignalCancellation$")
			command.Env = append(os.Environ(), "GO4T_RT_SIGNAL_TEST=1", "GORACE=atexit_sleep_ms=0")
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			ready, readErr := bufio.NewReader(stdout).ReadString('\n')
			if readErr != nil || ready != "ready\n" {
				command.Process.Kill()
				command.Wait()
				t.Fatalf("subprocess readiness = %q, %v", ready, readErr)
			}
			start := time.Now()
			if err := command.Process.Signal(signal); err != nil {
				command.Process.Kill()
				command.Wait()
				t.Fatal(err)
			}
			err = command.Wait()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 || strings.TrimSpace(stderr.String()) != "error: cancelled" || time.Since(start) >= time.Second {
				t.Fatalf("signal exit: error=%v stderr=%q elapsed=%s", err, stderr.String(), time.Since(start))
			}
		})
	}
}
