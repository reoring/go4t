// Package rt provides the erased values and primitives used by generated Go code.
package rt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Unit is the value returned by effects that have no meaningful result.
type Unit struct{}

// IO is a deferred effect executed with a cancellation/deadline environment.
type IO func(ctx context.Context) (any, error)

// Pure wraps a value in an IO without performing any effects.
func Pure(a any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return a, nil
	})
}

// Bind sequences effects, invoking f only if the first effect succeeds.
// f has the erased signature func(any) any and returns an IO.
func Bind(io any, f any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		value, err := io.(IO)(ctx)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return f.(func(any) any)(value).(IO)(ctx)
	})
}

// Fail constructs an IO that fails with the given string message.
func Fail(msg any) any {
	err := errors.New(msg.(string))
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, err
	})
}

// TryIO captures errors and panics from running an action as a generated Result.
// Like Bind, it is lazy; panics while constructing its argument are outside it.
func TryIO(io any) any {
	return IO(func(ctx context.Context) (value any, err error) {
		defer func() {
			if p := recover(); p != nil {
				value, err = ErrOf(fmt.Sprint(p)), nil
			}
		}()
		if failure := ctx.Err(); failure != nil {
			return ErrOf(failure.Error()), nil
		}
		result, failure := io.(IO)(ctx)
		if failure != nil {
			return ErrOf(failure.Error()), nil
		}
		return OkOf(result), nil
	})
}

// RunMain exits with status 1 for failed IO and status 2 for an uncaught panic.
func RunMain(io any) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		if p := recover(); p != nil {
			fmt.Fprintln(os.Stderr, "error: panic:", p)
			os.Exit(2)
		}
	}()
	_, err := io.(IO)(ctx)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "error: cancelled")
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

// Println constructs an IO that prints a string and a newline to stdout.
func Println(s any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

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

// A cancelled stdin read cannot interrupt an arbitrary Go Reader. Keep at most
// one read in flight and serialize access to the shared buffer; the caller can
// leave immediately while that read finishes when input or EOF arrives.
var stdinGate = make(chan struct{}, 1)

func readStdin(ctx context.Context, all bool) (string, error) {
	select {
	case stdinGate <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-stdinGate
		return "", err
	}
	reader := stdinReader
	type result struct {
		text string
		err  error
	}
	completed := make(chan result, 1)
	go func() {
		defer func() { <-stdinGate }()
		var text string
		var err error
		if all {
			var data []byte
			data, err = io.ReadAll(reader)
			text = string(data)
		} else {
			text, err = reader.ReadString('\n')
		}
		completed <- result{text, err}
	}()
	select {
	case result := <-completed:
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return result.text, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ReadLine defers reading one line from stdin. LF and CRLF terminators are removed;
// a final unterminated line is Some, and only an empty EOF is None.
func ReadLine() any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line, err := readStdin(ctx, false)
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
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		text, err := readStdin(ctx, true)
		if err != nil {
			return nil, err
		}
		return text, nil
	})
}

// Args defers constructing the program's arguments, excluding the executable.
func Args() any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		return ListFromStrings(programArgs), nil
	})
}

// Exit terminates only when its IO is executed. Runtime stdout is unbuffered.
func Exit(code any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		os.Exit(int(code.(int64)))
		return Unit{}, nil
	})
}

// GetEnv distinguishes an absent environment variable from a present empty one.
func GetEnv(name any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		value, ok := os.LookupEnv(name.(string))
		if !ok {
			return NoneOf(), nil
		}
		return SomeOf(value), nil
	})
}

// ReadFile defers reading a whole file as a string; a missing file fails the IO.
func ReadFile(path any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		text, err := os.ReadFile(path.(string))
		if err != nil {
			return nil, err
		}
		return string(text), nil
	})
}

// WriteFile defers writing content to path, creating parent directories.
func WriteFile(path, content any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

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
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if _, err := fmt.Fprintln(os.Stderr, s.(string)); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}

// AppendFile appends content, creating the file and its parents if needed.
func AppendFile(path, content any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		p := path.(string)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		_, writeErr := io.WriteString(f, content.(string))
		closeErr := f.Close()
		if writeErr != nil {
			return nil, writeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return Unit{}, nil
	})
}

// FileExists returns false only for a missing path; other stat errors fail the IO.
func FileExists(path any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		_, err := os.Stat(path.(string))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return nil, err
		}
		return true, nil
	})
}

// ListDir returns sorted entry names, not full paths.
func ListDir(path any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		entries, err := os.ReadDir(path.(string))
		if err != nil {
			return nil, err
		}
		if Shapes.Cons == nil || Shapes.Nil == nil {
			panic("rt.Shapes is not initialized: Cons and Nil are required")
		}
		list := Shapes.Nil()
		for i := len(entries) - 1; i >= 0; i-- {
			list = Shapes.Cons(entries[i].Name(), list)
		}
		return list, nil
	})
}

// RemoveFile removes a file or empty directory; a missing path fails the IO.
func RemoveFile(path any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if err := os.Remove(path.(string)); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}

// Print writes a string to stdout without adding a newline.
func Print(s any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if _, err := fmt.Fprint(os.Stdout, s.(string)); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}

// NowMillis reads the Unix wall clock in milliseconds when its IO runs.
func NowMillis() any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return time.Now().UnixMilli(), nil
	})
}

var monotonicOrigin = time.Now()

// MonotonicMillis returns elapsed milliseconds since runtime initialization.
func MonotonicMillis() any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return time.Since(monotonicOrigin).Milliseconds(), nil
	})
}

// FormatTime formats Unix milliseconds in UTC using a Go time layout.
func FormatTime(millis, layout any) any {
	return time.UnixMilli(millis.(int64)).UTC().Format(layout.(string))
}

// RandomInt samples the inclusive interval uniformly, including the full Int range.
// It is not a cryptographic random source.
func RandomInt(lo, hi any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		lower, upper := lo.(int64), hi.(int64)
		if lower > upper {
			return nil, errors.New("randomInt: lo must be <= hi")
		}
		width := uint64(upper) - uint64(lower) + 1
		if width == 0 {
			return int64(rand.Uint64()), nil
		}
		return int64(uint64(lower) + rand.Uint64N(width)), nil
	})
}

// RandomFloat samples [0, 1) uniformly when its IO runs.
// Like RandomInt, it is not a cryptographic random source.
func RandomFloat() any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return rand.Float64(), nil
	})
}

// Sleep waits for positive milliseconds; nonpositive durations return immediately.
func Sleep(millis any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		n := millis.(int64)
		if n > int64((1<<63-1)/time.Millisecond) {
			return nil, errors.New("sleep: duration out of range")
		}
		if n > 0 {
			timer := time.NewTimer(time.Duration(n) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return Unit{}, nil
	})
}
