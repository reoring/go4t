package laws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/modules"
	"go4t/types"
)

func loadSource(t *testing.T, source string) *types.Program {
	t.Helper()
	t.Setenv("GO4T_PATH", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "laws.gt"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := modules.Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func loadFixture(t *testing.T, name string) *types.Program {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return loadSource(t, string(source))
}

func diagnosticsByLaw(t *testing.T, p *types.Program) map[string]*types.Error {
	t.Helper()
	result := make(map[string]*types.Error)
	for _, diagnostic := range CheckProgram(p) {
		if diagnostic.File == "" || diagnostic.Pos.Line == 0 || diagnostic.End == (ast.Pos{}) {
			t.Errorf("unpositioned diagnostic: %+v", diagnostic)
		}
		name, _, ok := strings.Cut(strings.TrimPrefix(diagnostic.Msg, "law "), ": ")
		if !ok {
			name, _, ok = strings.Cut(strings.TrimPrefix(diagnostic.Msg, "law "), " depends on ")
		}
		if !ok {
			t.Fatalf("missing law name: %v", diagnostic)
		}
		if result[name] != nil {
			t.Fatalf("duplicate diagnostic for %s", name)
		}
		result[name] = diagnostic
	}
	return result
}

func assertNormalForms(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a failed proof")
	}
	for _, marker := range []string{"LHS: ", "RHS: "} {
		if !strings.Contains(err.Error(), marker) {
			t.Errorf("missing %q in diagnostic: %v", marker, err)
		}
	}
}

func TestCancel(t *testing.T) {
	for _, tc := range []struct{ file, bad, left, right string }{
		{"cancel.gt", "", "", ""},
		{"cancel_bad1.gt", "cancelIdempotent", "LHS: Pending(n#", "RHS: Cancelled(n#"},
		{"cancel_bad2.gt", "cancelPending", "LHS: Cancelled(0)", "RHS: Cancelled(n#"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			errors := diagnosticsByLaw(t, loadFixture(t, tc.file))
			if tc.bad == "" {
				if len(errors) != 0 {
					t.Fatal(errors)
				}
				return
			}
			if len(errors) != 1 {
				t.Fatalf("diagnostics = %v", errors)
			}
			err := errors[tc.bad]
			assertNormalForms(t, err)
			if !strings.Contains(err.Msg, tc.left) || !strings.Contains(err.Msg, tc.right) {
				t.Fatal(err)
			}
		})
	}
}

func TestFreshness(t *testing.T) {
	if errors := CheckProgram(loadFixture(t, "freshness.gt")); errors != nil {
		t.Fatal(errors)
	}
}

func TestProjection(t *testing.T) {
	if errors := CheckProgram(loadFixture(t, "projection.gt")); errors != nil {
		t.Fatal(errors)
	}
	errors := diagnosticsByLaw(t, loadFixture(t, "projection_bad.gt"))
	if len(errors) != 1 {
		t.Fatalf("diagnostics = %v", errors)
	}
	if err := errors["stampSelfEqual"]; err == nil || err.Msg != "law stampSelfEqual: Float is not supported in laws" {
		t.Fatalf("stamp equality diagnostic = %v", err)
	}
}

func TestLawDataTypeBoundary(t *testing.T) {
	for _, tc := range []struct{ name, declarations, want string }{
		{"float record", `type Box = Box{value: Float}`, ""},
		{"float recursive", `type Box = End | Link(Box, Float)`, ""},
		{"float nested argument", `type Box = Box{values: List[Float]}`, ""},
		{"io record", `type Box = Box{value: IO[Int]}`, "law depends on IO"},
		{"io after float", `type Box = Box{value: Float, effect: IO[Int]}`, "law depends on IO"},
		{"io recursive", `type Box = End | Link(Box, IO[Int])`, "law depends on IO"},
		{"io nested argument", `type Effect = Effect(IO[Int]); type Box = Box(List[Effect])`, "law depends on IO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := loadSource(t, "package main\n"+tc.declarations+"\nlaw identity(x Box) { x == x }\nproof identity(x) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			errors := CheckProgram(p)
			if tc.want == "" {
				if errors != nil {
					t.Fatal(errors)
				}
				return
			}
			if len(errors) != 1 || !strings.Contains(errors[0].Msg, tc.want) {
				t.Fatalf("diagnostics = %v, want %s", errors, tc.want)
			}
		})
	}
}

func TestLawFloatReadingDependency(t *testing.T) {
	p := loadSource(t, `package main
type Box = Box{value: Float}
func read(box Box) Int { let ignored = box.value; 1 }
law identity(box Box) { read(box) == 1 }
proof identity(box) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	errors := CheckProgram(p)
	if len(errors) != 1 || errors[0].Msg != "law identity: Float is not supported in laws" {
		t.Fatalf("Float-reading dependency diagnostic = %v", errors)
	}
}

func TestLawFloatTypeArguments(t *testing.T) {
	e := newEvaluator(loadFixture(t, "projection.gt"))
	for _, typ := range []types.Type{
		types.Float,
		types.App(&types.TCon{Name: "List"}, types.Float),
		types.App(&types.TCon{Name: "Option"}, types.Float),
		&types.TFunc{Params: []types.Type{types.Float}, Ret: types.Int},
		&types.TFunc{Params: []types.Type{types.Int}, Ret: types.Float},
	} {
		if got := e.unsupportedType(typ, false, make(map[string]bool)); got != "Float is not supported in laws" {
			t.Errorf("%s: got %q", typ, got)
		}
	}
	literal := &ast.FloatLit{Value: 1}
	*literal.AnnSlot() = &types.Ann{Type: types.Float}
	if _, err := e.evalExpr(literal, nil, nil); err == nil || err.Error() != "unsupported law expression *ast.FloatLit" {
		t.Fatalf("Float literal unexpectedly supported: %v", err)
	}
}

func TestRejectedDependencies(t *testing.T) {
	errors := diagnosticsByLaw(t, loadFixture(t, "rejected.gt"))
	for name, want := range map[string]string{
		"foreignDependency": "law depends on extern example.invalid/foreign.Read",
		"clonedPrimitive":   "law depends on extern go4t/rt.EqInt",
		"strictArgument":    "law depends on extern example.invalid/foreign.Read",
		"higherOrder":       "higher-order", "io": "law depends on IO",
		"loopDependency": "depends on loop which may not terminate",
		"divZero":        "division by zero", "divVariable": "nonconstant divisor",
	} {
		if err := errors[name]; err == nil || !strings.Contains(err.Msg, want) {
			t.Errorf("%s: got %v, want %s", name, err, want)
		}
	}
}

func TestEvaluatorSemantics(t *testing.T) {
	p := loadFixture(t, "semantics.gt")
	errors := diagnosticsByLaw(t, p)
	bad := map[string]bool{"notCaptured": true, "ignoreEq": true, "unknownFirst": true, "unknownGuard": true, "booleans": true, "letRecursive": true, "letRecursiveGeneric": true, "localFunctions": true}
	for name := range p.Laws {
		if (errors[name] != nil) != bad[name] {
			t.Errorf("%s: unexpected result %v", name, errors[name])
		}
	}
	for _, name := range []string{"notCaptured", "ignoreEq", "unknownFirst", "unknownGuard"} {
		assertNormalForms(t, errors[name])
	}
}

func TestIfGuards(t *testing.T) {
	p := loadFixture(t, "guard.gt")
	errors := diagnosticsByLaw(t, p)
	if len(errors) != 2 {
		t.Fatalf("diagnostics = %v", errors)
	}
	assertNormalForms(t, errors["wrongStale"])
	assertNormalForms(t, errors["wrongElse"])
}

func TestTacticPositionsAndRebinding(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"rebound", "if renamed >= 0 { refl } else { refl }", ""},
		{"decided", "if true { refl } else { refl }", "condition already decided"},
		{"repeated", "if renamed >= 0 { if renamed >= 0 { refl } else { refl } } else { refl }", "condition already decided"},
		{"rewrite", "rewrite identity(renamed + 0) { refl }", "induction hypothesis must structurally decrease"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := loadSource(t, "package main\nlaw identity(original Int) { original + 1 == 1 + original }\nproof identity(renamed) { "+tc.body+" }\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			errors := CheckProgram(p)
			if tc.want == "" {
				if errors != nil {
					t.Fatal(errors)
				}
				return
			}
			if len(errors) != 1 || !strings.Contains(errors[0].Msg, tc.want) {
				t.Fatalf("diagnostics = %v", errors)
			}
			assertNormalForms(t, errors[0])
			if errors[0].Pos.Line != 3 || errors[0].Pos.Col <= p.Proofs["identity"].Decl.Pos.Col {
				t.Fatalf("not positioned at tactic: %v", errors[0])
			}
		})
	}
}

func TestNestedPatternSplits(t *testing.T) {
	p := loadSource(t, `package main
 type R = R{pair: (Option[Int], List[Int]), unused: Int}
 law identity(r R) { r == r }
 proof identity(renamed) {
   match renamed {
     R{pair: (None, Nil), ..} => refl
     R{pair: (None, Cons(_, _)), ..} => refl
     R{pair: (Some(_), rest), ..} => match rest { Nil => refl; Cons(_, _) => refl }
   }
 }
 law aliased(xs List[Int]) { xs == xs }
 proof aliased(xs) { match xs { same => match same { Nil => refl; Cons(_, _) => refl } } }
 func main() IO[Unit] { ioPure(unit()) }
 `)
	if errors := CheckProgram(p); errors != nil {
		t.Fatal(errors)
	}
}

func TestFuelBackstop(t *testing.T) {
	p := loadSource(t, "package main\nlaw identity(n Int) { n + 1 == 1 + n }\nproof identity(n) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }\n")
	e := newEvaluator(p)
	law, err := e.extractLaw("identity")
	if err != nil {
		t.Fatal(err)
	}
	env, vars := e.proofEnvironment(law, p.Proofs[law.Name])
	e.fuel = 1
	if err := e.prove(law, p.Proofs[law.Name].Decl.Body, env, vars); err == nil || !strings.Contains(err.Error(), "normalization incomplete") {
		t.Fatalf("fuel failure = %v", err)
	}
}

func TestImportedProofsAreChecked(t *testing.T) {
	t.Setenv("GO4T_PATH", "")
	root := t.TempDir()
	sources := map[string]string{
		"main.gt":        "package main\nimport \"rules\"\nlaw use(n Int) { n + 1 == 1 + n }\nproof use(n) { rewrite rules.add(n) { refl } }\nfunc main() IO[Unit] { ioPure(unit()) }\n",
		"rules/law.gt":   "package rules\nlaw add(n Int) { n + 1 == 1 + n }\nlaw unused(n Int) { n == n + 1 }\n",
		"rules/proof.gt": "package rules\nproof add(renamed) { refl }\nproof unused(renamed) { refl }\n",
	}
	for name, source := range sources {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := modules.Load([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	errors := CheckProgram(p)
	if len(errors) != 1 || !strings.Contains(errors[0].Msg, "law unused:") {
		t.Fatalf("imported unused law not checked: %v", errors)
	}
	if errors[0].File != filepath.Join(root, "rules", "proof.gt") || errors[0].Pos.Line != 3 || errors[0].Pos.Col != 25 {
		t.Fatalf("wrong imported tactic position: %+v", errors[0])
	}
}

func TestOverlappingLawCyclesReportedOnce(t *testing.T) {
	p := loadSource(t, `package main
law a(b Bool, n Int) { n + 1 == n + 2 }
law b(b Bool, n Int) { n + 1 == n + 2 }
law c(b Bool, n Int) { n + 1 == n + 2 }
proof a(flag, n) { if flag { rewrite b(flag, n) { refl } } else { rewrite c(flag, n) { refl } } }
proof b(flag, n) { rewrite a(flag, n) { refl } }
proof c(flag, n) { rewrite a(flag, n) { refl } }
func main() IO[Unit] { ioPure(unit()) }
`)
	errors := CheckProgram(p)
	if len(errors) != 1 || !strings.Contains(errors[0].Msg, "law dependency cycle: a, b, c") {
		t.Fatalf("cycle diagnostics = %v", errors)
	}
}
