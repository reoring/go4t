package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

func TestLawDeclarations(t *testing.T) {
	f := parse(t, "package main\nlaw identity(x Int, y List[Int],) { x + 1 == 1 + x }\nproof identity(a, b,) { refl }\n")
	law := f.Decls[0].(*ast.LawDecl)
	if law.Name != "identity" || len(law.Params) != 2 || law.Params[0].Name != "x" || law.Params[1].Name != "y" {
		t.Fatalf("law = %#v", law)
	}
	typeName(t, law.Params[0].Type, "Int")
	typeName(t, law.Params[1].Type, "List", "Int")
	if exprShape(law.LHS) != "(x + 1)" || exprShape(law.RHS) != "(1 + x)" {
		t.Fatalf("sides = %s, %s", exprShape(law.LHS), exprShape(law.RHS))
	}
	proof := f.Decls[1].(*ast.ProofDecl)
	if proof.Name != "identity" || len(proof.Params) != 2 || proof.Params[0].Name != "a" || proof.Params[1].Name != "b" {
		t.Fatalf("proof = %#v", proof)
	}
	for _, param := range proof.Params {
		if param.Type != nil || param.Pat != nil {
			t.Fatalf("proof parameter must be an untyped name: %#v", param)
		}
	}
	if _, ok := proof.Body.(*ast.TRefl); !ok {
		t.Fatalf("proof body = %T", proof.Body)
	}
}

func TestLawEqualityPrecedence(t *testing.T) {
	for _, tc := range []struct{ body, left, right string }{
		{"a + b == c * d", "(a + b)", "(c * d)"},
		{"(a == b) == c", "(a == b)", "c"},
		{"(a || b) == (c && d)", "(a || b)", "(c && d)"},
		{"a == (b == c)", "a", "(b == c)"},
		{"(a == b)", "a", "b"},
		{"a\n == b", "a", "b"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			law := parse(t, "package main\nlaw f() { "+tc.body+" }").Decls[0].(*ast.LawDecl)
			if exprShape(law.LHS) != tc.left || exprShape(law.RHS) != tc.right {
				t.Fatalf("sides = %s, %s; want %s, %s", exprShape(law.LHS), exprShape(law.RHS), tc.left, tc.right)
			}
		})
	}
}

func TestProofTactics(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"refl", "refl"},
		{"match x { None => refl; Some(y) => refl }", "match x { None() => refl; Some(y) => refl }"},
		{"match x { | (Some(y), _) => refl | _ => refl }", "match x { (Some(y), _) => refl; _ => refl }"},
		{"match x { R{value: Some(y), ..} => refl }", "match x { R{value: Some(y), ..} => refl }"},
		{"match x { y => refl }", "match x { y => refl }"},
		{"match x {}", "match x {  }"},
		{"if x > 0 { refl } else { refl }", "if (x > 0) { refl } else { refl }"},
		{"rewrite f() { refl }", "rewrite f() { refl }"},
		{"rewrite other.f(x, x + 1,) { refl }", "rewrite other.f(x, (x + 1)) { refl }"},
		{"if ready(x) { rewrite f(x) { match x { Some(y) => match y { C => refl }; None => refl } } } else { refl }", "if ready(x) { rewrite f(x) { match x { Some(y) => match y { C() => refl }; None() => refl } } } else { refl }"},
		{"if true { refl }\nelse { refl }", "if true { refl } else { refl }"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			proof := parse(t, "package main\nproof f(x) { "+tc.body+" }").Decls[0].(*ast.ProofDecl)
			if got := tacticShape(proof.Body); got != tc.want {
				t.Fatalf("body = %s, want %s", got, tc.want)
			}
		})
	}
}

func tacticShape(tactic ast.Tactic) string {
	switch t := tactic.(type) {
	case *ast.TRefl:
		return "refl"
	case *ast.TMatch:
		arms := make([]string, len(t.Arms))
		for i, arm := range t.Arms {
			arms[i] = patternShape(arm.Pat) + " => " + tacticShape(arm.Body)
		}
		return "match " + t.Var.Name + " { " + strings.Join(arms, "; ") + " }"
	case *ast.TIf:
		return "if " + exprShape(t.Cond) + " { " + tacticShape(t.Then) + " } else { " + tacticShape(t.Else) + " }"
	case *ast.TRewrite:
		args := make([]string, len(t.Args))
		for i, arg := range t.Args {
			args[i] = exprShape(arg)
		}
		return "rewrite " + t.Law.Name + "(" + strings.Join(args, ", ") + ") { " + tacticShape(t.Body) + " }"
	default:
		return "unknown tactic"
	}
}

func TestLawProofErrors(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"law f() { a == b && c }", ambiguousLawBody},
		{"law f() { a == b; c == d }", `expected "}", found "c"`},
		{"law f() { let x = 1; x == x }", `expected expression, found "let"`},
		{"proof f(x Int) { refl }", `expected ")", found "Int"`},
		{"proof f(x) { if true { refl } }", `expected "else", found "}"`},
		{"proof f(x) { if true { refl } else if false { refl } else { refl } }", `expected "{", found "if"`},
		{"proof f(x) { match x { Some(y) if y > 0 => refl } }", "tactic match does not support guards"},
		{"proof f(x) { match x { 1 => refl } }", "tactic match supports only the literal patterns true and false"},
		{"proof f(x) { match x { R{value: 1.5} => refl } }", "tactic match supports only the literal patterns true and false"},
		{"proof f(x) { match x { (y, \"s\") => refl } }", "tactic match supports only the literal patterns true and false"},
		{"proof f(x) { match x { whole @ Some(y) => refl } }", "tactic match does not support as-patterns"},
		{"proof f(x) { match x { Some(y @ _) => refl } }", "tactic match does not support as-patterns"},
		{"proof f(x) { match x { [] => refl } }", "tactic match does not support list patterns"},
		{"proof f(x) { induction f(x) { _ => refl } }", `expected "}", found "("`},
		{"proof f(x) { refl; refl }", `expected "}", found "refl"`},
		{"proof f() { x }", `expected tactic, found "x"`},
		{"func f() Int { law g() { 1 == 1 }; 1 }", `expected expression, found "law"`},
		{"func f() Int { proof g() { refl }; 1 }", `expected expression, found "proof"`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			_, err := parser.Parse("package main\n" + tc.source)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			positioned, ok := err.(interface {
				GetPos() ast.Pos
				GetEnd() ast.Pos
			})
			if !ok || positioned.GetPos().Line != 2 || positioned.GetEnd().Col <= positioned.GetPos().Col {
				t.Fatalf("error is not positioned: %#v", err)
			}
		})
	}
}

func TestLawProofSpansAndWalk(t *testing.T) {
	f := parse(t, `package main
law f(x Int) { x == x }
proof f(y) {
  if y > 0 {
    rewrite other.f(y) {
      match y {
        Some(z) => refl
      }
    }
  } else {
    refl
  }
}
`)
	law := f.Decls[0].(*ast.LawDecl)
	proof := f.Decls[1].(*ast.ProofDecl)
	branch := proof.Body.(*ast.TIf)
	rewrite := branch.Then.(*ast.TRewrite)
	match := rewrite.Body.(*ast.TMatch)
	for _, tc := range []struct {
		node     ast.Node
		pos, end ast.Pos
	}{
		{law, ast.Pos{Line: 2, Col: 1}, ast.Pos{Line: 2, Col: 24}},
		{proof, ast.Pos{Line: 3, Col: 1}, ast.Pos{Line: 13, Col: 2}},
		{&proof.Params[0], ast.Pos{Line: 3, Col: 9}, ast.Pos{Line: 3, Col: 10}},
		{branch, ast.Pos{Line: 4, Col: 3}, ast.Pos{Line: 12, Col: 4}},
		{rewrite, ast.Pos{Line: 5, Col: 5}, ast.Pos{Line: 9, Col: 6}},
		{rewrite.Law, ast.Pos{Line: 5, Col: 13}, ast.Pos{Line: 5, Col: 20}},
		{match, ast.Pos{Line: 6, Col: 7}, ast.Pos{Line: 8, Col: 8}},
		{match.Var, ast.Pos{Line: 6, Col: 13}, ast.Pos{Line: 6, Col: 14}},
		{match.Arms[0].Body, ast.Pos{Line: 7, Col: 20}, ast.Pos{Line: 7, Col: 24}},
	} {
		if tc.node.GetPos() != tc.pos || tc.node.GetEnd() != tc.end {
			t.Errorf("%T span = %s-%s, want %s-%s", tc.node, tc.node.GetPos(), tc.node.GetEnd(), tc.pos, tc.end)
		}
	}
	if match.Arms[0].Pos != (ast.Pos{Line: 7, Col: 9}) || match.Arms[0].End != match.Arms[0].Body.GetEnd() {
		t.Errorf("arm span = %s-%s", match.Arms[0].Pos, match.Arms[0].End)
	}
	var visited []string
	for _, decl := range f.Decls {
		ast.Walk(decl, func(n ast.Node) {
			visited = append(visited, reflect.TypeOf(n).String())
			if n.GetPos() == (ast.Pos{}) || n.GetEnd() == (ast.Pos{}) {
				t.Errorf("missing span on %T", n)
			}
		})
	}
	want := []string{"*ast.LawDecl", "*ast.Param", "*ast.TypeName", "*ast.Ident", "*ast.Ident", "*ast.ProofDecl", "*ast.Param", "*ast.TIf", "*ast.Binary", "*ast.Ident", "*ast.IntLit", "*ast.TRewrite", "*ast.Ident", "*ast.Ident", "*ast.TMatch", "*ast.Ident", "*ast.TacticArm", "*ast.PCtor", "*ast.PVar", "*ast.TRefl", "*ast.TRefl"}
	if !reflect.DeepEqual(visited, want) {
		t.Fatalf("walk = %v, want %v", visited, want)
	}
}
