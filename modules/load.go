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

	"go4t/ast"
	"go4t/parser"
	"go4t/std"
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

// GoBinder supplies parsed bindings for a Go import path without the go: prefix.
// Implementations own binding generation and any build-time shim output.
type GoBinder interface {
	Bind(importPath string) (*ast.Module, error)
}

// Options configures import resolution. Without GoBinder, go: imports are errors.
type Options struct {
	GoBinder GoBinder
}

// Load accepts explicit entry files or a directory (all its .gt files).
// Imported directories are always loaded in lexical filename order.
func Load(inputs []string, options ...Options) ([]*ast.Module, error) {
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
	return LoadFiles(files, base, options...)
}

// LoadFiles resolves imports of already-parsed entry files. The caller may add
// test/eval entry declarations before checking the returned final module.
func LoadFiles(files []*ast.File, directory string, options ...Options) ([]*ast.Module, error) {
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
	root, _, err := FindRoot(base)
	if err != nil {
		return nil, err
	}
	if root == "" {
		root = base
	}
	l := loader{base: base, projectRoot: root, seen: map[string]*ast.Module{base: entry}, dependencies: make(map[string]string)}
	for _, option := range options {
		l.goBinder = option.GoBinder
	}
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
	base         string
	projectRoot  string
	goBinder     GoBinder
	seen         map[string]*ast.Module
	dependencies map[string]string
	order        []*ast.Module
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
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".gt") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .gt files in %s", dir)
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
			if imp.Alias == "" && strings.HasPrefix(imp.Path, "go:") {
				imp.Alias = target.Name
			}
		}
	}
	return nil
}
func (l *loader) importModule(importPath, from string) (*ast.Module, error) {
	bindingPath := strings.TrimPrefix(importPath, "go:")
	if bindingPath == "" || path.IsAbs(bindingPath) || path.Clean(bindingPath) != bindingPath || strings.Contains(bindingPath, "\\") || bindingPath == ".." || strings.HasPrefix(bindingPath, "../") {
		return nil, fmt.Errorf("invalid import path %q", importPath)
	}
	if strings.HasPrefix(importPath, "go:") {
		if m := l.seen[importPath]; m != nil {
			return m, nil
		}
		if l.goBinder == nil {
			return nil, fmt.Errorf("go import %q requires the go4t CLI", importPath)
		}
		m, err := l.goBinder.Bind(bindingPath)
		if err != nil {
			return nil, err
		}
		m.Path, m.ImportPath = importPath, importPath
		m.Name = m.Files[0].Package
		if err := samePackage(m); err != nil {
			return nil, err
		}
		l.seen[importPath] = m
		if err := l.imports(m, l.base); err != nil {
			return nil, err
		}
		l.order = append(l.order, m)
		return m, nil
	}
	dir, err := l.resolveDirectory(importPath, from)
	if err != nil {
		return nil, err
	}
	if dir != "" {
		if m := l.seen[dir]; m != nil {
			if strings.HasPrefix(importPath, "std/") {
				m.ImportPath = importPath
			}
			return m, nil
		}
		names, err := sourceFiles(dir)
		if err != nil {
			return nil, err
		}
		m := &ast.Module{Path: dir, ImportPath: importPath}
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
		names, err := fs.Glob(std.FS, strings.TrimPrefix(importPath, "std/")+"/*.gt")
		if err != nil {
			return nil, err
		}
		if len(names) > 0 {
			sort.Strings(names)
			m := &ast.Module{Path: importPath, ImportPath: importPath}
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
func (l *loader) resolveDirectory(importPath, from string) (string, error) {
	root, manifest, err := FindRoot(from)
	if err != nil {
		return "", err
	}
	if root == "" {
		root = l.base
	}
	relative := importPath
	if manifest != nil {
		relative = moduleRelative(importPath, manifest.Module)
	}
	if dir, err := existingDirectory(root, relative); dir != "" || err != nil {
		return dir, err
	}
	if manifest != nil {
		// Nested module paths take precedence over their parent requirement.
		var selected *Requirement
		for i := range manifest.Requires {
			req := &manifest.Requires[i]
			if (importPath == req.Path || strings.HasPrefix(importPath, req.Path+"/")) && (selected == nil || len(req.Path) > len(selected.Path)) {
				selected = req
			}
		}
		if selected != nil {
			dir := l.dependencies[selected.Path]
			if dir == "" {
				module, err := ResolveGoModule(l.projectRoot, selected.Path)
				if err != nil {
					return "", fmt.Errorf("resolve dependency %s: %w", selected.Path, err)
				}
				dir = module.Dir
				l.dependencies[selected.Path] = dir
			}
			if resolved, err := existingDirectory(dir, moduleRelative(importPath, selected.Path)); resolved != "" || err != nil {
				return resolved, err
			}
		}
	}
	for _, root := range filepath.SplitList(os.Getenv("GO4T_PATH")) {
		if root == "" {
			continue
		}
		if dir, err := existingDirectory(root, relative); dir != "" || err != nil {
			return dir, err
		}
	}
	return "", nil
}

func moduleRelative(importPath, modulePath string) string {
	if importPath == modulePath {
		return "."
	}
	return strings.TrimPrefix(importPath, modulePath+"/")
}

func existingDirectory(root, relative string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", nil
	}
	return dir, nil
}
