package types_test

import (
	"errors"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestIOTailLift(t *testing.T) {
	for _, tc := range []struct{ name, result, body string }{
		{"function tail", "IO[Int]", `42`},
		{"pure call", "IO[Unit]", `unit()`},
		{"inferred call result", "IO[Int]", `identity(42)`},
		{"if arms", "IO[Int]", `if true { 1 } else { ioPure(2) }`},
		{"match arms", "IO[Int]", `match true { true => 1; false => do { ioPure(0); 2 } }`},
		{"pure match", "IO[Option[Int]]", `match true { true => None; false => Some(2) }`},
		{"do result", "IO[Int]", `do { n <- ioPure(1); n + 1 }`},
		{"resolved do head", "IO[Int]", `let action = do { ioPure(1); 2 }; action`},
		{"annotated let", "IO[Int]", `let action: IO[Int] = 42; action`},
		{"annotated do let", "IO[Int]", `do { let action: IO[Int] = 42; action }`},
		{"lambda body", "func(Int) IO[Int]", `func(n) => n + 1`},
		{"local function", "IO[Int]", `func action() IO[Int] { 42 }; action()`},
		{"contextual constructor", "IO[Option[func(Sale) String]]", `Some(_.product)`},
		{"contextual record", "IO[Key[func(Sale) String]]", `Key{key: _.product}`},
		{"contextual lambda", "IO[func(Sale) String]", `func(s) => s.product`},
		{"local shadow", "IO[Int]", `let ioPure = func(n Int) => false; 42`},
		{"IO alias", "Action", `42`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, expectedSale+"\nalias Action = IO[Int]\nfunc value() "+tc.result+" { "+tc.body+" }"+pass5Main)}})
			if err != nil {
				t.Fatal(err)
			}
			if got := types.TypeString(types.AnnOf(p.Funcs["value"].Decl.Body.Result).Type); got != tc.result {
				t.Fatalf("type = %s, want %s", got, tc.result)
			}
		})
	}
}

func TestIOTailLiftBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"argument", `func consume(x IO[Int]) IO[Int] { x }; func value() IO[Int] { consume(1) }`, "type mismatch: expected IO[Int], got Int"},
		{"operator", `func value() IO[Bool] { ioPure(1) == 1 }`, "type mismatch: expected IO[Int], got Int"},
		{"list element", `func value() List[IO[Int]] { [1] }`, "type mismatch: expected List[IO[Int]], got List[Int]"},
		{"record field", `type Box = Box{action: IO[Int]}; func value() Box { Box{action: 1} }`, "type mismatch: expected IO[Int], got Int"},
		{"unresolved variable", `func value() IO[Int] { let f = func(x) => { let action: IO[Int] = x; x + 1 }; f(1) }`, "type mismatch: expected IO[Int], got Int"},
		{"nested IO", `func value() IO[Int] { let action = ioPure(ioPure(1)); action }`, "type mismatch: expected IO[Int], got IO[IO[Int]]"},
		{"rigid variable", `func value[A](x A) IO[A] { x }`, "type mismatch: expected IO[A], got A"},
		{"no extra IO layer", `func value() IO[IO[Int]] { ioPure(1) }`, "type mismatch: expected IO[Int], got Int"},
		{"literal diagnostic", `func value() IO[String] { 1 }`, "type mismatch: expected IO[String], got Int"},
		{"call diagnostic", `func value() IO[String] { unit() }`, "type mismatch: expected IO[String], got Unit"},
		{"other monad", `func value() Option[Int] { do { n <- Some(1); n } }`, "type mismatch: expected Option[Int], got Int"},
		{"unknown monad", `func value[M[_] Monad]() M[Int] { 1 }`, "type mismatch: expected M[Int], got Int"},
		{"do diagnostic", `func value() IO[String] { do { ioPure(0); 1 } }`, "type mismatch: expected IO[String], got Int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tc.source+pass5Main)}})
			var diagnostic *types.Error
			if p != nil || !errors.As(err, &diagnostic) || diagnostic.Msg != tc.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tc.want)
			}
		})
	}
}

func TestIOTailLiftPreludeIdentity(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gt": "package main\nfunc ioPure(n Int) Bool { false }\nfunc value() IO[Int] { 42 }\nfunc main() IO[Unit] { unit() }",
	})
	p, err := checkModuleFixture(t, root)
	if err != nil {
		t.Fatal(err)
	}
	call, ok := p.Funcs["value"].Decl.Body.Result.(*ast.Call)
	if !ok {
		t.Fatalf("lift = %T, want ordinary call", p.Funcs["value"].Decl.Body.Result)
	}
	id, ok := call.Fn.(*ast.Ident)
	if !ok || id.Name != "ioPure" || types.AnnOf(id).Ref != types.RefFunc {
		t.Fatalf("callee = %#v, want resolved prelude ioPure", call.Fn)
	}
	if p.Funcs[id.Name].Decl.Extern == nil || p.Funcs[id.Name].Decl.Extern.Name != "Pure" {
		t.Fatal("synthetic call did not bind the prelude primitive")
	}
}

func TestIOTailLiftUnresolvedVariable(t *testing.T) {
	p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, `
func value() IO[Int] {
  let f = func(x) => { let action: IO[Int] = x; action }
  f(ioPure(1))
}
`+pass5Main)}})
	if err != nil {
		t.Fatal(err)
	}
	ast.Walk(p.Funcs["value"].Decl, func(node ast.Node) {
		if expr, ok := node.(ast.Expr); ok && types.AnnOf(expr).ImplicitIO {
			t.Fatal("unresolved x acquired an implicit IO layer")
		}
	})
}
