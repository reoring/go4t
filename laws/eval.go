package laws

import (
	"fmt"
	"sort"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// Environments perform simultaneous, capture-avoiding substitution without
// rewriting the shared checked AST. Every residual binder gets a fresh ID.
type environment struct {
	parent *environment
	values map[string]*term
}

func (env *environment) lookup(name string) *term {
	for ; env != nil; env = env.parent {
		if value := env.values[name]; value != nil {
			return value
		}
	}
	return nil
}

func child(env *environment) *environment {
	return &environment{parent: env, values: make(map[string]*term)}
}

type closure struct {
	params []ast.Param
	expr   ast.Expr
	block  *ast.Block
	env    *environment
	dicts  dictionaryEnv
	scheme *types.Scheme
}

// Assumptions belong to proof and expression branches, not lexical scopes.
// Each condition is a term equal to value in that branch (Bool hypotheses are
// one instance). The stack also reaches calls, dictionaries and closure readback.
type assumption struct {
	condition, value *term
	parent           *assumption
}

type evaluator struct {
	p                 *types.Program
	ctors             map[string]*types.Ctor
	trustedModules    map[string]bool
	listFunctions     map[string]string
	listFunctor       *dictionary
	fuel, nextID      int
	solutions         map[int]*term
	assumptions       *assumption
	renormalizing     bool
	decreasing        map[string][]decreasingPosition
	decreasingMethods map[*ast.InstMethod][]decreasingPosition
	currentGoal       *rewriteRule
	// admissible, when set, further restricts counterexample candidates that
	// already satisfy the law's precondition. Production code leaves it nil;
	// tests use it to observe that a candidate sees no proof state.
	admissible func(*environment) (bool, error)
}

func newEvaluator(p *types.Program) *evaluator {
	e := &evaluator{p: p, fuel: StepLimit, ctors: make(map[string]*types.Ctor), solutions: make(map[int]*term), trustedModules: make(map[string]bool)}
	for _, module := range p.Modules {
		if module.ImportPath == "std/prelude" || module.ImportPath == "std/string" || module.ImportPath == "std/map" {
			e.trustedModules[module.Path] = true
		}
	}
	for _, data := range p.DataTypes {
		for _, ctor := range data.Ctors {
			e.ctors[ctor.Name] = ctor
		}
	}
	e.initListModels()
	return e
}

func (e *evaluator) step() error {
	if e.fuel == 0 {
		return fmt.Errorf("normalization incomplete: step limit %d exhausted", StepLimit)
	}
	e.fuel--
	return nil
}

func (e *evaluator) fresh(name string) *term {
	e.nextID++
	return &term{kind: "var", name: name, id: e.nextID}
}

func (e *evaluator) instantiate(t *term) *term {
	if t.kind == "var" {
		if value := e.solutions[t.id]; value != nil {
			return e.instantiate(value)
		}
	}
	var args []*term
	for i, arg := range t.args {
		value := e.instantiate(arg)
		if value != arg && args == nil {
			args = append([]*term(nil), t.args...)
		}
		if args != nil {
			args[i] = value
		}
	}
	if args != nil {
		result := *t
		result.args = args
		return &result
	}
	return t
}

func (e *evaluator) eval(expr ast.Expr, env *environment, dicts dictionaryEnv) (*term, error) {
	if err := e.step(); err != nil {
		return nil, err
	}
	ann := types.AnnOf(expr)
	if reason := e.unsupportedType(ann.Type, false, make(map[string]bool)); reason != "" {
		return nil, fmt.Errorf("%s at %s", reason, expr.GetPos())
	}
	value, err := e.evalExpr(expr, env, dicts)
	if err == nil && types.Unalias(ann.Type) == types.Int {
		return e.normal(value)
	}
	if err != nil || e.assumptions == nil {
		return value, err
	}
	if _, function := types.Unalias(ann.Type).(*types.TFunc); function {
		return value, nil
	}
	if types.Unalias(ann.Type) != types.Bool || value.kind == "bool" {
		return e.assume(value), nil
	}
	// Compare read-back operands, not source spelling. This also alpha-normalizes
	// lambdas and residual match binders without forcing function arguments
	// before beta reduction.
	value, err = e.normal(value)
	if err != nil {
		return nil, err
	}
	return e.assume(value), nil
}

func (e *evaluator) evalExpr(expr ast.Expr, env *environment, dicts dictionaryEnv) (*term, error) {
	ann := types.AnnOf(expr)
	switch x := expr.(type) {
	case *ast.IntLit:
		return integer(x.Value), nil
	case *ast.StringLit:
		return text(x.Value), nil
	case *ast.BoolLit:
		return boolean(x.Value), nil
	case *ast.Ident:
		if ann.Ref == types.RefLocal {
			value := env.lookup(x.Name)
			if value == nil {
				return nil, fmt.Errorf("unbound checked variable %s", x.Name)
			}
			if value.kind == "generalized" {
				ds, err := e.resolveAll(ann.Dicts, dicts)
				if err != nil {
					return nil, err
				}
				cl := value.closure
				bound, err := bindDictionaries(cl.dicts, cl.scheme.Constraints, ds)
				if err != nil {
					return nil, err
				}
				captured := child(cl.env)
				result, err := e.eval(cl.expr, captured, bound)
				if err != nil {
					return nil, err
				}
				if types.AnnOf(cl.expr).Recursive {
					// Self references are monomorphic: retain this particular
					// instantiation rather than reapplying LetScheme dictionaries.
					captured.values[x.Name] = result
				}
				return result, nil
			}
			return e.instantiate(value), nil
		}
		ds, err := e.resolveAll(ann.Dicts, dicts)
		if err != nil {
			return nil, err
		}
		switch ann.Ref {
		case types.RefCtor:
			ctor := ann.Ctor
			if ctor == nil {
				ctor = e.ctors[x.Name]
			}
			if ctor == nil {
				return nil, fmt.Errorf("missing constructor annotation for %s", x.Name)
			}
			if len(ctor.Fields) == 0 {
				return &term{kind: "ctor", ctor: ctor}, nil
			}
			return &term{kind: "ctorfn", ctor: ctor, arity: len(ctor.Fields)}, nil
		case types.RefFunc:
			return e.function(x.Name, ds)
		case types.RefMethod:
			ft, ok := types.Unalias(ann.Type).(*types.TFunc)
			if !ok {
				return nil, fmt.Errorf("method %s has no function type", x.Name)
			}
			return &term{kind: "method", name: x.Name, dicts: ds, arity: len(ft.Params)}, nil
		}
	case *ast.Call:
		fn, err := e.eval(x.Fn, env, dicts)
		if err != nil {
			return nil, err
		}
		args, err := e.evalAll(x.Args, env, dicts)
		if err != nil {
			return nil, err
		}
		return e.apply(fn, args)
	case *ast.Lambda:
		return &term{kind: "closure", arity: len(x.Params), closure: &closure{params: x.Params, expr: x.Body, env: env, dicts: dicts}}, nil
	case *ast.BlockExpr:
		return e.block(x.Block, env, dicts)
	case *ast.TupleLit:
		args, err := e.evalAll(x.Elems, env, dicts)
		return &term{kind: "tuple", args: args}, err
	case *ast.ListLit:
		args, err := e.evalAll(x.Elems, env, dicts)
		if err != nil {
			return nil, err
		}
		nilCtor, consCtor := e.ctors["Nil"], e.ctors["Cons"]
		if nilCtor == nil || consCtor == nil {
			return nil, fmt.Errorf("missing prelude List constructors")
		}
		result := &term{kind: "ctor", ctor: nilCtor}
		for i := len(args) - 1; i >= 0; i-- {
			result = &term{kind: "ctor", ctor: consCtor, args: []*term{args[i], result}}
		}
		return result, nil
	case *ast.RecordLit:
		values, err := e.evalAll(x.Values, env, dicts)
		if err != nil {
			return nil, err
		}
		fields := make([]*term, len(ann.Ctor.Fields))
		for i, value := range values {
			fields[ann.FieldIndexes[i]] = value
		}
		return &term{kind: "ctor", ctor: ann.Ctor, args: fields}, nil
	case *ast.Field:
		base, err := e.eval(x.X, env, dicts)
		if err != nil {
			return nil, err
		}
		indexes := []int{ann.FieldIndex}
		if len(ann.RecordCases) != 0 {
			indexes = caseIndexes(base, ann.RecordCases)
		}
		if indexes != nil && (base.kind == "ctor" || base.kind == "tuple") {
			return base.args[indexes[0]], nil
		}
		if len(ann.RecordCases) == 0 {
			value, stripped := projectUpdate(base, ann.Ctor, ann.FieldIndex)
			if value != nil {
				return value, nil
			}
			base = stripped
		}
		return &term{kind: "field", name: x.Name, index: ann.FieldIndex, ctor: ann.Ctor, cases: ann.RecordCases, args: []*term{base}}, nil
	case *ast.RecordUpdate:
		base, err := e.eval(x.X, env, dicts)
		if err != nil {
			return nil, err
		}
		values, err := e.evalAll(x.Values, env, dicts)
		if err != nil {
			return nil, err
		}
		indexes := ann.FieldIndexes
		if len(ann.RecordCases) != 0 {
			indexes = caseIndexes(base, ann.RecordCases)
		}
		if base.kind == "ctor" && indexes != nil {
			fields := append([]*term(nil), base.args...)
			for i, value := range values {
				fields[indexes[i]] = value
			}
			return &term{kind: "ctor", ctor: base.ctor, args: fields}, nil
		}
		return updateTerm(ann.Ctor, ann.FieldIndexes, ann.RecordCases, base, values), nil
	case *ast.Match:
		value, err := e.eval(x.Scrut, env, dicts)
		if err != nil {
			return nil, err
		}
		return e.match(x.Arms, value, env, dicts)
	case *ast.If:
		cond, err := e.eval(x.Cond, env, dicts)
		if err != nil {
			return nil, err
		}
		if cond.kind != "bool" {
			cond, err = e.normal(cond)
			if err != nil {
				return nil, err
			}
		}
		if cond.kind == "bool" {
			if cond.flag {
				return e.eval(x.Then, env, dicts)
			}
			return e.eval(x.Else, env, dicts)
		}
		left, err := e.evalIfBranch(x.Then, cond, true, env, dicts)
		if err != nil {
			return nil, err
		}
		right, err := e.evalIfBranch(x.Else, cond, false, env, dicts)
		return &term{kind: "if", args: []*term{cond, left, right}}, err
	case *ast.Unary:
		value, err := e.eval(x.X, env, dicts)
		if err != nil {
			return nil, err
		}
		return unary(x.Op, value), nil
	case *ast.Binary:
		return e.binary(x, env, dicts)
	case *ast.Interp:
		result := text(x.Texts[0])
		for i, expr := range x.Exprs {
			value, err := e.eval(expr, env, dicts)
			if err != nil {
				return nil, err
			}
			dict, err := e.resolve(ann.ShowDicts[i], dicts)
			if err != nil {
				return nil, err
			}
			shown, err := e.method("show", []*dictionary{dict}, []*term{value})
			if err != nil {
				return nil, err
			}
			if result.kind == "string" && result.name == "" {
				result = shown
			} else {
				result = addString(result, shown)
			}
			if x.Texts[i+1] != "" {
				result = addString(result, text(x.Texts[i+1]))
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported law expression %T", expr)
}

func (e *evaluator) evalIfBranch(expr ast.Expr, condition *term, flag bool, env *environment, dicts dictionaryEnv) (*term, error) {
	parent := e.assumptions
	e.addAssumption(condition, flag)
	defer func() { e.assumptions = parent }()
	value, err := e.eval(expr, env, dicts)
	if err != nil {
		return nil, err
	}
	return e.normal(value)
}

func (e *evaluator) evalAll(exprs []ast.Expr, env *environment, dicts dictionaryEnv) ([]*term, error) {
	args := make([]*term, len(exprs))
	for i, expr := range exprs {
		value, err := e.eval(expr, env, dicts)
		if err != nil {
			return nil, err
		}
		args[i] = value
	}
	return args, nil
}

func (e *evaluator) function(name string, dicts []*dictionary) (*term, error) {
	f := e.p.Funcs[name]
	if f == nil {
		return nil, fmt.Errorf("missing checked function %s", name)
	}
	if f.Decl.Extern != nil && !e.trustedExtern(f) {
		return nil, externError(f.Decl.Extern)
	}
	return &term{kind: "global", name: name, dicts: dicts, arity: len(f.Decl.Params)}, nil
}

func (e *evaluator) apply(fn *term, args []*term) (*term, error) {
	if err := e.step(); err != nil {
		return nil, err
	}
	switch fn.kind {
	case "ctorfn":
		return &term{kind: "ctor", ctor: fn.ctor, args: args}, nil
	case "global":
		if !e.unfoldGlobal(fn, args) {
			return &term{kind: "call", args: append([]*term{fn}, args...)}, nil
		}
		if model := e.listModel(fn.name); model != "" {
			return e.listOperation(model, args)
		}
		f := e.p.Funcs[fn.name]
		if f.Decl.Extern != nil {
			return e.extern(f.Decl.Extern, args)
		}
		dicts, err := bindDictionaries(nil, f.Scheme.Constraints, fn.dicts)
		if err != nil {
			return nil, err
		}
		env := child(nil)
		for i, param := range f.Decl.Params {
			env.values[param.Name] = args[i]
		}
		return e.block(f.Decl.Body, env, dicts)
	case "method":
		return e.method(fn.name, fn.dicts, args)
	case "closure":
		cl := fn.closure
		env := child(cl.env)
		for i, param := range cl.params {
			if param.Pat != nil {
				if err := e.bindPattern(param.Pat, args[i], env); err != nil {
					return nil, err
				}
			} else {
				env.values[param.Name] = args[i]
			}
		}
		if cl.block != nil {
			return e.block(cl.block, env, cl.dicts)
		}
		return e.eval(cl.expr, env, cl.dicts)
	default:
		return &term{kind: "call", args: append([]*term{fn}, args...)}, nil
	}
}

// Recursive globals unfold exactly when their certified decreasing argument
// has a constructor head, independent of the surrounding evaluation context.
// A fixed global's ordered dictionaries identify its termination environment.
func (e *evaluator) unfoldGlobal(fn *term, args []*term) bool {
	if model := e.listModel(fn.name); model != "" {
		return e.unfoldList(model, args)
	}
	for _, decrease := range e.decreasing[fn.name] {
		if sameDictionaries(decrease.dicts, fn.dicts) {
			arg := args[decrease.position]
			return arg.kind == "ctor" || arg.kind == "tuple"
		}
	}
	return true
}

func (e *evaluator) block(block *ast.Block, env *environment, dicts dictionaryEnv) (*term, error) {
	for i := 0; i < len(block.Stmts); i++ {
		if err := e.step(); err != nil {
			return nil, err
		}
		switch stmt := block.Stmts[i].(type) {
		case *ast.LetStmt:
			ann := types.AnnOf(stmt.Value)
			next := child(env)
			if ann.LetScheme != nil {
				captured := env
				if ann.Recursive {
					captured = next
				}
				next.values[stmt.Name] = &term{kind: "generalized", closure: &closure{expr: stmt.Value, env: captured, dicts: dicts, scheme: ann.LetScheme}}
			} else {
				captured := env
				if ann.Recursive {
					captured = next
				}
				value, err := e.eval(stmt.Value, captured, dicts)
				if err != nil {
					return nil, err
				}
				if stmt.Pat == nil {
					next.values[stmt.Name] = value
				} else if err := e.bindPattern(stmt.Pat, value, next); err != nil {
					return nil, err
				}
			}
			env = next
		case *ast.ExprStmt:
			if _, err := e.eval(stmt.X, env, dicts); err != nil {
				return nil, err
			}
		case *ast.FuncDecl:
			group := child(env)
			for ; i < len(block.Stmts); i++ {
				decl, ok := block.Stmts[i].(*ast.FuncDecl)
				if !ok {
					break
				}
				group.values[decl.Name] = &term{kind: "closure", arity: len(decl.Params), closure: &closure{params: decl.Params, block: decl.Body, env: group, dicts: dicts}}
			}
			i--
			env = group
		default:
			return nil, fmt.Errorf("unsupported law statement %T", stmt)
		}
	}
	return e.eval(block.Result, env, dicts)
}

// projectUpdate reduces (r{f: v, ...}).g for a single-constructor record whose
// base r is symbolic. The projection is v when g was updated; otherwise the
// projection moves to the update's base, recursively, and the stripped base
// is returned with value nil. Both steps are sound for any r because an update
// always yields the same constructor with the other fields copied.
// Multi-constructor (RecordCases) updates are not reduced here.
func projectUpdate(base *term, ctor *types.Ctor, index int) (value *term, stripped *term) {
	for base.kind == "update" && len(base.cases) == 0 && base.ctor == ctor {
		for i, updated := range base.indexes {
			if updated == index {
				return base.args[i+1], nil
			}
		}
		base = base.args[0]
	}
	if base.kind == "ctor" && base.ctor == ctor {
		return base.args[index], nil
	}
	return nil, base
}

// updateTerm builds r{...} over a non-constructor base. For single-constructor
// records a nested update r{f: a}{g: b} collapses into one update where the
// later write to a field wins, and fields are ordered by index, so
// r{f: a}{f: b} and r{f: b} (and r{f: a, g: b} and r{g: b, f: a}) share one
// normal form. Sound because an update always yields the same constructor with
// the other fields copied. Multi-constructor (cases) updates are left as is.
func updateTerm(ctor *types.Ctor, indexes []int, cases []types.RecordCase, base *term, values []*term) *term {
	if len(cases) != 0 {
		return &term{kind: "update", ctor: ctor, indexes: indexes, cases: cases, args: append([]*term{base}, values...)}
	}
	merged := map[int]*term{}
	if base.kind == "update" && len(base.cases) == 0 && base.ctor == ctor {
		for i, index := range base.indexes {
			merged[index] = base.args[i+1]
		}
		base = base.args[0]
	}
	for i, index := range indexes {
		merged[index] = values[i]
	}
	order := make([]int, 0, len(merged))
	for index := range merged {
		order = append(order, index)
	}
	sort.Ints(order)
	args := make([]*term, 1, len(order)+1)
	args[0] = base
	for _, index := range order {
		args = append(args, merged[index])
	}
	return &term{kind: "update", ctor: ctor, indexes: order, args: args}
}

func caseIndexes(value *term, cases []types.RecordCase) []int {
	for _, c := range cases {
		if value.kind == "ctor" && value.ctor == c.Ctor {
			return c.Indexes
		}
	}
	return nil
}
