package types

import (
	"fmt"
	"strings"

	"gofx/ast"
)

type instanceKey struct{ class, con string }

type checker struct {
	program   *Program
	names     map[string]bool
	cons      map[string]*TCon
	ctors     map[string]*Ctor
	methods   map[string]*Method
	instances map[instanceKey]*Instance
	nextMeta  int
}

// Check collects all declarations before checking bodies, so signatures and
// instances may refer forward across files. Files share one namespace and must
// be supplied in prelude-first order. On success every expression, including
// literals in patterns, has a fully resolved Ann. No input annotation is used
// as evidence: the same parsed files may safely be checked again.
// Declared identifiers may not use the generated-code prefixes dict_ or ext_.
//
// Unresolved ordinary metas default to Unit only when no class constraint
// depends on them. Unresolved higher-kinded metas are errors: Unit cannot be a
// kind-correct default for a type constructor. Failures return a positioned
// *Error and a nil program; partially checked input annotations are not usable.
func Check(files []*ast.File) (program *Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*Error); ok {
				program, err = nil, e
			} else {
				panic(r)
			}
		}
	}()
	c := &checker{
		program: &Program{Classes: make(map[string]*Class), DataTypes: make(map[string]*DataType), Funcs: make(map[string]*FuncInfo), Files: files},
		names:   make(map[string]bool), cons: make(map[string]*TCon), ctors: make(map[string]*Ctor),
		methods: make(map[string]*Method), instances: make(map[instanceKey]*Instance),
	}
	for _, con := range []*TCon{Int, String, Bool, Unit, IO} {
		c.cons[con.Name], c.names[con.Name] = con, true
	}
	c.collectNames(files)
	c.checkSuperclasses(files)
	c.collectSignatures(files)
	c.collectInstances(files)
	c.checkMain()
	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Extern == nil {
					f := c.program.Funcs[decl.Name]
					b := c.body(f.Scheme.Constraints, paramScope(f.Scheme.TParams))
					b.check(decl.Params, f.Scheme.Type, decl.Body)
				}
			case *ast.InstanceDecl:
				inst := c.instances[instanceKey{decl.Class, decl.TypeCon}]
				for _, method := range decl.Methods {
					m := c.methods[method.Name]
					sub := map[*TVar]Type{inst.Class.Param: inst.TypeCon}
					scope := paramScope(m.Scheme.TParams)
					scope[inst.Class.Param.Name] = inst.TypeCon
					b := c.body(subConstraints(m.Scheme.Constraints, sub), scope)
					params := make([]ast.Param, len(method.Params))
					for i, name := range method.Params {
						params[i].Name = name
					}
					b.check(params, substitute(m.Scheme.Type, sub).(*TFunc), method.Body)
				}
			}
		}
	}
	return c.program, nil
}

func fail(pos ast.Pos, format string, args ...any) {
	panic(&Error{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

func checkName(pos ast.Pos, name string) {
	if strings.HasPrefix(name, "dict_") || strings.HasPrefix(name, "ext_") {
		fail(pos, "identifier %q uses reserved prefix", name)
	}
}

func (c *checker) reserve(pos ast.Pos, name string) {
	checkName(pos, name)
	if c.names[name] {
		fail(pos, "duplicate declaration %s", name)
	}
	c.names[name] = true
}

func arityKind(arity int) Kind {
	if arity == 0 {
		return KStar{}
	}
	params := make([]Kind, arity)
	for i := range params {
		params[i] = KStar{}
	}
	return KArrow{Params: params, Ret: KStar{}}
}

func (c *checker) collectNames(files []*ast.File) {
	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.ClassDecl:
				c.reserve(decl.Pos, decl.Name)
				checkName(decl.Pos, decl.Param.Name)
				class := &Class{Name: decl.Name, Param: &TVar{Name: decl.Param.Name, Kind: arityKind(decl.Param.Arity)}, Super: decl.Super, Decl: decl}
				c.program.Classes[decl.Name] = class
				for _, method := range decl.Methods {
					c.reserve(method.Pos, method.Name)
					m := &Method{Name: method.Name, Class: class}
					class.Methods = append(class.Methods, m)
					c.methods[m.Name] = m
				}
			case *ast.TypeDecl:
				c.reserve(decl.Pos, decl.Name)
				data := &DataType{Name: decl.Name}
				seen := make(map[string]bool)
				for _, name := range decl.TParams {
					checkName(decl.Pos, name)
					if seen[name] {
						fail(decl.Pos, "duplicate type parameter %s", name)
					}
					seen[name] = true
					data.TParams = append(data.TParams, &TVar{Name: name, Kind: KStar{}})
				}
				c.program.DataTypes[data.Name] = data
				c.cons[data.Name] = &TCon{Name: data.Name, Kind: arityKind(len(data.TParams))}
				for i, ctor := range decl.Ctors {
					c.reserve(ctor.Pos, ctor.Name)
					ct := &Ctor{Name: ctor.Name, Data: data, Index: i}
					data.Ctors = append(data.Ctors, ct)
					c.ctors[ct.Name] = ct
				}
			case *ast.FuncDecl:
				c.reserve(decl.Pos, decl.Name)
				c.program.Funcs[decl.Name] = &FuncInfo{Name: decl.Name, Decl: decl}
			}
		}
	}
}

func (c *checker) checkSuperclasses(files []*ast.File) {
	state := make(map[string]uint8)
	var visit func(*Class)
	visit = func(class *Class) {
		if state[class.Name] == 2 {
			return
		}
		if state[class.Name] == 1 {
			fail(class.Decl.Pos, "superclass cycle involving %s", class.Name)
		}
		state[class.Name] = 1
		if len(class.Decl.Param.Constraints) != 0 {
			fail(class.Decl.Pos, "class parameter constraints must use the superclass declaration")
		}
		if class.Super != "" {
			super := c.program.Classes[class.Super]
			if super == nil {
				fail(class.Decl.Pos, "unknown superclass %s", class.Super)
			}
			requireKind(class.Decl.Pos, class.Param.Kind, super.Param.Kind)
			visit(super)
		}
		state[class.Name] = 2
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			if decl, ok := decl.(*ast.ClassDecl); ok {
				visit(c.program.Classes[decl.Name])
			}
		}
	}
}

func paramScope(params []*TVar) map[string]Type {
	scope := make(map[string]Type, len(params))
	for _, p := range params {
		scope[p.Name] = p
	}
	return scope
}

func (c *checker) scheme(pos ast.Pos, tparams []ast.TypeParam, params []ast.Param, ret ast.Type, scope map[string]Type) *Scheme {
	s := &Scheme{}
	for _, p := range tparams {
		checkName(pos, p.Name)
		if scope[p.Name] != nil {
			fail(pos, "duplicate type parameter %s", p.Name)
		}
		v := &TVar{Name: p.Name, Kind: arityKind(p.Arity)}
		scope[p.Name] = v
		s.TParams = append(s.TParams, v)
		if len(p.Constraints) > 1 {
			fail(pos, "at most one constraint per type parameter %s", p.Name)
		}
		for _, name := range p.Constraints {
			class := c.program.Classes[name]
			if class == nil {
				fail(pos, "unknown class %s", name)
			}
			requireKind(pos, v.Kind, class.Param.Kind)
			s.Constraints = append(s.Constraints, Constraint{Class: name, Arg: v})
		}
	}
	s.Type = &TFunc{Params: make([]Type, len(params))}
	seen := make(map[string]bool)
	for i, p := range params {
		checkName(pos, p.Name)
		if seen[p.Name] {
			fail(pos, "duplicate parameter %s", p.Name)
		}
		seen[p.Name] = true
		s.Type.Params[i] = c.valueType(p.Type, scope)
	}
	s.Type.Ret = c.valueType(ret, scope)
	return s
}

func (c *checker) collectSignatures(files []*ast.File) {
	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.ClassDecl:
				class := c.program.Classes[decl.Name]
				for i, method := range decl.Methods {
					class.Methods[i].Scheme = c.scheme(method.Pos, method.TParams, method.Params, method.Ret, map[string]Type{class.Param.Name: class.Param})
				}
			case *ast.TypeDecl:
				data := c.program.DataTypes[decl.Name]
				scope := paramScope(data.TParams)
				for i, ctor := range decl.Ctors {
					for _, field := range ctor.Fields {
						data.Ctors[i].Fields = append(data.Ctors[i].Fields, c.valueType(field, scope))
					}
				}
			case *ast.FuncDecl:
				c.program.Funcs[decl.Name].Scheme = c.scheme(decl.Pos, decl.TParams, decl.Params, decl.Ret, make(map[string]Type))
			}
		}
	}
}

func (c *checker) resolveType(t ast.Type, scope map[string]Type) Type {
	switch t := t.(type) {
	case *ast.TypeName:
		base := scope[t.Name]
		if base == nil {
			if con := c.cons[t.Name]; con != nil {
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
		if !ok || len(k.Params) != len(t.Args) {
			fail(t.Pos, "kind mismatch applying %s to %d arguments", t.Name, len(t.Args))
		}
		args := make([]Type, len(t.Args))
		for i, arg := range t.Args {
			args[i] = c.resolveType(arg, scope)
			requireKind(typePos(arg), kindOf(args[i]), k.Params[i])
		}
		return &TApp{Fun: base, Args: args}
	case *ast.FuncType:
		f := &TFunc{Params: make([]Type, len(t.Params)), Ret: c.valueType(t.Ret, scope)}
		for i, param := range t.Params {
			f.Params[i] = c.valueType(param, scope)
		}
		return f
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
	default:
		panic("missing syntactic type")
	}
}

func (c *checker) valueType(t ast.Type, scope map[string]Type) Type {
	resolved := c.resolveType(t, scope)
	requireKind(typePos(t), kindOf(resolved), KStar{})
	return resolved
}

func (c *checker) collectInstances(files []*ast.File) {
	for _, file := range files {
		for _, decl := range file.Decls {
			d, ok := decl.(*ast.InstanceDecl)
			if !ok {
				continue
			}
			class, con := c.program.Classes[d.Class], c.cons[d.TypeCon]
			if class == nil {
				fail(d.Pos, "unknown class %s", d.Class)
			}
			if con == nil {
				fail(d.Pos, "unknown type constructor %s", d.TypeCon)
			}
			requireKind(d.Pos, con.Kind, class.Param.Kind)
			key := instanceKey{class.Name, con.Name}
			if c.instances[key] != nil {
				fail(d.Pos, "duplicate instance %s[%s]", class.Name, con.Name)
			}
			inst := &Instance{Class: class, TypeCon: con, Decl: d}
			c.instances[key] = inst
			c.program.Instances = append(c.program.Instances, inst)
			seen := make(map[string]bool)
			for _, m := range d.Methods {
				checkName(m.Pos, m.Name)
				for _, name := range m.Params {
					checkName(m.Pos, name)
				}
				method := c.methods[m.Name]
				if method == nil || method.Class != class {
					fail(m.Pos, "unknown method %s in instance %s", m.Name, class.Name)
				}
				if seen[m.Name] {
					fail(m.Pos, "duplicate method %s", m.Name)
				}
				seen[m.Name] = true
				if len(m.Params) != len(method.Scheme.Type.Params) {
					fail(m.Pos, "arity mismatch for method %s: want %d parameters, got %d", m.Name, len(method.Scheme.Type.Params), len(m.Params))
				}
			}
			for _, m := range class.Methods {
				if !seen[m.Name] {
					fail(d.Pos, "missing method %s in instance %s", m.Name, class.Name)
				}
			}
		}
	}
	for _, inst := range c.program.Instances {
		if super := inst.Class.Super; super != "" && c.instances[instanceKey{super, inst.TypeCon.Name}] == nil {
			fail(inst.Decl.Pos, "no instance %s[%s] (required superclass)", super, inst.TypeCon.Name)
		}
	}
}

func (c *checker) checkMain() {
	f := c.program.Funcs["main"]
	pos := ast.Pos{Line: 1, Col: 1}
	if f != nil {
		pos = f.Decl.Pos
		if len(f.Scheme.TParams) == 0 && len(f.Scheme.Type.Params) == 0 {
			if app, ok := f.Scheme.Type.Ret.(*TApp); ok && app.Fun == IO && len(app.Args) == 1 && app.Args[0] == Unit {
				c.program.Main = f
				return
			}
		}
	}
	fail(pos, "main must have type func() IO[Unit]")
}
