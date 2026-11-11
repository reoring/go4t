package types

import (
	"sort"
	"strings"

	"github.com/reoring/go4t/ast"
)

// Keep surface declarations intact, including in the unnamed-unit path. Only
// FuncInfo owns the lowered declaration; its body shares the checked value.
func constantFunction(d *ast.ConstDecl) *ast.FuncDecl {
	ret := d.Type
	if ret == nil {
		value := d.Value
		if neg, ok := value.(*ast.Unary); ok && neg.Op == "-" {
			switch neg.X.(type) {
			case *ast.IntLit, *ast.FloatLit:
				value = neg.X
			}
		}
		name := ""
		switch value.(type) {
		case *ast.IntLit:
			name = "Int"
		case *ast.FloatLit:
			name = "Float"
		case *ast.StringLit:
			name = "String"
		case *ast.BoolLit:
			name = "Bool"
		}
		if name == "" {
			fail(d.Pos, "top-level let needs a type unless its value is a literal: let %s: T = …", diagnosticName(d.Name))
		}
		ret = &ast.TypeName{Pos: value.GetPos(), End: value.GetEnd(), Name: name}
	}
	return &ast.FuncDecl{Pos: d.Pos, End: d.End, Name: d.Name, Ret: ret,
		Body: &ast.Block{Pos: d.Value.GetPos(), End: d.Value.GetEnd(), Result: d.Value}}
}

// Constant dependencies include references through functions. Ordinary recursive
// functions remain legal; only a dependency path returning to a constant fails.
func (c *checker) checkConstantCycles() {
	var constants []string
	for name, f := range c.program.Funcs {
		if f.Constant != nil {
			constants = append(constants, name)
		}
	}
	if len(constants) == 0 {
		return
	}
	sort.Strings(constants)
	refs := make(map[string][]string)
	for name, f := range c.program.Funcs {
		if f.Decl.Body == nil {
			continue
		}
		ast.Walk(f.Decl.Body, func(n ast.Node) {
			if id, ok := n.(*ast.Ident); ok && AnnOf(id).Ref == RefFunc {
				refs[name] = append(refs[name], id.Name)
			}
		})
	}
	for _, root := range constants {
		visited := make(map[string]bool)
		var path []string
		var visit func(string)
		visit = func(name string) {
			if name == root && len(path) > 0 {
				var cycle []string
				for _, member := range path {
					if c.program.Funcs[member].Constant != nil {
						display := c.program.ModuleNames[member]
						if display == "" {
							display = member
						}
						cycle = append(cycle, display)
					}
				}
				sort.Strings(cycle)
				d := c.program.Funcs[root].Constant
				c.decl, c.file = d, c.sources[d]
				fail(d.Pos, "constant initialization cycle: %s", strings.Join(cycle, ", "))
			}
			if visited[name] {
				return
			}
			visited[name] = true
			path = append(path, name)
			for _, target := range refs[name] {
				visit(target)
			}
			path = path[:len(path)-1]
		}
		visit(root)
	}
}
