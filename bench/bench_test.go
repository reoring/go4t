package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// GOFX_BENCH_ROOT selects the compiler/runtime checkout. A copied harness can
// benchmark an unchanged checkout without writing generated files into it.
// GOFX_BENCH_RECORD skips budget enforcement, never workload/output validation.
func root(tb testing.TB) string {
	tb.Helper()
	path := os.Getenv("GOFX_BENCH_ROOT")
	if path == "" {
		path = ".."
	}
	path, err := filepath.Abs(path)
	if err != nil {
		tb.Fatal(err)
	}
	return path
}

func budgets(tb testing.TB) map[string]int64 {
	tb.Helper()
	data, err := os.ReadFile("budget.json")
	if err != nil {
		tb.Fatal(err)
	}
	var result map[string]int64
	if err := json.Unmarshal(data, &result); err != nil {
		tb.Fatal(err)
	}
	return result
}

var workloads = []struct{ name, output string }{
	{"sort", "1000000\n"},
	{"map", "5000050000\n"},
	{"fold", "49999995000000\n"},
	{"wc", "lines: 1000000, words: 2000000\n1000000 blue\n1000000 redxx\n"},
}

func TestBudgets(t *testing.T) {
	limits := budgets(t)
	if len(limits) != len(workloads) {
		t.Fatalf("budget count = %d, want %d", len(limits), len(workloads))
	}
	for _, w := range workloads {
		if limits[w.name] <= 0 {
			t.Errorf("missing positive nanosecond budget for %s", w.name)
		}
		if _, err := os.Stat(w.name + ".gfx"); err != nil {
			t.Error(err)
		}
	}
}

func BenchmarkPrograms(b *testing.B) {
	checkout := root(b)
	temp := b.TempDir()
	compiler := filepath.Join(temp, "gofx")
	command(b, checkout, "go", "build", "-o", compiler, "./cmd/gofx")
	limits := map[string]int64{}
	if os.Getenv("GOFX_BENCH_RECORD") == "" {
		limits = budgets(b)
	}
	for _, w := range workloads {
		b.Run(w.name, func(b *testing.B) {
			b.StopTimer()
			source, err := filepath.Abs(w.name + ".gfx")
			if err != nil {
				b.Fatal(err)
			}
			dir := filepath.Join(temp, w.name)
			command(b, checkout, compiler, "build", "-o", dir, source)
			binary := filepath.Join(dir, "program")
			command(b, checkout, "go", "build", "-o", binary, filepath.Join(dir, "main.go"))
			var input []byte
			if w.name == "wc" {
				input = bytes.Repeat([]byte("redxx blue\n"), 1000000)
			} // 10,000,000 bytes
			var elapsed time.Duration
			for range b.N {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				cmd := exec.CommandContext(ctx, binary)
				cmd.Stdin = bytes.NewReader(input)
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				b.StartTimer()
				started := time.Now()
				err := cmd.Run()
				duration := time.Since(started)
				b.StopTimer()
				elapsed += duration
				expired := ctx.Err()
				cancel()
				if expired != nil {
					b.Fatalf("%s exceeded 180 s: %v", w.name, expired)
				}
				if err != nil {
					b.Fatalf("%s: %v\n%s", w.name, err, stderr.String())
				}
				if stdout.String() != w.output {
					b.Fatalf("%s output = %q, want %q", w.name, stdout.String(), w.output)
				}
			}
			average := elapsed.Nanoseconds() / int64(b.N)
			b.ReportMetric(float64(average), "ns/op")
			if os.Getenv("GOFX_BENCH_RECORD") == "" && average > limits[w.name] {
				b.Fatalf("%s exceeded budget: %d ns/op > %d ns/op", w.name, average, limits[w.name])
			}
		})
	}
}

func command(tb testing.TB, dir, name string, args ...string) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		tb.Fatal(fmt.Sprintf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output))
	}
}
