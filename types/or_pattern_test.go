package types_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/format"
	"github.com/reoring/go4t/types"
)

func TestOrPatternChecking(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`type Color = A | B | C; func value(x Color) Int { match x { A | B | C => 1 } }`, ""},
		{`type Color = A | B | C; func value(x Color) Int { match x { A | B => 1 } }`, "non-exhaustive match; missing: C"},
		{`type Color = A | B; func value(x Option[Color]) Int { match x { Some(A | B) => 1; None => 0 } }`, ""},
		{`type Color = A | B | C; func value(x Option[Color]) Int { match x { Some(A | B) => 1; None => 0 } }`, "non-exhaustive match; missing: Some(C)"},
		{`type Color = A | B; func value(x Color) Int { match x { A | B if true => 1 } }`, "non-exhaustive match; missing: A, B"},
		{`type Color = A | B; func value(x Color) Int { match x { A | A | B => 1 } }`, ""},
		{`type Choice = Left(Int) | Right(Int); func value(x Choice) Int { match x { Left(n) | Right(n) => n } }`, ""},
		{`type Choice = Left(Int) | Right(Int); func value(x Choice) Int { match x { Left(n) | Right(m) => n } }`, "or-pattern alternatives must bind the same variables; m is not bound in every alternative"},
		{`func value(x Option[Int]) Int { match x { Some(n) | None => n } }`, "or-pattern alternatives must bind the same variables; n is not bound in every alternative"},
		{`type Choice = Left(Int) | Right(String); func value(x Choice) Int { match x { Left(n) | Right(n) => n } }`, "type mismatch: expected Int, got String"},
		{`func value(x (Int, Int)) Int { match x { (n | n, n) => n } }`, "duplicate pattern variable n"},
		{`func value() Int { let (n | n) = 1; n }`, "or-patterns are only allowed in match arms"},
		{`func value() Int { let n | n = 1; n }`, "or-patterns are only allowed in match arms"},
		{`func value() Int { let Some(n | n) = Some(1); n }`, "or-patterns are only allowed in match arms"},
		{`func value() Int { (func((n | n)) => n)(1) }`, "or-patterns are only allowed in match arms"},
		{`func value() Int { (func(n | n) => n)(1) }`, "or-patterns are only allowed in match arms"},
		{`func value() IO[Int] { do { (n | n) <- ioPure(1); ioPure(n) } }`, "or-patterns are only allowed in match arms"},
		{`func value() IO[Int] { do { n | n <- ioPure(1); ioPure(n) } }`, "or-patterns are only allowed in match arms"},
		{`func value(x Int) Int { match x { -1 | -2 => 1; _ => 0 } }`, ""},
		{`func value(x Float) Int { match x { -2.5 | -1.5 => 1; _ => 0 } }`, ""},
		{`func value(x String) Int { match x { "a" | "b" => 1; _ => 0 } }`, ""},
	} {
		t.Run(tt.source, func(t *testing.T) {
			_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tt.source+pass5Main)}})
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var typed *types.Error
			if !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestOrPatternExpansionAndHover(t *testing.T) {
	source := `type Choice = Left(Int) | Right(Int)
func value(x Choice) Int { match x { Left(n) | Right(n) => n } }
func pair(x (Bool, Bool)) Int { match x { (true | false, true | false) => 1 } }
type Row = Row{value: Bool, other: Int}
func record(x Row) Int { match x { Row{value: true | false, other: n} => n } }
` + pass5Main
	files := parseFiles(t, source)
	before := format.File(files[1])
	p, err := checkFixture(t, []*ast.Module{{Path: "patterns", Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	if got := format.File(files[1]); string(got) != string(before) {
		t.Fatal("checking changed the caller's source patterns")
	}
	for name, count := range map[string]int{"value": 2, "pair": 4, "record": 2} {
		m := p.Funcs[name].Decl.Body.Result.(*ast.Match)
		if len(m.Arms) != count {
			t.Fatalf("%s: arms = %d, want %d", name, len(m.Arms), count)
		}
		for _, arm := range m.Arms {
			if arm.Body != m.Arms[0].Body {
				t.Fatal("expanded arms do not share the checked body")
			}
			ast.Walk(arm.Pat, func(n ast.Node) {
				if _, ok := n.(*ast.POr); ok {
					t.Fatal("checked pattern retains POr")
				}
				if record, ok := n.(*ast.PRecord); ok && p.RecordPats[record].Ctor == nil {
					t.Fatal("expanded record has no annotation")
				}
			})
		}
	}
	for _, warning := range p.Warnings {
		if strings.Contains(warning.Msg, "n is unused") {
			t.Fatal(warning)
		}
	}
	index := p.PositionIndex()
	for _, needle := range []string{"Left(n)", "Right(n)"} {
		// parseFiles prefixes the package clause.
		binding := index.At(files[1].Name, indexPos("package main\n"+source, needle, strings.Index(needle, "n")))
		if binding == nil || types.TypeString(binding.Type) != "Int" {
			t.Fatalf("hover %s: %+v", needle, binding)
		}
	}
}

func TestOrPatternQualifiedConstructors(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gt": `package main
import "model"
func value(x model.Choice) Int { match x { model.Left(n) | model.Right(n) => n } }
func nested(x model.Row) Int { match x { model.Row{color: model.A | model.B} => 1 } }
func main() IO[Unit] { ioPure(unit()) }
`,
		"model/model.gt": `package model
type Choice = Left(Int) | Right(Int)
type Color = A | B
type Row = Row{color: Color}
`,
	})
	if _, err := checkModuleFixture(t, root); err != nil {
		t.Fatal(err)
	}
}

func TestOrPatternUnnamedRecheck(t *testing.T) {
	for _, source := range []string{
		`type Choice = Left(Int) | Right(Int); func value(x Choice) Int { match x { Left(n) | Right(n) => 1 } }`,
		`type Choice = Left(Int) | Right(Int) | Empty; func value(x Choice) Int { match x { Left(n) | Right(n) => n } }`,
	} {
		files := parseFiles(t, source+pass5Main)
		input := []*ast.Module{{Files: files}}
		before := string(format.File(files[1]))
		var first string
		for attempt := range 2 {
			p, err := checkFixture(t, input)
			diagnostics := ""
			if err != nil {
				diagnostics = err.Error()
			} else {
				for _, warning := range p.Warnings {
					diagnostics += warning.Error() + "\n"
				}
				m := p.Funcs["value"].Decl.Body.Result.(*ast.Match)
				if len(m.Arms) != 2 || len(m.SourceArms) != 1 {
					t.Fatalf("expanded/written arms = %d/%d", len(m.Arms), len(m.SourceArms))
				}
				if _, ok := m.SourceArms[0].Pat.(*ast.POr); !ok {
					t.Fatal("written or-pattern lost")
				}
			}
			if got := string(format.File(files[1])); got != before {
				t.Fatalf("check %d changed formatting:\n%s\nwant:\n%s", attempt, got, before)
			}
			if attempt == 0 {
				first = diagnostics
			} else if diagnostics != first {
				t.Fatalf("recheck diagnostics changed:\n%s\nwant:\n%s", diagnostics, first)
			}
		}
	}
}
