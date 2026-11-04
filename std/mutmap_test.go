package std_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go4t/ast"
	"go4t/codegen"
	"go4t/modules"
	"go4t/parser"
	"go4t/types"
)

func TestMutMapTypeContracts(t *testing.T) {
	if got := types.KindString(types.MutMap.Kind); got != "(*, *) -> *" {
		t.Fatalf("MutMap kind = %q", got)
	}
	for _, tt := range []struct {
		name, source, wantError string
	}{
		{"opaque", `func keep[K, V](m MutMap[K, V]) MutMap[K, V] { m }`, ""},
		{"qualified", `func keep[K, V](m mutmap.MutMap[K, V]) mutmap.MutMap[K, V] { m }`, ""},
		{"bare constructor", `func bad(m MutMap) Int { 0 }`, "kind mismatch"},
		{"one argument", `func bad(m MutMap[Int]) Int { 0 }`, "kind mismatch"},
		{"too many arguments", `func bad(m MutMap[Int, Int, Int]) Int { 0 }`, "kind mismatch"},
		{"reserved builtin", `type MutMap[K, V] = Wrapped(K, V)`, "duplicate declaration MutMap"},
		{"no constructor", `func bad() MutMap[Int, Int] { MutMap(1, 2) }`, "unknown identifier MutMap"},
		{"no equality", `func bad(m MutMap[Int, Int]) Bool { m == m }`, "no instance Eq[MutMap[Int, Int]]"},
		{"no show", `func bad(m MutMap[Int, Int]) String { show(m) }`, "no instance Show[MutMap[Int, Int]]"},
		{"no debug", `func bad(m MutMap[Int, Int]) String { debug(m) }`, "no instance Debug[MutMap[Int, Int]]"},
		{"new requires Ord", `func bad[K, V]() IO[MutMap[K, V]] { mutmap.new() }`, "no instance Ord[K]"},
		{"lookup needs no Ord", `func use[K, V](m MutMap[K, V], k K) IO[Option[V]] { mutmap.lookup(m, k) }`, ""},
		{"insert needs no Ord", `func use[K, V](m MutMap[K, V], k K, v V) IO[Unit] { mutmap.insert(m, k, v) }`, ""},
		{"delete needs no Ord", `func use[K, V](m MutMap[K, V], k K) IO[Unit] { mutmap.delete(m, k) }`, ""},
		{"toList needs no Ord", `func use[K, V](m MutMap[K, V]) IO[List[(K, V)]] { mutmap.toList(m) }`, ""},
		{"freeze needs no Ord", `func use[K, V](m MutMap[K, V]) IO[Map[K, V]] { mutmap.freeze(m) }`, ""},
		{"no pure new", `func bad() MutMap[Int, Int] { mutmap.new() }`, "type mismatch"},
		{"no pure lookup", `func bad(m MutMap[Int, Int]) Option[Int] { mutmap.lookup(m, 1) }`, "type mismatch"},
		{"no pure size", `func bad(m MutMap[Int, Int]) Int { mutmap.size(m) }`, "type mismatch"},
		{"no pure list", `func bad(m MutMap[Int, Int]) List[(Int, Int)] { mutmap.toList(m) }`, "type mismatch"},
		{"no pure freeze", `func bad(m MutMap[Int, Int]) Map[Int, Int] { mutmap.freeze(m) }`, "type mismatch"},
		{"wrong key", `func bad(m MutMap[Int, Int]) IO[Unit] { mutmap.insert(m, "x", 1) }`, "type mismatch: expected Int, got String"},
		{"wrong value", `func bad(m MutMap[Int, Int]) IO[Unit] { mutmap.insert(m, 1, "x") }`, "type mismatch: expected Int, got String"},
		{"size needs no Ord", `func use[K, V](m MutMap[K, V]) IO[Int] { mutmap.size(m) }`, ""},
		{"freeze returns pure map", `func use(m Map[Int, Int]) Int { map.mapSize(m) }`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user, err := parser.Parse("package main\nimport \"std/mutmap\"\nimport \"std/map\"\n" + tt.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			if err != nil {
				t.Fatal(err)
			}
			user.Name = "mutmap_contract.gt"
			mods, err := modules.LoadFiles([]*ast.File{user}, ".")
			if err != nil {
				t.Fatal(err)
			}
			_, err = types.Check(mods)
			if tt.wantError == "" {
				if err != nil { t.Fatal(err) }
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestMutMapCompiledOperations(t *testing.T) {
	const source = `package main
import "std/mutmap"
import "std/map"
import "std/list"
type Key = Key(Int) deriving Eq, Show
instance Ord[Key] { compare(a, b) { match (a, b) { (Key(x), Key(y)) => compare(y, x) } } }
func create[K Ord, V]() IO[MutMap[K, V]] { mutmap.new() }
func copy[K Ord, V](m Map[K, V]) IO[MutMap[K, V]] { mutmap.thaw(m) }
func main() IO[Unit] {
  do {
    m <- create()
    mutmap.insert(m, 2, "two")
    mutmap.insert(m, 1, "one")
    mutmap.insert(m, 3, "three")
    before <- mutmap.toList(m)
    frozen <- mutmap.freeze(m)
    mutmap.insert(m, 2, "new")
    mutmap.delete(m, 1)
    mutmap.delete(m, 99)
    got <- mutmap.lookup(m, 2)
    missing <- mutmap.lookup(m, 1)
    size <- mutmap.size(m)
    println(show(got == Some("new") && missing == None && size == 2))
    println(show(before == [(1, "one"), (2, "two"), (3, "three")]))
    println(show(map.mapToList(frozen) == before))
    copied <- copy(frozen)
    mutmap.delete(copied, 3)
    copiedSize <- mutmap.size(copied)
    println(show(copiedSize == 2 && map.mapSize(frozen) == 3))
    custom <- create()
    mutmap.insert(custom, Key(1), 10)
    mutmap.insert(custom, Key(3), 30)
    mutmap.insert(custom, Key(2), 20)
    customList <- mutmap.toList(custom)
    println(show(customList == [(Key(3), 30), (Key(2), 20), (Key(1), 10)]))
    strings <- create()
    mutmap.insert(strings, "日", 1)
    mutmap.insert(strings, "a", 2)
    stringList <- mutmap.toList(strings)
    println(show(stringList == [("a", 2), ("日", 1)]))
  }
}
`
	user, err := parser.Parse(source)
	if err != nil { t.Fatal(err) }
	user.Name = "mutmap_compiled.gt"
	mods, err := modules.LoadFiles([]*ast.File{user}, ".")
	if err != nil { t.Fatal(err) }
	program, err := types.Check(mods)
	if err != nil { t.Fatal(err) }
	generated, err := codegen.Generate(program, "main")
	if err != nil { t.Fatal(err) }
	dir := t.TempDir()
	args := []string{"run"}
	for name, source := range generated {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, source, 0o600); err != nil { t.Fatal(err) }
		args = append(args, path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "go", args...).CombinedOutput()
	if err != nil { t.Fatalf("generated mutmap checks: %v\n%s", err, output) }
	if want := strings.Repeat("true\n", 6); string(output) != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}
