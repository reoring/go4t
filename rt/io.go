// Package rt provides the erased values and primitives used by generated Go code.
package rt

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Unit is the value returned by effects that have no meaningful result.
type Unit struct{}

// IO is a deferred effect, returning either an erased value or an error.
type IO func() (any, error)

// Pure wraps a value in an IO without performing any effects.
func Pure(a any) any {
	return IO(func() (any, error) { return a, nil })
}

// Bind sequences effects, invoking f only if the first effect succeeds.
// f has the erased signature func(any) any and returns an IO.
func Bind(io any, f any) any {
	return IO(func() (any, error) {
		value, err := io.(IO)()
		if err != nil {
			return nil, err
		}
		return f.(func(any) any)(value).(IO)()
	})
}

// Fail constructs an IO that fails with the given string message.
func Fail(msg any) any {
	err := errors.New(msg.(string))
	return IO(func() (any, error) { return nil, err })
}

// RunMain runs a program's main effect and exits with status 1 on failure.
func RunMain(io any) {
	if _, err := io.(IO)(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// Println constructs an IO that prints a string and a newline to stdout.
func Println(s any) any {
	return IO(func() (any, error) {
		if _, err := fmt.Fprintln(os.Stdout, s.(string)); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}

// ShowInt returns the decimal representation of an Int.
func ShowInt(n any) any { return strconv.FormatInt(n.(int64), 10) }
