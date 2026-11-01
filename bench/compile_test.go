package bench

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gofx/ast"
	"gofx/codegen"
	"gofx/modules"
	"gofx/parser"
	"gofx/std"
	"gofx/types"
)

const compileN = 400
const diagnosticDepth = 32

var compileMetrics = []string{"check-ns", "alloc-bytes", "peak-heap-bytes", "go-bytes", "build-ns", "error-chars"}

func parseCheck(source, directory string) (*types.Program, error) {
	file, err := parser.Parse(source)
	if err != nil {
		return nil, err
	}
	file.Name = "scale.gfx"
	loaded, err := modules.LoadFiles([]*ast.File{file}, directory)
	if err != nil {
		return nil, err
	}
	return types.Check(loaded)
}

// stdSource preserves module namespaces while linking every embedded standard
// module into one program. Literal concatenation is invalid: modules have
// different package declarations and references such as prelude.pure/r.Err.
func stdSource(tb testing.TB) (string, int) {
	tb.Helper()
	var source strings.Builder
	source.WriteString("package main\n")
	lines := 0
	if err := fs.WalkDir(std.FS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".gfx") {
			return nil
		}
		data, err := std.FS.ReadFile(name)
		if err != nil {
			return err
		}
		lines += strings.Count(string(data), "\n")
		fmt.Fprintf(&source, "import %q\n", "std/"+path.Dir(name))
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
	source.WriteString("func main() IO[Unit] { ioPure(unit()) }\n")
	return source.String(), lines + strings.Count(source.String(), "\n")
}

func TestCompileSource(t *testing.T) {
	for _, n := range []int{0, 1, 8, compileN} {
		source := CompileSource(n)
		if source != CompileSource(n) {
			t.Fatal("generator is not deterministic")
		}
		if got := strings.Count(source, "\n"); got != 13*n+3 {
			t.Fatalf("N=%d: %d lines, want %d", n, got, 13*n+3)
		}
		file, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		var data, constrained, matches int
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.TypeDecl:
				data++
				if len(d.Deriving) != 3 {
					t.Fatalf("missing deriving on %s", d.Name)
				}
			case *ast.FuncDecl:
				if len(d.TParams) != 0 && len(d.TParams[0].Constraints) == 2 {
					constrained++
				}
			}
			ast.Walk(decl, func(node ast.Node) {
				if _, ok := node.(*ast.Match); ok {
					matches++
				}
			})
		}
		if data != n || constrained != n || matches != n {
			t.Fatalf("N=%d: data=%d constrained=%d matches=%d", n, data, constrained, matches)
		}
	}
	if _, err := parseCheck(CompileSource(8), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestCompileSourceNegative(t *testing.T) {
	defer func() {
		if got := recover(); got != "compile scale must be nonnegative" {
			t.Fatalf("panic = %v", got)
		}
	}()
	CompileSource(-1)
}

func TestCompileDiagnostic(t *testing.T) {
	_, err := parseCheck(CompileSource(1)+diagnosticProbe(diagnosticDepth), t.TempDir())
	if _, ok := err.(*types.Error); !ok || !strings.Contains(err.Error(), "type mismatch: expected ") || !strings.Contains(err.Error(), ", got ") {
		t.Fatalf("expected deep Int/Bool type mismatch, got %v", err)
	}
	t.Logf("depth=%d longest diagnostic=%d characters: %s", diagnosticDepth, utf8.RuneCountInString(err.Error()), err)
}

func TestCompileStd(t *testing.T) {
	source, _ := stdSource(t)
	program, err := parseCheck(source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codegen.Generate(program, "main"); err != nil {
		t.Fatal(err)
	}
}

// samplePeak reports the maximum observed live heap above the pre-check
// baseline. Sampling every millisecond (and at completion) is not an exact
// allocator high-water mark; total allocation is recorded separately.
func samplePeak(baseline uint64) func() uint64 {
	stop := make(chan struct{})
	result := make(chan uint64)
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		var peak uint64
		sample := func() {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			if stats.HeapAlloc > baseline && stats.HeapAlloc-baseline > peak {
				peak = stats.HeapAlloc - baseline
			}
		}
		for {
			select {
			case <-ticker.C:
				sample()
			case <-stop:
				sample()
				result <- peak
				return
			}
		}
	}()
	return func() uint64 { close(stop); return <-result }
}

func BenchmarkCompile(b *testing.B) {
	standard, standardLines := stdSource(b)
	for _, workload := range []struct {
		name, source string
		n, lines     int
	}{
		{"scale", CompileSource(compileN), compileN, 13*compileN + 3},
		{"std", standard, 0, standardLines},
	} {
		b.Run(workload.name, func(b *testing.B) {
			b.StopTimer()
			b.ReportAllocs()
			dir := b.TempDir()
			var elapsed, build time.Duration
			var allocated, peak, goBytes uint64
			var program *types.Program
			for range b.N {
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				finish := samplePeak(before.HeapAlloc)
				b.StartTimer()
				started := time.Now()
				var err error
				program, err = parseCheck(workload.source, dir)
				elapsed += time.Since(started)
				b.StopTimer()
				observed := finish()
				runtime.ReadMemStats(&after)
				allocated += after.TotalAlloc - before.TotalAlloc
				if observed > peak {
					peak = observed
				}
				if err != nil {
					b.Fatal(err)
				}
			}
			generated, err := codegen.Generate(program, "main")
			if err != nil {
				b.Fatal(err)
			}
			for name, data := range generated {
				goBytes += uint64(len(data))
				if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
					b.Fatal(err)
				}
			}
			mod := "module compilebench\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => " + filepath.ToSlash(root(b)) + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
				b.Fatal(err)
			}
			for range b.N {
				// Removing only the local binary forces linking on every iteration;
				// the normal Go build cache remains warm (no global cache mutation).
				binary := filepath.Join(dir, "program")
				if err := os.Remove(binary); err != nil && !os.IsNotExist(err) {
					b.Fatal(err)
				}
				started := time.Now()
				command(b, dir, "go", "build", "-o", binary, ".")
				build += time.Since(started)
			}
			_, err = parseCheck(workload.source+diagnosticProbe(diagnosticDepth), dir)
			if _, ok := err.(*types.Error); !ok || !strings.Contains(err.Error(), "type mismatch: expected ") || !strings.Contains(err.Error(), ", got ") {
				b.Fatalf("injected mismatch did not produce a type diagnostic: %v", err)
			}
			chars := utf8.RuneCountInString(err.Error())
			values := map[string]int64{
				"check-ns":        elapsed.Nanoseconds() / int64(b.N),
				"alloc-bytes":     int64(allocated / uint64(b.N)),
				"peak-heap-bytes": int64(peak),
				"go-bytes":        int64(goBytes),
				"build-ns":        build.Nanoseconds() / int64(b.N),
				"error-chars":     int64(chars),
			}
			b.ReportMetric(float64(workload.n), "N")
			b.ReportMetric(float64(workload.lines), "lines")
			b.ReportMetric(float64(values["check-ns"])/1e6, "check-ms")
			b.ReportMetric(float64(values["alloc-bytes"])/1e6, "alloc-MB")
			b.ReportMetric(float64(peak)/1e6, "peak-heap-MB")
			b.ReportMetric(float64(goBytes)/1e3, "go-KB")
			b.ReportMetric(float64(values["build-ns"])/1e6, "build-ms")
			b.ReportMetric(float64(chars), "error-chars")
			limits := budgets(b)
			for _, metric := range compileMetrics {
				key := "compile-" + workload.name + "-" + metric
				b.Logf("%s = %d", key, values[metric])
				if os.Getenv("GOFX_BENCH_RECORD") == "" && values[metric] > limits[key] {
					b.Errorf("%s exceeded budget: %d > %d", key, values[metric], limits[key])
				}
			}
		})
	}
}
