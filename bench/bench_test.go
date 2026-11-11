package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// GO4T_BENCH_ROOT selects the compiler/runtime checkout. A copied harness can
// benchmark an unchanged checkout without writing generated files into it.
// GO4T_BENCH_RECORD skips budget enforcement, never workload/output validation.
func root(tb testing.TB) string {
	tb.Helper()
	path := os.Getenv("GO4T_BENCH_ROOT")
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

var perfWorkloads = []struct{ name, output string }{
	{"sortints", "1072161167218\n"},
	{"nbody", "-0.001046538\n"},
	{"wordfreq", "word01597 571\nword02008 566\nword00951 565\nword03004 565\nword02452 558\nword02900 558\nword03013 558\nword02967 557\nword01772 556\nword02809 556\n"},
	{"mapheavy", "1000000\n"},
	{"mapheavy-mut", "1000000\n"},
	{"treefold", "2097151\n"},
}

func TestBudgets(t *testing.T) {
	limits := budgets(t)
	want := len(workloads) + len(perfWorkloads) + 2*len(compileMetrics)
	if len(limits) != want {
		t.Fatalf("budget count = %d, want %d", len(limits), want)
	}
	for _, w := range workloads {
		if limits[w.name] <= 0 {
			t.Errorf("missing positive nanosecond budget for %s", w.name)
		}
		if _, err := os.Stat(w.name + ".gt"); err != nil {
			t.Error(err)
		}
	}
	for _, w := range perfWorkloads {
		if limits["perf-"+w.name] <= 0 {
			t.Errorf("missing positive nanosecond budget for perf-%s", w.name)
		}
	}
	for _, workload := range []string{"scale", "std"} {
		for _, metric := range compileMetrics {
			key := "compile-" + workload + "-" + metric
			if limits[key] <= 0 {
				t.Errorf("missing positive budget for %s", key)
			}
		}
	}
}

func BenchmarkPrograms(b *testing.B) {
	checkout := root(b)
	temp := b.TempDir()
	compiler := filepath.Join(temp, "go4t")
	command(b, checkout, "go", "build", "-o", compiler, "./cmd/go4t")
	limits := map[string]int64{}
	if os.Getenv("GO4T_BENCH_RECORD") == "" {
		limits = budgets(b)
	}
	for _, w := range workloads {
		b.Run(w.name, func(b *testing.B) {
			b.StopTimer()
			source, err := filepath.Abs(w.name + ".gt")
			if err != nil {
				b.Fatal(err)
			}
			dir := filepath.Join(temp, w.name)
			command(b, checkout, compiler, "build", "--emit-go", dir, source)
			binary := filepath.Join(dir, "program")
			files, err := filepath.Glob(filepath.Join(dir, "*.go"))
			if err != nil || len(files) == 0 {
				b.Fatalf("generated module files: %v (%d files)", err, len(files))
			}
			command(b, checkout, "go", append([]string{"build", "-o", binary}, files...)...)
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
			if os.Getenv("GO4T_BENCH_RECORD") == "" && average > limits[w.name] {
				b.Fatalf("%s exceeded budget: %d ns/op > %d ns/op", w.name, average, limits[w.name])
			}
		})
	}
}

// The full-size kernels run once, with compilation and input generation outside
// GNU time. This is a regression ceiling, not a statistically stable benchmark.
// An explicit input directory permits offline runs; absent inputs skip that
// kernel rather than silently substituting a smaller workload.
// perfAttempts bounds how often one kernel is rerun before its ceiling fails.
const perfAttempts = 3

func TestPerfPrograms(t *testing.T) {
	if _, err := exec.LookPath("/usr/bin/time"); err != nil {
		t.Skip("performance budgets require /usr/bin/time")
	}
	checkout := root(t)
	temp := t.TempDir()
	inputs := os.Getenv("GO4T_PERF_INPUT_DIR")
	if inputs == "" {
		if _, err := os.Stat(filepath.Join(checkout, "bench", "perf", "gen", "main.go")); err != nil {
			t.Skip("performance input generator is unavailable")
		}
		inputs = filepath.Join(temp, "input")
		command(t, checkout, "go", "run", "./bench/perf/gen", inputs)
	}
	compiler := filepath.Join(temp, "go4t-PerfBudgets")
	command(t, checkout, "go", "build", "-o", compiler, "./cmd/go4t")
	limits := budgets(t)
	for _, w := range perfWorkloads {
		t.Run(w.name, func(t *testing.T) {
			input, err := os.Open(filepath.Join(inputs, w.name+".txt"))
			if os.IsNotExist(err) {
				t.Skip("performance input is unavailable")
			}
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			binary := filepath.Join(temp, w.name)
			source := filepath.Join(checkout, "bench", "perf", w.name, "main.gt")
			command(t, checkout, compiler, "build", "-o", binary, source)
			timing := filepath.Join(temp, w.name+".time")
			limit := limits["perf-"+w.name]
			// A shared runner adds time and never removes it, so the fastest of a
			// few runs is the measurement; a real regression is over the ceiling
			// in every one of them.
			var elapsed int64
			for attempt := 1; attempt <= perfAttempts; attempt++ {
				if _, err := input.Seek(0, io.SeekStart); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				cmd := exec.CommandContext(ctx, "/usr/bin/time", "-f", "%e", "-o", timing, binary)
				cmd.Env = append(os.Environ(), "GOMAXPROCS=1", "GOGC=100")
				cmd.Stdin = input
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				err := cmd.Run()
				cancel()
				if err != nil {
					t.Fatalf("%s: %v\n%s", w.name, err, stderr.String())
				}
				if stdout.String() != w.output {
					t.Fatalf("%s output = %q, want %q", w.name, stdout.String(), w.output)
				}
				data, err := os.ReadFile(timing)
				if err != nil {
					t.Fatal(err)
				}
				seconds, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
				if err != nil {
					t.Fatal(err)
				}
				run := int64(seconds * float64(time.Second))
				t.Logf("%s: run %d of %d: %d ns, ceiling %d ns; checksum matched", w.name, attempt, perfAttempts, run, limit)
				if attempt == 1 || run < elapsed {
					elapsed = run
				}
				if elapsed <= limit {
					break
				}
			}
			if os.Getenv("GO4T_BENCH_RECORD") == "" && elapsed > limit {
				t.Fatalf("%s exceeded budget in all %d runs: fastest %d ns > %d ns", w.name, perfAttempts, elapsed, limit)
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
