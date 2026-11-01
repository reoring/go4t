package codegen

import (
	"fmt"
	"strings"

	"go4t/ast"
	"go4t/types"
)

const specializationNodes = 40
const specializationCopies = 8

type specialization struct {
	function  *types.FuncInfo
	name, key string
	types     map[string]types.Type
	dicts     map[types.Constraint]types.DictRef
}

func (g *generator) resolvedDict(d types.DictRef) types.DictRef {
	if d.Kind == types.DictParam && g.special != nil {
		if concrete, ok := g.special.dicts[types.Constraint{Arg: d.Param, Class: d.ParamClass}]; ok {
			return concrete
		}
	}
	if d.Super != nil {
		super := g.resolvedDict(*d.Super)
		d.Super = &super
	}
	if len(d.Args) > 0 {
		args := make([]types.DictRef, len(d.Args))
		for i, a := range d.Args {
			args[i] = g.resolvedDict(a)
		}
		d.Args = args
	}
	return d
}
func concreteDict(d types.DictRef) bool {
	switch d.Kind {
	case types.DictInstance:
		if d.Instance == nil {
			return false
		}
		for _, arg := range d.Args {
			if !concreteDict(arg) {
				return false
			}
		}
		return true
	case types.DictSuper:
		return d.Super != nil && concreteDict(*d.Super)
	}
	return false
}

func (g *generator) specializationKey(id *ast.Ident) (string, []types.DictRef) {
	a := g.ann(id)
	if a.Ref != types.RefFunc || len(a.Dicts) == 0 {
		return "", nil
	}
	dicts := make([]types.DictRef, len(a.Dicts))
	parts := make([]string, len(a.Dicts))
	for i, d := range a.Dicts {
		dicts[i] = g.resolvedDict(d)
		if !concreteDict(dicts[i]) {
			return "", nil
		}
		parts[i] = g.dict(&dicts[i])
	}
	return strings.Join(parts, ";"), dicts
}
func (g *generator) specialized(id *ast.Ident) *specialization {
	key, _ := g.specializationKey(id)
	if key == "" {
		return nil
	}
	for _, s := range g.specials[id.Name] {
		if s.key == key {
			return s
		}
	}
	return nil
}

func (g *generator) planSpecializations(prog *types.Program) {
	eligible := make(map[string]bool)
	for name, f := range prog.Funcs {
		if f.Decl.Body == nil || len(f.Scheme.Constraints) == 0 || len(f.Scheme.TParams) == 0 {
			continue
		}
		count := 0
		ast.Walk(f.Decl, func(ast.Node) { count++ })
		eligible[name] = count <= specializationNodes
	}
	visit := func(n ast.Node) {
		call, ok := n.(*ast.Call)
		if !ok {
			return
		}
		id, ok := call.Fn.(*ast.Ident)
		if !ok || !eligible[id.Name] {
			return
		}
		f := prog.Funcs[id.Name]
		a := g.ann(id)
		if len(a.Inst) != len(f.Scheme.TParams) {
			return
		}
		key, dicts := g.specializationKey(id)
		if key == "" || g.specialized(id) != nil || len(g.specials[id.Name]) >= specializationCopies {
			return
		}
		s := &specialization{function: f, key: key, name: fmt.Sprintf("%s_special%d", mangle(id.Name), len(g.specials[id.Name])+1), types: make(map[string]types.Type), dicts: make(map[types.Constraint]types.DictRef)}
		for i, c := range f.Scheme.Constraints {
			v, ok := c.Arg.(*types.TVar)
			if !ok {
				return
			}
			s.dicts[c] = dicts[i]
			for j, p := range f.Scheme.TParams {
				if p == v {
					s.types[p.Name] = g.substitute(a.Inst[j])
				}
			}
		}
		g.specials[id.Name] = append(g.specials[id.Name], s)
		g.specialOrder = append(g.specialOrder, s)
	}
	for _, name := range sortedKeys(prog.Funcs) {
		ast.Walk(prog.Funcs[name].Decl, visit)
	}
	for _, instance := range prog.Instances {
		ast.Walk(instance.Decl, visit)
	}
	// Discover concrete calls exposed by earlier copies, in deterministic order.
	for i := 0; i < len(g.specialOrder); i++ {
		s := g.specialOrder[i]
		g.special, g.typeArgs = s, s.types
		ast.Walk(s.function.Decl, visit)
	}
	g.special, g.typeArgs = nil, nil
}
