package rt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPure(t *testing.T) {
	value := &struct{ name string }{name: "unchanged"}
	got, err := Pure(value).(IO)(context.Background())
	if got != value || err != nil {
		t.Fatalf("Pure result = (%v, %v), want (%v, nil)", got, err, value)
	}
}

func TestBindIsLazyAndOrdered(t *testing.T) {
	var steps []string
	left := IO(func(context.Context) (any, error) {
		steps = append(steps, "left")
		return int64(4), nil
	})
	bound := Bind(left, func(value any) any {
		steps = append(steps, "continuation")
		return IO(func(context.Context) (any, error) {
			steps = append(steps, "right")
			return value.(int64) + 3, nil
		})
	}).(IO)
	if len(steps) != 0 {
		t.Fatalf("Bind ran effects before execution: %v", steps)
	}
	got, err := bound(context.Background())
	if got != int64(7) || err != nil {
		t.Fatalf("Bind result = (%v, %v), want (7, nil)", got, err)
	}
	if want := []string{"left", "continuation", "right"}; len(steps) != len(want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	} else {
		for i := range want {
			if steps[i] != want[i] {
				t.Fatalf("steps = %v, want %v", steps, want)
			}
		}
	}
}

func TestBindShortCircuits(t *testing.T) {
	wantErr := &Failure{Kind: FailureKind, Message: "left failed"}
	called := false
	left := IO(func(context.Context) (any, error) { return nil, wantErr })
	got, err := Bind(left, func(any) any {
		called = true
		return Pure(Unit{})
	}).(IO)(context.Background())
	if called {
		t.Fatal("Bind invoked its continuation after failure")
	}
	if got != nil || err != wantErr {
		t.Fatalf("Bind result = (%v, %v), want (nil, %v)", got, err, wantErr)
	}
}

func TestBindPropagatesRightError(t *testing.T) {
	wantErr := &Failure{Kind: FailureKind, Message: "right failed"}
	got, err := Bind(Pure(int64(1)), func(any) any {
		return IO(func(context.Context) (any, error) { return nil, wantErr })
	}).(IO)(context.Background())
	if got != nil || err != wantErr {
		t.Fatalf("Bind result = (%v, %v), want (nil, %v)", got, err, wantErr)
	}
}

func TestIOMonadLaws(t *testing.T) {
	f := func(a any) any { return Pure(a.(int64) + 2) }
	g := func(a any) any { return Pure(a.(int64) * 3) }
	a := int64(5)
	m := Pure(a)
	for _, tt := range []struct {
		name  string
		left  any
		right any
	}{
		{"left identity", Bind(Pure(a), f), f(a)},
		{"right identity", Bind(m, Pure), m},
		{"associativity", Bind(Bind(m, f), g), Bind(m, func(x any) any { return Bind(f(x), g) })},
	} {
		t.Run(tt.name, func(t *testing.T) {
			left, leftErr := tt.left.(IO)(context.Background())
			right, rightErr := tt.right.(IO)(context.Background())
			if leftErr != nil || rightErr != nil || left != right {
				t.Fatalf("left = (%v, %v), right = (%v, %v)", left, leftErr, right, rightErr)
			}
		})
	}
}

func TestRunStepBoundaries(t *testing.T) {
	for _, stage := range []string{"success", "before", "after", "failure"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if stage == "before" {
				cancel()
			}
			calls := 0
			action := IO(func(context.Context) (any, error) {
				calls++
				if stage == "after" || stage == "failure" {
					cancel()
				}
				if stage == "failure" {
					return int64(9), os.ErrNotExist
				}
				return int64(9), nil
			})
			value, err := Run(ctx, action)
			if stage == "success" {
				if value != int64(9) || err != nil || calls != 1 {
					t.Fatalf("Run = (%v, %v), calls = %d", value, err, calls)
				}
				return
			}
			wantKind, wantCalls := Cancelled, 1
			if stage == "failure" {
				// The action error wins over cancellation, just as in Bind.
				wantKind = NotFound
			}
			if stage == "before" {
				wantCalls = 0
			}
			var failure *Failure
			if value != nil || !errors.As(err, &failure) || failure.Kind != wantKind || calls != wantCalls {
				t.Fatalf("Run = (%v, %v), calls = %d; want kind %v, calls %d", value, err, calls, wantKind, wantCalls)
			}
		})
	}
}

func TestRunPreservesClassifiedFailure(t *testing.T) {
	want := &Failure{Kind: FailureKind, Message: "step failed"}
	_, err := Run(context.Background(), IO(func(context.Context) (any, error) { return nil, want }))
	if err != want {
		t.Fatalf("Run error = %v, want original failure %v", err, want)
	}
}

func TestFail(t *testing.T) {
	got, err := Fail("failure message").(IO)(context.Background())
	if got != nil || !reflect.DeepEqual(err, &Failure{Kind: FailureKind, Message: "failure message"}) {
		t.Fatalf("Fail result = (%v, %v), want (nil, failure message)", got, err)
	}
}

func TestPrintln(t *testing.T) {
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = stdout
	t.Cleanup(func() {
		os.Stdout = previous
		stdout.Close()
	})
	action := Println("hello").(IO)
	info, err := stdout.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatal("Println wrote before its IO was run")
	}
	got, err := action(context.Background())
	if got != (Unit{}) || err != nil {
		t.Fatalf("Println result = (%v, %v), want (Unit{}, nil)", got, err)
	}
	if _, err := stdout.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	text, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != "hello\n" {
		t.Fatalf("stdout = %q, want %q", text, "hello\n")
	}
}

func TestShowInt(t *testing.T) {
	for _, tt := range []struct {
		value int64
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{-42, "-42"},
		{-1 << 63, "-9223372036854775808"},
	} {
		if got := ShowInt(tt.value); got != tt.want {
			t.Errorf("ShowInt(%d) = %v, want %q", tt.value, got, tt.want)
		}
	}
}

func TestShowBool(t *testing.T) {
	for _, tt := range []struct {
		value bool
		want  string
	}{
		{true, "true"},
		{false, "false"},
	} {
		if got := ShowBool(tt.value); got != tt.want {
			t.Errorf("ShowBool(%t) = %v, want %q", tt.value, got, tt.want)
		}
	}
}

func TestShowString(t *testing.T) {
	for _, value := range []string{"", "hello", "こんにちは", "line\n\"quoted\""} {
		if got := ShowString(value); got != value {
			t.Errorf("ShowString(%q) = %v, want %q", value, got, value)
		}
	}
}

func TestUnitValue(t *testing.T) {
	if got := UnitValue(); got != (Unit{}) {
		t.Fatalf("UnitValue() = %v, want Unit{}", got)
	}
}

func TestRunMain(t *testing.T) {
	switch os.Getenv("GO4T_RT_RUN_MAIN_TEST") {
	case "success":
		RunMain(Println("ran main"))
		os.Exit(0)
	case "failure":
		RunMain(Fail("main failed"))
		os.Exit(0)
	case "extern error":
		RunMain(IO(func(context.Context) (any, error) { return nil, errors.New("extern failed") }))
		os.Exit(0)
	case "panic":
		RunMain(IO(func(context.Context) (any, error) { panic("main panicked") }))
		os.Exit(0)
	case "error panic":
		RunMain(IO(func(context.Context) (any, error) { panic(errors.New("panic error")) }))
		os.Exit(0)
	}
	for _, tt := range []struct {
		name   string
		code   int
		stdout string
		stderr string
	}{
		{"success", 0, "ran main\n", ""},
		{"failure", 1, "", "error: main failed\n"},
		{"extern error", 1, "", "error: extern failed\n"},
		{"panic", 2, "", "error: panic: main panicked\n"},
		{"error panic", 2, "", "error: panic: panic error\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestRunMain$")
			command.Env = append(os.Environ(), "GO4T_RT_RUN_MAIN_TEST="+tt.name)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			var exitErr *exec.ExitError
			if err != nil && !errors.As(err, &exitErr) {
				t.Fatal(err)
			}
			if code := command.ProcessState.ExitCode(); code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if stdout.String() != tt.stdout || stderr.String() != tt.stderr {
				t.Errorf("stdout/stderr = (%q, %q), want (%q, %q)", stdout.String(), stderr.String(), tt.stdout, tt.stderr)
			}
		})
	}
}

func setTestInput(t *testing.T, reader io.Reader) {
	t.Helper()
	previous := stdinReader
	stdinReader = bufio.NewReader(reader)
	t.Cleanup(func() { stdinReader = previous })
}

func TestReadLine(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		name  string
		input string
		want  []any
	}{
		{"empty", "", []any{testNone{}, testNone{}}},
		{"terminated", "one\ntwo\r\n\n", []any{testSome{"one"}, testSome{"two"}, testSome{""}, testNone{}}},
		{"unterminated", "one\n日本🙂", []any{testSome{"one"}, testSome{"日本🙂"}, testNone{}}},
		{"bare CR", "one\r", []any{testSome{"one\r"}, testNone{}}},
		{"long line", strings.Repeat("x", 100000) + "\n", []any{testSome{strings.Repeat("x", 100000)}, testNone{}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setTestInput(t, strings.NewReader(tt.input))
			action := ReadLine().(IO)
			for i, want := range tt.want {
				got, err := action(context.Background())
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("read %d = (%#v, %v), want (%#v, nil)", i, got, err, want)
				}
			}
		})
	}
}

func TestReadLineAndReadAllShareInput(t *testing.T) {
	installTestShapes(t)
	setTestInput(t, strings.NewReader("first\r\nsecond\nthird"))
	readAll, readLine := ReadAll().(IO), ReadLine().(IO)
	if got, err := readLine(context.Background()); got != (testSome{"first"}) || err != nil {
		t.Fatalf("ReadLine = (%#v, %v), want Some(first)", got, err)
	}
	if got, err := readAll(context.Background()); got != "second\nthird" || err != nil {
		t.Fatalf("ReadAll = (%#v, %v), want remaining buffered input", got, err)
	}
	if got, err := readLine(context.Background()); got != (testNone{}) || err != nil {
		t.Fatalf("ReadLine after ReadAll = (%#v, %v), want None", got, err)
	}
	if got, err := readAll(context.Background()); got != "" || err != nil {
		t.Fatalf("ReadAll at EOF = (%#v, %v), want empty string", got, err)
	}
}

func TestFoldLines(t *testing.T) {
	long := strings.Repeat("x", 100000)
	for _, tt := range []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"terminated", "one\ntwo\n", []string{"one", "two"}},
		{"unterminated", "one\n日本🙂", []string{"one", "日本🙂"}},
		{"CRLF and blank lines", "one\r\n\r\ntwo\r\n", []string{"one", "", "two"}},
		{"bare CR", "one\r", []string{"one\r"}},
		{"long terminated line", long + "\nnext\n", []string{long, "next"}},
		{"long unterminated line", long, []string{long}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lines.txt")
			if err := os.WriteFile(path, []byte(tt.input), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), struct{}{}, "fold context")
			var lines []string
			step := func(acc, line any) any {
				if acc != int64(7+len(lines)) {
					t.Fatalf("step accumulator = %v, want %d", acc, 7+len(lines))
				}
				return IO(func(stepCtx context.Context) (any, error) {
					if stepCtx != ctx {
						t.Fatal("step did not receive the fold's context")
					}
					lines = append(lines, line.(string))
					return acc.(int64) + 1, nil
				})
			}
			got, err := FoldLines(path, int64(7), step).(IO)(ctx)
			if want := int64(7 + len(tt.want)); err != nil || got != want {
				t.Fatalf("FoldLines = (%v, %v), want (%v, nil)", got, err, want)
			}
			if !slices.Equal(lines, tt.want) {
				t.Fatalf("lines = %q, want %q", lines, tt.want)
			}
		})
	}
}

func TestFoldLinesIsLazyAndRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lines.txt")
	calls := 0
	action := FoldLines(path, "init", func(acc, line any) any {
		calls++
		return Pure(acc.(string) + ":" + line.(string))
	}).(IO)
	if calls != 0 {
		t.Fatal("FoldLines invoked step before execution")
	}
	for _, text := range []string{"one\ntwo\n", "three\n"} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		want := "init:" + strings.ReplaceAll(strings.TrimSuffix(text, "\n"), "\n", ":")
		if got, err := action(context.Background()); err != nil || got != want {
			t.Fatalf("FoldLines = (%v, %v), want (%q, nil)", got, err, want)
		}
	}
	if calls != 3 {
		t.Fatalf("step calls = %d, want 3", calls)
	}
}

func TestFoldLinesOpenErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		cancelled bool
		want      error
		kind      Kind
	}{
		{"missing", false, os.ErrNotExist, NotFound},
		{"cancelled before open", true, context.Canceled, Cancelled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			path := filepath.Join(t.TempDir(), "missing.txt")
			got, err := FoldLines(path, int64(0), func(any, any) any {
				t.Fatal("step called after open failure or cancellation")
				return Pure(int64(0))
			}).(IO)(ctx)
			var failure *Failure
			if got != nil || !errors.Is(err, tt.want) || !errors.As(err, &failure) || failure.Kind != tt.kind {
				t.Fatalf("FoldLines = (%v, %#v), want (nil, %s)", got, err, tt.kind)
			}
		})
	}
}

func TestFoldLinesStopsAndClosesFile(t *testing.T) {
	typedFailure := &Failure{Kind: FailureKind, Message: "step failed"}
	for _, tt := range []struct {
		name     string
		fail     error
		cancelAt int
		calls    int
		kind     Kind
	}{
		{"completion", nil, 0, 3, ""},
		{"typed step failure", typedFailure, 0, 1, FailureKind},
		{"permission step failure", os.ErrPermission, 0, 1, PermissionDenied},
		{"cancelled between lines", nil, 1, 1, Cancelled},
		{"cancelled after final unterminated line", nil, 3, 3, Cancelled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lines.txt")
			if err := os.WriteFile(path, []byte("one\ntwo\nthree"), 0o600); err != nil {
				t.Fatal(err)
			}
			// Count only this file, so unrelated runtime descriptors cannot make
			// the close assertion flaky. Closure checks use Linux procfs.
			countOpen := func() int {
				t.Helper()
				entries, err := os.ReadDir("/proc/self/fd")
				if errors.Is(err, os.ErrNotExist) {
					return -1
				}
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, entry := range entries {
					target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
					if errors.Is(err, os.ErrNotExist) {
						continue
					}
					if err != nil {
						t.Fatal(err)
					}
					if target == path {
						count++
					}
				}
				return count
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			got, err := FoldLines(path, int64(0), func(acc, line any) any {
				return IO(func(context.Context) (any, error) {
					calls++
					if count := countOpen(); count >= 0 && count != 1 {
						t.Fatalf("open file descriptors during step = %d, want 1", count)
					}
					if calls == tt.cancelAt {
						cancel()
					}
					return acc.(int64) + 1, tt.fail
				})
			}).(IO)(ctx)
			if calls != tt.calls {
				t.Fatalf("step calls = %d, want %d", calls, tt.calls)
			}
			if tt.kind == "" {
				if err != nil || got != int64(3) {
					t.Fatalf("FoldLines = (%v, %v), want (3, nil)", got, err)
				}
			} else {
				var failure *Failure
				if got != nil || !errors.As(err, &failure) || failure.Kind != tt.kind {
					t.Fatalf("FoldLines = (%v, %#v), want (nil, %s)", got, err, tt.kind)
				}
				if tt.fail == typedFailure && err != typedFailure {
					t.Fatalf("typed step error was replaced: %#v", err)
				}
				if tt.cancelAt > 0 && (!errors.Is(err, context.Canceled) || failure.Message != context.Canceled.Error()) {
					t.Fatalf("cancellation = %#v, want canonical Cancelled", err)
				}
			}
			if count := countOpen(); count < 0 {
				t.Log("file descriptor closure check unavailable without /proc/self/fd")
			} else if count != 0 {
				t.Fatalf("open file descriptors after FoldLines = %d, want 0", count)
			}
		})
	}
}

func TestReadAll(t *testing.T) {
	for _, input := range []string{"", "one\n日本\r\n🙂", strings.Repeat("long", 30000)} {
		setTestInput(t, strings.NewReader(input))
		if got, err := ReadAll().(IO)(context.Background()); got != input || err != nil {
			t.Fatalf("ReadAll = (%#v, %v), want (%q, nil)", got, err, input)
		}
	}
}

type failingInput struct{ err error }

func (r failingInput) Read([]byte) (int, error) { return 0, r.err }

func TestInputErrors(t *testing.T) {
	wantErr := errors.New("input failed")
	for _, tt := range []struct {
		name   string
		action func() any
	}{
		{"ReadLine", ReadLine},
		{"ReadAll", ReadAll},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setTestInput(t, failingInput{wantErr})
			if got, err := tt.action().(IO)(context.Background()); got != nil || !reflect.DeepEqual(err, &Failure{Kind: FailureKind, Message: wantErr.Error(), Cause: wantErr}) {
				t.Fatalf("%s = (%#v, %v), want (nil, %v)", tt.name, got, err, wantErr)
			}
		})
	}
}

func TestArgs(t *testing.T) {
	installTestShapes(t)
	previous := programArgs
	t.Cleanup(func() { programArgs = previous })
	programArgs = []string{"before"}
	action := Args().(IO)
	for _, want := range [][]string{nil, {"first", "--flag", "two words", "", "日本"}} {
		programArgs = want
		got, err := action(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if values := ListToStrings(got); !slices.Equal(values, want) {
			t.Errorf("Args = %q, want %q", values, want)
		}
	}
}

func TestGetEnv(t *testing.T) {
	installTestShapes(t)
	const name = "GO4T_RT_GET_ENV_TEST"
	t.Setenv(name, "before")
	action := GetEnv(name).(IO)
	t.Setenv(name, "日本 text")
	if got, err := action(context.Background()); got != (testSome{"日本 text"}) || err != nil {
		t.Fatalf("GetEnv = (%#v, %v), want Some of current value", got, err)
	}
	t.Setenv(name, "")
	if got, err := action(context.Background()); got != (testSome{""}) || err != nil {
		t.Fatalf("GetEnv empty = (%#v, %v), want Some of empty string", got, err)
	}
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	if got, err := action(context.Background()); got != (testNone{}) || err != nil {
		t.Fatalf("GetEnv missing = (%#v, %v), want None", got, err)
	}
}

func TestExit(t *testing.T) {
	if code := os.Getenv("GO4T_RT_EXIT_TEST"); code != "" {
		n := int64(0)
		if code == "7" {
			n = 7
		}
		action := Exit(n).(IO)
		RunMain(Println("before exit"))
		RunMain(action)
		RunMain(Println("after exit"))
		os.Exit(99)
	}
	for _, tt := range []struct {
		value string
		code  int
	}{
		{"0", 0},
		{"7", 7},
	} {
		t.Run(tt.value, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestExit$")
			command.Env = append(os.Environ(), "GO4T_RT_EXIT_TEST="+tt.value)
			output, err := command.CombinedOutput()
			var exitErr *exec.ExitError
			if err != nil && !errors.As(err, &exitErr) {
				t.Fatal(err)
			}
			if code := command.ProcessState.ExitCode(); code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if string(output) != "before exit\n" {
				t.Errorf("output = %q, want only the effect before Exit", output)
			}
		})
	}
}
