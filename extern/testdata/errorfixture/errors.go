package errorfixture

import (
	"errors"
	"fmt"
)

type sentinelError struct{ message string }

func (e *sentinelError) Error() string { return e.message }

var Sentinel error = &sentinelError{message: "sentinel"}

type Report struct {
	Err error
	N   int
}

type Nested struct {
	Reports []Report
	Errors  map[string]error
}

// The exported Go name must not shadow the builtin error handle.
type GoError struct{ N int }

func Classify(err error) string {
	if errors.Is(err, Sentinel) {
		return "sentinel"
	}
	return "other"
}

func IsSentinel(err error) bool    { return errors.Is(err, Sentinel) }
func Fail() error                 { return Sentinel }
func Wrap(msg string) error       { return fmt.Errorf("%s: %w", msg, Sentinel) }
func Succeed() error              { return nil }
func Errs() []error               { return []error{nil, Sentinel, Wrap("nested")} }
func EchoErrs(errs []error) []error { return errs }
func ErrorMap() map[string]error {
	return map[string]error{"nil": nil, "sentinel": Sentinel}
}
func EchoErrorMap(errs map[string]error) map[string]error { return errs }
func EchoReport(report Report) Report                   { return report }
func EchoNested(nested Nested) Nested                   { return nested }
func EchoError(err error) (error, int)                   { return err, 7 }
func NonTrailing(present bool) (error, int) {
	if present {
		return Sentinel, 7
	}
	return nil, 0
}
func TypedNil() (error, int)                        { return (*sentinelError)(nil), 0 }
func Invoke(f func(error) string) string            { return f(Sentinel) }
func InvokeNil(f func(error) string) string         { return f(nil) }
func Returned() func(error) string                 { return Classify }
func Variadic(errs ...error) string                { return Classify(errs[0]) }
func InvokeVariadic(f func(...error) string) string { return f(Sentinel) }
