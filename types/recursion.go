package types

import (
	"maps"
	"sort"

	"github.com/reoring/go4t/ast"
)

const recursionWarning = " calls itself in a non-tail position and no argument is a structural part of its parameter; go4t cannot bound the recursion depth, and stack exhaustion cannot be recovered. Use an accumulator so the call is a tail call"

// warnRecursion runs on fully checked bodies, so CLI commands and LSP clients
// receive the same warnings without importing the law checker or code generator.
func (p *Program) warnRecursion() {
	var ioInstance *Instance
	for _, instance := range p.Instances {
		if instance.TypeCon != IO || p.Owners[instance.Class.Name] != "std/prelude" || p.ModuleNames[instance.Class.Name] != "Monad" {
			continue
		}
		for _, file := range p.Files {
			if file.Name != "std/io/io.gt" {
				continue
			}
			for _, decl := range file.Decls {
				if decl == instance.Decl {
					ioInstance = instance
				}
			}
		}
	}
	for _, file := range p.Files {
		for _, decl := range file.Decls {
			f, ok := decl.(*ast.FuncDecl)
			if !ok || f.Body == nil {
				continue
			}
			scheme := p.Funcs[f.Name].Scheme
			io := false
			if app, ok := Unalias(scheme.Type.Ret).(*TApp); ok {
				io = Unalias(app.Fun) == IO
			}
			scan := recursionScan{program: p, name: f.Name, ioInstance: ioInstance}
			scan.tails = TailCalls(f.Name, RefFunc, f.Params, &ast.BlockExpr{Block: f.Body}, func(dicts []DictRef) bool {
				if len(dicts) != len(scheme.Constraints) {
					return false
				}
				for i, constraint := range scheme.Constraints {
					d := &dicts[i]
					for d != nil && d.Kind == DictSuper {
						d = d.Super
					}
					if d == nil || d.Kind != DictParam || d.Param != constraint.Arg || d.ParamClass != constraint.Class {
						return false
					}
				}
				return true
			}, func(e *ast.Do) bool {
				return io && scan.directIO(e)
			})
			env := make(map[string]recursionValue, len(f.Params))
			for i, param := range f.Params {
				env[param.Name] = recursionValue{root: i + 1}
			}
			scan.block(f.Body, env)
			if scan.bad != nil {
				name := p.ModuleNames[f.Name]
				if name == "" {
					name = f.Name
				}
				p.Warnings = append(p.Warnings, &Error{File: file.Name, Pos: scan.bad.Fn.GetPos(), End: scan.bad.GetEnd(), Msg: name + recursionWarning})
			}
		}
	}
	// Retain the checked-file order while interleaving all warning kinds by span.
	order := make(map[string]int, len(p.Files))
	for i, file := range p.Files {
		order[file.Name] = i
	}
	sort.SliceStable(p.Warnings, func(i, j int) bool {
		a, b := p.Warnings[i], p.Warnings[j]
		if a.File != b.File {
			return order[a.File] < order[b.File]
		}
		return before(a.Pos, b.Pos)
	})
}

type recursionValue struct {
	root, depth int
	fields      []recursionValue
}

type recursionScan struct {
	program    *Program
	name       string
	tails      map[*ast.Call]bool
	bad        *ast.Call
	ioInstance *Instance
}

// directIO uses the resolved dictionary, not the spelling of the block's type.
// A monad parameter retains Bind continuations, even if a caller supplies IO.
func (s *recursionScan) directIO(e *ast.Do) bool {
	d := AnnOf(e).Monad
	return s.ioInstance != nil && d != nil && d.Kind == DictInstance && d.Instance == s.ioInstance
}

func (s *recursionScan) pattern(pat ast.Pattern, value recursionValue, env map[string]recursionValue) {
	BindStructuralPattern(pat, value, s.program.RecordPats, func(value recursionValue, index int) recursionValue {
		if index < len(value.fields) {
			return value.fields[index]
		}
		return recursionValue{root: value.root, depth: value.depth + 1}
	}, func(name string, value recursionValue) { env[name] = value })
}

func (s *recursionScan) bind(name string, pat ast.Pattern, value recursionValue, env map[string]recursionValue) {
	if pat == nil {
		env[name] = value
	} else {
		s.pattern(pat, value, env)
	}
}

func (s *recursionScan) block(block *ast.Block, env map[string]recursionValue) recursionValue {
	for _, stmt := range block.Stmts {
		switch x := stmt.(type) {
		case *ast.LetStmt:
			s.bind(x.Name, x.Pat, s.expr(x.Value, env), env)
		case *ast.ExprStmt:
			s.expr(x.X, env)
		case *ast.FuncDecl:
			// Local recursive groups and their captures are outside this pass.
			env[x.Name] = recursionValue{}
		}
	}
	return s.expr(block.Result, env)
}

func (s *recursionScan) expr(expr ast.Expr, env map[string]recursionValue) recursionValue {
	if expr == nil {
		return recursionValue{}
	}
	switch x := expr.(type) {
	case *ast.Ident:
		if AnnOf(x).Ref == RefLocal {
			return env[x.Name]
		}
	case *ast.Call:
		s.expr(x.Fn, env)
		id, direct := x.Fn.(*ast.Ident)
		self := direct && id.Name == s.name && AnnOf(id).Ref == RefFunc && !s.tails[x]
		// Stack-depth warnings need any corresponding strict subterm, not the
		// law checker's stronger lexicographic termination certificate.
		bounded := false
		for i, arg := range x.Args {
			value := s.expr(arg, env)
			bounded = bounded || value.root == i+1 && value.depth > 0
		}
		if self && !bounded {
			if s.bad == nil || before(x.Fn.GetPos(), s.bad.Fn.GetPos()) {
				s.bad = x
			}
		}
	case *ast.Lambda:
		// Calls beneath a closure boundary are not direct body recursion.
	case *ast.BlockExpr:
		return s.block(x.Block, maps.Clone(env))
	case *ast.TupleLit:
		value := recursionValue{fields: make([]recursionValue, len(x.Elems))}
		for i, item := range x.Elems {
			value.fields[i] = s.expr(item, env)
		}
		return value
	case *ast.ListLit:
		for _, item := range x.Elems {
			s.expr(item, env)
		}
	case *ast.RecordLit:
		for _, item := range x.Values {
			s.expr(item, env)
		}
	case *ast.RecordUpdate:
		s.expr(x.X, env)
		for _, item := range x.Values {
			s.expr(item, env)
		}
	case *ast.Field:
		value := s.expr(x.X, env)
		return recursionValue{root: value.root, depth: value.depth + 1}
	case *ast.Match:
		value := s.expr(x.Scrut, env)
		for _, arm := range x.Arms {
			branch := maps.Clone(env)
			s.pattern(arm.Pat, value, branch)
			s.expr(arm.Guard, branch)
			s.expr(arm.Body, branch)
		}
	case *ast.If:
		s.expr(x.Cond, env)
		s.expr(x.Then, env)
		s.expr(x.Else, env)
	case *ast.Unary:
		s.expr(x.X, env)
	case *ast.Binary:
		s.expr(x.L, env)
		s.expr(x.R, env)
	case *ast.Interp:
		for _, item := range x.Exprs {
			s.expr(item, env)
		}
	case *ast.Do:
		env = maps.Clone(env)
		direct := s.directIO(x)
		for _, stmt := range x.Stmts {
			switch stmt := stmt.(type) {
			case *ast.DoLet:
				s.bind(stmt.Name, stmt.Pat, s.expr(stmt.Value, env), env)
			case *ast.DoBind:
				s.expr(stmt.X, env)
				// Non-IO Bind evaluates this action directly, but the rest
				// of the block is a continuation lambda, outside this pass.
				if !direct {
					return recursionValue{}
				}
				s.bind(stmt.Name, stmt.Pat, recursionValue{}, env)
			case *ast.DoExpr:
				s.expr(stmt.X, env)
				if !direct {
					return recursionValue{}
				}
			case *ast.FuncDecl:
				env[stmt.Name] = recursionValue{}
			}
		}
		s.expr(x.Result, env)
	}
	return recursionValue{}
}
