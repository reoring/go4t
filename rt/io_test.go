package rt

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestPure(t *testing.T) {
	value := &struct{ name string }{name: "unchanged"}
	got, err := Pure(value).(IO)()
	if got != value || err != nil {
		t.Fatalf("Pure result = (%v, %v), want (%v, nil)", got, err, value)
	}
}

func TestBindIsLazyAndOrdered(t *testing.T) {
	var steps []string
	left := IO(func() (any, error) {
		steps = append(steps, "left")
		return int64(4), nil
	})
	bound := Bind(left, func(value any) any {
		steps = append(steps, "continuation")
		return IO(func() (any, error) {
			steps = append(steps, "right")
			return value.(int64) + 3, nil
		})
	}).(IO)
	if len(steps) != 0 {
		t.Fatalf("Bind ran effects before execution: %v", steps)
	}
	got, err := bound()
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
	left := IO(func() (any, error) { return nil, wantErr })
	got, err := Bind(left, func(any) any {
		called = true
		return Pure(Unit{})
	}).(IO)()
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
		return IO(func() (any, error) { return nil, wantErr })
	}).(IO)()
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
			left, leftErr := tt.left.(IO)()
			right, rightErr := tt.right.(IO)()
			if leftErr != nil || rightErr != nil || left != right {
				t.Fatalf("left = (%v, %v), right = (%v, %v)", left, leftErr, right, rightErr)
			}
		})
	}
}

func TestFail(t *testing.T) {
	got, err := Fail("failure message").(IO)()
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
	got, err := action()
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
	}
	for _, tt := range []struct {
		name   string
		code   int
		stdout string
		stderr string
	}{
		{"success", 0, "ran main\n", ""},
		{"failure", 1, "", "error: main failed\n"},
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
