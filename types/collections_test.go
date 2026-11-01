package types_test

import (
	"errors"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/prelude"
	"gofx/types"
)

func TestCollectionBuiltinKinds(t *testing.T) {
	for _, tt := range []struct {
		con  *types.TCon
		want string
	}{
		{types.Map, "(*, *) -> *"}, {types.Set, "(*) -> *"}, {types.Array, "(*) -> *"},
	} {
		if got := types.KindString(tt.con.Kind); got != tt.want {
			t.Errorf("%s kind = %s, want %s", tt.con.Name, got, tt.want)
		}
	}
}

func TestCollectionConstraints(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"key requires Ord", `type Key = Key(Int)
func value() Map[Key, Int] { mapInsert(emptyMap(), Key(1), 2) }`, "no instance Ord[Key]"},
		{"set requires Ord", `type Key = Key(Int)
func value() Set[Key] { setInsert(emptySet(), Key(1)) }`, "no instance Ord[Key]"},
		{"Eq does not require Ord", `type Key = Key(Int) deriving Eq
func value(m Map[Key, Int], s Set[Key], a Array[Key]) Bool { m == m && s == s && a == a }`, ""},
		{"mapping does not require Ord", `type Key = Key(Int)
func value(m Map[Key, Int]) Map[Key, String] { map(m, func(v) => show(v)) }
func items(a Array[Int]) Array[String] { map(a, func(v) => show(v)) }`, ""},
		{"empty collections unconstrained", `type Key = Key(Int)
func value() Map[Key, Int] { emptyMap() }
func items() Set[Key] { emptySet() }`, ""},
		{"map arity", `func value(m Map[Int]) Int { 1 }`, "kind mismatch"},
		{"array arity", `func value(a Array) Int { 1 }`, "kind mismatch"},
		{"opaque map", `type Map[K, V] = Replacement`, "duplicate declaration Map"},
		{"opaque set", `type Set[A] = Replacement`, "duplicate declaration Set"},
		{"opaque array", `type Array[A] = Replacement`, "duplicate declaration Array"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parser.Parse(prelude.Source)
			if err != nil {
				t.Fatal(err)
			}
			p.Name = "prelude.gfx"
			u, err := parser.Parse("package main\n" + tt.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			if err != nil {
				t.Fatal(err)
			}
			u.Name = "collections.gfx"
			_, err = types.Check([]*ast.File{p, u})
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			var diagnostic *types.Error
			if !errors.As(err, &diagnostic) || diagnostic.File != "collections.gfx" || diagnostic.Pos.Line < 2 || diagnostic.Pos.Col < 1 {
				t.Fatalf("diagnostic lost file:line:col: %v", err)
			}
		})
	}
}
