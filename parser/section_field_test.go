package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/parser"
)

func TestProjectionSections(t *testing.T) {
	for _, path := range []string{"qty", "0", "a.b", "a.0.b", "0.1"} {
		for _, section := range []bool{true, false} {
			source := "(_." + path + ")"
			if !section {
				source = "map(xs, _." + path + ")"
			}
			t.Run(source, func(t *testing.T) {
				expr := result(t, source)
				if !section {
					call, ok := expr.(*ast.Call)
					if !ok {
						t.Fatalf("projection must not turn its enclosing call into a lambda: %T", expr)
					}
					expr = call.Args[1]
				}
				lambda, ok := expr.(*ast.Lambda)
				if !ok || len(lambda.Params) != 1 {
					t.Fatalf("expression = %#v, want unary lambda", expr)
				}
				if !lambda.Partial || lambda.Section != section {
					t.Fatalf("projection surface metadata = %#v", lambda)
				}
				want := lambda.Params[0].Name + "." + path
				if got := exprShape(lambda.Body); got != want {
					t.Fatalf("body = %s, want %s", got, want)
				}
				receiver := lambda.Body
				for {
					field, ok := receiver.(*ast.Field)
					if !ok {
						break
					}
					if field.Name == "0" && field.Index != 0 || field.Name == "1" && field.Index != 1 {
						t.Fatalf("tuple field has wrong index: %#v", field)
					}
					receiver = field.X
				}
				hole, ok := receiver.(*ast.Ident)
				if !ok || !hole.Placeholder || hole.Name != lambda.Params[0].Name {
					t.Fatalf("projection receiver = %#v, want bound placeholder", receiver)
				}
				if hole.Display != "_" || hole.DisplayName() != "_" || lambda.Params[0].Display != "_" {
					t.Fatalf("projection lost source spelling: ident=%#v param=%#v", hole, lambda.Params[0])
				}
			})
		}
	}
}

func TestProjectionPlaceholderScopes(t *testing.T) {
	outer := result(t, "f(_, _.a.b, _.0, g(_.qty))").(*ast.Lambda)
	if len(outer.Params) != 1 {
		t.Fatalf("outer arity = %d, want one bare placeholder", len(outer.Params))
	}
	call := outer.Body.(*ast.Call)
	seen := map[string]bool{outer.Params[0].Name: true}
	for _, expr := range []ast.Expr{call.Args[1], call.Args[2], call.Args[3].(*ast.Call).Args[0]} {
		projection := expr.(*ast.Lambda)
		if len(projection.Params) != 1 || seen[projection.Params[0].Name] {
			t.Fatalf("projection scope reused a binding: %#v", projection.Params)
		}
		seen[projection.Params[0].Name] = true
	}
	pipe := result(t, "xs |> map(_.qty)").(*ast.Call)
	if exprShape(pipe) != "map(xs, lambda)" {
		t.Fatalf("pipeline shape = %s", exprShape(pipe))
	}
}

func TestProjectionPlacementErrors(t *testing.T) {
	for _, source := range []string{
		"[_.qty]", "(1, _.qty)", "(_.qty, 1)", "xs |> _.qty",
		"f(_.qty + 1)", "(_.qty + 1)", "(1 + _.qty)", "f(_.qty())", "(_.qty())",
	} {
		t.Run(source, func(t *testing.T) {
			_, err := parser.ParseExpr(source, ast.Pos{Line: 4, Col: 7})
			want := fmt.Sprintf("4:%d: _ is only valid as a placeholder in a call or operator section", 7+strings.Index(source, "_"))
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

func TestPlaceholderDisplayNames(t *testing.T) {
	for _, source := range []string{"f(_, _)", "(_ + _)", "(_.product)", "(_.0)"} {
		t.Run(source, func(t *testing.T) {
			expr := result(t, source)
			holes := 0
			ast.Walk(expr, func(node ast.Node) {
				if ident, ok := node.(*ast.Ident); ok && ident.Placeholder {
					holes++
					if ident.Display != "_" || ident.DisplayName() != "_" {
						t.Fatalf("placeholder lost source spelling: %#v", ident)
					}
				}
			})
			if holes == 0 {
				t.Fatal("no placeholder checked")
			}
		})
	}
}

func TestBareProjectionCheckingContexts(t *testing.T) {
	for _, source := range []string{
		"_.qty", "_.0", "_.a.b", "Row{qty: _.qty}", "func(x) => _.qty", "{ _.qty }",
		"if true { _.qty } else { _.other }", "match true { true => _.qty; false => _.other }",
		"{ let key: func(Row) Int = _.qty; key }",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := parser.ParseExpr(source, ast.Pos{Line: 4, Col: 7}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
