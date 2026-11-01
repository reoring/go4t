package types_test

import (
	"errors"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/types"
)

func TestPass10LetPatterns(t *testing.T) {
	for _, source := range []string{
		`func value() Int { let (a, b) = (1, 2); a + b }`,
		`func value() String { let (_a, b): (Int, String) = (1, "two"); b }`,
		`func value[A](x A) A { let (a, _): (A, Int) = (x, 1); a }`,
		`type Point = Point{x: Int, y: Int}; func value() Int { let Point{y: y, x: x} = Point(1, 2); x + y }`,
		`type Point = Point{x: Int, y: Int}; func value() Int { let Point{x: x, ..} = Point(1, 2); x }`,
		`type Box = Box(Int); func value() Int { let Box(x) = Box(3); x }`,
		`func value() Int { let _ = "ignored"; 3 }`,
		`func value() Int { let whole @ (x, _) = (1, 2); x + whole.1 }`,
		`func value() Int { let x = func(a) => a; x(1); let (x, y) = (1, 2); x + y }`,
		`func value() IO[Int] { do { let (a, b) = (1, 2); ioPure(a + b) } }`,
		`func value() IO[String] { do { let (_a, b): (Int, String) = (1, "two"); ioPure(b) } }`,
		`type Box = Box{value: Int}; func value() IO[Int] { do { let Box{value: x} = Box(3); let _ = x; ioPure(x) } }`,
		`func value() IO[Int] { do { let whole @ (x, _) = (1, 2); ioPure(x + whole.1) } }`,
		`func value() IO[Int] { do { (a, b) <- ioPure((1, 2)); _ <- ioPure("ignored"); ioPure(a + b) } }`,
		`type Box = Box{value: Int}; func value() IO[Int] { do { Box{value: x} <- ioPure(Box(3)); ioPure(x) } }`,
		`func value() IO[Int] { do { let x = func(a) => a; ioPure(x(1)); whole @ (x, _) <- ioPure((1, 2)); ioPure(x + whole.1) } }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := parseFiles(t, source+pass5Main)
			for range 2 {
				p, err := types.Check(files)
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Warnings) != 0 {
					t.Fatalf("pattern binding warnings: %v", p.Warnings)
				}
				walkExprs(reflect.ValueOf(p.Funcs["value"].Decl.Body), func(e ast.Expr) {
					assertZonked(t, e)
					if types.AnnOf(e).LetScheme != nil {
						t.Fatalf("pattern binding generalized: %#v", types.AnnOf(e).LetScheme)
					}
				})
			}
		})
	}
}

func TestPass10StateBindPattern(t *testing.T) {
	p, err := types.Check(parseFiles(t, `
type State[S, A] = State(func(S) (A, S))
instance Functor[State[S]] {
 fmap(st, f) { State(func(s) => match runState(st, s) { (a, s2) => (f(a), s2) }) }
}
instance Monad[State[S]] {
 pure(a) { State(func(s) => (a, s)) }
 bind(st, f) { State(func(s) => match runState(st, s) { (a, s2) => runState(f(a), s2) }) }
}
func runState[S, A](st State[S, A], s S) (A, S) { match st { State(run) => run(s) } }
func value() State[Int, Int] {
 do { (a, s) <- State(func(n Int) => ((n, n + 1), n + 2)); pure(a + s) }
}
`+pass5Main))
	if err != nil {
		t.Fatal(err)
	}
	do := p.Funcs["value"].Decl.Body.Result.(*ast.Do)
	ann := types.AnnOf(do)
	if types.TypeString(ann.Type) != "State[Int, Int]" || ann.Monad == nil {
		t.Fatalf("State do annotation = %#v", ann)
	}
	assertInstanceDict(t, *ann.Monad, "Monad", "State")
	walkExprs(reflect.ValueOf(do), func(e ast.Expr) { assertZonked(t, e) })
}

func TestPass10Diagnostics(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`func value() Int { let Some(x) = Some(1); x }`, "let pattern must be irrefutable"},
		{`func value() Int { let (x, Some(y)) = (1, Some(2)); x + y }`, "let pattern must be irrefutable"},
		{`func value() Int { let 1 = 1; 1 }`, "let pattern must be irrefutable"},
		{`func value() IO[Int] { do { let Some(x) = Some(1); ioPure(x) } }`, "let pattern must be irrefutable"},
		{`func value() IO[Int] { do { Some(x) <- ioPure(Some(1)); ioPure(x) } }`, "bind pattern must be irrefutable"},
		{`func value() IO[Int] { do { (x, Some(y)) <- ioPure((1, Some(2))); ioPure(x + y) } }`, "bind pattern must be irrefutable"},
		{`type Shape = Dot{x: Int} | Empty; func value() Int { let Dot{x: x} = Dot(1); x }`, "let pattern must be irrefutable"},
		{`func value() Int { let (x, x) = (1, 2); x }`, "duplicate pattern variable x"},
		{`func value() IO[Int] { do { (x, x) <- ioPure((1, 2)); ioPure(x) } }`, "duplicate pattern variable x"},
		{`func value() Unit { let (id, _) = (func(x) => x, 0); id(1); id(true); unit() }`, "type mismatch: expected Int, got Bool"},
		{`func value() IO[Unit] { do { let (id, _) = (func(x) => x, 0); ioPure(id(1)); ioPure(id(true)); ioPure(unit()) } }`, "type mismatch: expected Int, got Bool"},
		{`func value() Unit { let (a, b): (Int, String) = (1, false); unit() }`, "type mismatch: expected Tuple2[Int, String], got Tuple2[Int, Bool]"},
		{`type Token = Token deriving Eq; instance Eq[Token] { eq(a, b) { true } }`, "duplicate instance Eq[Token]"},
		{`type Token = Token deriving Ord`, "deriving Ord for Token requires an Eq instance"},
		{`type Token = Token(func(Int) Int) deriving Eq`, "cannot derive Eq for Token: field of function type"},
		{`type Token = Token{f: func(Int) Int} deriving Ord; instance Eq[Token] { eq(a, b) { true } }`, "cannot derive Ord for Token: field of function type"},
		{`type Token = Token deriving Functor`, "cannot derive Functor for Token: unknown class"},
		{`type Token = Token deriving Eq, Eq`, "duplicate deriving class Eq for Token"},
		{`type Hidden = Hidden; type Token = Token(Hidden) deriving Eq`, "no instance Eq[Hidden]; hint: add 'deriving Eq' or write 'instance Eq[Hidden]'"},
		{`type Hidden = Hidden deriving Eq; type Token = Token(Hidden) deriving Eq, Ord`, "no instance Ord[Hidden]; hint: add 'deriving Ord' or write 'instance Ord[Hidden]'"},
		{`type Hidden = Hidden; type Token = Token(Hidden) deriving Show`, "no instance Show[Hidden]; hint: write 'instance Show[Hidden]' (or 'deriving Debug' for a structural form)"},
		{`type Token = Token(func(Int) Int) deriving Show`, "no instance Show[func(Int) Int]"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			files := parseFiles(t, tt.source+pass5Main)
			files[1].Name = "pass10.gfx"
			p, err := types.Check(files)
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want || typed.File != "pass10.gfx" || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = (%v, %v), want positioned %q", p, err, tt.want)
			}
		})
	}
}

func TestPass10DerivedInstances(t *testing.T) {
	files := parseFiles(t, pass5Show+`
type Color = Red | Green | Blue deriving Eq, Ord, Show
type Entry = Empty | Entry(Int, String) deriving Eq, Ord, Show
type Record = Record{number: Int, text: String} deriving Eq, Ord, Show
type Pair[A, B] = Pair(A, B) deriving Eq, Ord, Show
func equalPair() Bool { Pair(1, "a") == Pair(2, "b") }
func orderPair() Ordering { compare(Pair(1, "a"), Pair(2, "b")) }
func showPair() String { show(Pair(1, "a")) }
`+pass5Main)
	originalDecls := len(files[1].Decls)
	positions := make(map[string]ast.Pos)
	for _, d := range files[1].Decls {
		if d, ok := d.(*ast.TypeDecl); ok {
			positions[d.Name] = d.Pos
		}
	}
	for range 2 {
		p, err := types.Check(files)
		if err != nil {
			t.Fatal(err)
		}
		if len(files[1].Decls) != originalDecls+12 {
			t.Fatalf("declaration count = %d, want %d", len(files[1].Decls), originalDecls+12)
		}
		found := make(map[string]*types.Instance)
		for _, inst := range p.Instances {
			pos, derived := positions[inst.TypeCon.Name]
			if !derived {
				continue
			}
			found[inst.Class.Name+"/"+inst.TypeCon.Name] = inst
			if inst.Decl.Pos != pos || len(inst.Decl.Methods) != 1 || inst.Decl.Methods[0].Pos != pos || inst.Decl.Methods[0].Body.Pos != pos {
				t.Fatalf("derived instance lost type position: %#v", inst.Decl)
			}
			walkExprs(reflect.ValueOf(inst.Decl), func(e ast.Expr) {
				assertZonked(t, e)
				if e.GetPos() != pos {
					t.Fatalf("derived expression position = %v, want %v", e.GetPos(), pos)
				}
			})
		}
		if len(found) != 12 {
			t.Fatalf("derived instance count = %d, want 12", len(found))
		}
		for _, class := range []string{"Eq", "Ord", "Show"} {
			inst := found[class+"/Pair"]
			if inst == nil || len(inst.Params) != 2 || len(inst.Context) != 2 {
				t.Fatalf("Pair %s instance = %#v", class, inst)
			}
			for i, param := range []string{"A", "B"} {
				if inst.Params[i].Name != param || inst.Context[i].Class != class || inst.Context[i].Arg != inst.Params[i] {
					t.Fatalf("Pair %s context = %#v", class, inst.Context)
				}
			}
		}
		for _, tt := range []struct{ name, class string }{{"equalPair", "Eq"}, {"orderPair", "Ord"}, {"showPair", "Show"}} {
			e := p.Funcs[tt.name].Decl.Body.Result
			if call, ok := e.(*ast.Call); ok {
				e = call.Fn
			}
			dicts := types.AnnOf(e).Dicts
			if len(dicts) != 1 {
				t.Fatalf("%s dictionaries = %#v", tt.name, dicts)
			}
			assertInstanceDict(t, dicts[0], tt.class, "Pair")
			if len(dicts[0].Args) != 2 {
				t.Fatalf("%s dictionary arguments = %#v", tt.name, dicts[0].Args)
			}
			assertInstanceDict(t, dicts[0].Args[0], tt.class, "Int")
			assertInstanceDict(t, dicts[0].Args[1], tt.class, "String")
		}
		assertPass10DerivedShapes(t, found)
	}
}

func assertPass10DerivedShapes(t *testing.T, instances map[string]*types.Instance) {
	t.Helper()
	body := func(key string) *ast.Match {
		t.Helper()
		inst := instances[key]
		if inst == nil {
			t.Fatalf("missing instance %s", key)
		}
		m, ok := inst.Decl.Methods[0].Body.Result.(*ast.Match)
		if !ok {
			t.Fatalf("%s body is not a match", key)
		}
		return m
	}
	for _, name := range []string{"Color", "Entry", "Record", "Pair"} {
		m := body("Eq/" + name)
		if _, ok := m.Scrut.(*ast.TupleLit); !ok {
			t.Fatalf("Eq/%s does not match a tuple", name)
		}
		if name == "Record" || name == "Pair" {
			if len(m.Arms) != 1 {
				t.Fatalf("single-constructor Eq/%s has a fallback arm", name)
			}
			and, ok := m.Arms[0].Body.(*ast.Binary)
			if !ok || and.Op != "&&" {
				t.Fatalf("Eq/%s does not compare both fields", name)
			}
		} else if fallback, ok := m.Arms[len(m.Arms)-1].Body.(*ast.BoolLit); !ok || fallback.Value {
			t.Fatalf("Eq/%s missing false fallback", name)
		}
	}
	for _, tt := range []struct {
		name  string
		arm   int
		texts []string
	}{
		{"Entry", 1, []string{"Entry(", ", ", ")"}},
		{"Record", 0, []string{"Record{number: ", ", text: ", "}"}},
		{"Pair", 0, []string{"Pair(", ", ", ")"}},
	} {
		interp, ok := body("Show/" + tt.name).Arms[tt.arm].Body.(*ast.Interp)
		if !ok || !reflect.DeepEqual(interp.Texts, tt.texts) || len(types.AnnOf(interp).ShowDicts) != 2 {
			t.Fatalf("Show/%s interpolation = %#v", tt.name, interp)
		}
	}
	for i, name := range []string{"Red", "Green", "Blue"} {
		literal, ok := body("Show/Color").Arms[i].Body.(*ast.StringLit)
		if !ok || literal.Value != name {
			t.Fatalf("Show/Color arm %d = %#v", i, literal)
		}
	}
	ord := body("Ord/Color")
	if len(ord.Arms) != 7 {
		t.Fatalf("Ord/Color arms = %d, want 7", len(ord.Arms))
	}
	for i, name := range []string{"EQ", "LT", "GT", "EQ", "LT", "GT", "EQ"} {
		id, ok := ord.Arms[i].Body.(*ast.Ident)
		if !ok || id.Name != name {
			t.Fatalf("Ord/Color arm %d = %#v, want %s", i, id, name)
		}
	}
	lex := body("Ord/Pair").Arms[0].Body.(*ast.Match)
	for range 2 {
		call, ok := lex.Scrut.(*ast.Call)
		if !ok || call.Fn.(*ast.Ident).Name != "compare" || len(lex.Arms) != 2 || lex.Arms[0].Pat.(*ast.PCtor).Name != "EQ" {
			t.Fatalf("invalid lexicographic comparison: %#v", lex)
		}
		if next, ok := lex.Arms[0].Body.(*ast.Match); ok {
			lex = next
		} else if id, ok := lex.Arms[0].Body.(*ast.Ident); !ok || id.Name != "EQ" {
			t.Fatalf("lexicographic comparison does not terminate in EQ: %#v", lex)
		}
	}
}

func TestPass10DerivingAcrossFiles(t *testing.T) {
	files := parseFiles(t, `type Token = Token(Int) deriving Ord, Show`+pass5Main)
	later, err := parser.Parse(`package main
instance Eq[Token] { eq(a, b) { match (a, b) { (Token(x), Token(y)) => x == y } } }
type Later = Later deriving Ord, Show, Eq
`)
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, later)
	for range 2 {
		p, err := types.Check(files)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, inst := range p.Instances {
			if inst.TypeCon.Name == "Later" || inst.TypeCon.Name == "Token" {
				found++
			}
		}
		if found != 6 {
			t.Fatalf("cross-file instances = %d, want 6", found)
		}
	}
}

func TestPass10DerivingRecursiveAndPhantom(t *testing.T) {
	p, err := types.Check(parseFiles(t, `
type Tree[A] = Tip | Branch(A, Tree[A], Tree[A]) deriving Eq, Ord, Show
type Phantom[A] = Phantom deriving Eq, Ord, Show
func equalTree() Bool { Branch(1, Tip, Tip) == Branch(2, Tip, Tip) }
func orderTree() Ordering { compare(Branch(1, Tip, Tip), Branch(2, Tip, Tip)) }
func showTree() String { show(Branch(1, Tip, Tip)) }
`+pass5Main))
	if err != nil {
		t.Fatal(err)
	}
	for _, inst := range p.Instances {
		if inst.TypeCon.Name != "Tree" && inst.TypeCon.Name != "Phantom" {
			continue
		}
		if len(inst.Context) != 1 || inst.Context[0].Class != inst.Class.Name || inst.Context[0].Arg != inst.Params[0] {
			t.Fatalf("derived %s[%s] context = %#v", inst.Class.Name, inst.TypeCon.Name, inst.Context)
		}
		walkExprs(reflect.ValueOf(inst.Decl), func(e ast.Expr) { assertZonked(t, e) })
	}
}
