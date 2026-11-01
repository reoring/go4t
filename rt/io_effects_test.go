package rt

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTryIO(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		name   string
		action any
		want   any
	}{
		{"success", Pure(int64(42)), testOk{int64(42)}},
		{"nil success", Pure(nil), testOk{nil}},
		{"ioFail", Fail("failed"), testErr{"failed"}},
		{"extern error", IO(func() (any, error) { return int64(9), errors.New("extern failed") }), testErr{"extern failed"}},
		{"string panic", IO(func() (any, error) { panic("boom") }), testErr{"boom"}},
		{"error panic", IO(func() (any, error) { panic(errors.New("panic error")) }), testErr{"panic error"}},
		{"nil panic", IO(func() (any, error) { panic(nil) }), testErr{"panic called with nil argument"}},
		{"bind continuation panic", Bind(Pure(Unit{}), func(any) any { panic("continuation") }), testErr{"continuation"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryIO(tt.action).(IO)()
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("TryIO = (%#v, %v), want (%#v, nil)", got, err, tt.want)
			}
		})
	}
}

func TestTryIOIsLazyAndAllowsContinuation(t *testing.T) {
	installTestShapes(t)
	calls := 0
	action := TryIO(IO(func() (any, error) {
		calls++
		panic("caught")
	}))
	if calls != 0 {
		t.Fatal("TryIO executed eagerly")
	}
	continued := Bind(action, func(value any) any {
		if value != (testErr{"caught"}) {
			t.Fatalf("continuation received %#v", value)
		}
		return Pure("continued")
	}).(IO)
	for range 2 {
		if got, err := continued(); got != "continued" || err != nil {
			t.Fatalf("continuation = (%v, %v)", got, err)
		}
	}
	if calls != 2 {
		t.Fatalf("action ran %d times, want 2", calls)
	}
}

func TestIOFiles(t *testing.T) {
	installTestShapes(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config")
	write, read := WriteFile(path, "first\n"), ReadFile(path)
	exists := FileExists(path).(IO)
	if got, err := exists(); got != false || err != nil {
		t.Fatalf("deferred write created file: (%v, %v)", got, err)
	}
	assertIOUnit(t, write)
	if got, err := exists(); got != true || err != nil {
		t.Fatalf("FileExists = (%v, %v)", got, err)
	}
	assertIOUnit(t, AppendFile(path, "second 日本\n"))
	if got, err := read.(IO)(); got != "first\nsecond 日本\n" || err != nil {
		t.Fatalf("ReadFile = (%v, %v)", got, err)
	}
	assertIOUnit(t, WriteFile(path, "replaced"))
	if got, err := read.(IO)(); got != "replaced" || err != nil {
		t.Fatalf("WriteFile did not truncate: (%v, %v)", got, err)
	}
	assertIOUnit(t, AppendFile(filepath.Join(dir, "new", "log"), "created"))
	for _, name := range []string{"z", "a", "m"} {
		assertIOUnit(t, WriteFile(filepath.Join(dir, "sorted", name), ""))
	}
	got, err := ListDir(filepath.Join(dir, "sorted")).(IO)()
	if err != nil || !slices.Equal(ListToStrings(got), []string{"a", "m", "z"}) {
		t.Fatalf("ListDir = (%v, %v)", got, err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = ListDir(empty).(IO)()
	if err != nil || len(ListToStrings(got)) != 0 {
		t.Fatalf("empty ListDir = (%v, %v)", got, err)
	}
	remove := RemoveFile(path)
	if got, err := exists(); got != true || err != nil {
		t.Fatal("RemoveFile executed eagerly")
	}
	assertIOUnit(t, remove)
	if got, err := exists(); got != false || err != nil {
		t.Fatalf("removed FileExists = (%v, %v)", got, err)
	}
	for _, action := range []any{read, remove, ListDir(path), WriteFile(dir, "bad"), AppendFile(dir, "bad"), FileExists("bad\x00path")} {
		if got, err := action.(IO)(); got != nil || err == nil {
			t.Fatalf("expected filesystem error, got (%v, %v)", got, err)
		}
	}
	captured, err := TryIO(read).(IO)()
	failure, ok := captured.(testErr)
	if err != nil || !ok || !strings.Contains(failure.value.(string), path) {
		t.Fatalf("TryIO(ReadFile) = (%#v, %v)", captured, err)
	}
}

func assertIOUnit(t *testing.T, action any) {
	t.Helper()
	if got, err := action.(IO)(); got != (Unit{}) || err != nil {
		t.Fatalf("IO = (%#v, %v), want (Unit{}, nil)", got, err)
	}
}

func TestIOPrintStreams(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stream **os.File
		print  func(any) any
		want   string
	}{
		{"Print", &os.Stdout, Print, "hello"},
		{"Eprintln", &os.Stderr, Eprintln, "hello\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "output")
			if err != nil {
				t.Fatal(err)
			}
			old := *tt.stream
			*tt.stream = file
			t.Cleanup(func() { *tt.stream = old; file.Close() })
			action := tt.print("hello")
			info, err := file.Stat()
			if err != nil || info.Size() != 0 {
				t.Fatal("printing executed eagerly")
			}
			assertIOUnit(t, action)
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			text, err := io.ReadAll(file)
			if err != nil || string(text) != tt.want {
				t.Fatalf("output = (%q, %v), want %q", text, err, tt.want)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if got, err := action.(IO)(); got != nil || err == nil {
				t.Fatalf("closed stream = (%v, %v)", got, err)
			}
		})
	}
}

func TestIOTime(t *testing.T) {
	for _, tt := range []struct {
		millis       int64
		layout, want string
	}{
		{0, time.RFC3339, "1970-01-01T00:00:00Z"},
		{-1, "2006-01-02 15:04:05.000 MST", "1969-12-31 23:59:59.999 UTC"},
		{1234, "", ""},
	} {
		if got := FormatTime(tt.millis, tt.layout); got != tt.want {
			t.Errorf("FormatTime = %v, want %q", got, tt.want)
		}
	}
	action := NowMillis().(IO)
	before := time.Now().UnixMilli()
	got, err := action()
	after := time.Now().UnixMilli()
	if err != nil || got.(int64) < before || got.(int64) > after {
		t.Fatalf("NowMillis = (%v, %v), outside [%d, %d]", got, err, before, after)
	}
	start, err := MonotonicMillis().(IO)()
	if err != nil || start.(int64) < 0 {
		t.Fatalf("MonotonicMillis = (%v, %v)", start, err)
	}
	beforeSleep := time.Now()
	assertIOUnit(t, Sleep(int64(2)))
	if time.Since(beforeSleep) < 2*time.Millisecond {
		t.Fatal("Sleep returned too soon")
	}
	end, err := MonotonicMillis().(IO)()
	if err != nil || end.(int64) < start.(int64) {
		t.Fatalf("monotonic clock went backwards: (%v, %v)", end, err)
	}
	for _, n := range []int64{0, -1, -1 << 63} {
		assertIOUnit(t, Sleep(n))
	}
	if got, err := Sleep(int64(1<<63 - 1)).(IO)(); got != nil || err == nil || err.Error() != "sleep: duration out of range" {
		t.Fatalf("overflow sleep = (%v, %v)", got, err)
	}
}

func TestRandomIntBoundaries(t *testing.T) {
	for _, bounds := range [][2]int64{{0, 0}, {-9, -9}, {-8, -2}, {-1, 1}, {1<<63 - 1, 1<<63 - 1}, {-1 << 63, -1 << 63}, {-1 << 63, 1<<63 - 1}, {0, 1<<63 - 1}} {
		action := RandomInt(bounds[0], bounds[1]).(IO)
		for range 100 {
			got, err := action()
			if err != nil || got.(int64) < bounds[0] || got.(int64) > bounds[1] {
				t.Fatalf("RandomInt%v = (%v, %v)", bounds, got, err)
			}
		}
	}
	if got, err := RandomInt(int64(2), int64(1)).(IO)(); got != nil || err == nil || err.Error() != "randomInt: lo must be <= hi" {
		t.Fatalf("reversed bounds = (%v, %v)", got, err)
	}
}
