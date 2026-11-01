package types

import (
	"path"

	"go4t/ast"
)

// diagnosticName distinguishes resolved declarations from arbitrary source text.
// In particular a local named like another module's canonical name stays local.
type diagnosticName string

type nameContext struct {
	program *Program
	module  *ast.Module
	file    *ast.File
}

func (p *Program) namesFor(file string) nameContext {
	context := nameContext{program: p}
	for _, module := range p.Modules {
		for _, candidate := range module.Files {
			if candidate.Name == file {
				context.module, context.file = module, candidate
				return context
			}
		}
	}
	return context
}

func (n nameContext) name(canonical, display string) string {
	if display == "" {
		display = canonical
	}
	if n.program == nil {
		return display
	}
	if source := n.program.ModuleNames[canonical]; source != "" {
		display = source
	}
	owner := n.program.Owners[canonical]
	if owner == "" || n.module != nil && n.module.Path == owner {
		return display
	}
	var declaring *ast.Module
	for _, module := range n.program.Modules {
		if module.Path == owner {
			declaring = module
			break
		}
	}
	if declaring == nil {
		return display
	}
	preludeOwner := preludeOwners[display]
	if preludeOwner == "" {
		preludeOwner = "std/prelude"
	}
	if contains(PreludeExports, display) && declaring.ImportPath == preludeOwner {
		return display
	}
	if n.file != nil {
		for _, imp := range n.file.Imports {
			target := imp.Resolved
			if target == "" {
				target = imp.Path
			}
			if target == owner {
				alias := imp.Alias
				if alias == "" {
					alias = path.Base(imp.Path)
				}
				return alias + "." + display
			}
		}
	}
	// A transitive type may not be directly imported in this file. Its declared
	// package still gives a source-level name, never the generated Go identity.
	qualifier := declaring.Name
	if qualifier == "" {
		qualifier = path.Base(declaring.Path)
	}
	return qualifier + "." + display
}

// TypeString prints a type using the diagnosing (or hovered) file's imports.
func (p *Program) TypeString(t Type, file string) string {
	return printType(t, false, p.namesFor(file))
}

// TypeStringFull uses the same file context without depth elision.
func (p *Program) TypeStringFull(t Type, file string) string {
	return printType(t, true, p.namesFor(file))
}
