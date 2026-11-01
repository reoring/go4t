// Package rt provides the erased values and primitives used by generated Go code.
package rt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// ShowBool returns "true" or "false" for a Bool.
func ShowBool(b any) any { return strconv.FormatBool(b.(bool)) }

// ShowString returns a String unchanged.
func ShowString(s any) any { return s }

// Keep a single reader so ReadAll includes input buffered by earlier ReadLine calls.
var stdinReader = bufio.NewReader(os.Stdin)
var programArgs = os.Args[1:]

// ReadLine defers reading one line from stdin. LF and CRLF terminators are removed;
// a final unterminated line is Some, and only an empty EOF is None.
func ReadLine() any {
	return IO(func() (any, error) {
		line, err := stdinReader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		if err == io.EOF && line == "" {
			return NoneOf(), nil
		}
		if strings.HasSuffix(line, "\n") {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
		}
		return SomeOf(line), nil
	})
}

// ReadAll defers reading the remaining stdin, including any buffered bytes.
func ReadAll() any {
	return IO(func() (any, error) {
		text, err := io.ReadAll(stdinReader)
		if err != nil {
			return nil, err
		}
		return string(text), nil
	})
}

// Args defers constructing the program's arguments, excluding the executable.
func Args() any {
	return IO(func() (any, error) {
		return ListFromStrings(programArgs), nil
	})
}

// Exit terminates only when its IO is executed. Runtime stdout is unbuffered.
func Exit(code any) any {
	return IO(func() (any, error) {
		os.Exit(int(code.(int64)))
		return Unit{}, nil
	})
}

// GetEnv distinguishes an absent environment variable from a present empty one.
func GetEnv(name any) any {
	return IO(func() (any, error) {
		value, ok := os.LookupEnv(name.(string))
		if !ok {
			return NoneOf(), nil
		}
		return SomeOf(value), nil
	})
}

// ReadFile defers reading a whole file as a string; a missing file fails the IO.
func ReadFile(path any) any {
	return IO(func() (any, error) {
		text, err := os.ReadFile(path.(string))
		if err != nil {
			return nil, err
		}
		return string(text), nil
	})
}

// WriteFile defers writing content to path, creating parent directories.
func WriteFile(path, content any) any {
	return IO(func() (any, error) {
		p := path.(string)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, []byte(content.(string)), 0o644); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}

// Eprintln constructs an IO that prints a string and a newline to stderr.
func Eprintln(s any) any {
	return IO(func() (any, error) {
		if _, err := fmt.Fprintln(os.Stderr, s.(string)); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}
