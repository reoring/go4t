package types_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// Keep an AST-built case for embedders that do not use the source parser.
func TestLawAST(t *testing.T) {
	files := parseFiles(t, `func main() IO[Unit] { ioPure(unit()) }`)
	law := &ast.LawDecl{
		Name:   "identity",
		Params: []ast.Param{{Name: "x", Type: &ast.TypeName{Name: "Int"}}},
		LHS:    &ast.Ident{Name: "x"}, RHS: &ast.Ident{Name: "x"},
	}
	proof := &ast.ProofDecl{Name: "identity", Params: []ast.Param{{Name: "renamed"}}, Body: &ast.TRefl{}}
	files[1].Decls = append(files[1].Decls, law, proof)
	p, err := types.Check([]*ast.Module{{Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	if *law.LHS.AnnSlot() == nil || *law.RHS.AnnSlot() == nil {
		t.Fatal("law sides were not checked")
	}
	info, checkedProof := p.Laws["identity"], p.Proofs["identity"]
	if info == nil || info.Decl != law || checkedProof == nil || checkedProof.Decl != proof ||
		info.Proof != checkedProof || checkedProof.Law != info ||
		len(info.ParamTypes) != 1 || info.ParamTypes[0] != types.Int {
		t.Fatalf("law/proof output contract: law=%+v proof=%+v", info, checkedProof)
	}
	if p.Funcs["identity"] != nil || p.Main != p.Funcs["main"] {
		t.Fatal("laws changed the runtime function namespace")
	}
}

const lawMain = "\nfunc main() IO[Unit] { ioPure(unit()) }\n"

func checkLawSource(t *testing.T, source string, named bool) (*types.Program, error) {
	t.Helper()
	files := parseFiles(t, source+lawMain)
	files[0].Name, files[1].Name = "prelude.gt", "laws.gt"
	mod := &ast.Module{Files: files}
	if named {
		mod.Path, mod.Name = "/laws", "main"
	}
	return types.Check([]*ast.Module{mod})
}

func TestLawDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		name, source, want string
	}{
		{"missing proof", `law identity(x Int) { x == x }`, "law identity has no proof"},
		{"missing law", `proof identity(x) { refl }`, "proof identity has no law identity"},
		{"duplicate law", `law identity(x Int) { x == x }; law identity(x Int) { x == x }; proof identity(x) { refl }`, "duplicate law identity"},
		{"duplicate proof", `law identity(x Int) { x == x }; proof identity(x) { refl }; proof identity(x) { refl }`, "duplicate proof identity"},
		{"law collides with function", `func identity(x Int) Int { x }; law identity(x Int) { x == x }; proof identity(x) { refl }`, "duplicate declaration identity"},
		{"function collides with law", `law identity(x Int) { x == x }; proof identity(x) { refl }; func identity(x Int) Int { x }`, "duplicate declaration identity"},
		{"law collides with method", `class C[A] { identity(x A) A }; law identity(x Int) { x == x }; proof identity(x) { refl }`, "duplicate declaration identity"},
		{"method collides with law", `law identity(x Int) { x == x }; proof identity(x) { refl }; class C[A] { identity(x A) A }`, "duplicate declaration identity"},
		{"parameter count", `law identity(x Int) { x == x }; proof identity() { refl }`, "proof identity: want 1 parameters, got 0"},
		{"duplicate law parameter", `law identity(x Int, x Int) { x == x }; proof identity(a, b) { refl }`, "duplicate parameter x"},
		{"duplicate proof parameter", `law identity(x Int, y Int) { x == y }; proof identity(a, a) { refl }`, "duplicate parameter a"},
		{"side type", `law identity(x Int) { x == true }; proof identity(x) { refl }`, "type mismatch: expected Int, got Bool"},
		{"requires type", `law identity(x Int) requires x { x == x }; proof identity(x) { refl }`, "type mismatch: expected Bool, got Int"},
		{"requires Float", `law identity(x Int) requires { let f = 1.0; true } { x == x }; proof identity(x) { refl }`, "laws do not support Float"},
		{"requires scope", `law identity(x Int) requires y == 0 { x == x }; proof identity(x) { refl }`, "unknown identifier y"},
		{"requires sample", `law identity(x Int) requires x > 0 { x == x }; proof identity(x) { sample(10) }`, "sample does not support requires"},
		{"float parameter", `law identity(x Float) { 1 == 1 }; proof identity(x) { refl }`, "laws do not support Float"},
		{"float tuple", `law identity(x (Int, Float)) { 1 == 1 }; proof identity(x) { refl }`, "laws do not support Float"},
		{"float argument", `law identity(x Option[Float]) { 1 == 1 }; proof identity(x) { refl }`, "laws do not support Float"},
		{"float list argument", `law identity(x List[Float]) { 1 == 1 }; proof identity(x) { refl }`, "laws do not support Float"},
		{"float function parameter", `law identity(f func(Float) Int) { 1 == 1 }; proof identity(f) { refl }`, "laws do not support Float"},
		{"float function result", `law identity(f func(Int) Float) { 1 == 1 }; proof identity(f) { refl }`, "laws do not support Float"},
		{"float record field", `type Box = Box{value: Float}; law identity(x Box) { x.value == x.value }; proof identity(x) { refl }`, "laws do not support Float"},
		{"float alias", `alias Number = Float; law identity(x Number) { x == x }; proof identity(x) { refl }`, "laws do not support Float"},
		{"float phantom alias", `alias Ignored[A] = Int; law identity(x Ignored[Float]) { x == x }; proof identity(x) { refl }`, "laws do not support Float"},
		{"float sides", `law identity() { 1.0 == 1.0 }; proof identity() { refl }`, "laws do not support Float"},
		{"float subterm", `law identity() { { let x = 1.0; 1 } == 1 }; proof identity() { refl }`, "laws do not support Float"},
		{"unknown variable", `law identity(x Int) { y == x }; proof identity(x) { refl }`, "unknown identifier y"},
		{"not callable", `law identity(x Int) { x == x }; proof identity(x) { refl }; func bad() Int { identity(1) }`, "unknown identifier identity"},
		{"not a value", `law identity(x Int) { x == x }; proof identity(x) { refl }; func bad() Int { identity }`, "unknown identifier identity"},
		{"primitive split", `law identity(x Int) { x == x }; proof identity(x) { match x { _ => refl } }`, "cannot split Int"},
		{"bool split", `law identity(x Bool) { x == x }; proof identity(x) { match x { _ => refl } }`, "cannot split Bool"},
		{"unknown split", `law identity(x Int) { x == x }; proof identity(x) { match y { _ => refl } }`, "match variable y is not a proof variable"},
		{"global split", `func value() Option[Int] { None }; law identity(x Int) { x == x }; proof identity(x) { match value { _ => refl } }`, "match variable value is not a proof variable"},
		{"if type", `law identity(x Int) { x == x }; proof identity(x) { if x { refl } else { refl } }`, "type mismatch: expected Bool, got Int"},
		{"unknown rewrite", `law identity(x Int) { x == x }; proof identity(x) { rewrite missing(x) { refl } }`, "unknown law missing"},
		{"function rewrite", `func value(x Int) Int { x }; law identity(x Int) { x == x }; proof identity(x) { rewrite value(x) { refl } }`, "unknown law value"},
		{"rewrite count", `law identity(x Int) { x == x }; proof identity(x) { rewrite identity() { refl } }`, "arity mismatch: want 1 arguments, got 0"},
		{"rewrite type", `law identity(x Int) { x == x }; proof identity(x) { rewrite identity(true) { refl } }`, "type mismatch: expected Int, got Bool"},
		{"proof rebinding", `law identity(original Int) { original == original }; proof identity(renamed) { rewrite identity(original) { refl } }`, "unknown identifier original"},
		{"pattern type", `law identity(x Option[Int]) { x == x }; proof identity(x) { match x { Cons(_, _) => refl } }`, "type mismatch: expected Option[Int], got List["},
		{"duplicate pattern", `law identity(x (Int, Int)) { x == x }; proof identity(x) { match x { (a, a) => refl } }`, "duplicate pattern variable a"},
		{"missing constructor", `law identity(x Option[Int]) { x == x }; proof identity(x) { match x { Some(_) => refl } }`, "non-exhaustive match; missing: None"},
		{"nested witness", `law identity(x Option[Option[Int]]) { x == x }; proof identity(x) { match x { None => refl; Some(Some(_)) => refl } }`, "non-exhaustive match; missing: Some(None)"},
		{"empty match", `law identity(x Option[Int]) { x == x }; proof identity(x) { match x {} }`, "non-exhaustive match; missing: None, Some(_)"},
		{"sibling scope", `law identity(x Option[Int]) { x == x }; proof identity(x) { match x { Some(y) => refl; None => if y == 0 { refl } else { refl } } }`, "unknown identifier y"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, named := range []bool{false, true} {
				p, err := checkLawSource(t, tt.source, named)
				if err == nil || p != nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("named=%v: got %v, want %q", named, err, tt.want)
				}
			}
		})
	}
}

func TestLawFloatBearingData(t *testing.T) {
	for _, source := range []string{
		`type Box = Box{value: Float}; law identity(x Box) { x == x }; proof identity(x) { refl }`,
		`type Chain = End | Link(Chain, Float); law identity(x Chain) { x == x }; proof identity(x) { refl }`,
		`type Box = Box{value: List[Float]}; law identity(x Box) { x == x }; proof identity(x) { refl }`,
		`type Box = Box{value: Float}; alias Wrapped = Box; law identity(x Wrapped) { x == x }; proof identity(x) { refl }`,
		`type Box = Box{value: Float}; law identity(x List[Box]) { x == x }; proof identity(x) { refl }`,
	} {
		for _, named := range []bool{false, true} {
			if _, err := checkLawSource(t, source, named); err != nil {
				t.Fatalf("named=%v: %s: %v", named, source, err)
			}
		}
	}
}

func TestLawCancelIdempotent(t *testing.T) {
	const source = `
type Order = Pending(Int) | Cancelled(Int) | Shipped(Int)
func cancel(order Order) Order {
  match order {
    Pending(n) => Cancelled(n)
    Cancelled(n) => Cancelled(n)
    Shipped(n) => Shipped(n)
  }
}
law cancelIdempotent(order Order) {
  cancel(cancel(order)) == cancel(order)
}
proof cancelIdempotent(renamed) {
  match renamed {
    Pending(pending) => refl
    Cancelled(cancelled) => refl
    Shipped(shipped) => refl
  }
}`
	p, err := checkLawSource(t, source, true)
	if err != nil {
		t.Fatal(err)
	}
	law, proof := p.Laws["cancelIdempotent"], p.Proofs["cancelIdempotent"]
	if law == nil || proof == nil || law.Proof != proof || proof.Law != law ||
		p.Owners[law.Decl.Name] != "/laws" || p.ModuleNames[law.Decl.Name] != "cancelIdempotent" {
		t.Fatalf("law output: law=%+v proof=%+v", law, proof)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("proof binders should be exempt: %v", p.Warnings)
	}
	if len(proof.PatVars) != 3 {
		t.Fatalf("pattern vars = %v", proof.PatVars)
	}
	for pat, typ := range proof.PatVars {
		if typ != types.Int {
			t.Errorf("%s has type %s", pat.Name, types.TypeString(typ))
		}
	}
	for _, decl := range []ast.Decl{law.Decl, proof.Decl} {
		ast.Walk(decl, func(n ast.Node) {
			if expr, ok := n.(ast.Expr); ok {
				assertZonked(t, expr)
			}
		})
	}
	base, err := checkLawSource(t, source[:strings.Index(source, "law cancelIdempotent")], true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Funcs) != len(base.Funcs) || p.Funcs["cancelIdempotent"] != nil ||
		p.Main != p.Funcs["main"] || types.TypeString(p.Main.Scheme.Type) != types.TypeString(base.Main.Scheme.Type) {
		t.Fatal("law declarations changed Funcs/Main")
	}
	for name, fn := range base.Funcs {
		if p.Funcs[name] == nil || types.TypeString(p.Funcs[name].Scheme.Type) != types.TypeString(fn.Scheme.Type) {
			t.Errorf("function %s changed", name)
		}
	}
}

func TestLawNestedTacticsAndIndex(t *testing.T) {
	const source = `
type Box = Box{value: Option[(Int, Bool)]}
law identity(x Int) { x == x }
proof identity(renamed) { rewrite identity(renamed) { refl } }
law nested(input Box, documented Int) { input == input }
proof nested(outer, unused) {
  match outer {
    Box{value: item} => match item {
      None => refl
      Some(pair) => match pair {
        (number, flag) => if flag {
          rewrite identity(number) { refl }
        } else {
          if unused == number { refl } else { refl }
        }
      }
    }
  }
}`
	p, err := checkLawSource(t, source, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", p.Warnings)
	}
	proof := p.Proofs["nested"]
	want := map[string]string{"item": "Option[Tuple2[Int, Bool]]", "pair": "Tuple2[Int, Bool]", "number": "Int", "flag": "Bool"}
	if len(proof.PatVars) != len(want) {
		t.Fatalf("PatVars = %v", proof.PatVars)
	}
	for pat, typ := range proof.PatVars {
		if got := types.TypeString(typ); got != want[pat.Name] {
			t.Errorf("%s: got %s, want %s", pat.Name, got, want[pat.Name])
		}
	}
	ast.Walk(proof.Decl, func(n ast.Node) {
		if expr, ok := n.(ast.Expr); ok {
			assertZonked(t, expr)
		}
		if rewrite, ok := n.(*ast.TRewrite); ok {
			ann := types.AnnOf(rewrite.Law)
			if ann.Ref != types.RefLaw || types.TypeString(ann.Type) != "func(Int) Int" {
				t.Errorf("rewrite annotation = %+v", ann)
			}
		}
	})
	index := p.PositionIndex()
	fullSource := "package main\n" + source + lawMain
	for _, tt := range []struct {
		needle, definition string
		offset, defOffset  int
	}{
		{"match item", "value: item", 6, 7},
		{"match pair", "Some(pair)", 6, 5},
		{"if flag", "number, flag", 3, 8},
		{"identity(number)", "law identity", 0, 4},
		{"identity(number)", "(number, flag)", 9, 1},
	} {
		binding := index.At("laws.gt", indexPos(fullSource, tt.needle, tt.offset))
		if binding == nil || binding.Definition == nil ||
			binding.Definition.NamePos != indexPos(fullSource, tt.definition, tt.defOffset) {
			t.Errorf("binding for %q = %+v", tt.needle, binding)
		}
	}
	var laws, proofs int
	for _, symbol := range index.Symbols("laws.gt") {
		switch symbol.Kind {
		case types.LawSymbol:
			laws++
		case types.ProofSymbol:
			proofs++
		}
	}
	if laws != 2 || proofs != 2 {
		t.Errorf("law/proof symbols = %d/%d", laws, proofs)
	}
}

func TestLawModuleOwnershipAndVisibility(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gt": `package main
import "rules"
law identity(x Int) { x == x }
func main() IO[Unit] { ioPure(unit()) }`,
		"proof.gt": `package main
import "rules"
proof identity(renamed) { rewrite rules.identity(renamed) { refl } }`,
		"rules/law.gt": `package rules
law identity(x Int) { x == x }`,
		"rules/proof.gt": `package rules
proof identity(renamed) { rewrite identity(renamed) { refl } }`,
	})
	p, err := checkModuleFixture(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Laws) != 2 || len(p.Proofs) != 2 {
		t.Fatalf("laws/proofs = %d/%d", len(p.Laws), len(p.Proofs))
	}
	entry := p.Laws["identity"]
	rewrite := entry.Proof.Decl.Body.(*ast.TRewrite)
	imported := p.Laws[rewrite.Law.Name]
	if imported == nil || imported == entry || p.ModuleNames[rewrite.Law.Name] != "identity" ||
		p.Owners[rewrite.Law.Name] == p.Owners["identity"] {
		t.Fatalf("imported law identity = %q", rewrite.Law.Name)
	}
	if types.AnnOf(rewrite.Law).Ref != types.RefLaw || imported.Proof.Law != imported ||
		p.Funcs[rewrite.Law.Name] != nil || p.Funcs["identity"] != nil {
		t.Fatal("law ownership or reference classification was lost")
	}
}

func TestLawModuleErrors(t *testing.T) {
	for _, tt := range []struct {
		name, source, extra, want string
	}{
		{"same module required", `proof identity(x) { refl }`, "", "proof identity has no law identity"},
		{"import alias required", `law local(x Int) { x == x }; proof local(x) { rewrite missing.identity(x) { refl } }`, "", "unknown import alias missing"},
		{"not implicitly imported", `law local(x Int) { x == x }; proof local(x) { rewrite identity(x) { refl } }`, "", "unknown law identity"},
		{"private law", `law local(x Int) { x == x }; proof local(x) { rewrite rules._private(x) { refl } }`, "", "_private is private to module"},
		{"file scoped import", `law local(x Int) { x == x }`, `proof local(x) { rewrite rules.identity(x) { refl } }`, "unknown import alias rules"},
		{"not an exported value", `func bad() Int { rules.identity(1) }`, "", "has no exported name identity"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sources := map[string]string{
				"main.gt": "package main\nimport \"rules\"\n" + tt.source + lawMain,
				"rules/laws.gt": `package rules
law identity(x Int) { x == x }
proof identity(x) { refl }
law _private(x Int) { x == x }
proof _private(x) { refl }`,
			}
			if tt.extra != "" {
				sources["other.gt"] = "package main\n" + tt.extra
			}
			_, err := checkModuleFixture(t, moduleFixture(t, sources))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLawOrdinaryLocalsStillWarn(t *testing.T) {
	p, err := checkLawSource(t, `
law identity(documented Int) { { let forgotten = 1; 2 } == 2 }
proof identity(alsoDocumented) { if { let ignored = 1; true } { refl } else { refl } }
`, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 2 ||
		p.Warnings[0].Msg != "forgotten is unused; prefix with _ to silence" ||
		p.Warnings[1].Msg != "ignored is unused; prefix with _ to silence" {
		t.Fatalf("warnings = %v", p.Warnings)
	}
}

func TestLawPatternASTValidation(t *testing.T) {
	for _, pattern := range []ast.Pattern{
		&ast.PLit{X: &ast.IntLit{Value: 0}},
		&ast.PAs{Name: "all", Pat: &ast.PWild{}},
		&ast.PCtor{Name: "Nil", List: true},
	} {
		files := parseFiles(t, lawMain)
		files[1].Decls = append(files[1].Decls,
			&ast.LawDecl{
				Name: "identity", Params: []ast.Param{{Name: "x", Type: &ast.TypeName{Name: "List", Args: []ast.Type{&ast.TypeName{Name: "Int"}}}}},
				LHS: &ast.Ident{Name: "x"}, RHS: &ast.Ident{Name: "x"},
			},
			&ast.ProofDecl{
				Name: "identity", Params: []ast.Param{{Name: "x"}},
				Body: &ast.TMatch{Var: &ast.Ident{Name: "x"}, Arms: []ast.TacticArm{{Pat: pattern, Body: &ast.TRefl{}}}},
			},
		)
		_, err := types.Check([]*ast.Module{{Files: files}})
		if err == nil || !strings.Contains(err.Error(), "tactic match does not support") {
			t.Errorf("%T: %v", pattern, err)
		}
	}
}

func TestLawAdditionalShapes(t *testing.T) {
	for _, source := range []string{
		// Law equality does not require an Eq instance for its result.
		`type Opaque = Opaque(Int)
law identity(x Opaque) { x == x }
proof identity(renamed) { match renamed { Opaque(documented) => refl } }`,
		// Forward, zero-argument and self law references are all legal here.
		`proof first() { rewrite second() { rewrite first() { refl } } }
law first() { 1 == 1 }
law second() { 1 == 1 }
proof second() { refl }`,
		// Deeply nested structural patterns bind fully zonked types.
		`type Box = Box{value: Option[(Int, Bool)]}
law identity(x Box) { x == x }
proof identity(x) {
  match x {
    Box{value: Some((n, flag))} => if flag { refl } else { refl }
    Box{value: None} => refl
  }
}`,
		// Catchalls are structural patterns, and may introduce a proof variable.
		`law identity(x Option[Int]) { x == x }
proof identity(x) { match x { value => match value { Some(n) => refl; None => refl } } }`,
		// Recursive data types must not loop in the Float exclusion pass.
		`type Tree[A] = Tip(A) | Branch(Tree[A], Tree[A])
law identity(x Tree[Int]) { x == x }
proof identity(x) { match x { _ => refl } }`,
		// Shadowed bindings stay separate in PatVars and inner expressions.
		`law identity(x Option[Option[Int]]) { x == x }
proof identity(x) {
  match x { Some(x) => match x { Some(x) => if x == 1 { refl } else { refl }; None => refl }; None => refl }
}`,
	} {
		p, err := checkLawSource(t, source, false)
		if err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
		if len(p.Warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", p.Warnings)
		}
		for _, proof := range p.Proofs {
			for pat, typ := range proof.PatVars {
				if strings.Contains(types.TypeString(typ), "?") {
					t.Errorf("unresolved type for %s: %s", pat.Name, types.TypeString(typ))
				}
			}
			ast.Walk(proof.Decl, func(n ast.Node) {
				if expr, ok := n.(ast.Expr); ok {
					assertZonked(t, expr)
				}
			})
		}
	}
}
