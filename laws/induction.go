package laws

import (
	"fmt"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// induction elaborates to ordinary self-rewrites. In particular, rewriteRule
// remains the sole authority for descent, argument order and rewrite direction.
func (e *evaluator) induction(law lawGoal, tactic *ast.TMatch, arm ast.TacticArm, value *term, split proofVar, env *environment, vars map[string]proofVar) (ast.Tactic, error) {
	position := -1
	for i, param := range law.Params {
		if env.lookup(param.Name) == split.value {
			position = i
			break
		}
	}
	if position < 0 {
		return nil, fmt.Errorf("induction variable %s is not a quantified variable", tactic.Var.DisplayName())
	}
	con, args := typeHead(split.typ)
	fields := sampleFields(e.p.DataTypes[con.Name], args, value.ctor)
	body := arm.Body
	for field := len(fields) - 1; field >= 0; field-- {
		if types.TypeString(fields[field]) != types.TypeString(split.typ) {
			continue
		}
		rewrite := &ast.TRewrite{Pos: arm.Pos, End: arm.End, Law: &ast.Ident{Name: law.Name, Display: sourceName(e.p, law.Name)}, Body: body}
		for i, param := range law.Params {
			name := fmt.Sprintf("$induction%d_%d", field, i)
			v := proofVar{value: env.lookup(param.Name), typ: param.Type}
			if i == position {
				v = proofVar{value: value.args[field], typ: fields[field], ancestors: append(append([]int(nil), split.ancestors...), split.value.id)}
			}
			vars[name] = v
			rewrite.Args = append(rewrite.Args, &ast.Ident{ExprBase: ast.ExprBase{Pos: arm.Pos, End: arm.End, Ann: &types.Ann{Ref: types.RefLocal, Type: v.typ}}, Name: name})
		}
		body = rewrite
	}
	return body, nil
}

// Bare induction elaborates inside the evaluator, not the checked source AST:
// editor indexes and formatting must never expose synthetic field binders.
func (e *evaluator) inductionArms(tactic *ast.TMatch, typ types.Type) []ast.TacticArm {
	con, _ := typeHead(typ)
	var arms []ast.TacticArm
	for ci, ctor := range e.p.DataTypes[con.Name].Ctors {
		pat := &ast.PCtor{Pos: tactic.Pos, End: tactic.End, Name: ctor.Name}
		for fi := range ctor.Fields {
			pat.Args = append(pat.Args, &ast.PVar{Pos: tactic.Pos, End: tactic.End, Name: fmt.Sprintf("$induction%d_%d", ci, fi)})
		}
		arms = append(arms, ast.TacticArm{Pos: tactic.Pos, End: tactic.End, Pat: pat, Body: &ast.TRefl{Pos: tactic.Pos, End: tactic.End}})
	}
	return arms
}
