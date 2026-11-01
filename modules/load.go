// Package modules loads directory modules and their transitive imports.
package modules

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gofx/ast"
	"gofx/parser"
	"gofx/std"
)

// Error preserves the importing source and span for CLI and editor diagnostics.
type Error struct {
	File     string
	Pos, End ast.Pos
	Msg      string
}

func (e *Error) Error() string   { return fmt.Sprintf("%s:%s: %s", e.File, e.Pos, e.Msg) }
func (e *Error) GetPos() ast.Pos { return e.Pos }
func (e *Error) GetEnd() ast.Pos { return e.End }

// Load accepts explicit entry files or a directory (all its .gfx files).
// Imported directories are always loaded in lexical filename order.
func Load(inputs []string) ([]*ast.Module, error) {
	var files []*ast.File
	base := "."
	seen := make(map[string]bool)
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		names := []string{input}
		if info.IsDir() {
			names, err = sourceFiles(input)
			if err != nil {
				return nil, err
			}
		}
		for _, name := range names {
			absolute, err := filepath.Abs(name)
			if err != nil {
				return nil, err
			}
			if seen[absolute] {
				continue
			}
			if len(files) != 0 && filepath.Dir(absolute) != base {
				return nil, fmt.Errorf("entry files must belong to one directory")
			}
			seen[absolute] = true
			source, err := os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			file, err := parse(string(source), name)
			if err != nil {
				return nil, err
			}
			files = append(files, file)
			if len(files) == 1 {
				base = filepath.Dir(absolute)
			}
		}
	}
	return LoadFiles(files, base)
}

// LoadFiles resolves imports of already-parsed entry files. The caller may add
// test/eval entry declarations before checking the returned final module.
func LoadFiles(files []*ast.File, directory string) ([]*ast.Module, error) {
	base, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	name := "main"
	if len(files) != 0 {
		name = files[0].Package
	}
	entry := &ast.Module{Path: base, Name: name, Files: files}
	if err := samePackage(entry); err != nil {
		return nil, err
	}
	l := loader{base: base, seen: map[string]*ast.Module{base: entry}}
	if _, err := l.importModule("std/prelude", base); err != nil {
		return nil, err
	}
	if err := l.imports(entry, base); err != nil {
		return nil, err
	}
	l.order = append(l.order, entry)
	return l.order, nil
}

type loader struct {
	base  string
	seen  map[string]*ast.Module
	order []*ast.Module
}

func parse(source, name string) (*ast.File, error) {
	f, err := parser.Parse(source)
	if err != nil {
		if p, ok := err.(interface {
			GetPos() ast.Pos
			GetEnd() ast.Pos
		}); ok {
			return nil, &Error{File: name, Pos: p.GetPos(), End: p.GetEnd(), Msg: strings.TrimPrefix(err.Error(), p.GetPos().String()+": ")}
		}
		return nil, err
	}
	f.Name = name
	return f, nil
}
func sourceFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".gfx") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .gfx files in %s", dir)
	}
	return files, nil
}
func samePackage(m *ast.Module) error {
	for _, f := range m.Files {
		if f.Package != m.Name {
			return &Error{File: f.Name, Pos: ast.Pos{Line: 1, Col: 1}, Msg: fmt.Sprintf("package %s differs from %s", f.Package, m.Name)}
		}
	}
	return nil
}
func (l *loader) imports(m *ast.Module, dir string) error {
	for _, f := range m.Files {
		for i := range f.Imports {
			imp := &f.Imports[i]
			target, err := l.importModule(imp.Path, dir)
			if err != nil {
				return &Error{File: f.Name, Pos: imp.Pos, End: imp.End, Msg: err.Error()}
			}
			imp.Resolved = target.Path
		}
	}
	return nil
}
func (l *loader) importModule(importPath, from string) (*ast.Module, error) {
	if importPath == "" || path.IsAbs(importPath) || path.Clean(importPath) != importPath || strings.Contains(importPath, "\\") || importPath == ".." || strings.HasPrefix(importPath, "../") {
		return nil, fmt.Errorf("invalid import path %q", importPath)
	}
	root, moduleName, err := moduleRoot(from, l.base)
	if err != nil {
		return nil, err
	}
	relative := importPath
	if moduleName != "" {
		if importPath == moduleName {
			relative = "."
		} else {
			relative = strings.TrimPrefix(importPath, moduleName+"/")
		}
	}
	roots := append([]string{root}, filepath.SplitList(os.Getenv("GOFX_PATH"))...)
	for _, root := range roots {
		if root == "" {
			continue
		}
		dir, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		if m := l.seen[dir]; m != nil {
			return m, nil
		}
		names, err := sourceFiles(dir)
		if err != nil {
			return nil, err
		}
		m := &ast.Module{Path: dir}
		l.seen[dir] = m
		for _, name := range names {
			source, err := os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			f, err := parse(string(source), name)
			if err != nil {
				return nil, err
			}
			m.Files = append(m.Files, f)
		}
		m.Name = m.Files[0].Package
		if err := samePackage(m); err != nil {
			return nil, err
		}
		if err := l.imports(m, dir); err != nil {
			return nil, err
		}
		l.order = append(l.order, m)
		return m, nil
	}
	if strings.HasPrefix(importPath, "std/") {
		if m := l.seen[importPath]; m != nil {
			return m, nil
		}
		names, err := fs.Glob(std.FS, strings.TrimPrefix(importPath, "std/")+"/*.gfx")
		if err != nil {
			return nil, err
		}
		if len(names) > 0 {
			sort.Strings(names)
			m := &ast.Module{Path: importPath}
			l.seen[importPath] = m
			for _, name := range names {
				source, err := std.FS.ReadFile(name)
				if err != nil {
					return nil, err
				}
				f, err := parse(string(source), "std/"+name)
				if err != nil {
					return nil, err
				}
				m.Files = append(m.Files, f)
			}
			m.Name = m.Files[0].Package
			if err := samePackage(m); err != nil {
				return nil, err
			}
			if err := l.imports(m, l.base); err != nil {
				return nil, err
			}
			l.order = append(l.order, m)
			return m, nil
		}
	}
	return nil, fmt.Errorf("module %q not found", importPath)
}
func moduleRoot(dir, fallback string) (string, string, error) {
	for {
		contents, err := os.ReadFile(filepath.Join(dir, "gofx.mod"))
		if err == nil {
			fields := strings.Fields(string(contents))
			if len(fields) != 2 || fields[0] != "module" || strings.Contains(strings.TrimSpace(string(contents)), "\n") {
				return "", "", fmt.Errorf("invalid gofx.mod in %s: expected module <name>", dir)
			}
			return dir, fields[1], nil
		}
		if !os.IsNotExist(err) {
			return "", "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fallback, "", nil
		}
		dir = parent
	}
}
