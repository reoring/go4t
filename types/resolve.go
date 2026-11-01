package types

import "gofx/ast"

func (b *bodyChecker) finish() {
	// Inspect all obligations before defaulting anything: a meta shared by an
	// unconstrained expression and a class use must never silently become Unit.
	for _, o := range b.obligations {
		for _, constraint := range o.constraints {
			if unresolved(constraint.Arg) {
				fail(o.expr.GetPos(), "unresolved class constraint %s[%s]", constraint.Class, TypeString(constraint.Arg))
			}
		}
	}
	for _, expr := range b.exprs {
		ann := AnnOf(expr)
		ann.Type = zonk(expr.GetPos(), ann.Type)
		for i, inst := range ann.Inst {
			ann.Inst[i] = zonk(expr.GetPos(), inst)
		}
	}
	for _, o := range b.obligations {
		ann := AnnOf(o.expr)
		if o.monad {
			dict := b.dictionary(o.expr.GetPos(), o.constraints[0])
			ann.Monad = &dict
			continue
		}
		if len(o.constraints) != 0 {
			ann.Dicts = make([]DictRef, len(o.constraints))
			for i, constraint := range o.constraints {
				ann.Dicts[i] = b.dictionary(o.expr.GetPos(), constraint)
			}
		}
	}
	for _, expr := range b.comparisons {
		typ := AnnOf(expr.L).Type
		if typ != Int && typ != String && typ != Bool {
			fail(expr.GetPos(), "comparison requires Int, String or Bool, got %s", TypeString(typ))
		}
		if typ == Bool && expr.Op != "==" && expr.Op != "!=" {
			fail(expr.GetPos(), "ordering comparison requires Int or String, got Bool")
		}
	}
	for _, expr := range b.matches {
		b.checkExhaustive(expr)
	}
}

func (b *bodyChecker) dictionary(pos ast.Pos, constraint Constraint) DictRef {
	arg := prune(constraint.Arg)
	if param, ok := arg.(*TVar); ok {
		for _, bound := range b.constraints {
			if bound.Arg != param {
				continue
			}
			class := b.c.program.Classes[bound.Class]
			dict := DictRef{Kind: DictParam, Class: bound.Class, Param: param.Name, ParamClass: bound.Class}
			for class != nil {
				if class.Name == constraint.Class {
					return dict
				}
				if class.Super == "" {
					break
				}
				parent := dict
				dict = DictRef{Kind: DictSuper, Class: class.Super, Super: &parent}
				class = b.c.program.Classes[class.Super]
			}
		}
	}
	if con, ok := arg.(*TCon); ok {
		if inst := b.c.instances[instanceKey{constraint.Class, con.Name}]; inst != nil {
			return DictRef{Kind: DictInstance, Class: constraint.Class, Instance: inst}
		}
	}
	fail(pos, "no instance %s[%s]", constraint.Class, TypeString(arg))
	return DictRef{}
}
