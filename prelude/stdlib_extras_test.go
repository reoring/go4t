package prelude

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
	"gofx/parser"
	"gofx/types"
)

func TestListExtrasAndBoundaries(t *testing.T) {
	checks := []struct{ name, expression string }{
		{"zipWith empty left", `zipWith(empty, xs, add) == empty`},
		{"zipWith empty right", `zipWith(xs, empty, add) == empty`},
		{"zipWith singleton", `zipWith([2], [3], add) == [5]`},
		{"zipWith shortest", `zipWith(xs, [10, 20], add) == [11, 22]`},
		{"unzip empty", `unzip(emptyPairs) == (empty, emptyStrings)`},
		{"unzip singleton", `unzip([(1, "a")]) == ([1], ["a"])`},
		{"unzip order", `unzip([(1, "a"), (2, "b")]) == ([1, 2], ["a", "b"])`},
		{"any empty", `!any(empty, never)`},
		{"any singleton", `any([2], even) && !any([1], even)`},
		{"any short circuit", `any([1, 0], func(x) => 1 / x == 1)`},
		{"all empty", `all(empty, never)`},
		{"all singleton", `all([2], even) && !all([1], even)`},
		{"all short circuit", `!all([1, 0], func(x) => 1 / x == 0)`},
		{"find empty", `find(empty, never) == None`},
		{"find singleton", `find([2], even) == Some(2) && find([1], even) == None`},
		{"find first", `find([1, 2, 4], even) == Some(2)`},
		{"find short circuit", `find([1, 0], func(x) => 1 / x == 1) == Some(1)`},
		{"findIndex empty", `findIndex(empty, never) == None`},
		{"findIndex singleton", `findIndex([2], even) == Some(0) && findIndex([1], even) == None`},
		{"findIndex first", `findIndex([1, 2, 4], even) == Some(1)`},
		{"findIndex short circuit", `findIndex([1, 0], func(x) => 1 / x == 1) == Some(0)`},
		{"partition empty", `partition(empty, never) == (empty, empty)`},
		{"partition singleton", `partition([2], even) == ([2], empty) && partition([1], even) == (empty, [1])`},
		{"partition stable", `partition([1, 2, 3, 4], even) == ([2, 4], [1, 3])`},
		{"nub empty", `nub(empty) == empty`},
		{"nub singleton", `nub([2]) == [2]`},
		{"nub first order", `nub([3, 1, 3, 2, 1]) == [3, 1, 2]`},
		{"nub Eq dictionary", `nub([Ticket(1), Ticket(-1), Ticket(2)]) == [Ticket(1), Ticket(2)]`},
		{"range descending", `range(5, 1) == empty`},
		{"range equal", `range(2, 2) == empty`},
		{"range singleton", `range(2, 3) == [2]`},
		{"range negative", `range(-2, 2) == [-2, -1, 0, 1]`},
		{"range maximum", `range(maxInt() - 1, maxInt()) == [maxInt() - 1]`},
		{"range minimum", `range(minInt(), minInt() + 1) == [minInt()]`},
		{"replicate negative", `replicate(-1, 2) == empty && replicate(minInt(), 2) == empty`},
		{"replicate zero", `replicate(0, 2) == empty`},
		{"replicate singleton", `replicate(1, 2) == [2]`},
		{"replicate many", `replicate(3, 2) == [2, 2, 2]`},
		{"intercalate empty", `intercalate([0], emptyLists) == empty`},
		{"intercalate singleton", `intercalate([0], [[1, 2]]) == [1, 2]`},
		{"intercalate separator", `intercalate([0, -1], [[1], [2, 3], [4]]) == [1, 0, -1, 2, 3, 0, -1, 4]`},
		{"intercalate empty parts", `intercalate([0], [empty, empty, [1], empty]) == [0, 0, 1, 0]`},
		{"intercalate no separator", `intercalate(empty, [[1], [2]]) == [1, 2]`},
		{"flatten empty", `flatten(emptyLists) == empty`},
		{"flatten singleton", `flatten([[1]]) == [1]`},
		{"flatten order", `flatten([empty, [1, 2], empty, [3]]) == xs`},
		{"last empty", `last(empty) == None`},
		{"last singleton", `last([2]) == Some(2)`},
		{"last many", `last(xs) == Some(3)`},
		{"init empty", `init(empty) == empty`},
		{"init singleton", `init([2]) == empty`},
		{"init many", `init(xs) == [1, 2]`},
		{"splitAt empty", `splitAt(2, empty) == (empty, empty)`},
		{"splitAt singleton", `splitAt(1, [2]) == ([2], empty)`},
		{"splitAt negative", `splitAt(-1, xs) == (empty, xs) && splitAt(minInt(), xs) == (empty, xs)`},
		{"splitAt zero", `splitAt(0, xs) == (empty, xs)`},
		{"splitAt interior", `splitAt(2, xs) == ([1, 2], [3])`},
		{"splitAt past end", `splitAt(maxInt(), xs) == (xs, empty)`},
		{"span empty", `span(empty, never) == (empty, empty)`},
		{"span singleton", `span([2], even) == ([2], empty) && span([1], even) == (empty, [1])`},
		{"span prefix", `span([2, 4, 1, 6], even) == ([2, 4], [1, 6])`},
		{"span short circuit", `span([1, 0], func(x) => 1 / x == 0) == (empty, [1, 0])`},
		{"takeWhile empty", `takeWhile(empty, never) == empty`},
		{"takeWhile singleton", `takeWhile([2], even) == [2] && takeWhile([1], even) == empty`},
		{"takeWhile prefix", `takeWhile([2, 4, 1, 6], even) == [2, 4]`},
		{"dropWhile empty", `dropWhile(empty, never) == empty`},
		{"dropWhile singleton", `dropWhile([2], even) == empty && dropWhile([1], even) == [1]`},
		{"dropWhile prefix", `dropWhile([2, 4, 1, 6], even) == [1, 6]`},
		{"dropWhile short circuit", `dropWhile([1, 0], func(x) => 1 / x == 0) == [1, 0]`},
		{"lookup empty", `lookup(1, emptyPairs) == None`},
		{"lookup singleton", `lookup(1, [(1, "a")]) == Some("a") && lookup(2, [(1, "a")]) == None`},
		{"lookup first", `lookup(1, [(1, "a"), (2, "b"), (1, "c")]) == Some("a")`},
		{"lookup Eq dictionary", `lookup(Ticket(-1), [(Ticket(1), "a")]) == Some("a")`},
		{"indexed empty", `indexed(emptyStrings) == emptyPairs`},
		{"indexed singleton", `indexed(["a"]) == [(0, "a")]`},
		{"indexed zero based", `indexed(["a", "b"]) == [(0, "a"), (1, "b")]`},
		{"iterate negative", `iterate(-1, 1, func(x) => 1 / 0) == empty && iterate(minInt(), 1, addOne) == empty`},
		{"iterate zero", `iterate(0, 1, func(x) => 1 / 0) == empty`},
		{"iterate singleton no step", `iterate(1, 1, func(x) => 1 / 0) == [1]`},
		{"iterate steps", `iterate(4, 2, func(x) => x * 2) == [2, 4, 8, 16]`},
		{"product empty", `product(empty) == 1`},
		{"product singleton", `product([-2]) == -2`},
		{"product many", `product([2, 3, 4]) == 24 && product([2, 0, 3]) == 0`},
		{"count empty", `count(empty, never) == 0`},
		{"count singleton", `count([2], even) == 1 && count([1], even) == 0`},
		{"count many", `count([1, 2, 3, 4], even) == 2`},
		{"unfold empty", `unfold(0, down) == empty`},
		{"unfold singleton", `unfold(1, down) == [1]`},
		{"unfold order", `unfold(3, down) == [3, 2, 1]`},
		{"chunksOf empty", `chunksOf(2, empty) == emptyLists`},
		{"chunksOf singleton", `chunksOf(2, [1]) == [[1]]`},
		{"chunksOf zero", `chunksOf(0, xs) == emptyLists`},
		{"chunksOf negative", `chunksOf(-1, xs) == emptyLists && chunksOf(minInt(), xs) == emptyLists`},
		{"chunksOf remainder", `chunksOf(2, xs) == [[1, 2], [3]]`},
		{"chunksOf one", `chunksOf(1, xs) == [[1], [2], [3]]`},
		{"chunksOf past length", `chunksOf(maxInt(), xs) == [xs]`},
		{"sortOn empty", `sortOn(empty, addOne) == empty`},
		{"sortOn singleton", `sortOn([2], addOne) == [2]`},
		{"sortOn key", `sortOn([3, 1, 2], func(x) => 0 - x) == [3, 2, 1]`},
		{"sortOn stable", `sortOn([(2, "a"), (1, "b"), (2, "c")], fst) == [(1, "b"), (2, "a"), (2, "c")]`},
		{"sortOn Ord dictionary", `sortOn([1, 3, 2], func(x) => Key(x)) == [3, 2, 1]`},
		{"lazy force", `force(delay(func(_) => 2 + 3)) == 5`},
		{"lazy repeat force", `force(deferred) == 7 && force(deferred) == 7`},
		{"head empty", `head(empty) == None`},
		{"head singleton", `head([2]) == Some(2)`},
		{"tail empty", `tail(empty) == empty`},
		{"tail singleton", `tail([2]) == empty`},
		{"take negative", `take(-1, xs) == empty && take(minInt(), xs) == empty`},
		{"take empty", `take(1, empty) == empty`},
		{"take singleton", `take(1, [2]) == [2]`},
		{"take past length", `take(maxInt(), xs) == xs`},
		{"drop negative", `drop(-1, xs) == xs && drop(minInt(), xs) == xs`},
		{"drop empty", `drop(1, empty) == empty`},
		{"drop singleton", `drop(1, [2]) == empty`},
		{"drop past length", `drop(maxInt(), xs) == empty`},
		{"fold empty", `fold(empty, 9, add) == 9`},
		{"fold left order", `fold(xs, 0, func(a, b) => a * 10 + b) == 123`},
		{"length boundaries", `length(empty) == 0 && length([1]) == 1`},
		{"append boundaries", `append(empty, xs) == xs && append(xs, empty) == xs`},
		{"filter boundaries", `filter(empty, never) == empty && filter([2], even) == [2] && filter([1], even) == empty`},
		{"reverse boundaries", `reverse(empty) == empty && reverse([2]) == [2] && reverse(xs) == [3, 2, 1]`},
		{"zip boundaries", `zip(empty, emptyStrings) == emptyPairs && zip(xs, ["a"]) == [(1, "a")]`},
		{"sum boundaries", `sum(empty) == 0 && sum([2]) == 2`},
		{"concatMap boundaries", `concatMap(empty, func(x) => [x]) == empty && concatMap(xs, func(x) => [x, x]) == [1, 1, 2, 2, 3, 3]`},
		{"elem boundaries", `!elem(1, empty) && elem(1, [1]) && !elem(2, [1])`},
		{"sort boundaries", `sort(empty) == empty && sort([2]) == [2] && sort([3, 1, 2]) == xs`},
		{"sortBy boundaries", `sortBy(empty, compare) == empty && sortBy([2], compare) == [2]`},
		{"sort reversed large", `sort(reverse(range(0, 10000))) == range(0, 10000)`},
		{"sortBy stability", `sortBy([(1, "a"), (0, "b"), (1, "c")], func(a, b) => compare(a.0, b.0)) == [(0, "b"), (1, "a"), (1, "c")]`},
		{"maximum boundaries", `maximum(empty) == None && maximum([2]) == Some(2)`},
		{"minimum boundaries", `minimum(empty) == None && minimum([2]) == Some(2)`},
		{"map boundaries", `map(empty, addOne) == empty && map([2], addOne) == [3]`},
		{"flatMap boundaries", `flatMap(empty, func(x) => [x]) == empty`},
		{"forEach empty", `forEach(empty, func(x) => Some(unit())) == Some(unit())`},
		{"mapM empty", `mapM(empty, func(x) => Some(x)) == Some(empty)`},
		{"mapM singleton", `mapM([2], func(x) => Some(x + 1)) == Some([3])`},
		{"splitOnce tuple bridge", `splitOnce("=", "日=🙂=x") == Some(("日", "🙂=x"))`},
		{"regexCaptures bridge", `regexCaptures("([A-Z]+) (.*)", "INFO 日🙂") == Some(["INFO", "日🙂"])`},
	}
	var source, want strings.Builder
	source.WriteString(`package main

type Ticket = Ticket(Int)
instance Eq[Ticket] { eq(a, b) { match (a, b) { (Ticket(x), Ticket(y)) => abs(x) == abs(y) } } }
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
  let deferred = delay(func(_) => 7)
  let unforced = delay(func(_) => 1 / 0)
  do {
`)
	for _, check := range checks {
		fmt.Fprintf(&source, "println(%q + show(%s))\n", check.name+": ", check.expression)
		fmt.Fprintf(&want, "%s: true\n", check.name)
	}
	source.WriteString("}\n}\n")
	pre, err := parser.Parse(Source)
	if err != nil {
		t.Fatal(err)
	}
	pre.Name = "prelude.gfx"
	user, err := parser.Parse(source.String())
	if err != nil {
		t.Fatal(err)
	}
	user.Name = "stdlib_extras_test.gfx"
	program, err := types.Check([]*ast.File{pre, user})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	// The pure surface cannot observe thunk invocation counts. Exercise the same
	// generated delay/force ABI with a counted Go thunk to prove non-memoisation.
	generated = append(generated, []byte(`
func init() {
  calls := 0
  lazy := delay_(func(_ any) any { calls++; return int64(7) })
  if calls != 0 { panic("delay evaluated its thunk") }
  if force_(lazy) != int64(7) || force_(lazy) != int64(7) || calls != 2 {
    panic("force must evaluate its thunk on every call")
  }
  pair := rt_tuple2(int64(1), "two")
  a, b := rt_untuple2(pair)
  if a != int64(1) || b != "two" { panic("tuple shape round trip") }
}
`)...)
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, generated, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", path)
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
		{"sortOn requires key Ord", `sortOn([Opaque], func(x) => x)`, "no instance Ord[Opaque]"},
		{"lookup requires key Eq", `lookup(Opaque, [(Opaque, 1)])`, "no instance Eq[Opaque]"},
		{"charAt requires Int index", `charAt("x", "0")`, "type mismatch: expected Int, got String"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pre, err := parser.Parse(Source)
			if err != nil {
				t.Fatal(err)
			}
			pre.Name = "prelude.gfx"
			user, err := parser.Parse("package main\ntype Opaque = Opaque\nfunc main() IO[Unit] { let value = " + tt.expression + "; println(\"ok\") }")
			if err != nil {
				t.Fatal(err)
			}
			user.Name = "stdlib_extras_error.gfx"
			_, err = types.Check([]*ast.File{pre, user})
			checkError, ok := err.(*types.Error)
			if !ok || checkError.File != user.Name || checkError.Pos.Line != 3 || checkError.Pos.Col <= 0 || !strings.Contains(checkError.Msg, tt.message) {
				t.Fatalf("error = %#v, want positioned %q", err, tt.message)
			}
		})
	}
}
