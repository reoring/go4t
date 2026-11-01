package rt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
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
	wantErr := errors.New("left failed")
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
	wantErr := errors.New("right failed")
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

func TestFail(t *testing.T) {
	got, err := Fail("failure message").(IO)(context.Background())
	if got != nil || err == nil || err.Error() != "failure message" {
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
	switch os.Getenv("GOFX_RT_RUN_MAIN_TEST") {
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
			command.Env = append(os.Environ(), "GOFX_RT_RUN_MAIN_TEST="+tt.name)
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
			if got, err := tt.action().(IO)(context.Background()); got != nil || err != wantErr {
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
	const name = "GOFX_RT_GET_ENV_TEST"
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
	if code := os.Getenv("GOFX_RT_EXIT_TEST"); code != "" {
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
			command.Env = append(os.Environ(), "GOFX_RT_EXIT_TEST="+tt.value)
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
