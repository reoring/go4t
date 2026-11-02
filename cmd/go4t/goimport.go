package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"go4t/ast"
	"go4t/extern"
	"go4t/lexer"
	"go4t/parser"
	"go4t/types"
)

// goImportBinder keeps generated bindings in memory. Only build/run supply an
// output directory; their Go companions live beneath that build output.
type goImportBinder struct {
	loader    extern.Loader
	outputDir string
}

func newGoImportBinder(directory string) *goImportBinder {
	if root, err := moduleRoot(directory); err == nil {
		directory = root
	}
	return &goImportBinder{loader: extern.Loader{Dir: directory}}
}

func (b *goImportBinder) Bind(importPath string) (*ast.Module, error) {
	pkg, err := b.loader.Load(importPath)
	if err != nil {
		return nil, err
	}
	name := goIdentifier(path.Base(importPath))
	lexer.VisitFixedTokens(func(kind lexer.Kind, text string) {
		if kind == lexer.Keyword && text == name {
			name += "_"
		}
	})
	options := extern.BindOptions{Package: name, Float: true, Prelude: make(map[string]bool)}
	for _, exported := range types.PreludeExports {
		options.Prelude[exported] = true
	}
	shimPath, shimDir := "go4tbind/"+importPath, ""
	if b.outputDir != "" {
		relative, err := filepath.Rel(b.loader.Dir, b.outputDir)
		if err != nil || !filepath.IsLocal(relative) {
			return nil, fmt.Errorf("go import %q requires build output inside a Go module", "go:"+importPath)
		}
		shimDir = filepath.Join(b.outputDir, "go4tbind", filepath.FromSlash(importPath))
		shimPath, _, err = prepareShimDir(shimDir)
		if err != nil {
			return nil, err
		}
	}
	// Use the same declarations for checking and building; a placeholder shim
	// path is sufficient when the generated Go source will not be executed.
	source, shim, err := extern.BindWithShim(pkg, options, shimPath, name)
	if err != nil {
		return nil, err
	}
	file, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse Go bindings for %q: %w", importPath, err)
	}
	file.Name = "go:" + importPath + "/bindings.gt"
	if shimDir != "" {
		shimFile := filepath.Join(shimDir, "go4t_shim_"+goIdentifier(path.Base(importPath))+".go")
		if err := os.WriteFile(shimFile, shim, 0o644); err != nil {
			return nil, err
		}
	}
	return &ast.Module{Name: file.Package, Files: []*ast.File{file}}, nil
}
