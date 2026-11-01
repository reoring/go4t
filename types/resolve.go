package types

import (
	"fmt"

	"gofx/ast"
)

func (b *bodyChecker) finish() {
	warningsStart := len(b.c.program.Warnings)
	b.finishLets()
	// Defer arithmetic validation so arguments and return context select its
	// concrete type. Unconstrained arithmetic keeps the original Int default.
	for _, expr := range b.arithmetic {
		typ := prune(AnnOf(expr).Type)
		if _, ok := typ.(*TMeta); ok {
			unify(expr.GetPos(), Int, typ)
			continue
		}
		if typ == Int || typ == Float {
			continue
		}
		if binary, ok := expr.(*ast.Binary); ok && binary.Op == "+" {
			if typ == String {
				continue
			}
			fail(expr.GetPos(), "addition requires Int, Float or String, got %s", TypeString(typ))
		}
		fail(expr.GetPos(), "arithmetic requires Int or Float, got %s", TypeString(typ))
	}
	// Explicit class uses must resolve before defaulting. Operator obligations
	// alone retain ordinary Unit defaulting, then resolve Eq/Ord for that type.
	for _, o := range b.obligations {
		if o.defaultable {
			continue
		}
		for _, constraint := range o.constraints {
			if unresolved(constraint.Arg) {
				fail(o.expr.GetPos(), "unresolved class constraint %s[%s]", constraint.Class, TypeString(constraint.Arg))
			}
		}
	}
	b.prepareDefaults()
	for _, expr := range b.exprs {
		ann := AnnOf(expr)
		ann.Type = zonk(expr.GetPos(), ann.Type, b.defaultMeta)
		for i, inst := range ann.Inst {
			ann.Inst[i] = zonk(expr.GetPos(), inst, b.defaultMeta)
		}
		if ann.LetScheme != nil {
			ann.LetScheme.Type = zonk(expr.GetPos(), ann.LetScheme.Type, b.defaultMeta).(*TFunc)
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
			dicts := make([]DictRef, len(o.constraints))
			if o.show {
				ann.ShowDicts = dicts
			} else {
				ann.Dicts = dicts
			}
			for i, constraint := range o.constraints {
				pos := o.expr.GetPos()
				if o.show {
					pos = o.expr.(*ast.Interp).Exprs[i].GetPos()
				}
				dicts[i] = b.dictionary(pos, constraint)
			}
		}
	}
	for _, check := range b.patterns {
		typ := zonk(check.pos, check.typ, b.defaultMeta)
		if !b.c.covers([][]ast.Pattern{{check.pat}}, []Type{typ}) {
			fail(check.pos, "%s pattern must be irrefutable", check.context)
		}
	}
	for _, expr := range b.matches {
		b.checkExhaustive(expr)
	}
	b.sortWarnings(warningsStart)
}

func (b *bodyChecker) dictionary(pos ast.Pos, constraint Constraint) DictRef {
	constraint.Arg = zonk(pos, constraint.Arg, b.defaultMeta)
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
		if inst := b.c.instances[instanceKey{constraint.Class, con.Name}]; inst != nil && len(inst.headParams()) == len(args) {
			dict := DictRef{Kind: DictInstance, Class: constraint.Class, Instance: inst}
			if len(inst.Context) != 0 {
				sub := make(map[*TVar]Type, len(inst.headParams()))
				for i, param := range inst.headParams() {
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
