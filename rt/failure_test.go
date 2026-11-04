package rt

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClassifyError(t *testing.T) {
	if got := ClassifyError(nil); got != nil {
		t.Fatalf("ClassifyError(nil) = %v", got)
	}
	for _, tt := range []struct {
		name string
		err  error
		kind Kind
	}{
		{"missing", &os.PathError{Op: "open", Path: "missing", Err: os.ErrNotExist}, NotFound},
		{"permission", &os.PathError{Op: "open", Path: "private", Err: os.ErrPermission}, PermissionDenied},
		{"exists", &os.LinkError{Op: "link", Old: "a", New: "b", Err: os.ErrExist}, AlreadyExists},
		{"timeout", context.DeadlineExceeded, TimeoutKind},
		{"cancelled", context.Canceled, Cancelled},
		{"network", &net.DNSError{Err: "no such host", Name: "invalid"}, Network},
		{"network timeout", &net.DNSError{Err: "timed out", IsTimeout: true}, TimeoutKind},
		{"URL timeout", &url.Error{Op: "Get", URL: "http://localhost", Err: &net.DNSError{IsTimeout: true}}, TimeoutKind},
		{"URL cancellation", &url.Error{Op: "Get", URL: "http://localhost", Err: context.Canceled}, Cancelled},
		{"URL failure", &url.Error{Op: "Get", URL: "bad", Err: errors.New("bad URL")}, Network},
		{"failure", errors.New("foreign failure"), FailureKind},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, wrapped := range []bool{false, true} {
				err := tt.err
				if wrapped {
					err = fmt.Errorf("outer: %w", err)
				}
				message := err.Error()
				if tt.kind == TimeoutKind {
					message = context.DeadlineExceeded.Error()
				} else if tt.kind == Cancelled {
					message = context.Canceled.Error()
				}
				want := &Failure{Kind: tt.kind, Message: message, Cause: err}
				got := ClassifyError(err)
				if !reflect.DeepEqual(got, want) || got.Error() != message {
					t.Fatalf("ClassifyError(%v) = %#v, want %#v", err, got, want)
				}
				if got.(*Failure).Cause != err || errors.Unwrap(got) != err {
					t.Fatal("original error identity was not retained as the cause")
				}
				if ClassifyError(got) != got || ClassifyError(fmt.Errorf("wrapped: %w", got)) != got {
					t.Fatal("classified failure identity was not preserved")
				}
			}
		})
	}
}

type testStatusError struct{ reason error }

func (e *testStatusError) Error() string { return "pods/example: " + e.reason.Error() }
func (e *testStatusError) Unwrap() error { return e.reason }

func TestClassifyErrorClassifierCause(t *testing.T) {
	previous := errorClassifiers
	t.Cleanup(func() { errorClassifiers = previous })
	errorClassifiers = nil
	RegisterErrorClassifier(func(err error) (Kind, string, bool) {
		var status *testStatusError
		if errors.As(err, &status) {
			return NotFound, "classified: " + err.Error(), true
		}
		return "", "", false
	})
	sentinel := errors.New("not found")
	status := &testStatusError{reason: sentinel}
	for _, original := range []error{status, fmt.Errorf("request: %w", status)} {
		failure := ClassifyError(original).(*Failure)
		if failure.Kind != NotFound || failure.Message != "classified: "+original.Error() || failure.Cause != original {
			t.Fatalf("classifier lost kind, message or cause: %#v", failure)
		}
		if !errors.Is(failure, sentinel) || !failure.Is(sentinel) {
			t.Fatal("classified failure lost its foreign sentinel")
		}
		if errors.Is(failure, os.ErrNotExist) || failure.Is(os.ErrNotExist) {
			t.Fatal("kind-based sentinel must not override a non-nil cause")
		}
		var recovered *testStatusError
		if !errors.As(failure, &recovered) || recovered != status {
			t.Fatal("classified failure lost its concrete foreign error")
		}
	}
}

func TestClassifyErrorNestedFailureCause(t *testing.T) {
	cause := &testStatusError{reason: errors.New("conflict")}
	inner := &Failure{Kind: FailureKind, Message: "Conflict: " + cause.Error(), Cause: cause}
	wrapped := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", inner))
	got := ClassifyError(wrapped)
	if got != inner || got.(*Failure).Cause != cause {
		t.Fatalf("nested failure changed: %#v, want %#v", got, inner)
	}
	if !errors.Is(got, cause.reason) || errors.Unwrap(got) != cause {
		t.Fatal("nested failure lost its cause chain")
	}
}

func TestFailureSentinelPredicates(t *testing.T) {
	sentinels := []struct {
		kind Kind
		err  error
	}{
		{NotFound, os.ErrNotExist},
		{PermissionDenied, os.ErrPermission},
		{AlreadyExists, os.ErrExist},
		{TimeoutKind, context.DeadlineExceeded},
		{Cancelled, context.Canceled},
	}
	for _, tt := range sentinels {
		failure := &Failure{Kind: tt.kind, Message: tt.err.Error()}
		if errors.Unwrap(failure) != nil {
			t.Fatal("a cause-free failure must unwrap to nil")
		}
		for _, target := range sentinels {
			if got := errors.Is(failure, target.err); got != (tt.kind == target.kind) {
				t.Errorf("errors.Is(%#v, %v) = %v", failure, target.err, got)
			}
		}
		if !errors.Is(fmt.Errorf("wrapped: %w", failure), tt.err) {
			t.Errorf("wrapped %s lost its sentinel", tt.kind)
		}
		if errors.Is(&Failure{Kind: FailureKind, Message: tt.err.Error()}, tt.err) {
			t.Errorf("message text falsely classified %s", tt.kind)
		}
	}
}

func TestRaiseRoundTrip(t *testing.T) {
	installTestShapes(t)
	for _, kind := range []Kind{NotFound, PermissionDenied, AlreadyExists, TimeoutKind, Cancelled, Panic, Network, FailureKind} {
		for _, message := range []string{"", "failure 日本語"} {
			value := testIoError{string(kind), message}
			action := Raise(value).(IO)
			for range 2 {
				got, err := action(context.Background())
				if got != nil || !reflect.DeepEqual(err, &Failure{Kind: kind, Message: message}) {
					t.Fatalf("Raise = (%v, %#v)", got, err)
				}
				got, err = TryIO(action).(IO)(context.Background())
				if err != nil || got != (testErr{value}) {
					t.Fatalf("TryIO(Raise) = (%#v, %v)", got, err)
				}
			}
		}
	}
}

func TestIOFailureBoundaries(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, action := range []any{ReadFile(missing), RemoveFile(missing), ListDir(missing)} {
		_, err := action.(IO)(context.Background())
		failure, ok := err.(*Failure)
		if !ok || failure.Kind != NotFound {
			t.Fatalf("filesystem error = %#v, want NotFound", err)
		}
	}
	for _, action := range []any{Fail(""), RandomInt(int64(2), int64(1)), Sleep(int64(1<<63 - 1)), Timeout(int64(1<<63-1), Pure(Unit{}))} {
		_, err := action.(IO)(context.Background())
		failure, ok := err.(*Failure)
		if !ok || failure.Kind != FailureKind {
			t.Fatalf("IO error = %#v, want Failure", err)
		}
	}
	for _, input := range []func() any{ReadLine, ReadAll} {
		setTestInput(t, failingInput{os.ErrPermission})
		_, err := input().(IO)(context.Background())
		if !reflect.DeepEqual(err, &Failure{Kind: PermissionDenied, Message: os.ErrPermission.Error(), Cause: os.ErrPermission}) {
			t.Fatalf("stdin error = %#v", err)
		}
	}
}

func TestForeignFailuresAtSequenceAndConcurrentBoundaries(t *testing.T) {
	foreign := IO(func(context.Context) (any, error) { return nil, os.ErrPermission })
	for _, action := range []IO{
		Bind(foreign, func(any) any { t.Fatal("failed bind continued"); return Pure(nil) }).(IO),
		Bind(Pure(nil), func(any) any { return foreign }).(IO),
		IO(func(ctx context.Context) (any, error) { return runConcurrentIO(ctx, foreign) }),
	} {
		_, err := action(context.Background())
		if !reflect.DeepEqual(err, &Failure{Kind: PermissionDenied, Message: os.ErrPermission.Error(), Cause: os.ErrPermission}) {
			t.Fatalf("foreign failure = %#v", err)
		}
	}
	_, err := runConcurrentIO(context.Background(), IO(func(context.Context) (any, error) { panic(os.ErrPermission) }))
	if !reflect.DeepEqual(err, &Failure{Kind: Panic, Message: os.ErrPermission.Error()}) {
		t.Fatalf("concurrent panic = %#v, want Panic rather than PermissionDenied", err)
	}
}
