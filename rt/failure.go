package rt

import (
	"context"
	"errors"
	"net"
	"os"
)

// Kind identifies one of the constructors of std/io.IoError.
type Kind string

const (
	NotFound         Kind = "NotFound"
	PermissionDenied Kind = "PermissionDenied"
	AlreadyExists    Kind = "AlreadyExists"
	TimeoutKind      Kind = "Timeout"
	Cancelled        Kind = "Cancelled"
	Panic            Kind = "Panic"
	Network          Kind = "Network"
	FailureKind      Kind = "Failure"
)

// Failure is the runtime representation of every failed IO outcome.
type Failure struct {
	Kind    Kind
	Message string
}

func (f *Failure) Error() string { return f.Message }

// Is retains standard sentinel predicates after the foreign error is classified.
// Messages alone never identify cancellation or a filesystem error.
func (f *Failure) Is(target error) bool {
	switch f.Kind {
	case NotFound:
		return target == os.ErrNotExist
	case PermissionDenied:
		return target == os.ErrPermission
	case AlreadyExists:
		return target == os.ErrExist
	case TimeoutKind:
		return target == context.DeadlineExceeded
	case Cancelled:
		return target == context.Canceled
	default:
		return false
	}
}

// ErrorClassifier recognizes a foreign error without changing unrecognized errors.
// The boolean reports whether its kind and message should be used.
type ErrorClassifier func(error) (Kind, string, bool)

var errorClassifiers []ErrorClassifier

// RegisterErrorClassifier appends a classifier in precedence order. Generated
// shims register during package initialization, before any IO is executed.
func RegisterErrorClassifier(classifier ErrorClassifier) {
	errorClassifiers = append(errorClassifiers, classifier)
}

// ClassifyError converts foreign errors at the IO boundary. Success and already
// classified failures pass through unchanged, including wrapped typed failures.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	if failure, ok := err.(*Failure); ok {
		return failure
	}
	var failure *Failure
	if errors.As(err, &failure) {
		return failure
	}
	for _, classify := range errorClassifiers {
		if kind, message, ok := classify(err); ok {
			return &Failure{Kind: kind, Message: message}
		}
	}
	kind, message := FailureKind, err.Error()
	var network net.Error
	switch {
	case errors.Is(err, os.ErrNotExist):
		kind = NotFound
	case errors.Is(err, os.ErrPermission):
		kind = PermissionDenied
	case errors.Is(err, os.ErrExist):
		kind = AlreadyExists
	case errors.Is(err, context.DeadlineExceeded):
		kind, message = TimeoutKind, context.DeadlineExceeded.Error()
	case errors.Is(err, context.Canceled):
		kind, message = Cancelled, context.Canceled.Error()
	case errors.As(err, &network):
		if network.Timeout() {
			kind, message = TimeoutKind, context.DeadlineExceeded.Error()
			break
		}
		kind = Network
	}
	return &Failure{Kind: kind, Message: message}
}

// failureValue constructs the generated IoError carried by an Err result.
func failureValue(err error) any {
	if Shapes.IoError == nil {
		panic("rt.Shapes is not initialized: IoError is required")
	}
	failure := ClassifyError(err).(*Failure)
	return Shapes.IoError(string(failure.Kind), failure.Message)
}

// Raise constructs a deferred failure from a generated IoError.
func Raise(value any) any {
	if Shapes.IoErrorOf == nil {
		panic("rt.Shapes is not initialized: IoErrorOf is required")
	}
	kind, message := Shapes.IoErrorOf(value)
	failure := &Failure{Kind: Kind(kind), Message: message}
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		return nil, failure
	})
}
