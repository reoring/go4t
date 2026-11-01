package codegen

import (
	"testing"

	"go4t/ast"
	"go4t/types"
)

func floatLiteral(value float64) *ast.FloatLit {
	return &ast.FloatLit{ExprBase: annotated(&types.Ann{Type: types.Float}), Value: value}
}

func TestFloatCodegenFormatting(t *testing.T) {
	p := fixture()
	extern(p, "showFloat", []string{"number"}, []types.Type{types.Float}, "go4t/rt", "ShowFloat")
	extern(p, "showFloatFixed", []string{"number", "decimals"}, []types.Type{types.Float, types.Int}, "go4t/rt", "ShowFloatFixed")
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
	assertSource(t, p, "== float64(1.5)")
}
