package codegen

import (
	"fmt"
	goast "go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"path"
	"strconv"
	"strings"

	"go4t/ast"
	"go4t/types"
)

// Generate emits one Go file per source module in a single Go package.
// A single Go package permits global instance dictionaries and mutually recursive
// module references without imposing Go's visibility or import-cycle semantics.
func Generate(prog *types.Program, goPackage string) (map[string][]byte, error) {
	return GenerateInDir(prog, goPackage, "")
}

// GenerateInDir resolves Go extern packages in dir's module context.
// An empty dir uses the current working directory, as Generate does.
func GenerateInDir(prog *types.Program, goPackage, dir string) (map[string][]byte, error) {
	source, err := generateInDir(prog, goPackage, dir)
	if err != nil {
		return nil, err
	}
	if goPackage == "" {
		goPackage = "main"
	}
	if len(prog.Modules) == 0 {
		out := map[string][]byte{"main.go": source}
		if err := addGoExports(prog, goPackage, nil, nil, out); err != nil {
			return nil, err
		}
		return out, nil
	}
	prefixes := make(map[string]string)
	usedPrefixes := make(map[string]bool)
	files := make(map[*ast.File]string)
	declarationOwners := make(map[ast.Decl]string)
	for _, m := range prog.Modules {
		prefix := goIdentifier(path.Base(strings.ReplaceAll(m.Path, "\\", "/")))
		if m.Path == "" {
			prefix = m.Name
			if prefix == "" {
				prefix = "main"
			}
		}
		base := prefix
		for index := 2; usedPrefixes[prefix]; index++ {
			prefix = fmt.Sprintf("%s_%d", base, index)
		}
		usedPrefixes[prefix] = true
		prefixes[m.Path] = prefix
		for _, f := range m.Files {
			files[f] = m.Path
			for _, d := range f.Decls {
				declarationOwners[d] = m.Path
			}
		}
	}
	root := prog.Modules[len(prog.Modules)-1].Path
	renamed := make(map[string]string)
	owners := make(map[string]string)
	sourceName := func(name string) string {
		if n := prog.ModuleNames[name]; n != "" {
			return n
		}
		return name
	}
	register := func(old, new, owner string) { renamed[old] = new; owners[new] = owner }
	for _, f := range prog.Funcs {
		owner := declarationOwners[f.Decl]
		register(mangle(f.Name), prefixes[owner]+"_"+sourceName(f.Name)+"_", owner)
	}
	for _, m := range prog.Modules {
		for _, f := range m.Files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.TypeDecl:
					owner := m.Path
					register(d.Name, prefixes[owner]+"_"+sourceName(d.Name), owner)
					for _, c := range d.Ctors {
						old := d.Name + "_" + c.Name
						register(old, prefixes[owner]+"_"+sourceName(d.Name)+"_"+sourceName(c.Name), owner)
					}
				case *ast.ClassDecl:
					register(d.Name, prefixes[m.Path]+"_"+sourceName(d.Name), m.Path)
				}
			}
		}
	}
	for _, inst := range prog.Instances {
		owner := declarationOwners[inst.Decl]
		// Derived instances are appended to the checked file after module loading.
		if owner == "" && root != "" {
			for _, f := range prog.Files {
				for _, d := range f.Decls {
					if d == inst.Decl {
						owner = files[f]
					}
				}
			}
		}
		register(instanceName(inst), "dict_"+prefixes[owner]+"_"+instanceName(inst), owner)
	}
	// Rename Go identifiers, not strings, comments, or //line source locations.
	// Foreign selectors are not declarations in the go4t namespace. In
	// particular a record Error and its Error method both spell Error_Error,
	// but only the local constructor may be module-mangled.
	foreignSet := token.NewFileSet()
	foreignFile, err := parser.ParseFile(foreignSet, "generated.go", source, 0)
	if err != nil {
		return nil, err
	}
	foreignAliases := make(map[string]bool)
	for _, spec := range foreignFile.Imports {
		name := path.Base(strings.Trim(spec.Path.Value, "\""))
		if spec.Name != nil {
			name = spec.Name.Name
		}
		foreignAliases[name] = true
	}
	var isForeign func(goast.Expr) bool
	isForeign = func(e goast.Expr) bool {
		switch e := e.(type) {
		case *goast.Ident:
			return foreignAliases[e.Name]
		case *goast.SelectorExpr:
			return isForeign(e.X)
		}
		return false
	}
	foreignOffsets := make(map[int]bool)
	goast.Inspect(foreignFile, func(n goast.Node) bool {
		if selector, ok := n.(*goast.SelectorExpr); ok && isForeign(selector.X) {
			foreignOffsets[foreignSet.PositionFor(selector.Sel.Pos(), false).Offset] = true
		}
		return true
	})
	set := token.NewFileSet()
	file := set.AddFile("generated.go", set.Base(), len(source))
	var scan scanner.Scanner
	scan.Init(file, source, nil, scanner.ScanComments)
	var text strings.Builder
	previous := 0
	for {
		pos, tok, literal := scan.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.IDENT || foreignOffsets[file.Offset(pos)] {
			continue
		}
		name := renamed[literal]
		// Copies retain their original function's module and public mangling.
		if name == "" {
			if split := strings.LastIndex(literal, "_special"); split >= 0 {
				base := renamed[literal[:split]]
				copy, err := strconv.Atoi(literal[split+len("_special"):])
				if base != "" && err == nil && copy >= 1 && copy <= specializationCopies {
					name = base + literal[split:]
					register(literal, name, owners[base])
				}
			}
		}
		if name == "" {
			continue
		}
		offset := file.Offset(pos)
		text.Write(source[previous:offset])
		text.WriteString(name)
		previous = offset + len(literal)
	}
	text.Write(source[previous:])
	source = []byte(text.String())
	set = token.NewFileSet()
	parsed, err := parser.ParseFile(set, "generated.go", source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	imports := make(map[string]string)
	for _, spec := range parsed.Imports {
		alias := path.Base(strings.Trim(spec.Path.Value, "\""))
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		imports[alias] = spec.Path.Value
	}
	bodies := make(map[string]*strings.Builder)
	usedImports := make(map[string]map[string]bool)
	for _, m := range prog.Modules {
		bodies[m.Path] = &strings.Builder{}
		usedImports[m.Path] = make(map[string]bool)
	}
	previous = set.PositionFor(parsed.Name.End(), false).Offset
	for _, d := range parsed.Decls {
		end := set.PositionFor(d.End(), false).Offset
		if general, ok := d.(*goast.GenDecl); ok && general.Tok == token.IMPORT {
			previous = end
			continue
		}
		owner := root
		switch d := d.(type) {
		case *goast.FuncDecl:
			if d.Recv == nil {
				if o, ok := owners[d.Name.Name]; ok {
					owner = o
				}
			} else {
				typ := d.Recv.List[0].Type
				if p, ok := typ.(*goast.StarExpr); ok {
					typ = p.X
				}
				if id, ok := typ.(*goast.Ident); ok {
					if o, ok := owners[id.Name]; ok {
						owner = o
					}
				}
			}
		case *goast.GenDecl:
			if len(d.Specs) > 0 {
				if spec, ok := d.Specs[0].(*goast.TypeSpec); ok {
					if o, ok := owners[spec.Name.Name]; ok {
						owner = o
					}
				}
			}
		}
		goast.Inspect(d, func(n goast.Node) bool {
			if selector, ok := n.(*goast.SelectorExpr); ok {
				if id, ok := selector.X.(*goast.Ident); ok && imports[id.Name] != "" {
					usedImports[owner][id.Name] = true
				}
			}
			return true
		})
		bodies[owner].Write(source[previous:end])
		bodies[owner].WriteByte('\n')
		previous = end
	}
	out := make(map[string][]byte, len(bodies))
	for _, module := range prog.Modules {
		owner := module.Path
		body := bodies[owner]
		var header strings.Builder
		fmt.Fprintf(&header, "// Code generated by go4t. DO NOT EDIT.\npackage %s\n", goPackage)
		for _, alias := range sortedKeys(usedImports[owner]) {
			fmt.Fprintf(&header, "import %s %s\n", alias, imports[alias])
		}
		header.WriteString(body.String())
		formatted, err := format.Source([]byte(header.String()))
		if err != nil {
			return nil, fmt.Errorf("format module %s: %w", owner, err)
		}
		name := prefixes[owner] + ".go"
		if owner == root {
			name = "main.go"
		} else if name == "main.go" {
			name = "main_module.go"
		}
		for out[name] != nil || goPackage != "main" && name == "go4t_export.go" {
			name = strings.TrimSuffix(name, ".go") + "_.go"
		}
		out[name] = formatted
	}
	if err := addGoExports(prog, goPackage, prefixes, renamed, out); err != nil {
		return nil, err
	}
	return out, nil
}
func goIdentifier(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name = b.String()
	if name == "" {
		return "module"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "module_" + name
	}
	return name
}
