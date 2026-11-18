package codegen

import (
	"strings"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// emissionProgram is a read-only projection of the checked program. Module
// order and naming metadata stay intact, so an omitted module cannot rename
// another module's exports. Files retain source locations for diagnostics.
func emissionProgram(p *types.Program, library bool) *types.Program {
	if p == nil || len(p.Modules) <= 1 {
		return p
	}
	out := *p
	out.Funcs = make(map[string]*types.FuncInfo)
	out.DataTypes = make(map[string]*types.DataType)
	out.Classes = make(map[string]*types.Class)
	out.Instances = nil
	r := reachability{p: p, out: &out, instances: make(map[*types.Instance]bool), byHead: make(map[string][]*types.Instance), aliases: make(map[string]*ast.AliasDecl), seenTypes: make(map[string]bool)}
	for _, i := range p.Instances {
		r.byHead[i.TypeCon.Name] = append(r.byHead[i.TypeCon.Name], i)
	}
	for _, f := range p.Files {
		for _, decl := range f.Decls {
			if d, ok := decl.(*ast.AliasDecl); ok {
				r.aliases[d.Name] = d
			}
		}
	}
	entry := p.Modules[len(p.Modules)-1]
	for _, file := range entry.Files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				r.function(d.Name)
			case *ast.ConstDecl:
				r.function(d.Name)
			case *ast.TypeDecl:
				r.named(d.Name)
			case *ast.AliasDecl:
				r.named(d.Name)
			case *ast.ClassDecl:
				r.class(d.Name)
			case *ast.InstanceDecl:
				for _, i := range r.byHead[d.TypeCon] {
					if i.Decl == d {
						r.instance(i)
					}
				}
			}
		}
	}
	if library {
		r.mirrorDictionaries(entry.Path)
	}
	// List/Option, Ordering and the tuple callbacks share one runtime shape
	// initializer. Map and Set conversions also call through these callbacks.
	for _, name := range []string{"List", "Option", "Ordering", "Tuple2", "Tuple3", "Map", "Set"} {
		if r.seenTypes[name] {
			r.named("List")
			r.named("Option")
			r.named("Ordering")
			break
		}
	}
	for _, i := range p.Instances { // preserve declaration order
		if r.instances[i] {
			out.Instances = append(out.Instances, i)
		}
	}
	return &out
}

type reachability struct {
	p, out    *types.Program
	instances map[*types.Instance]bool
	byHead    map[string][]*types.Instance
	aliases   map[string]*ast.AliasDecl
	seenTypes map[string]bool
}

func (r *reachability) function(name string) {
	f := r.p.Funcs[name]
	if f == nil || r.out.Funcs[name] != nil {
		return
	}
	r.out.Funcs[name] = f
	r.scheme(f.Scheme)
	ast.Walk(f.Decl, r.visit)
}

func (r *reachability) typ(t types.Type) {
	switch t := types.Unalias(t).(type) {
	case *types.TCon:
		r.named(t.Name)
	case *types.TFunc:
		for _, a := range t.Params {
			r.typ(a)
		}
		r.typ(t.Ret)
	case *types.TApp:
		r.typ(t.Fun)
		for _, a := range t.Args {
			r.typ(a)
		}
	}
}

func (r *reachability) named(name string) {
	if r.seenTypes[name] {
		return
	}
	r.seenTypes[name] = true
	if d := r.p.DataTypes[name]; d != nil {
		r.out.DataTypes[name] = d
		for _, c := range d.Ctors {
			for _, f := range c.Fields {
				r.typ(f)
			}
		}
	} else if alias := r.aliases[name]; alias != nil {
		ast.Walk(alias.Type, r.visit)
	}
}

// Exported String methods call Debug even when no source function does.
// Resolve precisely the same concrete dictionaries as the Go mirror emitter,
// including nested substituted fields and all instance-context arguments.
func (r *reachability) mirrorDictionaries(root string) {
	g := mirrorGenerator{prog: r.p, root: root, dataTypes: make(map[string]*types.DataType), sources: &generator{instances: make(map[string]*types.Instance)}}
	for _, i := range r.p.Instances {
		g.sources.instances[instanceName(i)] = i
	}
	seen := make(map[string]bool)
	var visit func(types.Type)
	visit = func(t types.Type) {
		key := mirrorKey(t)
		if seen[key] {
			return
		}
		seen[key] = true
		_, args := mirrorHead(t)
		for _, a := range args {
			visit(a)
		}
		if data := g.data(t); data != nil && !mirrorEnum(data) {
			if d, ok := g.concreteDictionary("Debug", t); ok {
				r.dict(d)
			}
			for _, c := range data.Ctors {
				for _, field := range c.Fields {
					visit(field)
				}
			}
		}
	}
	for _, f := range r.p.Funcs {
		if !g.entryFunction(f) || g.sourceName(f.Name) == "main" || strings.HasPrefix(g.sourceName(f.Name), "_") || g.functionReason(f) != "" {
			continue
		}
		for _, t := range f.Scheme.Type.Params {
			visit(t)
		}
		visit(f.Scheme.Type.Ret)
	}
}

func (r *reachability) class(name string) {
	c := r.p.Classes[name]
	if c == nil || r.out.Classes[name] != nil {
		return
	}
	r.out.Classes[name] = c
	r.class(c.Super)
	for _, m := range c.Methods {
		r.scheme(m.Scheme)
	}
}

func (r *reachability) scheme(s *types.Scheme) {
	r.typ(s.Type)
	for _, c := range s.Constraints {
		r.class(c.Class)
	}
}

func (r *reachability) instance(i *types.Instance) {
	if i == nil || r.instances[i] {
		return
	}
	r.instances[i] = true
	r.class(i.Class.Name)
	r.named(i.TypeCon.Name)
	for _, other := range r.byHead[i.TypeCon.Name] {
		if other.Class.Name == i.Class.Super {
			r.instance(other)
		}
	}
	for _, ctx := range i.Context {
		r.class(ctx.Class)
	}
	for _, d := range i.SuperArgs {
		r.dict(d)
	}
	ast.Walk(i.Decl, r.visit)
}

func (r *reachability) dict(d types.DictRef) {
	r.instance(d.Instance)
	r.class(d.Class)
	if d.Super != nil {
		r.dict(*d.Super)
	}
	for _, a := range d.Args {
		r.dict(a)
	}
}

func (r *reachability) visit(n ast.Node) {
	if t, ok := n.(*ast.TypeName); ok {
		r.named(t.Name)
	}
	x, ok := n.(ast.Expr)
	if !ok {
		return
	}
	a, ok := (*x.AnnSlot()).(*types.Ann)
	if !ok { // literal patterns have no expression annotation
		return
	}
	r.typ(a.Type)
	if a.Ctor != nil {
		r.named(a.Ctor.Data.Name)
	}
	if id, ok := x.(*ast.Ident); ok && a.Ref == types.RefFunc {
		r.function(id.Name)
	}
	for _, t := range a.Inst {
		r.typ(t)
	}
	for _, d := range a.Dicts {
		r.dict(d)
	}
	for _, d := range a.ShowDicts {
		r.dict(d)
	}
	if a.Monad != nil {
		r.dict(*a.Monad)
	}
	if a.LetScheme != nil {
		r.scheme(a.LetScheme)
	}
}
