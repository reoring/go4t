package format_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/format"
	"gofx/parser"
)

func TestGolden(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"indent declarations operators", "package main;func f(x Int) Int{let y=x+2*3;y-1};func g() Int{f(2)}", "package main\n\nfunc f(x Int) Int {\n  let y = x + 2 * 3\n  y - 1\n}\n\nfunc g() Int {\n  f(2)\n}\n"},
		{"arms", "package main\nfunc f(x Int) Int {match x {|1=>2|_=>{let y=3;y}}}", "package main\n\nfunc f(x Int) Int {\n  match x {\n    1 => 2\n    | _ => {\n      let y = 3\n      y\n    }\n  }\n}\n"},
		{"commas and record", "package main\nfunc f() R {R{a:1,b:[2,3,],}}", "package main\n\nfunc f() R {\n  R{a: 1, b: [2, 3]}\n}\n"},
		{"long record", "package main\nfunc f() R {R{alpha: \"" + strings.Repeat("a", 58) + "\", beta: 2}}", "package main\n\nfunc f() R {\n  R{\n    alpha: \"" + strings.Repeat("a", 58) + "\",\n    beta: 2\n  }\n}\n"},
		{"pipeline", "package main\nfunc f() Int { value |> firstTransformation(12345) |> secondTransformation(67890) |> finishAll }", "package main\n\nfunc f() Int {\n  value\n    |> firstTransformation(12345)\n    |> secondTransformation(67890)\n    |> finishAll\n}\n"},
		{"short pipeline sections", "package main\nfunc f() Int {1|>add(_,2)|> (_*3)}", "package main\n\nfunc f() Int {\n  1 |> add(_, 2) |> (_ * 3)\n}\n"},
		{"precedence", "package main\nfunc f() Int {(1+2)*(3-(4-5))}", "package main\n\nfunc f() Int {\n  (1 + 2) * (3 - (4 - 5))\n}\n"},
		{"leading trailing arms", "// file\npackage main // package\n// function\nfunc f(x Int) Int {\n// result\nmatch x {\n// first\n1=>2 // two\n// other\n_=>3\n}\n} // end\n// eof\n", "// file\npackage main // package\n\n// function\nfunc f(x Int) Int {\n  // result\n  match x {\n    // first\n    1 => 2 // two\n    // other\n    _ => 3\n  }\n} // end\n// eof\n"},
		{"record comments", "package main\nfunc f() R { R{\n// a\na:1, // first\n// b\nb:2 // last\n} }", "package main\n\nfunc f() R {\n  R{\n    // a\n    a: 1, // first\n    // b\n    b: 2 // last\n  }\n}\n"},
		{"opening record comment", "package main\nfunc f() R {R{ // open\na: 1}}", "package main\n\nfunc f() R {\n  R{ // open\n    a: 1\n  }\n}\n"},
		{"interpolation comment", "package main\nfunc f() String { \"${ // note\n1}\" }", "package main\n\nfunc f() String {\n  \"${ // note\n  1}\"\n}\n"},
		{"multiline record arm", "package main\nfunc f() R { match x { _ => R{a: \"" + strings.Repeat("x", 62) + "\", b: 2} } }", "package main\n\nfunc f() R {\n  match x {\n    | _ => R{\n      a: \"" + strings.Repeat("x", 62) + "\",\n      b: 2\n    }\n  }\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := format.Source([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
			assertProperties(t, tc.source)
		})
	}
}

func TestWidthBoundary(t *testing.T) {
	for _, width := range []int{79, 80, 81} {
		// Two spaces + R{a: " + payload + ", b: 2}.
		payload := strings.Repeat("x", width-len(`  R{a: "", b: 2}`))
		src := "package main\nfunc f() R {R{a: \"" + payload + "\", b: 2}}"
		got, err := format.Source([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		wrapped := strings.Contains(string(got), "R{\n")
		if wrapped != (width > 80) {
			t.Errorf("width %d wrapped=%t:\n%s", width, wrapped, got)
		}
		assertProperties(t, src)
	}
}

func TestSyntaxRoundTrip(t *testing.T) {
	for _, expr := range []string{
		`1.0`, `2e3`, `"$${literal} $$$$ end"`, `"hello ${1 + 2} $$!"`,
		`f(_, g(_), (_ + 2))`, `(_ |> f)`, `(_ |> _)`, `x |> (y |> f)`,
		`(func(x) => x + 1)(2)`, `(if true { f } else { g })(1)`,
		`{ 1; -2 + 3 }`, `{ let (x) = 1; x }`, `do { (x) <- action(); x }`,
		`func((x)) => x`, `{ 1; -2 |> f }`,
		`do { let x = 1; func f(y Int) Int { y + x }; f(x) }`,
		`{ 1; -2 }`, `match x { whole @ Some((a, _)) if a > 0 => a; _ => 0 }`,
		`match x { R{a: a, ..} => a }`,
		`f(1, // one
  2)`, `f( // open
  2)`, `match x { // arms
  _ => 1 }`,
		`{ // open
  1 // result
  // closing
  }`,
		`"nested ${match x { Some(a) => a; None => 0 }}"`,
		`R{ // open
  a: 1, // field
  b: 2
  }`,
		`"${f(1, // interpolation
  2)}"`,
		`"${ // leading interpolation
  1}"`,
	} {
		t.Run(expr, func(t *testing.T) { assertProperties(t, "package main\nfunc f() T { "+expr+" }\n") })
	}
}

func TestRepositoryProperties(t *testing.T) {
	var paths []string
	err := filepath.WalkDir("../examples", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".gfx" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "../prelude/prelude.gfx")
	if len(paths) < 32 {
		t.Fatalf("missing corpus: %d files", len(paths))
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			assertProperties(t, string(source))
		})
	}
}

func assertProperties(t *testing.T, src string) {
	t.Helper()
	before, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	first := format.File(before)
	after, err := parser.Parse(string(first))
	if err != nil {
		t.Fatalf("formatted parse: %v\n%s", err, first)
	}
	second := format.File(after)
	if string(first) != string(second) {
		t.Fatalf("not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !reflect.DeepEqual(structure(before), structure(after)) {
		t.Fatalf("AST changed:\nbefore: %#v\nafter: %#v\nformatted:\n%s", structure(before), structure(after), first)
	}
	if len(before.Comments) != len(after.Comments) {
		t.Fatalf("comments lost: %d -> %d", len(before.Comments), len(after.Comments))
	}
	for i, c := range before.Comments {
		if strings.TrimRight(c.Text, " \t\r") != after.Comments[i].Text || c.Trailing != after.Comments[i].Trailing {
			t.Errorf("comment changed: %#v -> %#v", c, after.Comments[i])
		}
	}
}

// structure compares the semantic parse trees without positions, annotations,
// display filenames or surface-only metadata. Synthesized placeholder bindings
// are alpha-renamed by traversal order; their source-position-based spelling is
// deliberately not a semantic part of the tree. User names are never renamed.
func structure(f *ast.File) any {
	names := map[string]string{}
	for _, d := range f.Decls {
		ast.Walk(d, func(n ast.Node) {
			if l, ok := n.(*ast.Lambda); ok && l.Partial {
				for _, p := range l.Params {
					names[p.Name] = fmt.Sprintf("$%d", len(names))
				}
			}
		})
	}
	var visit func(reflect.Value) any
	visit = func(v reflect.Value) any {
		if !v.IsValid() {
			return nil
		}
		if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil
			}
			return visit(v.Elem())
		}
		if v.Type() == reflect.TypeOf(ast.Pos{}) {
			return nil
		}
		switch v.Kind() {
		case reflect.Struct:
			out := map[string]any{"$type": v.Type().String()}
			for i := range v.NumField() {
				field := v.Type().Field(i).Name
				if field == "Ann" || field == "Comments" || field == "Pipe" || field == "Partial" || field == "Section" || field == "Placeholder" || field == "Source" {
					continue
				}
				if v.Type() == reflect.TypeOf(ast.File{}) && field == "Name" {
					continue
				}
				if field == "Name" && v.Field(i).Kind() == reflect.String {
					if name, ok := names[v.Field(i).String()]; ok {
						out[field] = name
						continue
					}
				}
				out[field] = visit(v.Field(i))
			}
			return out
		case reflect.Slice:
			out := make([]any, v.Len())
			for i := range v.Len() {
				out[i] = visit(v.Index(i))
			}
			return out
		default:
			return v.Interface()
		}
	}
	return visit(reflect.ValueOf(f))
}

func TestSourceError(t *testing.T) {
	_, err := format.Source([]byte("package main\nfunc"))
	if err == nil || err.Error() != "2:5: expected identifier, found end of file" {
		t.Fatalf("error: %v", err)
	}
}
