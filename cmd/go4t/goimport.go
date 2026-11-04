package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go4t/ast"
	"go4t/extern"
	"go4t/lexer"
	"go4t/parser"
	"go4t/types"
)

// goImportBinder reuses content-addressed bindings across compilations.
// Build/run/test/eval supply an output directory for generated Go companions.
type goImportBinder struct {
	loader            extern.Loader
	outputDir         string
	shimPrefix        string
	cacheEntries      map[string]string
	bound             map[string]*ast.Module
	visited           map[string]bool
	sideEffectImports []string
}

func newGoImportBinder(directory string) *goImportBinder {
	if root, err := moduleRoot(directory); err == nil {
		directory = root
	}
	return &goImportBinder{loader: extern.Loader{Dir: directory}}
}

func (b *goImportBinder) Bind(importPath string) (*ast.Module, error) {
	if module := b.bound[importPath]; module != nil {
		return module, nil
	}
	var pkg *extern.Package
	var err error
	// Loading one package records export files for all its dependencies. A hit
	// for those dependencies needs neither another go list nor Go type loading.
	if b.loader.ExportFile(importPath) == "" {
		pkg, err = b.loader.Load(importPath)
		if err != nil {
			return nil, err
		}
	}
	options := goImportOptions(importPath)
	name := options.Package
	shimPath, err := b.shimPath(importPath)
	if err != nil {
		return nil, err
	}
	entry, cacheErr := b.cacheEntry(importPath, options, shimPath)
	var file *ast.File
	var shim []byte
	var metadata *extern.BindingMetadata
	if cacheErr != nil {
		traceBindingCache(importPath, "unavailable", cacheErr)
	} else if entry == "" {
		traceBindingCache(importPath, "disabled", nil)
	} else {
		file, shim, metadata, cacheErr = readBindingCache(entry)
		if cacheErr == nil {
			traceBindingCache(importPath, "hit", nil)
		} else {
			traceBindingCache(importPath, "miss", cacheErr)
		}
	}
	if file == nil {
		if pkg == nil {
			pkg, err = b.loader.Load(importPath)
			if err != nil {
				return nil, err
			}
		}
		b.cacheClassifier(entry, extern.HasErrorClassifier(pkg))
		// Checking uses the same declarations, with a placeholder shim path.
		var source string
		source, shim, metadata, err = extern.BindWithShim(pkg, options, shimPath, name)
		if err != nil {
			return nil, err
		}
		file, err = parser.Parse(source)
		if err != nil {
			return nil, fmt.Errorf("parse Go bindings for %q: %w", importPath, err)
		}
		if entry != "" {
			if err := writeBindingCache(entry, source, shim, metadata); err != nil {
				traceBindingCache(importPath, "write failed", err)
			}
		}
	}
	file.Name = "go:" + importPath + "/bindings.gt"
	if b.outputDir != "" {
		shimDir := filepath.Join(b.outputDir, "go4tbind", filepath.FromSlash(importPath))
		if err := os.MkdirAll(shimDir, 0o755); err != nil {
			return nil, err
		}
		shimFile := filepath.Join(shimDir, "go4t_shim_"+goIdentifier(path.Base(importPath))+".go")
		if err := os.WriteFile(shimFile, shim, 0o644); err != nil {
			return nil, err
		}
	}
	module := &ast.Module{Name: file.Package, Files: []*ast.File{file}, ForeignRecordKeys: metadata.RecordKeys, ForeignHandleKeys: metadata.HandleKeys}
	module.ForeignBindings = &genericGoBindings{binder: b, owner: importPath, shimPath: shimPath, baseEntry: entry}
	if b.bound == nil {
		b.bound = make(map[string]*ast.Module)
	}
	b.bound[importPath] = module
	if b.outputDir != "" {
		if err := b.bindClassifiers(importPath); err != nil {
			return nil, err
		}
	}
	return module, nil
}

func goImportOptions(importPath string) extern.BindOptions {
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
	return options
}

func (b *goImportBinder) shimPath(importPath string) (string, error) {
	if b.outputDir == "" {
		return "go4tbind/" + importPath, nil
	}
	if b.shimPrefix == "" {
		relative, err := filepath.Rel(b.loader.Dir, b.outputDir)
		if err != nil || !filepath.IsLocal(relative) {
			return "", fmt.Errorf("go import %q requires build output inside a Go module", "go:"+importPath)
		}
		b.shimPrefix, _, err = prepareShimDir(filepath.Join(b.outputDir, "go4tbind"))
		if err != nil {
			return "", err
		}
	}
	return b.shimPrefix + "/" + importPath, nil
}

func (b *goImportBinder) cacheEntry(importPath string, options extern.BindOptions, shimPath string) (string, error) {
	if entry, ok := b.cacheEntries[importPath]; ok {
		return entry, nil
	}
	entry, err := bindingCacheEntry(&b.loader, importPath, options, shimPath, options.Package)
	if err == nil {
		if b.cacheEntries == nil {
			b.cacheEntries = make(map[string]string)
		}
		b.cacheEntries[importPath] = entry
	}
	return entry, err
}

func (b *goImportBinder) cacheClassifier(entry string, present bool) {
	if entry != "" {
		if err := writeClassifierCache(entry, present); err != nil {
			traceBindingCache(entry, "classifier write failed", err)
		}
	}
}

// bindClassifiers visits each Go dependency once, independently of the go4t
// type-owner imports. A classifier-only dependency uses exactly the same Bind
// cache and shim generation as an explicit go: import, without adding its
// declarations to the user's module namespace.
func (b *goImportBinder) bindClassifiers(importPath string) error {
	if b.visited[importPath] {
		return nil
	}
	if b.visited == nil {
		b.visited = make(map[string]bool)
	}
	b.visited[importPath] = true
	// Internal and vendored packages cannot be imported by the generated shim.
	// Still traverse their imports; standard-library packages are not excluded.
	segments := "/" + importPath + "/"
	if importPath != "C" && !strings.Contains(segments, "/internal/") && !strings.Contains(segments, "/vendor/") {
		shimPath, err := b.shimPath(importPath)
		if err != nil {
			return err
		}
		entry, cacheErr := b.cacheEntry(importPath, goImportOptions(importPath), shimPath)
		present, err := readClassifierCache(entry)
		if cacheErr != nil || err != nil {
			pkg, err := b.loader.Load(importPath)
			if err != nil {
				return err
			}
			present = extern.HasErrorClassifier(pkg)
			b.cacheClassifier(entry, present)
			traceBindingCache(importPath, "classifier miss", cacheErr)
		} else {
			traceBindingCache(importPath, "classifier hit", nil)
		}
		if present {
			if _, err := b.Bind(importPath); err != nil {
				return err
			}
			b.sideEffectImports = append(b.sideEffectImports, shimPath)
			if os.Getenv("GO4T_TRACE") == "1" {
				fmt.Fprintf(os.Stderr, "go4t bind classifier %s\n", importPath)
			}
		}
	}
	for _, dependency := range b.loader.Dependencies(importPath) {
		if err := b.bindClassifiers(dependency); err != nil {
			return err
		}
	}
	return nil
}
