package types_test

import (
	"errors"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestMissingPatternWitnesses(t *testing.T) {
	for _, tt := range []struct{ source, missing string }{
		{`func value(x Option[Int]) Int { match x { None => 0 } }`, "Some(_)"},
		{`func value(x Option[Option[Int]]) Int { match x { None => 0; Some(None) => 1 } }`, "Some(Some(_))"},
		{`func value(x (Bool, Int)) Int { match x { (false, _) => 0 } }`, "(true, _)"},
		{`func value(x (Option[Int], Bool)) Int { match x { (None, _) => 0; (Some(_), false) => 1 } }`, "(Some(_), true)"},
		{`func value(x Option[Int]) Int { match x { None => 0; Some(0) => 1 } }`, "Some(1)"},
		{`func value(x String) Int { match x { "" => 0 } }`, `"_"`},
		{`func value(x Option[Int]) Int { match x { _ if false => 0 } }`, "None, Some(_)"},
		{`type Many = A | B | C | D | E | F | G; func value(x Many) Int { match x { A => 0 } }`, "B, C, D, E, F"},
		{`func value(x (Int, Bool)) Int { match x { (_, false) => 0; (0, true) => 1 } }`, "(1, true)"},
		{`func value(x (String, Bool)) Int { match x { (_, false) => 0; ("", true) => 1 } }`, `("_", true)`},
		{`func value(x List[Int]) Int { match x { Nil => 0; Cons(0, _) => 1 } }`, "[1, ..]"},
		{`type Rec = Rec{left: Option[Int], right: Bool}; func value(x Rec) Int { match x { Rec{right: false, ..} => 0; Rec{left: None, right: true} => 1 } }`, "Rec(Some(_), true)"},
		{`func value(x Float) Int { match x { 0.0 => 0; 1.0 => 1 } }`, "2"},
		{`func value(x (Float, Bool)) Int { match x { (_, false) => 0; (0.0, true) => 1 } }`, "(1, true)"},
		{`func value(x (Float, Bool)) Int { match x { (_, false) => 0; (0.0, true) => 1; (1.5, false) => 2 } }`, "(1, true), (1.5, true)"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			files := parseFiles(t, tt.source+pass5Main)
			_, err := checkFixture(t, []*ast.Module{{Files: files}})
			var e *types.Error
			if !errors.As(err, &e) || !strings.HasSuffix(e.Msg, "missing: "+tt.missing) {
				t.Fatalf("Check = %v, want missing: %s", err, tt.missing)
			}
		})
	}
}

func TestInstanceDerivingHints(t *testing.T) {
	for _, class := range []string{"Show", "Debug", "Eq", "Ord"} {
		extra, expr := "", "show(Hidden)"
		switch class {
		case "Debug":
			extra, expr = "class Debug[A] { debug(a A) String }; ", "debug(Hidden)"
		case "Eq":
			expr = "Hidden == Hidden"
		case "Ord":
			expr = "Hidden < Hidden"
		}
		ret := "String"
		if class == "Eq" || class == "Ord" {
			ret = "Bool"
		}
		_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, extra+"type Hidden = Hidden; func value() "+ret+" { "+expr+" }"+pass5Main)}})
		want := "hint: add 'deriving " + class + "' or write 'instance " + class + "[Hidden]'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %s", class, err, want)
		}
	}
	for _, source := range []string{
		`type Hidden = Hidden(Int, Int); func value() String { show(Hidden(1, 2)) }`,
		`type Hidden = Empty | Hidden{value: Int}; func value() String { show(Empty) }`,
	} {
		_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, source+pass5Main)}})
		var typed *types.Error
		want := "no instance Show[Hidden]; hint: write 'instance Show[Hidden]' (or 'deriving Debug' for a structural form)"
		if !errors.As(err, &typed) || typed.Msg != want {
			t.Errorf("non-enum hint: %v, want %s", err, want)
		}
	}
	_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, `func value() String { show(unit()) }`+pass5Main)}})
	if err == nil || strings.Contains(err.Error(), "hint:") {
		t.Fatalf("builtin hint: %v", err)
	}
	_, err = checkFixture(t, []*ast.Module{{Files: parseFiles(t, `type Hidden = Hidden; instance Ord[Hidden] { compare(a, b) { EQ } }`+pass5Main)}})
	if err == nil || !strings.Contains(err.Error(), "no instance Eq[Hidden] (required superclass); hint: add 'deriving Eq' or write 'instance Eq[Hidden]'") {
		t.Fatalf("superclass hint: %v", err)
	}
}

func TestUnusedLocalWarnings(t *testing.T) {
	for _, tt := range []struct {
		source string
		unused []string
	}{
		{`func value(top Int) Int { let forgotten = 1; 2 }`, []string{"forgotten"}},
		{`func value() Int { (func(arg Int) => 1)(2) }`, []string{"arg"}},
		{`func value(x Option[Int]) Int { match x { Some(item) => 1; None => 0 } }`, []string{"item"}},
		{`func value() Int { let (first, second) = (1, 2); first }`, []string{"second"}},
		{`func value() Int { let whole @ (first, _) = (1, 2); first }`, []string{"whole"}},
		{`func value() IO[Int] { do { ignored <- ioPure(1); let forgotten = 2; ioPure(3) } }`, []string{"ignored", "forgotten"}},
		{`func value() Int { let x = 1; let x = 2; x }`, []string{"x"}},
		{`func value() Int { let x = 1; let f = func(x Int) => x; f(2) }`, []string{"x"}},
		{`func value() Int { let _ignored = 1; (func(_arg Int) => 2)(3) }`, nil},
		{`func value() Int { let (_first, second) = (1, 2); second }`, nil},
		{`func value() Int { let _whole @ (_first, second) = (1, 2); second }`, nil},
		{`func value() IO[Int] { do { (_first, second) <- ioPure((1, 2)); ioPure(second) } }`, nil},
		{`type Pair = Pair{left: Int, right: Int}; func value() Int { let Pair{right: unused, left: used} = Pair(1, 2); used }`, []string{"unused"}},
		{`func value() Int { let x = 1; let f = func() => x; f() }`, nil},
		{`func value(x Option[Int]) Int { match x { Some(item) if item > 0 => 1; _ => 0 } }`, nil},
		{`func value() Int { let loop = func(n Int) => if n == 0 { 0 } else { loop(n - 1) }; 1 }`, nil},
		{`func value() IO[Int] { do { let loop = func(n Int) => if n == 0 { 0 } else { loop(n - 1) }; ioPure(1) } }`, nil},
		{`func value() Int { let loop = func(loop Int) => loop; 1 }`, []string{"loop"}},
		{`func value() Int { func local(arg Int) Int { 1 }; local(2) }`, []string{"arg"}},
		{`func value() IO[Int] { do { func local(arg Int) Int { 1 }; ioPure(local(2)) } }`, []string{"arg"}},
		{`func value(top Int) Int { func local(_arg Int) Int { top }; local(2) }`, nil},
		{`func value() Int { let f = (_ + 1); 2 |> f }`, nil},
		{`func value() Int { let shadow = 1; func shadow() Int { 2 }; shadow() }`, []string{"shadow"}},
		{`func value() IO[Int] { do { let shadow = 1; func shadow() Int { 2 }; ioPure(shadow()) } }`, []string{"shadow"}},
	} {
		t.Run(tt.source, func(t *testing.T) {
			files := parseFiles(t, tt.source+pass5Main)
			files[1].Name = "user.gt"
			for range 2 {
				p, err := checkFixture(t, []*ast.Module{{Files: files}})
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Warnings) != len(tt.unused) {
					t.Fatalf("warnings = %v, want %v", p.Warnings, tt.unused)
				}
				for i, name := range tt.unused {
					w := p.Warnings[i]
					if w.Msg != name+" is unused; prefix with _ to silence" {
						t.Errorf("warning = %s", w.Msg)
					}
					if w.File != "user.gt" || w.Pos.Line != 2 || w.End.Line != 2 ||
						w.End.Col-w.Pos.Col != len(name) ||
						tt.source[w.Pos.Col-1:w.End.Col-1] != name {
						t.Errorf("warning span = %#v", w)
					}
				}
			}
		})
	}
}

func TestDiagnosticEndPositions(t *testing.T) {
	files := parseFiles(t, "func value() Int { \"long text\" }"+pass5Main)
	_, err := checkFixture(t, []*ast.Module{{Files: files}})
	var e *types.Error
	if !errors.As(err, &e) {
		t.Fatal(err)
	}
	value := files[1].Decls[0].(*ast.FuncDecl).Body.Result
	if e.End != value.GetEnd() || e.End.Col-e.Pos.Col != len(`"long text"`) {
		t.Fatalf("error span = %#v", e)
	}
}

func TestDeclarationDiagnosticEnd(t *testing.T) {
	for _, source := range []string{
		"type Broken = Broken deriving Unknown",
		"type Broken = Broken deriving Eq, Eq",
		"func main() Int { 1234 }",
	} {
		files := parseFiles(t, source)
		_, err := checkFixture(t, []*ast.Module{{Files: files}})
		var e *types.Error
		if !errors.As(err, &e) || e.End != files[1].Decls[0].GetEnd() {
			t.Fatalf("%s: diagnostic span = %#v", source, e)
		}
	}
}
