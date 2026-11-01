package codegen

import (
	"testing"

	"gofx/ast"
	"gofx/types"
)

func floatLiteral(value float64) *ast.FloatLit {
	return &ast.FloatLit{ExprBase: annotated(&types.Ann{Type: types.Float}), Value: value}
}

func TestFloatCodegen(t *testing.T) {
	for _, tt := range []struct{ op, runtime string }{
		{"+", "AddFloat"}, {"-", "SubFloat"}, {"*", "MulFloat"}, {"/", "DivFloat"},
	} {
		t.Run(tt.runtime, func(t *testing.T) {
			p := fixture()
			p.Main.Decl.Body.Result = &ast.Binary{
				ExprBase: annotated(&types.Ann{Type: types.Float}),
				Op:       tt.op, L: floatLiteral(1.5), R: floatLiteral(0),
			}
			// Even division by a literal zero must call the runtime, not create
			// a Go constant-division expression rejected by the Go compiler.
			assertSource(t, p, "return rt."+tt.runtime+"(float64(1.5), float64(0))")
		})
	}
	p := fixture()
	p.Main.Decl.Body.Result = &ast.Unary{Op: "-", X: floatLiteral(0)}
	assertSource(t, p, "return rt.NegFloat(float64(0))")
}

func TestFloatCodegenFormatting(t *testing.T) {
	p := fixture()
	extern(p, "showFloat", []string{"number"}, []types.Type{types.Float}, "gofx/rt", "ShowFloat")
	extern(p, "showFloatFixed", []string{"number", "decimals"}, []types.Type{types.Float, types.Int}, "gofx/rt", "ShowFloatFixed")
	p.Main.Decl.Body.Result = tupleLiteral(
		call(function("showFloat", fnType(types.Float)), floatLiteral(2000)),
		call(function("showFloatFixed", fnType(types.Float, types.Int)), floatLiteral(0.001), integer(3)),
	)
	assertSource(t, p,
		"return ext_1.ShowFloat(number_)",
		"return ext_1.ShowFloatFixed(number_, decimals_)",
		"showFloat_(float64(2000))",
		"showFloatFixed_(float64(0.001), int64(3))")
}

func TestFloatLiteralPatternCodegen(t *testing.T) {
	p := fixture()
	p.Main.Decl.Body.Result = &ast.Match{
		Scrut: floatLiteral(1.5),
		Arms: []ast.Arm{
			{Pat: &ast.PLit{X: floatLiteral(1.5)}, Body: boolean(true)},
			{Pat: &ast.PWild{}, Body: boolean(false)},
		},
	}
	assertSource(t, p, ".(float64) == float64(1.5)")
}
