package types

import (
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/reoring/go4t/ast"
)

// PreludeExports is the deliberately small implicit namespace. A reexported
// data declaration brings its constructors, but not unrelated class methods.
var PreludeExports = strings.Fields("Functor Monad Show Debug Eq Ord Foldable Traversable Ordering Option Result List Unit map flatMap fold filter length append reverse head tail show debug println print eprintln ioPure ioFail unit fst snd compare eq min max toFloat LT EQ GT None Some Err Ok Nil Cons")

var preludeOwners = map[string]string{
	"List": "std/list", "Nil": "std/list", "Cons": "std/list", "fold": "std/list", "filter": "std/list", "length": "std/list", "append": "std/list", "reverse": "std/list", "head": "std/list", "tail": "std/list",
	"Result": "std/result", "Err": "std/result", "Ok": "std/result",
	"println": "std/io", "print": "std/io", "eprintln": "std/io", "ioPure": "std/io", "ioFail": "std/io", "toFloat": "std/float",
}

// JSON deriving and runtime shape registration use these canonical identities,
// without adding any names to the implicit prelude namespace.
var jsonIntrinsics = strings.Fields("Json JNull JBool JInt JNum JStr JArr JObj Encode Decode encode decode _jsonObject _jsonInt _jsonErrorAt _jsonFieldPath _jsonAt _jsonDecodeAt _jsonField _jsonArray _jsonValues _jsonDecodeList _jsonDecodeFields")

type moduleScope struct {
	module        *ast.Module
	values, types map[string]string
	laws, proofs  map[string]string
	prefix        string
	entry         bool
}
type moduleResolver struct {
	modules                       []*ast.Module
	scopes                        map[string]*moduleScope
	implicitValues, implicitTypes map[string]string
	methods                       map[string]map[string]string
	names, owners                 map[string]string
	foreignRecordKeys             map[string]string
	foreignHandleKeys             map[string]ast.ForeignHandle
	warnings                      []*Error
	file                          *ast.File
	scope                         *moduleScope
	imports                       map[string]*moduleScope
}

// qualifyModules separates source namespaces before the ordinary HM checker.
// Syntax is copied, not rewritten in the caller: repeated checks and editor
// queries retain source spellings and positions, even after a failed check.
func qualifyModules(input []*ast.Module) (*moduleResolver, []*ast.File) {
	r := &moduleResolver{scopes: make(map[string]*moduleScope), methods: make(map[string]map[string]string), names: make(map[string]string), owners: make(map[string]string), implicitValues: make(map[string]string), implicitTypes: make(map[string]string)}
	r.foreignRecordKeys = make(map[string]string)
	r.foreignHandleKeys = make(map[string]ast.ForeignHandle)
	// An unnamed compilation unit has no import identity; useful for embedding
	// the checker with an explicitly supplied language environment.
	if len(input) == 1 && input[0].Path == "" {
		r.modules = input
		r.foreignRecordKeys = input[0].ForeignRecordKeys
		r.foreignHandleKeys = input[0].ForeignHandleKeys
		return r, input[0].Files
	}
	prefixes := make(map[string]bool)
	standard := make(map[string]*moduleScope)
	for index, original := range input {
		m := cloneSyntax(reflect.ValueOf(original)).Interface().(*ast.Module)
		if r.scopes[m.Path] != nil {
			fail(ast.Pos{Line: 1, Col: 1}, "duplicate module %s", m.Path)
		}
		prefix := identifier(path.Base(strings.ReplaceAll(m.Path, "\\", "/")))
		base := prefix
		for index := 2; prefixes[prefix]; index++ {
			prefix = fmt.Sprintf("%s_%d", base, index)
		}
		prefixes[prefix] = true
		s := &moduleScope{
			module: m, values: make(map[string]string), types: make(map[string]string),
			laws: make(map[string]string), proofs: make(map[string]string),
			prefix: prefix, entry: index == len(input)-1,
		}
		r.scopes[m.Path] = s
		if strings.HasPrefix(m.ImportPath, "std/") {
			standard[m.ImportPath] = s
		}
		r.modules = append(r.modules, m)
		for _, f := range m.Files {
			r.file = f
			if m.Name != "" && f.Package != m.Name {
				r.fail(ast.Pos{Line: 1, Col: 1}, "package %s differs from %s", f.Package, m.Name)
			}
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					r.declare(s, s.values, d.Name, d.Pos, false)
				case *ast.LawDecl:
					if s.laws[d.Name] != "" {
						r.fail(d.Pos, "duplicate law %s", d.Name)
					}
					if s.values[d.Name] != "" {
						r.fail(d.Pos, "duplicate declaration %s", d.Name)
					}
					r.declare(s, s.laws, d.Name, d.Pos, true)
				case *ast.ProofDecl:
					if s.proofs[d.Name] != "" {
						r.fail(d.Pos, "duplicate proof %s", d.Name)
					}
					r.declare(s, s.proofs, d.Name, d.Pos, true)
				case *ast.TypeDecl:
					canonical := r.declare(s, s.types, d.Name, d.Pos, true)
					if key := m.ForeignRecordKeys[d.Name]; key != "" {
						r.foreignRecordKeys[canonical] = key
					}
					if handle, ok := m.ForeignHandleKeys[d.Name]; ok {
						r.foreignHandleKeys[canonical] = handle
					}
					for _, c := range d.Ctors {
						r.declare(s, s.values, c.Name, c.Pos, true)
					}
				case *ast.AliasDecl:
					r.declare(s, s.types, d.Name, d.Pos, true)
				case *ast.ClassDecl:
					class := r.declare(s, s.types, d.Name, d.Pos, true)
					methods := make(map[string]string)
					for _, m := range d.Methods {
						methods[m.Name] = r.declare(s, s.values, m.Name, m.Pos, false)
					}
					r.methods[class] = methods
				}
			}
		}
	}
	for module, names := range map[string][]string{
		"std/prelude": {"Unit", "GoError"}, "std/map": {"Map"}, "std/set": {"Set"},
		"std/array": {"Array"}, "std/conc": {"Fiber"}, "std/io": {"IO"},
		"std/string": {"String"}, "std/float": {"Float"}, "std/ref": {"Ref"},
		"std/mutmap": {"MutMap"},
		"std/chan":   {"Chan", "RecvChan", "SendChan"},
	} {
		if scope := standard[module]; scope != nil {
			for _, name := range names {
				if scope.types[name] == "" {
					scope.types[name] = name
				}
			}
		}
	}
	prelude := standard["std/prelude"]
	if prelude != nil {
		// Link reexports by declaration identity rather than copying declarations.
		for name, owner := range preludeOwners {
			if s := standard[owner]; s != nil {
				if n := s.values[name]; n != "" {
					prelude.values[name] = n
				}
				if n := s.types[name]; n != "" {
					prelude.types[name] = n
				}
			}
		}
		for _, name := range PreludeExports {
			if n := prelude.values[name]; n != "" {
				r.implicitValues[name] = n
			}
			if n := prelude.types[name]; n != "" {
				r.implicitTypes[name] = n
			}
		}
	}
	var files []*ast.File
	for _, m := range r.modules {
		r.scope = r.scopes[m.Path]
		for _, f := range m.Files {
			r.file = f
			r.imports = make(map[string]*moduleScope)
			for _, imp := range f.Imports {
				alias := imp.Alias
				if alias == "" {
					alias = path.Base(imp.Path)
				}
				target := imp.Resolved
				if target == "" {
					target = imp.Path
				}
				s := r.scopes[target]
				if s == nil {
					r.fail(imp.Pos, "module %q not loaded", imp.Path)
				}
				if alias == "_" || alias == "." || r.imports[alias] != nil {
					r.fail(imp.Pos, "duplicate or invalid import alias %s", alias)
				}
				r.imports[alias] = s
			}
			for _, d := range f.Decls {
				r.declaration(d)
			}
			files = append(files, f)
		}
	}
	return r, files
}

func identifier(s string) string {
	var b strings.Builder
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "module"
	}
	name := strings.ToLower(b.String())
	if name[0] >= '0' && name[0] <= '9' {
		return "module_" + name
	}
	return name
}
func (r *moduleResolver) fail(pos ast.Pos, format string, args ...any) {
	var end ast.Pos
	for _, decl := range r.file.Decls {
		if candidate := ast.EndAt(decl, pos); candidate != (ast.Pos{}) {
			end = candidate
		}
	}
	panic(&Error{File: r.file.Name, Pos: pos, End: end, Msg: fmt.Sprintf(format, args...)})
}
func (r *moduleResolver) declare(s *moduleScope, table map[string]string, name string, pos ast.Pos, typ bool) string {
	checkName(pos, name)
	if !typ {
		checkValueName(pos, name)
		if s.laws[name] != "" {
			r.fail(pos, "duplicate declaration %s", name)
		}
	}
	if table[name] != "" {
		r.fail(pos, "duplicate declaration %s", name)
	}
	canonical := s.prefix + "_" + name
	if typ && capitalised(strings.TrimLeft(name, "_")) {
		canonical = strings.ToUpper(canonical[:1]) + canonical[1:]
	}
	if s.entry && !contains(PreludeExports, name) && !contains(strings.Fields("fmap pure bind foldr traverse"), name) {
		canonical = name
	}
	owner := preludeOwners[name]
	if owner == "" {
		owner = "std/prelude"
	}
	// Fixed compiler primitives (desugaring and deriving) retain semantic names;
	// codegen gives them the same module-prefixed Go ABI as other declarations.
	if s.module.ImportPath == owner && (contains(PreludeExports, name) || contains(strings.Fields("fmap pure bind foldr traverse"), name)) {
		canonical = name
	}
	if s.module.ImportPath == "std/json" && contains(jsonIntrinsics, name) {
		canonical = name
	}
	for r.owners[canonical] != "" && r.owners[canonical] != s.module.Path {
		canonical += "_"
	}
	table[name] = canonical
	r.names[canonical] = name
	r.owners[canonical] = s.module.Path
	return canonical
}
func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
func builtinType(name string) bool {
	switch name {
	case "Int", "Float", "String", "Bool", "Unit", "GoError", "IO", "Map", "Set", "Array", "Fiber", "Ref", "MutMap", "Chan", "RecvChan", "SendChan":
		return true
	}
	return strings.HasPrefix(name, "Tuple")
}
func (r *moduleResolver) lookup(name string, pos ast.Pos, typ bool) string {
	own, implicit := r.scope.values, r.implicitValues
	if typ {
		own, implicit = r.scope.types, r.implicitTypes
	}
	if alias, member, ok := strings.Cut(name, "."); ok {
		target := r.imports[alias]
		if target == nil {
			r.fail(pos, "unknown import alias %s", alias)
		}
		if strings.HasPrefix(member, "_") {
			r.fail(pos, "%s is private to module %s", member, target.module.Path)
		}
		table := target.values
		if typ {
			table = target.types
		}
		if resolved := table[member]; resolved != "" {
			return resolved
		}
		if typ && table[member+"Go"] != "" && contains(PreludeExports, member) {
			r.fail(pos, "module %s has no exported name %s; the Go type %s is bound as %s.%sGo because %s is a prelude name", target.module.Path, member, member, alias, member, member)
		}
		r.fail(pos, "module %s has no exported name %s", target.module.Path, member)
	}
	if resolved := own[name]; resolved != "" {
		return resolved
	}
	if resolved := implicit[name]; resolved != "" {
		return resolved
	}
	if typ && builtinType(name) {
		return name
	}
	kind := "identifier"
	if typ {
		kind = "type"
	}
	r.fail(pos, "unknown %s %s", kind, name)
	return ""
}
func scopeCopy(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (r *moduleResolver) tparams(ps []ast.TypeParam, scope map[string]bool) {
	for _, p := range ps {
		scope[p.Name] = true
	}
	for i := range ps {
		for j, c := range ps[i].Constraints {
			ps[i].Constraints[j] = r.lookup(c, r.file.Decls[0].GetPos(), true)
		}
	}
}
func (r *moduleResolver) typ(t ast.Type, scope map[string]bool) {
	if t == nil {
		return
	}
	switch t := t.(type) {
	case *ast.TypeName:
		if !scope[t.Name] {
			t.Name = r.lookup(t.Name, t.Pos, true)
		}
		for _, a := range t.Args {
			r.typ(a, scope)
		}
	case *ast.FuncType:
		for _, p := range t.Params {
			r.typ(p, scope)
		}
		r.typ(t.Ret, scope)
	case *ast.TupleType:
		for _, e := range t.Elems {
			r.typ(e, scope)
		}
	}
}
func (r *moduleResolver) shadow(name string, pos ast.Pos) {
	if path := r.scope.module.ImportPath; strings.HasPrefix(path, "std/") || strings.HasPrefix(path, "go:") {
		return
	}
	if r.implicitValues[name] != "" || r.implicitTypes[name] != "" {
		r.warnings = append(r.warnings, &Error{File: r.file.Name, Pos: pos, Msg: "shadows prelude name " + name})
	}
}
func (r *moduleResolver) declaration(d ast.Decl) {
	ts := make(map[string]bool)
	switch d := d.(type) {
	case *ast.FuncDecl:
		r.shadow(d.Name, d.Pos)
		d.Name = r.scope.values[d.Name]
		r.function(d, ts, make(map[string]bool))
	case *ast.LawDecl:
		d.Name = r.scope.laws[d.Name]
		vs := make(map[string]bool, len(d.Params))
		for _, p := range d.Params {
			r.typ(p.Type, ts)
			vs[p.Name] = true
		}
		if d.Requires != nil {
			d.Requires = r.expr(d.Requires, ts, vs)
		}
		d.LHS = r.expr(d.LHS, ts, vs)
		d.RHS = r.expr(d.RHS, ts, vs)
	case *ast.ProofDecl:
		d.Name = r.scope.proofs[d.Name]
		vs := make(map[string]bool, len(d.Params))
		for _, p := range d.Params {
			vs[p.Name] = true
		}
		r.tactic(d.Body, ts, vs)
	case *ast.TypeDecl:
		r.shadow(d.Name, d.Pos)
		d.Name = r.scope.types[d.Name]
		for _, p := range d.TParams {
			ts[p] = true
		}
		for i := range d.Ctors {
			ctor := &d.Ctors[i]
			r.shadow(ctor.Name, ctor.Pos)
			ctor.Name = r.scope.values[ctor.Name]
			for _, f := range ctor.Fields {
				r.typ(f, ts)
			}
		}
		for i, c := range d.Deriving {
			if r.scope.types[c] != "" || r.implicitTypes[c] != "" || strings.Contains(c, ".") {
				d.Deriving[i] = r.lookup(c, d.Pos, true)
			}
		}
	case *ast.AliasDecl:
		r.shadow(d.Name, d.Pos)
		d.Name = r.scope.types[d.Name]
		for _, p := range d.TParams {
			ts[p] = true
		}
		r.typ(d.Type, ts)
	case *ast.ClassDecl:
		r.shadow(d.Name, d.Pos)
		d.Name = r.scope.types[d.Name]
		ts[d.Param.Name] = true
		if d.Super != "" {
			d.Super = r.lookup(d.Super, d.Pos, true)
		}
		for i := range d.Methods {
			m := &d.Methods[i]
			r.shadow(m.Name, m.Pos)
			m.Name = r.scope.values[m.Name]
			scope := scopeCopy(ts)
			r.tparams(m.TParams, scope)
			for _, p := range m.Params {
				r.typ(p.Type, scope)
			}
			r.typ(m.Ret, scope)
		}
	case *ast.InstanceDecl:
		d.Class = r.lookup(d.Class, d.Pos, true)
		d.TypeCon = r.lookup(d.TypeCon, d.Pos, true)
		for _, p := range d.Params {
			ts[p] = true
		}
		for _, p := range d.Fixed {
			ts[p] = true
		}
		for i := range d.Context {
			d.Context[i].Class = r.lookup(d.Context[i].Class, d.Context[i].Pos, true)
		}
		for i := range d.Methods {
			m := &d.Methods[i]
			if n := r.methods[d.Class][m.Name]; n != "" {
				m.Name = n
			}
			vs := make(map[string]bool)
			for _, p := range m.Params {
				vs[p] = true
			}
			r.block(m.Body, ts, vs)
		}
	}
}
func (r *moduleResolver) function(f *ast.FuncDecl, ts, vs map[string]bool) {
	ts = scopeCopy(ts)
	vs = scopeCopy(vs)
	r.tparams(f.TParams, ts)
	for i := range f.Params {
		p := &f.Params[i]
		r.typ(p.Type, ts)
		vs[p.Name] = true
	}
	r.typ(f.Ret, ts)
	if f.Body != nil {
		r.block(f.Body, ts, vs)
	}
}
func (r *moduleResolver) pattern(p ast.Pattern, vs map[string]bool) {
	if p == nil {
		return
	}
	switch p := p.(type) {
	case *ast.PVar:
		vs[p.Name] = true
	case *ast.PAs:
		vs[p.Name] = true
		r.pattern(p.Pat, vs)
	case *ast.PCtor:
		p.Name = r.lookup(p.Name, p.Pos, false)
		for _, a := range p.Args {
			r.pattern(a, vs)
		}
	case *ast.PRecord:
		p.Ctor = r.lookup(p.Ctor, p.Pos, false)
		for _, a := range p.Pats {
			r.pattern(a, vs)
		}
	case *ast.PTuple:
		for _, a := range p.Elems {
			r.pattern(a, vs)
		}
	}
}

// Laws have import visibility but never enter the ordinary value namespace.
func (r *moduleResolver) law(name string, pos ast.Pos) string {
	if alias, member, qualified := strings.Cut(name, "."); qualified {
		target := r.imports[alias]
		if target == nil {
			r.fail(pos, "unknown import alias %s", alias)
		}
		if strings.HasPrefix(member, "_") {
			r.fail(pos, "%s is private to module %s", member, target.module.Path)
		}
		if resolved := target.laws[member]; resolved != "" {
			return resolved
		}
	} else if resolved := r.scope.laws[name]; resolved != "" {
		return resolved
	}
	r.fail(pos, "unknown law %s", name)
	return ""
}

func (r *moduleResolver) tactic(t ast.Tactic, ts, vs map[string]bool) {
	switch t := t.(type) {
	case *ast.TRefl, *ast.TSample:
	case *ast.TMatch:
		if !vs[t.Var.Name] {
			r.fail(t.Var.Pos, "match variable %s is not a proof variable", t.Var.DisplayName())
		}
		for _, arm := range t.Arms {
			local := scopeCopy(vs)
			r.pattern(arm.Pat, local)
			r.tactic(arm.Body, ts, local)
		}
	case *ast.TIf:
		t.Cond = r.expr(t.Cond, ts, vs)
		r.tactic(t.Then, ts, vs)
		r.tactic(t.Else, ts, vs)
	case *ast.TRewrite:
		t.Law.Display = t.Law.DisplayName()
		t.Law.Name = r.law(t.Law.Name, t.Law.Pos)
		for i, arg := range t.Args {
			t.Args[i] = r.expr(arg, ts, vs)
		}
		r.tactic(t.Body, ts, vs)
	}
}
func (r *moduleResolver) let(name string, p ast.Pattern, t ast.Type, value *ast.Expr, ts, vs map[string]bool) {
	r.typ(t, ts)
	valueScope := vs
	if _, ok := (*value).(*ast.Lambda); ok && name != "" {
		valueScope = scopeCopy(vs)
		valueScope[name] = true
	}
	*value = r.expr(*value, ts, valueScope)
	if name != "" {
		vs[name] = true
	}
	r.pattern(p, vs)
}
func (r *moduleResolver) block(b *ast.Block, ts, outer map[string]bool) {
	vs := scopeCopy(outer)
	for index, s := range b.Stmts {
		switch s := s.(type) {
		case *ast.LetStmt:
			r.let(s.Name, s.Pat, s.Type, &s.Value, ts, vs)
		case *ast.FuncDecl:
			for _, statement := range b.Stmts[index:] {
				function, ok := statement.(*ast.FuncDecl)
				if !ok {
					break
				}
				vs[function.Name] = true
			}
			r.function(s, ts, vs)
		case *ast.ExprStmt:
			s.X = r.expr(s.X, ts, vs)
		}
	}
	b.Result = r.expr(b.Result, ts, vs)
}
func (r *moduleResolver) expr(e ast.Expr, ts, vs map[string]bool) ast.Expr {
	if e == nil {
		return nil
	}
	expr := func(e ast.Expr) ast.Expr { return r.expr(e, ts, vs) }
	exprs := func(es []ast.Expr) {
		for i, e := range es {
			es[i] = expr(e)
		}
	}
	switch e := e.(type) {
	case *ast.Ident:
		if !vs[e.Name] {
			e.Display = e.DisplayName()
			e.Name = r.lookup(e.Name, e.Pos, false)
		}
	case *ast.Field:
		if id, ok := e.X.(*ast.Ident); ok && !vs[id.Name] && r.imports[id.Name] != nil {
			return &ast.Ident{ExprBase: e.ExprBase, Name: r.lookup(id.Name+"."+e.Name, id.Pos, false), Display: id.Name + "." + e.Name}
		}
		e.X = expr(e.X)
	case *ast.RecordLit:
		e.Ctor = r.lookup(e.Ctor, e.Pos, false)
		exprs(e.Values)
	case *ast.RecordUpdate:
		if field, ok := e.X.(*ast.Field); ok {
			if id, ok := field.X.(*ast.Ident); ok && !vs[id.Name] && r.imports[id.Name] != nil && capitalised(strings.TrimLeft(field.Name, "_")) {
				exprs(e.Values)
				base := e.ExprBase
				base.Pos = id.Pos
				return &ast.RecordLit{ExprBase: base, Ctor: r.lookup(id.Name+"."+field.Name, id.Pos, false), Names: e.Names, Values: e.Values}
			}
		}
		e.X = expr(e.X)
		exprs(e.Values)
	case *ast.Call:
		e.Fn = expr(e.Fn)
		exprs(e.Args)
	case *ast.Lambda:
		scope := scopeCopy(vs)
		for i := range e.Params {
			p := &e.Params[i]
			r.typ(p.Type, ts)
			if p.Name != "" {
				scope[p.Name] = true
			}
			r.pattern(p.Pat, scope)
		}
		e.Body = r.expr(e.Body, ts, scope)
	case *ast.BlockExpr:
		r.block(e.Block, ts, vs)
	case *ast.Match:
		e.Scrut = expr(e.Scrut)
		for i := range e.Arms {
			a := &e.Arms[i]
			scope := scopeCopy(vs)
			r.pattern(a.Pat, scope)
			a.Guard = r.expr(a.Guard, ts, scope)
			a.Body = r.expr(a.Body, ts, scope)
		}
	case *ast.If:
		e.Cond = expr(e.Cond)
		e.Then = expr(e.Then)
		e.Else = expr(e.Else)
	case *ast.Do:
		scope := scopeCopy(vs)
		for index, s := range e.Stmts {
			switch s := s.(type) {
			case *ast.DoLet:
				r.let(s.Name, s.Pat, s.Type, &s.Value, ts, scope)
			case *ast.DoBind:
				s.X = r.expr(s.X, ts, scope)
				if s.Name != "" {
					scope[s.Name] = true
				}
				r.pattern(s.Pat, scope)
			case *ast.FuncDecl:
				for _, statement := range e.Stmts[index:] {
					function, ok := statement.(*ast.FuncDecl)
					if !ok {
						break
					}
					scope[function.Name] = true
				}
				r.function(s, ts, scope)
			case *ast.DoExpr:
				s.X = r.expr(s.X, ts, scope)
			}
		}
		e.Result = r.expr(e.Result, ts, scope)
	case *ast.Binary:
		e.L = expr(e.L)
		e.R = expr(e.R)
	case *ast.Unary:
		e.X = expr(e.X)
	case *ast.ListLit:
		exprs(e.Elems)
	case *ast.TupleLit:
		exprs(e.Elems)
	case *ast.Interp:
		exprs(e.Exprs)
	}
	return e
}

// cloneSyntax copies only syntax data. Semantic annotations are deliberately
// excluded: after a previous check they contain cyclic graphs back into syntax.
func cloneSyntax(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(cloneSyntax(v.Elem()))
		return out
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(cloneSyntax(v.Elem()))
		return out
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(cloneSyntax(v.Index(i)))
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i := range v.NumField() {
			if v.Type().Field(i).Name == "ForeignBindings" {
				out.Field(i).Set(v.Field(i))
			} else if v.Type().Field(i).Name != "Ann" {
				out.Field(i).Set(cloneSyntax(v.Field(i)))
			}
		}
		return out
	default:
		return v
	}
}

func (r *moduleResolver) mainName() string {
	if len(r.modules) == 0 {
		return "main"
	}
	m := r.modules[len(r.modules)-1]
	if s := r.scopes[m.Path]; s != nil {
		return s.values["main"]
	}
	return "main"
}
func (r *moduleResolver) orderedWarnings() []*Error {
	sort.SliceStable(r.warnings, func(i, j int) bool {
		a, b := r.warnings[i], r.warnings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Pos.Line != b.Pos.Line {
			return a.Pos.Line < b.Pos.Line
		}
		return a.Pos.Col < b.Pos.Col
	})
	return r.warnings
}
