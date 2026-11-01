package types

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gofx/ast"
)

type instanceKey struct{ class, con string }

type checker struct {
	program    *Program
	cons       map[string]*TCon
	ctors      map[string]*Ctor
	methods    map[string]*Method
	instances  map[instanceKey]*Instance
	nextMeta   int
	file       string
	sources    map[ast.Decl]string
	derivedOrd []*ast.TypeDecl
	decl       ast.Decl // current source declaration for diagnostic spans
}

// Check collects all declarations before checking bodies, so signatures and
// instances may refer forward across files. Files share type, constructor and
// value namespaces and must be supplied in prelude-first order. Every expression,
// including literals in patterns, has a fully resolved Ann. No input annotation is used
// as evidence: the same parsed files may safely be checked again.
// Declared identifiers may not use the generated-code prefixes dict_ or ext_.
//
// List literals use the prelude List data type. Parameterized instance heads
// bind rigid variables; their contexts supply dictionaries during body checking
// and become recursive dictionary arguments at each concrete instance use.
// Tuples are builtin product types, and as-patterns bind their whole sub-value.
// Partial instance heads bind leading Fixed variables; applications and inferred
// higher-kinded instantiations are normalized to a single constructor and args.
//
// Unannotated name-bound let values generalize non-environment metas subject to
// the value restriction; constrained values expose LetScheme for dictionary codegen.
// Let annotations resolve in the enclosing type-parameter scope and stay rigid.
// Record patterns resolve constructors and named field indexes in RecordPats.
// Lambda, let and bind patterns must cover their whole type and bind monomorphic
// variables; match guards are Bool and do not contribute to coverage. Fields and
// tuple positions resolve indexes statically; interpolation resolves Show and
// comparisons resolve Eq/Ord. Deriving clauses expand once into ordinary instance
// declarations in each input file and are consumed before collection.
// Unconstrained ordinary metas (including comparison operands) default to Unit
// with source-ordered warnings. Higher-kinded or explicit class ambiguities
// remain errors. Failures return a positioned *Error and a nil program; partial
// annotations are not usable.
func Check(files []*ast.File) (program *Program, err error) {
	var c *checker
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*Error); ok {
				if c != nil {
					e.File = c.file
				}
				program, err = nil, e
			} else {
				panic(r)
			}
		}
		if e, ok := err.(*Error); ok && c != nil {
			c.completeSpan(e)
		}
	}()
	c = &checker{
		program: &Program{
			Classes: make(map[string]*Class), DataTypes: make(map[string]*DataType),
			Funcs: make(map[string]*FuncInfo), Files: files,
			RecordPats: make(map[*ast.PRecord]RecordPat),
		},
		cons: make(map[string]*TCon), ctors: make(map[string]*Ctor),
		methods: make(map[string]*Method), instances: make(map[instanceKey]*Instance),
		sources: make(map[ast.Decl]string),
	}
	for _, file := range files {
		c.file = file.Name
		for _, decl := range file.Decls {
			c.decl = decl
			if d, ok := decl.(*ast.TypeDecl); ok {
				for _, class := range d.Deriving {
					if class == "Ord" {
						c.derivedOrd = append(c.derivedOrd, d)
					}
				}
			}
		}
		if err := expandDeriving(file); err != nil {
			return nil, err
		}
	}
	for _, con := range []*TCon{Int, String, Bool, Unit, IO} {
		c.cons[con.Name] = con
	}
	c.collectNames(files)
	c.checkSuperclasses(files)
	c.collectSignatures(files)
	c.collectInstances(files)
	c.checkMain()
	for _, file := range files {
		c.file = file.Name
		for _, decl := range file.Decls {
			c.decl = decl
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Extern == nil {
					f := c.program.Funcs[decl.Name]
					b := c.body(f.Scheme.Constraints, paramScope(f.Scheme.TParams))
					b.check(decl.Params, f.Scheme.Type, decl.Body)
				}
			case *ast.InstanceDecl:
				inst := c.instances[instanceKey{decl.Class, decl.TypeCon}]
				head := inst.head()
				for _, method := range decl.Methods {
					m := c.methods[method.Name]
					sub := map[*TVar]Type{inst.Class.Param: head}
					scope := paramScope(inst.headParams())
					if scope[inst.Class.Param.Name] == nil {
						scope[inst.Class.Param.Name] = head
					}
					for _, param := range m.Scheme.TParams {
						scope[param.Name] = param
					}
					constraints := inst.Context
					if len(m.Scheme.Constraints) != 0 {
						constraints = make([]Constraint, len(inst.Context), len(inst.Context)+len(m.Scheme.Constraints))
						copy(constraints, inst.Context)
						constraints = append(constraints, subConstraints(m.Scheme.Constraints, sub)...)
					}
					b := c.body(constraints, scope)
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

func capitalised(name string) bool {
	first, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(first)
}

func checkValueName(pos ast.Pos, name string) {
	checkName(pos, name)
	first, _ := utf8.DecodeRuneInString(name)
	if !unicode.IsLower(first) && first != '_' {
		fail(pos, "value name must start with a lower-case letter")
	}
}

func (c *checker) typeCon(name string) *TCon {
	if con := c.cons[name]; con != nil {
		return con
	}
	if suffix, ok := strings.CutPrefix(name, "Tuple"); ok {
		if n, err := strconv.Atoi(suffix); err == nil && n >= 2 && strconv.Itoa(n) == suffix {
			return Tuple(n)
		}
	}
	return nil
}

func (c *checker) reserveType(pos ast.Pos, name string) {
	checkName(pos, name)
	if c.typeCon(name) != nil || c.program.Classes[name] != nil {
		fail(pos, "duplicate declaration %s", name)
	}
}

func (c *checker) reserveValue(pos ast.Pos, name string) {
	checkValueName(pos, name)
	if c.program.Funcs[name] != nil || c.methods[name] != nil {
		fail(pos, "duplicate declaration %s", name)
	}
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
		c.file = file.Name
		for _, decl := range file.Decls {
			c.decl = decl
			c.sources[decl] = file.Name
			switch decl := decl.(type) {
			case *ast.ClassDecl:
				c.reserveType(decl.Pos, decl.Name)
				checkName(decl.Pos, decl.Param.Name)
				class := &Class{Name: decl.Name, Param: &TVar{Name: decl.Param.Name, Kind: arityKind(decl.Param.Arity)}, Super: decl.Super, Decl: decl}
				c.program.Classes[decl.Name] = class
				for _, method := range decl.Methods {
					c.reserveValue(method.Pos, method.Name)
					m := &Method{Name: method.Name, Class: class}
					class.Methods = append(class.Methods, m)
					c.methods[m.Name] = m
				}
			case *ast.TypeDecl:
				c.reserveType(decl.Pos, decl.Name)
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
					checkName(ctor.Pos, ctor.Name)
					if !capitalised(ctor.Name) {
						fail(ctor.Pos, "constructor name must be capitalised")
					}
					if c.ctors[ctor.Name] != nil {
						fail(ctor.Pos, "duplicate declaration %s", ctor.Name)
					}
					seenFields := make(map[string]bool, len(ctor.FieldNames))
					for _, name := range ctor.FieldNames {
						checkValueName(ctor.Pos, name)
						if seenFields[name] {
							fail(ctor.Pos, "duplicate field %s in constructor %s", name, ctor.Name)
						}
						seenFields[name] = true
					}
					ct := &Ctor{Name: ctor.Name, Data: data, Index: i, FieldNames: ctor.FieldNames}
					data.Ctors = append(data.Ctors, ct)
					c.ctors[ct.Name] = ct
				}
			case *ast.FuncDecl:
				c.reserveValue(decl.Pos, decl.Name)
				c.program.Funcs[decl.Name] = &FuncInfo{Name: decl.Name, Decl: decl}
			}
		}
	}
}

func (c *checker) checkSuperclasses(files []*ast.File) {
	state := make(map[string]uint8)
	var visit func(*Class)
	visit = func(class *Class) {
		c.file = c.sources[class.Decl]
		c.decl = class.Decl
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
		c.file = file.Name
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
		checkValueName(pos, p.Name)
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
		c.file = file.Name
		for _, decl := range file.Decls {
			c.decl = decl
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

func (c *checker) collectInstances(files []*ast.File) {
	for _, file := range files {
		c.file = file.Name
		for _, decl := range file.Decls {
			c.decl = decl
			d, ok := decl.(*ast.InstanceDecl)
			if !ok {
				continue
			}
			class, con := c.program.Classes[d.Class], c.typeCon(d.TypeCon)
			if class == nil {
				fail(d.Pos, "unknown class %s", d.Class)
			}
			if con == nil {
				fail(d.Pos, "unknown type constructor %s", d.TypeCon)
			}
			names := d.Params
			if len(d.Fixed) != 0 {
				names = d.Fixed
			}
			residual := con.Kind
			if len(names) != 0 {
				kind, ok := con.Kind.(KArrow)
				if !ok || len(names) > len(kind.Params) {
					fail(d.Pos, "kind mismatch applying %s to %d arguments", con.Name, len(names))
				}
				residual = kind.Ret
				if len(names) < len(kind.Params) {
					residual = KArrow{Params: kind.Params[len(names):], Ret: kind.Ret}
				}
				requireKind(d.Pos, class.Param.Kind, residual)
			} else {
				requireKind(d.Pos, con.Kind, class.Param.Kind)
			}
			key := instanceKey{class.Name, con.Name}
			if c.instances[key] != nil {
				fail(d.Pos, "duplicate instance %s[%s]", class.Name, con.Name)
			}
			inst := &Instance{Class: class, TypeCon: con, Decl: d}
			scope := make(map[string]*TVar, len(names))
			for _, name := range names {
				checkName(d.Pos, name)
				if scope[name] != nil {
					fail(d.Pos, "duplicate type parameter %s", name)
				}
				param := &TVar{Name: name, Kind: KStar{}}
				scope[name] = param
				if _, partial := residual.(KArrow); partial {
					inst.Fixed = append(inst.Fixed, param)
				} else {
					inst.Params = append(inst.Params, param)
				}
			}
			constrained := make(map[string]bool, len(d.Context))
			for _, constraint := range d.Context {
				required := c.program.Classes[constraint.Class]
				if required == nil {
					fail(constraint.Pos, "unknown class %s", constraint.Class)
				}
				param := scope[constraint.Param]
				if param == nil {
					fail(constraint.Pos, "unknown instance type parameter %s", constraint.Param)
				}
				if constrained[param.Name] {
					fail(constraint.Pos, "at most one constraint per type parameter %s", param.Name)
				}
				constrained[param.Name] = true
				requireKind(constraint.Pos, param.Kind, required.Param.Kind)
				inst.Context = append(inst.Context, Constraint{Class: constraint.Class, Arg: param})
			}
			c.instances[key] = inst
			c.program.Instances = append(c.program.Instances, inst)
			seen := make(map[string]bool)
			for _, m := range d.Methods {
				checkValueName(m.Pos, m.Name)
				for _, name := range m.Params {
					checkValueName(m.Pos, name)
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
	for _, decl := range c.derivedOrd {
		c.file = c.sources[decl]
		c.decl = decl
		if c.instances[instanceKey{"Eq", decl.Name}] == nil {
			fail(decl.Pos, "deriving Ord for %s requires an Eq instance", decl.Name)
		}
	}
	for _, inst := range c.program.Instances {
		c.file = c.sources[inst.Decl]
		c.decl = inst.Decl
		if super := inst.Class.Super; super != "" {
			parent := c.instances[instanceKey{super, inst.TypeCon.Name}]
			if parent == nil {
				fail(inst.Decl.Pos, "no instance %s[%s] (required superclass)%s", super, inst.TypeCon.Name, c.instanceHint(super, inst.head()))
			}
			args, ok := c.superclassArgs(inst, parent)
			if !ok {
				fail(inst.Decl.Pos, "superclass instance %s[%s] context is not implied", super, inst.TypeCon.Name)
			}
			inst.SuperArgs = args
		}
	}
}

func (inst *Instance) headParams() []*TVar {
	if len(inst.Fixed) != 0 {
		return inst.Fixed
	}
	return inst.Params
}

func (inst *Instance) head() Type {
	params := inst.headParams()
	args := make([]Type, len(params))
	for i, param := range params {
		args[i] = param
	}
	return App(inst.TypeCon, args...)
}

// Superclass contexts are expressed using the subclass instance's variables.
// Resolving each required dictionary reuses the same superclass projection as
// method calls and permits stronger, reordered, or additional child constraints.
func (c *checker) superclassArgs(inst, parent *Instance) ([]DictRef, bool) {
	if len(inst.Fixed) != len(parent.Fixed) || len(inst.Params) != len(parent.Params) {
		return nil, false
	}
	if len(parent.Context) == 0 {
		return nil, true
	}
	b := c.body(inst.Context, nil)
	args := make([]DictRef, len(parent.Context))
	for i, constraint := range parent.Context {
		for j, param := range parent.headParams() {
			if constraint.Arg == param {
				constraint.Arg = inst.headParams()[j]
				break
			}
		}
		dict, message := b.resolveDictionary(constraint)
		if message != "" {
			return nil, false
		}
		args[i] = dict
	}
	return args, true
}

func (c *checker) checkMain() {
	f := c.program.Funcs["main"]
	pos := ast.Pos{Line: 1, Col: 1}
	if f != nil {
		c.file = c.sources[f.Decl]
		c.decl = f.Decl
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
