package types

import (
	"fmt"
	"strings"

	"gofx/ast"
)

func (b *bodyChecker) finish() {
	warningsStart := len(b.c.program.Warnings)
	b.finishLets()
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
				required := b.strongestConstraint(constraint, o.constraints)
				dicts[i] = b.upcastDictionary(b.dictionary(pos, required), constraint.Class)
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

// Keep the declared slots and their order, but use the strongest available
// dictionary for redundant superclass requirements on the same parameter.
func (b *bodyChecker) strongestConstraint(required Constraint, available []Constraint) Constraint {
	for _, candidate := range available {
		if prune(candidate.Arg) == prune(required.Arg) && b.subclass(candidate.Class, required.Class) {
			required = candidate
		}
	}
	return required
}

func (b *bodyChecker) upcastDictionary(dict DictRef, class string) DictRef {
	for dict.Class != class {
		parent := dict
		dict = DictRef{Kind: DictSuper, Class: b.c.program.Classes[dict.Class].Super, Super: &parent}
	}
	return dict
}

// Context arguments are resolved recursively on smaller head arguments. Rigid
// arguments terminate at the in-scope dictionary, including recursive uses of
// the current instance inside its own methods.
func (b *bodyChecker) resolveDictionary(constraint Constraint) (DictRef, string) {
	arg := prune(constraint.Arg)
	if param, ok := arg.(*TVar); ok {
		required := b.strongestConstraint(constraint, b.constraints)
		for _, bound := range b.constraints {
			if bound.Arg == param && bound.Class == required.Class {
				dict := DictRef{Kind: DictParam, Class: bound.Class, Param: param.Name, ParamClass: bound.Class}
				return b.upcastDictionary(dict, constraint.Class), ""
			}
		}
		if b.types[param.Name] == param {
			var classes []string
			for _, bound := range b.constraints {
				if bound.Arg == param {
					classes = append(classes, bound.Class)
				}
			}
			if len(classes) == 0 {
				return DictRef{}, fmt.Sprintf("no instance %s[%s] (type parameter %s has no class constraints)", constraint.Class, param.Name, param.Name)
			}
			return DictRef{}, fmt.Sprintf("no instance %s[%s] (type parameter %s is constrained only by %s)", constraint.Class, param.Name, param.Name, strings.Join(classes, " + "))
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
					required := b.strongestConstraint(context, inst.Context)
					required.Arg = substitute(required.Arg, sub)
					nested, message := b.resolveDictionary(required)
					if message != "" {
						return DictRef{}, fmt.Sprintf("%s (required by instance %s[%s])", message, inst.Class.Name, TypeString(inst.head()))
					}
					dict.Args[i] = b.upcastDictionary(nested, context.Class)
				}
			}
			return dict, ""
		}
	}
	return DictRef{}, fmt.Sprintf("no instance %s[%s]", constraint.Class, TypeString(arg))
}
