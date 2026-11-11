package types

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/reoring/go4t/ast"
)

type instanceKey struct{ class, con string }

type checker struct {
	program             *Program
	cons                map[string]*TCon
	aliases             map[string]*typeAlias
	instDecls           map[*ast.InstanceDecl]*Instance
	ctors               map[string]*Ctor
	methods             map[string]*Method
	instances           map[instanceKey]*Instance
	nextMeta            int
	file                string
	sources             map[ast.Decl]string
	derivedOrd          []*ast.TypeDecl
	decl                ast.Decl // current source declaration for diagnostic spans
	mainName            string
	moduleFiles         map[string]string
	foreignAmbiguous    map[*ast.Ident]bool
	hasForeignTemplates bool
}

// Check collects module declarations before checking bodies. Imports are
// file-scoped, instances are global, and the entry module is last. Expressions
// in Program.Files carry resolved annotations; named input modules are preserved.
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
//
// Laws and proofs form a separate, module-owned namespace. Their expressions
// receive ordinary annotations, but equations impose no Eq constraint and
// neither declaration is emitted as a function. Tactic split variables retain
// their types in ProofInfo.PatVars; proof and law binders are documentation,
// rather than candidates for unused-variable warnings.
func Check(modules []*ast.Module) (program *Program, err error) {
	var c *checker
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*Error); ok {
				if c != nil && e.File == "" {
					e.File = c.file
				}
				program, err = nil, e
			} else {
				panic(r)
			}
		}
		if e, ok := err.(*Error); ok && c != nil {
			e.names = c.program.namesFor(e.File)
			if e.detail != nil {
				e.Msg = e.detail.render(false, e.names)
			}
			c.completeSpan(e)
		}
	}()
	resolved, files := qualifyModules(modules)
	c = &checker{
		program: &Program{
			Classes: make(map[string]*Class), DataTypes: make(map[string]*DataType),
			Funcs: make(map[string]*FuncInfo), Files: files,
			Laws: make(map[string]*LawInfo), Proofs: make(map[string]*ProofInfo),
			Modules: resolved.modules, ModuleNames: resolved.names, Owners: resolved.owners,
			ForeignRecordKeys: resolved.foreignRecordKeys,
			ForeignHandleKeys: resolved.foreignHandleKeys,
			Warnings:          resolved.orderedWarnings(),
			RecordPats:        make(map[*ast.PRecord]RecordPat),
		},
		cons: make(map[string]*TCon), ctors: make(map[string]*Ctor),
		methods: make(map[string]*Method), instances: make(map[instanceKey]*Instance),
		aliases: make(map[string]*typeAlias), instDecls: make(map[*ast.InstanceDecl]*Instance),
		sources:  make(map[ast.Decl]string),
		mainName: resolved.mainName(), moduleFiles: make(map[string]string),
	}
	for _, module := range resolved.modules {
		for _, file := range module.Files {
			c.moduleFiles[file.Name] = module.Path
		}
	}
	expandJSONTuples(files)
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
		if err := expandDeriving(file, c.program.ModuleNames); err != nil {
			return nil, err
		}
	}
	for _, con := range []*TCon{Int, Float, String, Bool, Unit, GoError, IO, Map, Set, Array, Fiber, Ref, MutMap, Chan, RecvChan, SendChan} {
		c.cons[con.Name] = con
	}
	c.collectNames(files)
	c.checkAliases(files)
	c.checkSuperclasses(files)
	c.collectSignatures(files)
	for _, f := range c.program.Funcs {
		c.hasForeignTemplates = c.hasForeignTemplates || foreignTemplate(f)
	}
	c.collectInstances(files)
	c.pairLaws(files)
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
			case *ast.ConstDecl:
				f := c.program.Funcs[decl.Name]
				b := c.body(nil, nil)
				b.check(nil, f.Scheme.Type, f.Decl.Body)
			case *ast.InstanceDecl:
				inst := c.instDecls[decl]
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
	c.checkConstantCycles()
	c.checkLaws(files)
	c.instantiateForeign()
	c.program.warnRecursion()
	return c.program, nil
}

func fail(pos ast.Pos, format string, args ...any) {
	panic(NewError(pos, format, args...))
}

func checkName(pos ast.Pos, name string) {
	if strings.HasPrefix(name, "dict_") || strings.HasPrefix(name, "ext_") {
		fail(pos, "identifier %q uses reserved prefix", name)
	}
}

func capitalised(name string) bool {
	first, _ := utf8.DecodeRuneInString(strings.TrimLeft(name, "_"))
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
	if c.typeCon(name) != nil || c.program.Classes[name] != nil || c.aliases[name] != nil {
		fail(pos, "duplicate declaration %s", diagnosticName(name))
	}
}

func (c *checker) reserveValue(pos ast.Pos, name string) {
	checkValueName(pos, name)
	if c.program.Funcs[name] != nil || c.methods[name] != nil || c.program.Laws[name] != nil {
		fail(pos, "duplicate declaration %s", diagnosticName(name))
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
				c.cons[data.Name] = &TCon{Name: data.Name, Display: c.program.ModuleNames[data.Name], Kind: arityKind(len(data.TParams))}
				for i, ctor := range decl.Ctors {
					checkName(ctor.Pos, ctor.Name)
					if !capitalised(ctor.Name) {
						fail(ctor.Pos, "constructor name must be capitalised")
					}
					if c.ctors[ctor.Name] != nil {
						fail(ctor.Pos, "duplicate declaration %s", diagnosticName(ctor.Name))
					}
					seenFields := make(map[string]bool, len(ctor.FieldNames))
					for _, name := range ctor.FieldNames {
						checkValueName(ctor.Pos, name)
						if seenFields[name] {
							fail(ctor.Pos, "duplicate field %s in constructor %s", name, diagnosticName(ctor.Name))
						}
						seenFields[name] = true
					}
					ct := &Ctor{Name: ctor.Name, Data: data, Index: i, FieldNames: ctor.FieldNames}
					data.Ctors = append(data.Ctors, ct)
					c.ctors[ct.Name] = ct
				}
			case *ast.AliasDecl:
				c.reserveType(decl.Pos, decl.Name)
				c.collectAlias(decl)
			case *ast.FuncDecl:
				c.reserveValue(decl.Pos, decl.Name)
				c.program.Funcs[decl.Name] = &FuncInfo{Name: decl.Name, Decl: decl}
			case *ast.ConstDecl:
				c.reserveValue(decl.Pos, decl.Name)
				c.program.Funcs[decl.Name] = &FuncInfo{Name: decl.Name, Decl: constantFunction(decl), Constant: decl}
			case *ast.LawDecl:
				checkName(decl.Pos, decl.Name)
				if c.program.Laws[decl.Name] != nil {
					fail(decl.Pos, "duplicate law %s", diagnosticName(decl.Name))
				}
				if c.program.Funcs[decl.Name] != nil || c.methods[decl.Name] != nil {
					fail(decl.Pos, "duplicate declaration %s", diagnosticName(decl.Name))
				}
				// ADR 0021 is not implemented yet: reject rather than ignore the
				// quantifier.
				if len(decl.TParams) > 0 {
					fail(decl.Pos, "law type parameters are not supported")
				}
				c.program.Laws[decl.Name] = &LawInfo{Decl: decl}
			case *ast.ProofDecl:
				checkName(decl.Pos, decl.Name)
				if c.program.Proofs[decl.Name] != nil {
					fail(decl.Pos, "duplicate proof %s", diagnosticName(decl.Name))
				}
				c.program.Proofs[decl.Name] = &ProofInfo{Decl: decl, PatVars: make(map[*ast.PVar]Type)}
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
			fail(class.Decl.Pos, "superclass cycle involving %s", diagnosticName(class.Name))
		}
		state[class.Name] = 1
		if len(class.Decl.Param.Constraints) != 0 {
			fail(class.Decl.Pos, "class parameter constraints must use the superclass declaration")
		}
		if class.Super != "" {
			super := c.program.Classes[class.Super]
			if super == nil {
				fail(class.Decl.Pos, "unknown superclass %s", diagnosticName(class.Super))
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
		seen := make(map[string]bool, len(p.Constraints))
		for _, name := range p.Constraints {
			if seen[name] {
				fail(pos, "duplicate constraint %s for type parameter %s", diagnosticName(name), p.Name)
			}
			seen[name] = true
			class := c.program.Classes[name]
			if class == nil {
				fail(pos, "unknown class %s", diagnosticName(name))
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
			fail(pos, "duplicate parameter %s", p.DisplayName())
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
			case *ast.ConstDecl:
				f := c.program.Funcs[decl.Name]
				f.Scheme = c.scheme(decl.Pos, nil, nil, f.Decl.Ret, nil)
				if ioItem(f.Scheme.Type.Ret) != nil {
					fail(decl.Pos, "a top-level let cannot be an IO action; write func %s() IO[…]", diagnosticName(decl.Name))
				}
			case *ast.LawDecl:
				law := c.program.Laws[decl.Name]
				for _, param := range decl.Params {
					law.ParamTypes = append(law.ParamTypes, c.valueType(param.Type, nil))
				}
			}
		}
	}
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
			class := c.program.Classes[d.Class]
			if class == nil {
				fail(d.Pos, "unknown class %s", diagnosticName(d.Class))
			}
			names := d.Params
			if len(d.Fixed) != 0 {
				names = d.Fixed
			}
			con, names := c.instanceHead(d, names)
			residual := con.Kind
			if len(names) != 0 {
				kind, ok := con.Kind.(KArrow)
				if !ok || len(names) > len(kind.Params) {
					fail(d.Pos, "kind mismatch applying %s to %d arguments", con, len(names))
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
			if previous := c.instances[key]; previous != nil {
				oldModule := c.moduleFiles[c.sources[previous.Decl]]
				newModule := c.moduleFiles[file.Name]
				if oldModule != newModule {
					fail(d.Pos, "duplicate instance %s[%s] in modules %s and %s", diagnosticName(class.Name), con, oldModule, newModule)
				}
				fail(d.Pos, "duplicate instance %s[%s]", diagnosticName(class.Name), con)
			}
			inst := &Instance{Class: class, TypeCon: con, Decl: d}
			scope := make(map[string]*TVar, len(names))
			for _, name := range names {
				checkName(d.Pos, name)
				// User-defined names remain fresh variables: a user's type A
				// must not specialize every generic prelude instance using A.
				if concrete := c.typeCon(name); concrete == Int || concrete == Float || concrete == String || concrete == Bool || concrete == Unit {
					inst.HeadArgs = append(inst.HeadArgs, concrete)
					continue
				}
				if scope[name] != nil {
					fail(d.Pos, "duplicate type parameter %s", name)
				}
				param := &TVar{Name: name, Kind: KStar{}}
				scope[name] = param
				inst.HeadArgs = append(inst.HeadArgs, param)
				if _, partial := residual.(KArrow); partial {
					inst.Fixed = append(inst.Fixed, param)
				} else {
					inst.Params = append(inst.Params, param)
				}
			}
			constrained := make(map[Constraint]bool, len(d.Context))
			for _, constraint := range d.Context {
				required := c.program.Classes[constraint.Class]
				if required == nil {
					fail(constraint.Pos, "unknown class %s", diagnosticName(constraint.Class))
				}
				param := scope[constraint.Param]
				if param == nil {
					fail(constraint.Pos, "unknown instance type parameter %s", constraint.Param)
				}
				key := Constraint{Class: constraint.Class, Arg: param}
				if constrained[key] {
					fail(constraint.Pos, "duplicate constraint %s for type parameter %s", diagnosticName(constraint.Class), param.Name)
				}
				constrained[key] = true
				requireKind(constraint.Pos, param.Kind, required.Param.Kind)
				inst.Context = append(inst.Context, key)
			}
			c.instances[key] = inst
			c.instDecls[d] = inst
			c.program.Instances = append(c.program.Instances, inst)
			seen := make(map[string]bool)
			for _, m := range d.Methods {
				checkValueName(m.Pos, m.Name)
				for _, name := range m.Params {
					checkValueName(m.Pos, name)
				}
				method := c.methods[m.Name]
				if method == nil || method.Class != class {
					fail(m.Pos, "unknown method %s in instance %s", diagnosticName(m.Name), diagnosticName(class.Name))
				}
				if seen[m.Name] {
					fail(m.Pos, "duplicate method %s", diagnosticName(m.Name))
				}
				seen[m.Name] = true
				if len(m.Params) != len(method.Scheme.Type.Params) {
					fail(m.Pos, "arity mismatch for method %s: want %d parameters, got %d", diagnosticName(m.Name), len(method.Scheme.Type.Params), len(m.Params))
				}
			}
			for _, m := range class.Methods {
				if !seen[m.Name] {
					fail(d.Pos, "missing method %s in instance %s", diagnosticName(m.Name), diagnosticName(class.Name))
				}
			}
		}
	}
	for _, decl := range c.derivedOrd {
		c.file = c.sources[decl]
		c.decl = decl
		if c.instances[instanceKey{"Eq", decl.Name}] == nil {
			fail(decl.Pos, "deriving Ord for %s requires an Eq instance", diagnosticName(decl.Name))
		}
	}
	for _, inst := range c.program.Instances {
		c.file = c.sources[inst.Decl]
		c.decl = inst.Decl
		if super := inst.Class.Super; super != "" {
			parent := c.instances[instanceKey{super, inst.TypeCon.Name}]
			if parent == nil {
				fail(inst.Decl.Pos, "no instance %s[%s] (required superclass)%s", diagnosticName(super), inst.TypeCon, c.instanceHint(super, inst.head()))
			}
			args, ok := c.superclassArgs(inst, parent)
			if !ok {
				fail(inst.Decl.Pos, "superclass instance %s[%s] context is not implied", diagnosticName(super), inst.TypeCon)
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
	return App(inst.TypeCon, inst.HeadArgs...)
}

// Direct heads may fix nullary types (Map[String, V]); matching stays
// constructor-keyed and never unifies or overlaps competing instances.
func (inst *Instance) matchHead(args []Type) (map[*TVar]Type, bool) {
	if len(args) != len(inst.HeadArgs) {
		return nil, false
	}
	var sub map[*TVar]Type
	for i, head := range inst.HeadArgs {
		if param, ok := head.(*TVar); ok {
			if sub == nil {
				sub = make(map[*TVar]Type, len(inst.headParams()))
			}
			sub[param] = args[i]
		} else if prune(args[i]) != head {
			return nil, false
		}
	}
	return sub, true
}

// Superclass contexts are expressed using the subclass instance's variables.
// Resolving each required dictionary reuses the same superclass projection as
// method calls and permits stronger, reordered, or additional child constraints.
func (c *checker) superclassArgs(inst, parent *Instance) ([]DictRef, bool) {
	sub, matches := parent.matchHead(inst.HeadArgs)
	if !matches {
		return nil, false
	}
	if len(parent.Context) == 0 {
		return nil, true
	}
	b := c.body(inst.Context, nil)
	args := make([]DictRef, len(parent.Context))
	for i, constraint := range parent.Context {
		constraint.Arg = substitute(constraint.Arg, sub)
		dict, message := b.resolveDictionary(constraint)
		if message != nil {
			return nil, false
		}
		args[i] = dict
	}
	return args, true
}

func (c *checker) checkMain() {
	f := c.program.Funcs[c.mainName]
	pos := ast.Pos{Line: 1, Col: 1}
	if f != nil {
		c.file = c.sources[f.Decl]
		c.decl = f.Decl
		pos = f.Decl.Pos
		if len(f.Scheme.TParams) == 0 && len(f.Scheme.Type.Params) == 0 {
			if app, ok := prune(f.Scheme.Type.Ret).(*TApp); ok && prune(app.Fun) == IO && len(app.Args) == 1 && prune(app.Args[0]) == Unit {
				c.program.Main = f
				return
			}
		}
	}
	fail(pos, "main must have type func() IO[Unit]")
}
