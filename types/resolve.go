package types

import (
	"fmt"
	"strings"

	"gofx/ast"
)

// Alias bodies are resolved once in their own parameter scope. Keeping the
// template separate from applications both detects declaration cycles and lets
// finite uses such as Identity[Identity[Int]] expand normally.
type typeAlias struct {
	decl      *ast.AliasDecl
	params    []*TVar
	typ       Type
	resolving bool
}

func (c *checker) collectAlias(decl *ast.AliasDecl) {
	alias := &typeAlias{decl: decl}
	seen := make(map[string]bool, len(decl.TParams))
	for _, name := range decl.TParams {
		checkName(decl.Pos, name)
		if seen[name] {
			fail(decl.Pos, "duplicate type parameter %s", name)
		}
		seen[name] = true
		alias.params = append(alias.params, &TVar{Name: name, Kind: KStar{}})
	}
	c.aliases[decl.Name] = alias
}

func (c *checker) checkAliases(files []*ast.File) {
	for _, file := range files {
		c.file = file.Name
		for _, decl := range file.Decls {
			if decl, ok := decl.(*ast.AliasDecl); ok {
				c.resolveAlias(c.aliases[decl.Name], decl.Pos)
			}
		}
	}
}

func (c *checker) resolveAlias(alias *typeAlias, pos ast.Pos) Type {
	if alias.typ != nil {
		return alias.typ
	}
	if alias.resolving {
		fail(pos, "type alias cycle involving %s", alias.decl.Name)
	}
	alias.resolving = true
	file := c.file
	c.file = c.sources[alias.decl]
	alias.typ = c.resolveType(alias.decl.Type, paramScope(alias.params))
	c.file = file
	alias.resolving = false
	return alias.typ
}

func (c *checker) applyAlias(t *ast.TypeName, alias *typeAlias, scope map[string]Type) Type {
	if len(t.Args) != len(alias.params) {
		fail(t.Pos, "arity mismatch for alias %s: want %d arguments, got %d", t.Name, len(alias.params), len(t.Args))
	}
	template := c.resolveAlias(alias, t.Pos)
	if len(t.Args) == 0 {
		return template
	}
	sub := make(map[*TVar]Type, len(t.Args))
	for i, arg := range t.Args {
		sub[alias.params[i]] = c.valueType(arg, scope)
	}
	return substitute(template, sub)
}

func (c *checker) resolveType(t ast.Type, scope map[string]Type) Type {
	switch t := t.(type) {
	case *ast.TypeName:
		base := scope[t.Name]
		if base == nil {
			if alias := c.aliases[t.Name]; alias != nil {
				return c.applyAlias(t, alias, scope)
			}
			if con := c.typeCon(t.Name); con != nil {
				base = con
			}
		}
		if base == nil {
			fail(t.Pos, "unknown type %s", t.Name)
		}
		if len(t.Args) == 0 {
			return base
		}
		k, ok := kindOf(base).(KArrow)
		if !ok || len(t.Args) > len(k.Params) {
			fail(t.Pos, "kind mismatch applying %s to %d arguments", t.Name, len(t.Args))
		}
		args := make([]Type, len(t.Args))
		for i, arg := range t.Args {
			args[i] = c.resolveType(arg, scope)
			requireKind(typePos(arg), kindOf(args[i]), k.Params[i])
		}
		return App(base, args...)
	case *ast.FuncType:
		f := &TFunc{Params: make([]Type, len(t.Params)), Ret: c.valueType(t.Ret, scope)}
		for i, param := range t.Params {
			f.Params[i] = c.valueType(param, scope)
		}
		return f
	case *ast.TupleType:
		args := make([]Type, len(t.Elems))
		for i, elem := range t.Elems {
			args[i] = c.valueType(elem, scope)
		}
		return App(Tuple(len(args)), args...)
	default:
		panic("missing syntactic type")
	}
}

func typePos(t ast.Type) ast.Pos {
	switch t := t.(type) {
	case *ast.TypeName:
		return t.Pos
	case *ast.FuncType:
		return t.Pos
	case *ast.TupleType:
		return t.Pos
	default:
		panic("missing syntactic type")
	}
}

func (c *checker) valueType(t ast.Type, scope map[string]Type) Type {
	resolved := c.resolveType(t, scope)
	requireKind(typePos(t), kindOf(resolved), KStar{})
	return resolved
}

// Instance coherence remains constructor-keyed: expansion must preserve the
// existing head form of a constructor applied to distinct fresh parameters.
// Reordering is allowed, but specialization, nesting, repetition and erasure
// would require a different instance matching model.
func (c *checker) instanceHead(d *ast.InstanceDecl, names []string) (*TCon, []string) {
	if c.aliases[d.TypeCon] == nil {
		con := c.typeCon(d.TypeCon)
		if con == nil {
			fail(d.Pos, "unknown type constructor %s", d.TypeCon)
		}
		return con, names
	}
	scope := make(map[string]Type, len(names))
	head := &ast.TypeName{Pos: d.Pos, Name: d.TypeCon}
	for _, name := range names {
		checkName(d.Pos, name)
		if scope[name] != nil {
			fail(d.Pos, "duplicate type parameter %s", name)
		}
		scope[name] = &TVar{Name: name, Kind: KStar{}}
		head.Args = append(head.Args, &ast.TypeName{Pos: d.Pos, Name: name})
	}
	expanded := c.resolveType(head, scope)
	var args []Type
	if app, ok := expanded.(*TApp); ok {
		expanded, args = app.Fun, app.Args
	}
	con, ok := expanded.(*TCon)
	if !ok || len(args) != len(names) {
		fail(d.Pos, "instance head must be a type constructor")
	}
	ordered := make([]string, len(args))
	seen := make(map[*TVar]bool, len(args))
	for i, arg := range args {
		param, ok := arg.(*TVar)
		if !ok || seen[param] || scope[param.Name] != param {
			fail(d.Pos, "instance head must be a type constructor")
		}
		seen[param] = true
		ordered[i] = param.Name
	}
	return con, ordered
}

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
	b.warnUnused()
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
				dict := DictRef{Kind: DictParam, Class: bound.Class, Param: param, ParamClass: bound.Class}
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
		if inst := b.c.instances[instanceKey{constraint.Class, con.Name}]; inst != nil {
			sub, matches := inst.matchHead(args)
			if !matches {
				return DictRef{}, fmt.Sprintf("no instance %s[%s]%s", constraint.Class, TypeString(arg), b.c.instanceHint(constraint.Class, arg))
			}
			dict := DictRef{Kind: DictInstance, Class: constraint.Class, Instance: inst}
			if len(inst.Context) != 0 {
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
	return DictRef{}, fmt.Sprintf("no instance %s[%s]%s", constraint.Class, TypeString(arg), b.c.instanceHint(constraint.Class, arg))
}
