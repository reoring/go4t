package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenericGoImportRun(t *testing.T) {
	dir := toolsModule(t)
	t.Setenv("GO4T_CACHE", t.TempDir())
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import "go:slices"
import "go:maps"
import "go:sync"
import "std/map"
import "std/list"
func clone[A](xs List[A]) IO[List[A]] { slices.clone(xs) }
func minValue[A Ord](xs List[A]) IO[A] { slices.min(xs) }
func nested[A](xs List[A]) IO[List[A]] { clone(xs) }
func main() IO[Unit] {
 do {
  xs <- nested([3, 1, 2])
  ys <- nested(["b", "a"])
  low <- minValue(xs)
  let has = slices.contains(xs, _)
  yes <- has(2)
  let index = slices.index
  at <- index(xs, 1)
  keys <- maps.keys(map.mapFromList([("b", 2), ("a", 1)]))
  println(show(low))
  println(show(yes))
  println(show(at))
  println(show(list.sort(keys)))
  println(show(ys))
  cached <- sync.onceValue(func() => do { println("once"); ioPure(7) })
  a <- cached()
  b <- cached()
  pair <- sync.onceValues(func() => ioPure(("value", true)))
  result <- pair()
  println(show(a + b))
  println(result.0)
  println(show(result.1))
  slices.sort(xs)
  println(show(xs))
 }
}
`)
	for _, phase := range []string{"cold", "warm"} {
		out, stderr, status := toolsCommand(t, dir, "", "run", file)
		want := "1\ntrue\n1\n[a, b]\n[b, a]\nonce\n14\nvalue\ntrue\n[3, 1, 2]\n"
		if status != 0 || out != want || stderr != "" { t.Fatalf("%s: status=%d stdout=%q stderr=%s", phase, status, out, stderr) }
	}
	shims, err := filepath.Glob(filepath.Join(dir, "main_gen", "go4tbind", "slices", "instances", "*", "shim.go"))
	if err != nil || len(shims) != 6 { t.Fatalf("distinct slices instantiations: %d, %v", len(shims), err) }
}

func TestGenericGoDiagnostics(t *testing.T) {
	cases := []struct { name, source, want string }{
		{"comparable", `func main() IO[Unit] { do { found <- slices.contains([[1]], [1]); println(show(found)) } }`, "does not satisfy comparable"},
		{"ordered", `func main() IO[Unit] { do { n <- slices.min([true]); println(show(n)) } }`, "does not satisfy cmp.Ordered"},
		{"ambiguous", `func main() IO[Unit] { do { slices.clone([]); ioPure(unit()) } }`, "cannot determine type arguments before defaulting"},
		{"iterator", `func main() IO[Unit] { do { let values: List[Int] = []; let action: IO[List[Int]] = slices.sorted(values); action; ioPure(unit()) } }`, "iterator parameters are unsupported"},
		{"pure callback", `func produce() Int { 1 }
func main() IO[Unit] { do { sync.onceValue(produce); ioPure(unit()) } }`, "type mismatch"},
		{"local polymorphism", `func main() IO[Unit] { let copy = func(xs) => slices.clone(xs); do { copy([1]); ioPure(unit()) } }`, "cannot determine concrete type arguments"},
		{"unbounded", `func expand[A](xs List[A]) IO[Unit] { do { slices.clone(xs); expand([xs]) } }
func main() IO[Unit] { expand([1]) }`, "unbounded polymorphic instantiation"},
	}
	for _, tc := range cases { t.Run(tc.name, func(t *testing.T) {
		dir := toolsModule(t)
		file := toolsWrite(t, filepath.Join(dir, "main.gt"), "package main\nimport \"go:slices\"\nimport \"go:sync\"\n"+tc.source+"\n")
		out, stderr, status := toolsCommand(t, dir, "", "check", file)
		if status != 1 || out != "" || !strings.Contains(stderr, tc.want) || !strings.Contains(stderr, "main.gt:") { t.Fatalf("status=%d stdout=%q stderr=%s; want %q", status, out, stderr, tc.want) }
		if _, err := os.Stat(filepath.Join(dir, "main_gen")); !os.IsNotExist(err) { t.Fatalf("check emitted build output: %v", err) }
	}) }
}

func TestGenericTemplateCache(t *testing.T) {
	dir := toolsModule(t)
	t.Setenv("GO4T_CACHE", t.TempDir())
	var entry string
	for _, value := range []string{"[1]", "[\"s\"]"} {
		file := toolsWrite(t, filepath.Join(dir, "main.gt"), "package main\nimport \"go:slices\"\nfunc main() IO[Unit] { do { slices.clone("+value+"); ioPure(unit()) } }\n")
		out, stderr, status := toolsCommand(t, dir, "", "check", file)
		if status != 0 || out != "" || stderr != "" { t.Fatalf("check: %d %q %s", status, out, stderr) }
		b := newGoImportBinder(dir)
		m, err := b.Bind("slices")
		if err != nil || m.ForeignBindings == nil { t.Fatalf("cached port: %v", err) }
		if entry != "" && entry != b.cacheEntries["slices"] { t.Fatal("caller changed package template key") }
		entry = b.cacheEntries["slices"]
		_, _, _, err = readBindingCache(entry)
		if err != nil { t.Fatal(err) }
	}
}

func TestGenericBeyondOptimizerLimits(t *testing.T) {
	dir := toolsModule(t)
	var source strings.Builder
	source.WriteString("package main\nimport \"go:slices\"\nfunc large[A](xs List[A]) IO[List[A]] { if length([")
	source.WriteString(strings.Repeat("1,", 50) + "1]) == 51 { slices.clone(xs) } else { slices.clone(xs) } }\nfunc main() IO[Unit] { do {\n")
	value := "1"
	for range 9 {
		value = "[" + value + "]"
		source.WriteString("large(" + value + ");\n")
	}
	source.WriteString("println(\"nine instances\") } }\n")
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), source.String())
	out, stderr, status := toolsCommand(t, dir, "", "run", file)
	if status != 0 || out != "nine instances\n" || stderr != "" {
		t.Fatalf("required specialization: %d %q %s", status, out, stderr)
	}
	shims, err := filepath.Glob(filepath.Join(dir, "main_gen", "go4tbind", "slices", "instances", "*", "shim.go"))
	if err != nil || len(shims) != 9 { t.Fatalf("required copies: %d, %v", len(shims), err) }
}
