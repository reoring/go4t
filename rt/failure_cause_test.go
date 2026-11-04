package rt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestTryIOCause(t *testing.T) {
	installTestShapes(t)
	original := fmt.Errorf("read: %w", os.ErrNotExist)
	inner := &Failure{Kind: NotFound, Message: "classified message", Cause: original}
	for _, tt := range []struct {
		name   string
		action any
		want   any
	}{
		{"success", Pure(int64(42)), testOk{int64(42)}},
		{"nil success", Pure(nil), testOk{nil}},
		{"local failure", Fail("failed"), testErr{testIoFailure{testIoError{"Failure", "failed"}, testNone{}}}},
		{"foreign error", IO(func(context.Context) (any, error) { return nil, original }), testErr{testIoFailure{testIoError{"NotFound", original.Error()}, testSome{original}}}},
		{"nested failure", IO(func(context.Context) (any, error) { return nil, fmt.Errorf("outer: %w", inner) }), testErr{testIoFailure{testIoError{"NotFound", inner.Message}, testSome{original}}}},
		{"string panic", IO(func(context.Context) (any, error) { panic("boom") }), testErr{testIoFailure{testIoError{"Panic", "boom"}, testNone{}}}},
		{"error panic", IO(func(context.Context) (any, error) { panic(original) }), testErr{testIoFailure{testIoError{"Panic", original.Error()}, testNone{}}}},
		{"nil panic", IO(func(context.Context) (any, error) { panic(nil) }), testErr{testIoFailure{testIoError{"Panic", "panic called with nil argument"}, testNone{}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryIOCause(tt.action).(IO)(context.Background())
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("TryIOCause = (%#v, %v), want (%#v, nil)", got, err, tt.want)
			}
			if result, ok := got.(testErr); ok {
				failure := result.value.(testIoFailure)
				if cause, ok := failure.cause.(testSome); ok && cause.value != original {
					t.Fatalf("cause = %#v, want original error identity", cause.value)
				}
				legacy, err := TryIO(tt.action).(IO)(context.Background())
				if err != nil || legacy != (testErr{failure.failure}) {
					t.Fatalf("TryIO changed its IoError surface: (%#v, %v)", legacy, err)
				}
			}
		})
	}
}

func TestTryIOCauseCancellation(t *testing.T) {
	installTestShapes(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	action := IO(func(context.Context) (any, error) {
		t.Fatal("cancelled capture ran its action")
		return nil, nil
	})
	got, err := TryIOCause(action).(IO)(ctx)
	want := testErr{testIoFailure{testIoError{"Cancelled", context.Canceled.Error()}, testSome{context.Canceled}}}
	if err != nil || got != want {
		t.Fatalf("TryIOCause cancellation = (%#v, %v), want %#v", got, err, want)
	}
}

func TestTryIOCauseIsLazyAndAllowsContinuation(t *testing.T) {
	installTestShapes(t)
	calls := 0
	original := errors.New("foreign failure")
	action := TryIOCause(IO(func(context.Context) (any, error) {
		calls++
		return nil, original
	}))
	if calls != 0 {
		t.Fatal("TryIOCause executed eagerly")
	}
	continued := Bind(action, func(value any) any {
		failure := value.(testErr).value.(testIoFailure)
		if failure.cause != (testSome{original}) {
			t.Fatalf("continuation lost cause: %#v", failure)
		}
		return Pure("continued")
	}).(IO)
	for range 2 {
		if got, err := continued(context.Background()); got != "continued" || err != nil {
			t.Fatalf("continuation = (%v, %v)", got, err)
		}
	}
	if calls != 2 {
		t.Fatalf("action ran %d times, want 2", calls)
	}
}

func TestRaiseCauseRoundTrip(t *testing.T) {
	installTestShapes(t)
	original := &testStatusError{reason: errors.New("foreign sentinel")}
	for _, kind := range []Kind{NotFound, PermissionDenied, AlreadyExists, TimeoutKind, Cancelled, Panic, Network, FailureKind} {
		for _, message := range []string{"", "failure 日本語"} {
			if kind == TimeoutKind {
				message = context.DeadlineExceeded.Error()
			} else if kind == Cancelled {
				message = context.Canceled.Error()
			}
			for _, cause := range []error{nil, original} {
				var option any = testNone{}
				if cause != nil {
					option = testSome{cause}
				}
				value := testIoFailure{testIoError{string(kind), message}, option}
				action := RaiseCause(value).(IO)
				for range 2 {
					got, err := action(context.Background())
					want := &Failure{Kind: kind, Message: message, Cause: cause}
					if got != nil || !reflect.DeepEqual(err, want) || err.(*Failure).Cause != cause {
						t.Fatalf("RaiseCause = (%v, %#v), want %#v", got, err, want)
					}
					if cause != nil && !errors.Is(err, original.reason) {
						t.Fatal("RaiseCause lost the foreign sentinel")
					}
					got, err = TryIOCause(action).(IO)(context.Background())
					if err != nil || got != (testErr{value}) {
						t.Fatalf("TryIOCause(RaiseCause) = (%#v, %v)", got, err)
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, err := action(ctx); !errors.Is(err, context.Canceled) || err.(*Failure).Cause != context.Canceled {
					t.Fatalf("RaiseCause ignored executing cancellation: %#v", err)
				}
			}
		}
	}
}

func TestGoErrorMessage(t *testing.T) {
	for _, message := range []string{"", "original 日本語"} {
		original := errors.New(message)
		if got := GoErrorMessage(original); got != message {
			t.Fatalf("GoErrorMessage = %q, want %q", got, message)
		}
	}
}
