package types_test

import (
	"errors"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/format"
	"github.com/reoring/go4t/types"
)

func TestDeclsConstants(t *testing.T) {
	files := parseFiles(t, `let base = -10
let count: Int = increase(base)
let flag = true
let text = "yes"
let ratio = -1.5
let _private: Int = count
func increase(n Int) Int { n + 1 }
func value() Int { let base = 3; base + _private }
`+pass5Main)
	before := string(format.File(files[1]))
	for range 2 {
		program, err := checkFixture(t, []*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		f := program.Funcs["count"]
		if f.Constant == nil || len(f.Decl.Params) != 0 || types.TypeString(f.Scheme.Type.Ret) != "Int" {
			t.Fatalf("constant function = %#v", f)
		}
		if got := string(format.File(files[1])); got != before {
			t.Fatalf("checking changed surface syntax:\n%s", got)
		}
	}
}

func TestDeclsConstantDiagnostics(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{`let x = 1 + 2`, `top-level let needs a type unless its value is a literal: let x: T = …`},
		{`let x: IO[Int] = ioPure(1)`, `a top-level let cannot be an IO action; write func x() IO[…]`},
		{`let a: Int = b; let b: Int = a`, `constant initialization cycle: a, b`},
		{`let a: Int = a`, `constant initialization cycle: a`},
		{`let a: Int = value(); func value() Int { a }`, `constant initialization cycle: a`},
		{`let x = 1; func value() Int { x() }`, `x is a constant, not a function; write x without ()`},
	} {
		_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tc.source+"\n"+pass5Main)}})
		var diagnostic *types.Error
		if !errors.As(err, &diagnostic) || diagnostic.Msg != tc.want {
			t.Errorf("%s: error=%v, want %s", tc.source, err, tc.want)
		}
	}
}
