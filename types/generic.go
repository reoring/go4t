package types

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"

	"go4t/ast"
)

const foreignTemplatePrefix = "go4t:generic/"

func foreignTemplate(f *FuncInfo) bool {
	return f != nil && f.Decl.Extern != nil && strings.HasPrefix(f.Decl.Extern.ImportPath, foreignTemplatePrefix)
}

// Remember ambiguity before ordinary metas default to Unit. Foreign inference
// may never use that default to manufacture a Go type argument.
func (b *bodyChecker) rememberForeignAmbiguity() {
	if !b.c.hasForeignTemplates { return }
	for _, e := range b.exprs {
		id, ok := e.(*ast.Ident)
		if !ok || AnnOf(id).Ref != RefFunc { continue }
		for _, t := range AnnOf(id).Inst {
			if unresolved(t) {
				if b.c.foreignAmbiguous == nil { b.c.foreignAmbiguous = make(map[*ast.Ident]bool) }
				b.c.foreignAmbiguous[id] = true
				break
			}
		}
	}
}

// instantiateForeign performs required (not optimization-budgeted) expansion.
// Only helpers transitively containing a foreign template require copies; other
// polymorphic functions retain their ordinary erased representation.
func (c *checker) instantiateForeign() {
	if !c.hasForeignTemplates { return }
	p := c.program
	required := make(map[string]bool)
	ports := make(map[string]ast.ForeignBindings)
	files := make(map[*ast.FuncDecl]*ast.File)
	for _, m := range p.Modules {
		if m.ForeignBindings != nil { ports[strings.TrimPrefix(m.ImportPath, "go:")] = m.ForeignBindings }
	}
	for _, file := range p.Files {
		for _, d := range file.Decls { if f, ok := d.(*ast.FuncDecl); ok { files[f] = file } }
	}
	for name, f := range p.Funcs { if foreignTemplate(f) { required[name] = true } }
	if len(required) == 0 { return }
	for changed := true; changed; {
		changed = false
		for name, f := range p.Funcs {
			if required[name] || len(f.Scheme.TParams) == 0 || f.Decl.Body == nil { continue }
			ast.Walk(f.Decl.Body, func(n ast.Node) {
				if id, ok := n.(*ast.Ident); ok && AnnOf(id).Ref == RefFunc && required[id.Name] {
					required[name] = true; changed = true
				}
			})
		}
	}
	copies := make(map[string]*FuncInfo)
	active := make(map[string]string)
	var visit func(ast.Node)
	var instantiate func(*ast.Ident) *FuncInfo
	instantiate = func(id *ast.Ident) *FuncInfo {
		f := p.Funcs[id.Name]
		a := AnnOf(id)
		if c.foreignAmbiguous[id] { fail(id.Pos, "generic Go call %s: cannot determine type arguments before defaulting", id.DisplayName()) }
		if !foreignConcrete(a.Type) { fail(id.Pos, "generic Go call %s: cannot determine concrete type arguments", id.DisplayName()) }
		key := f.Name + ":" + foreignTypeKey(a.Type)
		if copy := copies[key]; copy != nil { return copy }
		if previous := active[f.Name]; previous != "" && previous != key {
			fail(id.Pos, "generic Go call %s: unbounded polymorphic instantiation", id.DisplayName())
		}
		savedFile, savedDecl := c.file, c.decl
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(*Error); ok && e.File == "" { e.File = c.file }
				c.file, c.decl = savedFile, savedDecl
				panic(r)
			}
			c.file, c.decl = savedFile, savedDecl
		}()
		name := fmt.Sprintf("ext_go_%x", sha256.Sum256([]byte(key)))
		decl := cloneSyntax(reflect.ValueOf(f.Decl)).Interface().(*ast.FuncDecl)
		decl.Name, decl.TParams = name, nil
		sig := a.Type.(*TFunc)
		copy := &FuncInfo{Name: name, Decl: decl, Scheme: &Scheme{Type: sig}}
		if foreignTemplate(f) {
			owner := strings.TrimPrefix(f.Decl.Extern.ImportPath, foreignTemplatePrefix)
			port := ports[owner]
			if port == nil { fail(id.Pos, "generic Go call %s requires a go: import", id.DisplayName()) }
			typ := foreignSyntax(p, a.Type, id.Pos).(*ast.FuncType)
			ref, err := port.Instantiate(f.Decl.Extern.Name, typ)
			if err != nil { fail(id.Pos, "generic Go call %s: %s", id.DisplayName(), err) }
			decl.Extern = ref
		} else {
			active[f.Name] = key
			defer delete(active, f.Name)
			copies[key] = copy // ordinary monomorphic recursion reuses this copy
			c.file, c.decl = files[f.Decl].Name, decl
			scope := make(map[string]Type, len(f.Scheme.TParams))
			for i, param := range f.Scheme.TParams { scope[param.Name] = a.Inst[i] }
			b := c.body(nil, scope)
			b.check(decl.Params, sig, decl.Body)
			visit(decl.Body)
		}
		copies[key] = copy
		p.Funcs[name] = copy
		p.Owners[name] = p.Owners[f.Name]
		// ModuleNames also controls emitted Go names. Keep each required copy's
		// unique identity instead of collapsing all tuples to the source name.
		file := files[f.Decl]
		file.Decls = append(file.Decls, decl)
		files[decl] = file
		return copy
	}
	visit = func(root ast.Node) {
		ast.Walk(root, func(n ast.Node) {
			id, ok := n.(*ast.Ident)
			if !ok || AnnOf(id).Ref != RefFunc || !required[id.Name] { return }
			copy := instantiate(id)
			id.Name = copy.Name
			AnnOf(id).Inst, AnnOf(id).Dicts = nil, nil
		})
	}
	// Stable source order also makes the first error deterministic.
	for _, file := range p.Files {
		c.file = file.Name
		original := append([]ast.Decl(nil), file.Decls...)
		for _, d := range original {
			if f, ok := d.(*ast.FuncDecl); ok && required[f.Name] { continue }
			c.decl = d
			visit(d)
		}
	}
	for _, file := range p.Files {
		kept := file.Decls[:0]
		for _, d := range file.Decls {
			if f, ok := d.(*ast.FuncDecl); ok && required[f.Name] { delete(p.Funcs, f.Name); continue }
			kept = append(kept, d)
		}
		file.Decls = kept
	}
}

func foreignConcrete(t Type) bool {
	switch t := prune(t).(type) {
	case *TVar, *TMeta: return false
	case *TApp:
		if !foreignConcrete(t.Fun) { return false }
		for _, a := range t.Args { if !foreignConcrete(a) { return false } }
	case *TFunc:
		for _, a := range t.Params { if !foreignConcrete(a) { return false } }
		return foreignConcrete(t.Ret)
	}
	return true
}

// foreignSyntax uses canonical owners rather than display aliases. This keeps
// a known foreign record/handle distinct from lookalike local or foreign types.
func foreignSyntax(p *Program, t Type, pos ast.Pos) ast.Type {
	switch t := prune(t).(type) {
	case *TCon:
		name := t.Name
		if key := p.ForeignRecordKeys[t.Name]; key != "" {
			name = "go:" + key
		} else {
			for _, m := range p.Modules {
				if m.Path != p.Owners[t.Name] { continue }
				source := p.ModuleNames[t.Name]
				if source == "" { source = t.Name }
				if strings.HasPrefix(m.ImportPath, "go:") {
					name = m.ImportPath + "." + source
				} else if strings.HasPrefix(m.ImportPath, "std/") {
					name = source
				} else if p.DataTypes[t.Name] != nil {
					name = "local:" + t.Name
				}
			}
		}
		return &ast.TypeName{Pos: pos, Name: name}
	case *TApp:
		args := make([]ast.Type, len(t.Args))
		for i, a := range t.Args { args[i] = foreignSyntax(p, a, pos) }
		if con, ok := t.Fun.(*TCon); ok { if _, tuple := IsTuple(con); tuple { return &ast.TupleType{Pos: pos, Elems: args} } }
		fun := foreignSyntax(p, t.Fun, pos).(*ast.TypeName)
		fun.Args = args
		return fun
	case *TFunc:
		args := make([]ast.Type, len(t.Params))
		for i, a := range t.Params { args[i] = foreignSyntax(p, a, pos) }
		return &ast.FuncType{Pos: pos, Params: args, Ret: foreignSyntax(p, t.Ret, pos)}
	default:
		fail(pos, "cannot determine concrete foreign type argument %s", t)
		return nil
	}
}

// Diagnostic type printers deliberately abbreviate names and deep types. Keys
// instead frame every canonical constructor name and recurse without elision.
func foreignTypeKey(t Type) string {
	switch t := prune(t).(type) {
	case *TCon:
		return fmt.Sprintf("%d:%s", len(t.Name), t.Name)
	case *TApp:
		var out strings.Builder
		out.WriteString("app(" + foreignTypeKey(t.Fun))
		for _, a := range t.Args { out.WriteString("," + foreignTypeKey(a)) }
		return out.String() + ")"
	case *TFunc:
		var out strings.Builder
		out.WriteString("func(")
		for _, a := range t.Params { out.WriteString(foreignTypeKey(a) + ",") }
		return out.String() + ")" + foreignTypeKey(t.Ret)
	default:
		panic("foreignTypeKey requires concrete types")
	}
}
