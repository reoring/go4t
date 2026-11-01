package std_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gofx/ast"
	"gofx/codegen"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestListExtrasAndBoundaries(t *testing.T) {
	checks := []struct{ name, expression string }{
		{"zipWith empty left", `list.zipWith(empty, xs, add) == empty`},
		{"zipWith empty right", `list.zipWith(xs, empty, add) == empty`},
		{"zipWith singleton", `list.zipWith([2], [3], add) == [5]`},
		{"zipWith shortest", `list.zipWith(xs, [10, 20], add) == [11, 22]`},
		{"unzip empty", `list.unzip(emptyPairs) == (empty, emptyStrings)`},
		{"unzip singleton", `list.unzip([(1, "a")]) == ([1], ["a"])`},
		{"unzip order", `list.unzip([(1, "a"), (2, "b")]) == ([1, 2], ["a", "b"])`},
		{"any empty", `!list.any(empty, never)`},
		{"any singleton", `list.any([2], even) && !list.any([1], even)`},
		{"any short circuit", `list.any([1, 0], func(x) => 1 / x == 1)`},
		{"all empty", `list.all(empty, never)`},
		{"all singleton", `list.all([2], even) && !list.all([1], even)`},
		{"all short circuit", `!list.all([1, 0], func(x) => 1 / x == 0)`},
		{"find empty", `list.find(empty, never) == None`},
		{"find singleton", `list.find([2], even) == Some(2) && list.find([1], even) == None`},
		{"find first", `list.find([1, 2, 4], even) == Some(2)`},
		{"find short circuit", `list.find([1, 0], func(x) => 1 / x == 1) == Some(1)`},
		{"findIndex empty", `list.findIndex(empty, never) == None`},
		{"findIndex singleton", `list.findIndex([2], even) == Some(0) && list.findIndex([1], even) == None`},
		{"findIndex first", `list.findIndex([1, 2, 4], even) == Some(1)`},
		{"findIndex short circuit", `list.findIndex([1, 0], func(x) => 1 / x == 1) == Some(0)`},
		{"partition empty", `list.partition(empty, never) == (empty, empty)`},
		{"partition singleton", `list.partition([2], even) == ([2], empty) && list.partition([1], even) == (empty, [1])`},
		{"partition stable", `list.partition([1, 2, 3, 4], even) == ([2, 4], [1, 3])`},
		{"nub empty", `list.nub(empty) == empty`},
		{"nub singleton", `list.nub([2]) == [2]`},
		{"nub first order", `list.nub([3, 1, 3, 2, 1]) == [3, 1, 2]`},
		{"nub Eq dictionary", `list.nub([Ticket(1), Ticket(-1), Ticket(2)]) == [Ticket(1), Ticket(2)]`},
		{"range descending", `list.range(5, 1) == empty`},
		{"range equal", `list.range(2, 2) == empty`},
		{"range singleton", `list.range(2, 3) == [2]`},
		{"range negative", `list.range(-2, 2) == [-2, -1, 0, 1]`},
		{"range maximum", `list.range(float.maxInt() - 1, float.maxInt()) == [float.maxInt() - 1]`},
		{"range minimum", `list.range(float.minInt(), float.minInt() + 1) == [float.minInt()]`},
		{"replicate negative", `list.replicate(-1, 2) == empty && list.replicate(float.minInt(), 2) == empty`},
		{"replicate zero", `list.replicate(0, 2) == empty`},
		{"replicate singleton", `list.replicate(1, 2) == [2]`},
		{"replicate many", `list.replicate(3, 2) == [2, 2, 2]`},
		{"intercalate empty", `list.intercalate([0], emptyLists) == empty`},
		{"intercalate singleton", `list.intercalate([0], [[1, 2]]) == [1, 2]`},
		{"intercalate separator", `list.intercalate([0, -1], [[1], [2, 3], [4]]) == [1, 0, -1, 2, 3, 0, -1, 4]`},
		{"intercalate empty parts", `list.intercalate([0], [empty, empty, [1], empty]) == [0, 0, 1, 0]`},
		{"intercalate no separator", `list.intercalate(empty, [[1], [2]]) == [1, 2]`},
		{"flatten empty", `list.flatten(emptyLists) == empty`},
		{"flatten singleton", `list.flatten([[1]]) == [1]`},
		{"flatten order", `list.flatten([empty, [1, 2], empty, [3]]) == xs`},
		{"last empty", `list.last(empty) == None`},
		{"last singleton", `list.last([2]) == Some(2)`},
		{"last many", `list.last(xs) == Some(3)`},
		{"init empty", `list.init(empty) == empty`},
		{"init singleton", `list.init([2]) == empty`},
		{"init many", `list.init(xs) == [1, 2]`},
		{"splitAt empty", `list.splitAt(2, empty) == (empty, empty)`},
		{"splitAt singleton", `list.splitAt(1, [2]) == ([2], empty)`},
		{"splitAt negative", `list.splitAt(-1, xs) == (empty, xs) && list.splitAt(float.minInt(), xs) == (empty, xs)`},
		{"splitAt zero", `list.splitAt(0, xs) == (empty, xs)`},
		{"splitAt interior", `list.splitAt(2, xs) == ([1, 2], [3])`},
		{"splitAt past end", `list.splitAt(float.maxInt(), xs) == (xs, empty)`},
		{"span empty", `list.span(empty, never) == (empty, empty)`},
		{"span singleton", `list.span([2], even) == ([2], empty) && list.span([1], even) == (empty, [1])`},
		{"span prefix", `list.span([2, 4, 1, 6], even) == ([2, 4], [1, 6])`},
		{"span short circuit", `list.span([1, 0], func(x) => 1 / x == 0) == (empty, [1, 0])`},
		{"takeWhile empty", `list.takeWhile(empty, never) == empty`},
		{"takeWhile singleton", `list.takeWhile([2], even) == [2] && list.takeWhile([1], even) == empty`},
		{"takeWhile prefix", `list.takeWhile([2, 4, 1, 6], even) == [2, 4]`},
		{"dropWhile empty", `list.dropWhile(empty, never) == empty`},
		{"dropWhile singleton", `list.dropWhile([2], even) == empty && list.dropWhile([1], even) == [1]`},
		{"dropWhile prefix", `list.dropWhile([2, 4, 1, 6], even) == [1, 6]`},
		{"dropWhile short circuit", `list.dropWhile([1, 0], func(x) => 1 / x == 0) == [1, 0]`},
		{"lookup empty", `list.lookup(1, emptyPairs) == None`},
		{"lookup singleton", `list.lookup(1, [(1, "a")]) == Some("a") && list.lookup(2, [(1, "a")]) == None`},
		{"lookup first", `list.lookup(1, [(1, "a"), (2, "b"), (1, "c")]) == Some("a")`},
		{"lookup Eq dictionary", `list.lookup(Ticket(-1), [(Ticket(1), "a")]) == Some("a")`},
		{"indexed empty", `list.indexed(emptyStrings) == emptyPairs`},
		{"indexed singleton", `list.indexed(["a"]) == [(0, "a")]`},
		{"indexed zero based", `list.indexed(["a", "b"]) == [(0, "a"), (1, "b")]`},
		{"iterate negative", `list.iterate(-1, 1, func(x) => 1 / 0) == empty && list.iterate(float.minInt(), 1, addOne) == empty`},
		{"iterate zero", `list.iterate(0, 1, func(x) => 1 / 0) == empty`},
		{"iterate singleton no step", `list.iterate(1, 1, func(x) => 1 / 0) == [1]`},
		{"iterate steps", `list.iterate(4, 2, func(x) => x * 2) == [2, 4, 8, 16]`},
		{"product empty", `list.product(empty) == 1`},
		{"product singleton", `list.product([-2]) == -2`},
		{"product many", `list.product([2, 3, 4]) == 24 && list.product([2, 0, 3]) == 0`},
		{"count empty", `list.count(empty, never) == 0`},
		{"count singleton", `list.count([2], even) == 1 && list.count([1], even) == 0`},
		{"count many", `list.count([1, 2, 3, 4], even) == 2`},
		{"unfold empty", `list.unfold(0, down) == empty`},
		{"unfold singleton", `list.unfold(1, down) == [1]`},
		{"unfold order", `list.unfold(3, down) == [3, 2, 1]`},
		{"chunksOf empty", `list.chunksOf(2, empty) == emptyLists`},
		{"chunksOf singleton", `list.chunksOf(2, [1]) == [[1]]`},
		{"chunksOf zero", `list.chunksOf(0, xs) == emptyLists`},
		{"chunksOf negative", `list.chunksOf(-1, xs) == emptyLists && list.chunksOf(float.minInt(), xs) == emptyLists`},
		{"chunksOf remainder", `list.chunksOf(2, xs) == [[1, 2], [3]]`},
		{"chunksOf one", `list.chunksOf(1, xs) == [[1], [2], [3]]`},
		{"chunksOf past length", `list.chunksOf(float.maxInt(), xs) == [xs]`},
		{"sortOn empty", `list.sortOn(empty, addOne) == empty`},
		{"sortOn singleton", `list.sortOn([2], addOne) == [2]`},
		{"sortOn key", `list.sortOn([3, 1, 2], func(x) => 0 - x) == [3, 2, 1]`},
		{"sortOn stable", `list.sortOn([(2, "a"), (1, "b"), (2, "c")], fst) == [(1, "b"), (2, "a"), (2, "c")]`},
		{"sortOn Ord dictionary", `list.sortOn([1, 3, 2], func(x) => Key(x)) == [3, 2, 1]`},
		{"lazy force", `lazy.force(lazy.delay(func(_) => 2 + 3)) == 5`},
		{"lazy repeat force", `lazy.force(deferred) == 7 && lazy.force(deferred) == 7`},
		{"head empty", `head(empty) == None`},
		{"head singleton", `head([2]) == Some(2)`},
		{"tail empty", `tail(empty) == empty`},
		{"tail singleton", `tail([2]) == empty`},
		{"take negative", `list.take(-1, xs) == empty && list.take(float.minInt(), xs) == empty`},
		{"take empty", `list.take(1, empty) == empty`},
		{"take singleton", `list.take(1, [2]) == [2]`},
		{"take past length", `list.take(float.maxInt(), xs) == xs`},
		{"drop negative", `list.drop(-1, xs) == xs && list.drop(float.minInt(), xs) == xs`},
		{"drop empty", `list.drop(1, empty) == empty`},
		{"drop singleton", `list.drop(1, [2]) == empty`},
		{"drop past length", `list.drop(float.maxInt(), xs) == empty`},
		{"fold empty", `fold(empty, 9, add) == 9`},
		{"fold left order", `fold(xs, 0, func(a, b) => a * 10 + b) == 123`},
		{"length boundaries", `length(empty) == 0 && length([1]) == 1`},
		{"append boundaries", `append(empty, xs) == xs && append(xs, empty) == xs`},
		{"filter boundaries", `filter(empty, never) == empty && filter([2], even) == [2] && filter([1], even) == empty`},
		{"reverse boundaries", `reverse(empty) == empty && reverse([2]) == [2] && reverse(xs) == [3, 2, 1]`},
		{"zip boundaries", `list.zip(empty, emptyStrings) == emptyPairs && list.zip(xs, ["a"]) == [(1, "a")]`},
		{"sum boundaries", `list.sum(empty) == 0 && list.sum([2]) == 2`},
		{"concatMap boundaries", `list.concatMap(empty, func(x) => [x]) == empty && list.concatMap(xs, func(x) => [x, x]) == [1, 1, 2, 2, 3, 3]`},
		{"elem boundaries", `!list.elem(1, empty) && list.elem(1, [1]) && !list.elem(2, [1])`},
		{"sort boundaries", `list.sort(empty) == empty && list.sort([2]) == [2] && list.sort([3, 1, 2]) == xs`},
		{"sortBy boundaries", `list.sortBy(empty, compare) == empty && list.sortBy([2], compare) == [2]`},
		{"sort reversed large", `list.sort(reverse(list.range(0, 10000))) == list.range(0, 10000)`},
		{"sortBy stability", `list.sortBy([(1, "a"), (0, "b"), (1, "c")], func(a, b) => compare(a.0, b.0)) == [(0, "b"), (1, "a"), (1, "c")]`},
		{"maximum boundaries", `list.maximum(empty) == None && list.maximum([2]) == Some(2)`},
		{"minimum boundaries", `list.minimum(empty) == None && list.minimum([2]) == Some(2)`},
		{"map boundaries", `map(empty, addOne) == empty && map([2], addOne) == [3]`},
		{"flatMap boundaries", `flatMap(empty, func(x) => [x]) == empty`},
		{"forEach empty", `list.forEach(empty, func(x) => Some(unit())) == Some(unit())`},
		{"mapM empty", `list.mapM(empty, func(x) => Some(x)) == Some(empty)`},
		{"mapM singleton", `list.mapM([2], func(x) => Some(x + 1)) == Some([3])`},
		{"splitOnce tuple bridge", `string.splitOnce("=", "日=🙂=x") == Some(("日", "🙂=x"))`},
		{"regexCaptures bridge", `regex.regexCaptures("([A-Z]+) (.*)", "INFO 日🙂") == Some(["INFO", "日🙂"])`},
	}
	var source, want strings.Builder
	source.WriteString(`package main
import "std/list"
import "std/lazy"
import "std/string"
import "std/regex"
import "std/float"
type Ticket = Ticket(Int)
instance Eq[Ticket] { eq(a, b) { match (a, b) { (Ticket(x), Ticket(y)) => float.abs(x) == float.abs(y) } } }
type Key = Key(Int) deriving Eq
instance Ord[Key] { compare(a, b) { match (a, b) { (Key(x), Key(y)) => compare(y, x) } } }
func down(n Int) Option[(Int, Int)] { if n <= 0 { None } else { Some((n, n - 1)) } }
func main() IO[Unit] {
  let empty: List[Int] = Nil
  let emptyStrings: List[String] = Nil
  let emptyPairs: List[(Int, String)] = Nil
  let emptyLists: List[List[Int]] = Nil
  let xs = [1, 2, 3]
  let add = func(a Int, b Int) => a + b
  let addOne = func(x Int) => x + 1
  let even = func(x Int) => x % 2 == 0
  let never = func(x Int) => x / 0 == 0
  let deferred = lazy.delay(func(_) => 7)
  let unforced = lazy.delay(func(_) => 1 / 0)
  do {
`)
	for _, check := range checks {
		fmt.Fprintf(&source, "println(%q + show(%s))\n", check.name+": ", check.expression)
		fmt.Fprintf(&want, "%s: true\n", check.name)
	}
	source.WriteString("}\n}\n")
	user, err := parser.Parse(source.String())
	if err != nil {
		t.Fatal(err)
	}
	user.Name = "stdlib_extras_test.gfx"
	mods, err := modules.LoadFiles([]*ast.File{user}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(mods)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	// The pure surface cannot observe thunk invocation counts. Exercise the same
	// generated delay/force ABI with a counted Go thunk to prove non-memoisation.
	generated["assertions.go"] = []byte(`package main
func init() {
  calls := 0
  lazy := lazy_delay_(func(_ any) any { calls++; return int64(7) })
  if calls != 0 { panic("delay evaluated its thunk") }
  if lazy_force_(lazy) != int64(7) || lazy_force_(lazy) != int64(7) || calls != 2 {
    panic("force must evaluate its thunk on every call")
  }
  pair := rt_tuple2(int64(1), "two")
  a, b := rt_untuple2(pair)
  if a != int64(1) || b != "two" { panic("tuple shape round trip") }
}
`)
	dir := t.TempDir()
	args := []string{"run"}
	for name, source := range generated {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, source, 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated prelude checks failed: %v\n%s", err, output)
	}
	if string(output) != want.String() {
		t.Errorf("prelude checks:\n%s\nwant every named check to be true", output)
	}
}

func TestExtrasTypeErrors(t *testing.T) {
	for _, tt := range []struct {
		name, expression, message string
	}{
		{"sortOn requires key Ord", `list.sortOn([Opaque], func(x) => x)`, "no instance Ord[Opaque]"},
		{"lookup requires key Eq", `list.lookup(Opaque, [(Opaque, 1)])`, "no instance Eq[Opaque]"},
		{"charAt requires Int index", `string.charAt("x", "0")`, "type mismatch: expected Int, got String"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user, err := parser.Parse("package main\nimport \"std/list\"\nimport \"std/string\"\ntype Opaque = Opaque\nfunc main() IO[Unit] { let value = " + tt.expression + "; println(\"ok\") }")
			if err != nil {
				t.Fatal(err)
			}
			user.Name = "stdlib_extras_error.gfx"
			mods, err := modules.LoadFiles([]*ast.File{user}, ".")
			if err != nil {
				t.Fatal(err)
			}
			_, err = types.Check(mods)
			checkError, ok := err.(*types.Error)
			if !ok || checkError.File != user.Name || checkError.Pos.Line != 5 || checkError.Pos.Col <= 0 || !strings.Contains(checkError.Msg, tt.message) {
				t.Fatalf("error = %#v, want positioned %q", err, tt.message)
			}
		})
	}
}
