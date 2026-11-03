// Package extern analyzes Go packages and shares their foreign-function contracts
// between the binding generator and the go4t code generator.
package extern

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Function describes an exported package-level Go function, including its exact
// parameter and result types. Methods are not package-level extern targets.
type Function struct {
	Name      string
	Signature *types.Signature
}

// Package contains exported functions and methods in lexical name order.
type Package struct {
	Path      string
	Name      string
	Functions []Function
	Methods   []Method
	goPackage *types.Package
	imports   []*types.Package
	jsonMarshaler, jsonUnmarshaler *types.Interface
}

// Lookup finds an exported function by its Go name.
func (p *Package) Lookup(name string) (Function, bool) {
	for _, f := range p.Functions {
		if f.Name == name {
			return f, true
		}
	}
	return Function{}, false
}

// Loader caches packages for one compilation. Its zero value is ready to use;
// Dir, if set, selects the module context in which go list resolves imports.
// It does not cache across compilations, so edits to a Go dependency are visible.
type Loader struct {
	Dir      string
	packages map[string]*Package
	exports  map[string]string
	importer types.Importer
}

// Load resolves a package using the active Go toolchain, module replacements,
// build constraints and export data. Only the standard library is required.
func (l *Loader) Load(path string) (*Package, error) {
	if p := l.packages[path]; p != nil {
		return p, nil
	}
	if path == "" || strings.HasPrefix(path, "-") || strings.Contains(path, "...") {
		return nil, fmt.Errorf("invalid Go import path %q", path)
	}
	// JSON-scalar mirrors must use the same importer as the package being bound.
	// Load its interfaces even when the package only implements their methods
	// structurally and does not import encoding/json itself.
	if l.importer == nil && path != "encoding/json" {
		if _, err := l.Load("encoding/json"); err != nil {
			return nil, err
		}
	}
	cmd := exec.Command("go", "list", "-json", "-export", "-deps", path)
	cmd.Dir = l.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("load Go package %q: %s", path, strings.TrimSpace(stderr.String()))
	}
	if l.exports == nil {
		l.exports = make(map[string]string)
		l.packages = make(map[string]*Package)
		l.importer = importer.ForCompiler(token.NewFileSet(), "gc", func(path string) (io.ReadCloser, error) {
			file := l.exports[path]
			if file == "" {
				return nil, fmt.Errorf("no export data for %q", path)
			}
			return os.Open(file)
		})
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var target string
	var targetImports []string
	for {
		var listed struct {
			ImportPath string
			Name       string
			Export     string
			DepOnly    bool
			Imports    []string
		}
		if err := decoder.Decode(&listed); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode go list for %q: %w", path, err)
		}
		l.exports[listed.ImportPath] = listed.Export
		if !listed.DepOnly {
			if target != "" {
				return nil, fmt.Errorf("expected one Go package for %q", path)
			}
			target = listed.ImportPath
			targetImports = listed.Imports
		}
	}
	if target == "" {
		return nil, fmt.Errorf("no Go package for %q", path)
	}
	pkg, err := l.importer.Import(target)
	if err != nil {
		return nil, fmt.Errorf("read Go package %q: %w", path, err)
	}
	p := &Package{Path: pkg.Path(), Name: pkg.Name(), goPackage: pkg}
	jsonPackage, err := l.importer.Import("encoding/json")
	if err != nil {
		return nil, fmt.Errorf("read JSON interfaces: %w", err)
	}
	p.jsonMarshaler = jsonPackage.Scope().Lookup("Marshaler").Type().Underlying().(*types.Interface)
	p.jsonUnmarshaler = jsonPackage.Scope().Lookup("Unmarshaler").Type().Underlying().(*types.Interface)
	// Export-data dependencies can have only the names referenced by this
	// package populated. Import direct dependencies explicitly so interface
	// discovery sees their complete exported scopes, not just those names.
	for _, dependency := range targetImports {
		if dependency == "C" {
			continue // cgo's pseudo-package has no exported Go interfaces
		}
		imported, err := l.importer.Import(dependency)
		if err != nil {
			return nil, fmt.Errorf("read Go dependency %q of %q: %w", dependency, path, err)
		}
		p.imports = append(p.imports, imported)
	}
	for _, name := range pkg.Scope().Names() {
		if f, ok := pkg.Scope().Lookup(name).(*types.Func); ok && f.Exported() {
			p.Functions = append(p.Functions, Function{Name: name, Signature: f.Type().(*types.Signature)})
		}
	}
	p.Methods = exportedMethods(pkg)
	l.packages[path] = p
	return p, nil
}

// SignatureString prints a stable Go signature without parameter names.
func SignatureString(sig *types.Signature) string {
	tuple := func(t *types.Tuple) *types.Tuple {
		vars := make([]*types.Var, t.Len())
		for i := range vars {
			vars[i] = types.NewVar(token.NoPos, nil, "", t.At(i).Type())
		}
		return types.NewTuple(vars...)
	}
	// Preserve type parameter names for diagnostics on unsupported generic funcs.
	if sig.TypeParams().Len() != 0 {
		return types.TypeString(sig, nil)
	}
	return types.TypeString(types.NewSignatureType(nil, nil, nil, tuple(sig.Params()), tuple(sig.Results()), sig.Variadic()), nil)
}
