package types_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

const recursionMessage = " recurses in a non-tail position on a non-structural argument; depth is unbounded and stack exhaustion cannot be recovered. Use an accumulator so the call is a tail call"

func TestRecursionWarnings(t *testing.T) {
	for _, tc := range []struct {
		name, source, offending string
	}{
		{"arithmetic", `func sumTo(n Int) Int { if n == 0 { 0 } else { n + sumTo(n - 1) } }`, "sumTo(n - 1)"},
		{"one per function", `func sumTo(n Int) Int { if n == 0 { 0 } else { sumTo(n - 1) + sumTo(n - 2) } }`, "sumTo(n - 1)"},
		{"accumulator", `func sumTo(n Int, acc Int) Int { if n == 0 { acc } else { sumTo(n - 1, acc + n) } }`, ""},
		{"list", `func sumTo(xs List[Int], f func(Int) Int) List[Int] { match xs { Nil => Nil; Cons(x, r) => Cons(f(x), sumTo(r, f)) } }`, ""},
		{"tree", `type Tree = Leaf | Branch(Tree, Tree); func sumTo(t Tree) Int { match t { Leaf => 0; Branch(l, r) => 1 + sumTo(l) + sumTo(r) } }`, ""},
		{"tuple and alias", `func sumTo(xs List[Int], n Int) Int { match (xs, n) { (Nil, _) => 0; (Cons(_, r), k) => { let next = r; 1 + sumTo(next, k + 1) } } }`, ""},
		{"record", `type Tree = Leaf | Branch{left: Tree, right: Tree}; func sumTo(t Tree) Int { match t { Leaf => 0; Branch{right: r, ..} => 1 + sumTo(r) } }`, ""},
		{"unchanged prefix", `func sumTo(n Int, xs List[Int]) Int { match xs { Nil => n; Cons(_, r) => n + sumTo(n, r) } }`, ""},
		{"changed prefix", `func sumTo(n Int, xs List[Int]) Int { match xs { Nil => n; Cons(_, r) => n + sumTo(n - 1, r) } }`, ""},
		{"take shaped", `func sumTo(n Int, xs List[Int]) List[Int] { if n <= 0 { Nil } else { match xs { Nil => Nil; Cons(x, r) => Cons(x, sumTo(n - 1, r)) } } }`, ""},
		{"wrong parameter", `func sumTo(xs List[Int], ys List[Int]) Int { match xs { Nil => 0; Cons(_, r) => 1 + sumTo(ys, r) } }`, "sumTo(ys, r)"},
		{"reconstruction", `func sumTo(xs List[Int]) Int { match xs { Nil => 0; Cons(x, r) => 1 + sumTo(Cons(x, r)) } }`, "sumTo(Cons(x, r))"},
		{"shadowed provenance", `func sumTo(xs List[Int]) Int { match xs { Nil => 0; Cons(_, r) => { let r = xs; 1 + sumTo(r) } } }`, "sumTo(r)"},
		{"mutual", `func sumTo(n Int) Int { n + other(n - 1) }; func other(n Int) Int { n + sumTo(n - 1) }`, ""},
		{"local lambda", `func sumTo(n Int) Int { let loop = func(k Int) => if k == 0 { 0 } else { k + loop(k - 1) }; loop(n) }`, ""},
		{"local function", `func sumTo(n Int) Int { func loop(k Int) Int { if k == 0 { 0 } else { k + loop(k - 1) } }; loop(n) }`, ""},
		{"closure boundary", `func sumTo(n Int) Int { let f = func(k Int) => k + sumTo(k - 1); f(n) }`, ""},
		{"tail branches and let", `func sumTo(n Int) Int { match n { 0 => 0; _ => { let k = n - 1; if k == 0 { 0 } else { sumTo(k) } } } }`, ""},
		{"let initializer", `func sumTo(n Int) Int { let x = sumTo(n - 1); x }`, "sumTo(n - 1)"},
		{"tail arguments", `func sumTo(n Int) Int { sumTo(sumTo(n - 1)) }`, "sumTo(n - 1)"},
		{"guard", `func sumTo(n Int) Int { match n { _ if sumTo(n - 1) == 0 => 0; _ => n } }`, "sumTo(n - 1)"},
		{"IO tail", `func sumTo(n Int) IO[Int] { if n == 0 { 0 } else { do { let k = n - 1; x <- ioPure(k); sumTo(x) } } }`, ""},
		{"IO bind", `func sumTo(n Int) IO[Int] { do { x <- sumTo(n - 1); ioPure(x + n) } }`, "sumTo(n - 1)"},
		{"dictionary passthrough", `func sumTo[A Show](n Int, a A) String { if n == 0 { show(a) } else { sumTo(n - 1, a) } }`, ""},
		{"dictionary change", `func sumTo[A Show](n Int, a A) String { if n == 0 { show(a) } else { sumTo(n - 1, n) } }`, "sumTo(n - 1, n)"},
		{"tuple reassembly", `func sumTo(xs List[Int]) Int { let (ys, _) = (xs, 0); 1 + sumTo(ys) }`, "sumTo(ys)"},
		{"tuple projection", `func sumTo(pair (Int, Int)) Int { 1 + sumTo((pair.0, pair.1)) }`, "sumTo((pair.0, pair.1))"},
		{"constructor result", `func sumTo(n Int) Option[Int] { Some(match sumTo(n - 1) { Some(x) => x; None => n }) }`, "sumTo(n - 1)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "package main\n" + tc.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n"
			file, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			file.Name = "recursion.gt"
			mods, err := modules.LoadFiles([]*ast.File{file}, ".")
			if err != nil {
				t.Fatal(err)
			}
			p, err := types.Check(mods)
			if err != nil {
				t.Fatal(err)
			}
			var warnings []*types.Error
			for _, w := range p.Warnings {
				if strings.Contains(w.Msg, "recurses in a non-tail position") {
					warnings = append(warnings, w)
				}
			}
			if tc.offending == "" {
				if len(warnings) != 0 {
					t.Fatal(warnings)
				}
				return
			}
			if len(warnings) != 1 {
				t.Fatalf("warnings = %v", warnings)
			}
			w := warnings[0]
			col := strings.Index(tc.source, tc.offending) + 1
			if w.Msg != "sumTo"+recursionMessage || w.File != file.Name || w.Pos != (ast.Pos{Line: 2, Col: col}) || w.End != (ast.Pos{Line: 2, Col: col + len(tc.offending)}) {
				t.Fatalf("warning = %#v, want call at 2:%d", w, col)
			}
		})
	}
}

func TestRecursionWarningSourceName(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"go4t.mod":       "module example\n",
		"main.gt":        "package main\nimport \"example/count\"\nfunc main() IO[Unit] { println(show(count.sumTo(3))) }\n",
		"count/count.gt": "package count\nfunc sumTo(n Int) Int { if n == 0 { 0 } else { n + sumTo(n - 1) } }\n",
	})
	p, err := checkModuleFixture(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 1 || p.Warnings[0].Msg != "sumTo"+recursionMessage {
		t.Fatalf("qualified source warning: %v", p.Warnings)
	}
}
