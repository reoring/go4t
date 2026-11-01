package types

import (
	"fmt"

	"gofx/ast"
)

func (b *bodyChecker) finish() {
	// Defer + validation so arguments and return context can resolve its type.
	// Otherwise unconstrained addition keeps the original Int default.
	for _, expr := range b.additions {
		typ := prune(AnnOf(expr).Type)
		if _, ok := typ.(*TMeta); ok {
			unify(expr.GetPos(), Int, typ)
		} else if typ != Int && typ != String {
			fail(expr.GetPos(), "addition requires Int or String, got %s", TypeString(typ))
		}
	}
	// Inspect all obligations before defaulting unconstrained metas to Unit:
	// a meta shared by an ordinary expression and a class use must not default.
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
	constraint.Arg = zonk(pos, constraint.Arg)
	dict, message := b.resolveDictionary(constraint)
	if message != "" {
		fail(pos, "%s", message)
	}
	return dict
}

// Context arguments are resolved recursively on smaller head arguments. Rigid
// arguments terminate at the in-scope dictionary, including recursive uses of
// the current instance inside its own methods.
func (b *bodyChecker) resolveDictionary(constraint Constraint) (DictRef, string) {
	arg := prune(constraint.Arg)
	if param, ok := arg.(*TVar); ok {
		boundClass := ""
		for _, bound := range b.constraints {
			if bound.Arg != param {
				continue
			}
			boundClass = bound.Class
			class := b.c.program.Classes[bound.Class]
			dict := DictRef{Kind: DictParam, Class: bound.Class, Param: param.Name, ParamClass: bound.Class}
			for class != nil {
				if class.Name == constraint.Class {
					return dict, ""
				}
				if class.Super == "" {
					break
				}
				parent := dict
				dict = DictRef{Kind: DictSuper, Class: class.Super, Super: &parent}
				class = b.c.program.Classes[class.Super]
			}
		}
		if b.types[param.Name] == param {
			if boundClass == "" {
				return DictRef{}, fmt.Sprintf("no instance %s[%s] (type parameter %s has no class constraints)", constraint.Class, param.Name, param.Name)
			}
			return DictRef{}, fmt.Sprintf("no instance %s[%s] (type parameter %s is constrained only by %s)", constraint.Class, param.Name, param.Name, boundClass)
		}
	}
	head := arg
	var args []Type
	if app, ok := arg.(*TApp); ok {
		head, args = prune(app.Fun), app.Args
	}
	if con, ok := head.(*TCon); ok {
		if inst := b.c.instances[instanceKey{constraint.Class, con.Name}]; inst != nil && len(inst.Params) == len(args) {
			dict := DictRef{Kind: DictInstance, Class: constraint.Class, Instance: inst}
			if len(inst.Context) != 0 {
				sub := make(map[*TVar]Type, len(inst.Params))
				for i, param := range inst.Params {
					sub[param] = args[i]
				}
				dict.Args = make([]DictRef, len(inst.Context))
				for i, context := range inst.Context {
					required := Constraint{Class: context.Class, Arg: substitute(context.Arg, sub)}
					nested, message := b.resolveDictionary(required)
					if message != "" {
						return DictRef{}, fmt.Sprintf("%s (required by instance %s[%s])", message, inst.Class.Name, TypeString(inst.head()))
					}
					dict.Args[i] = nested
				}
			}
			return dict, ""
		}
	}
	return DictRef{}, fmt.Sprintf("no instance %s[%s]", constraint.Class, TypeString(arg))
}
