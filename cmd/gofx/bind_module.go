package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gofx/extern"
	"gofx/lexer"
	"gofx/modules"
	"gofx/types"
)

func runBindModule(modulePath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, _, err := modules.FindRoot(cwd)
	if err != nil {
		return err
	}
	if root == "" {
		return fmt.Errorf("gofx bind --module: no gofx.mod found above current directory; run gofx init <module-path>")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("gofx bind --module: no go.mod in project root %s: %w", root, err)
	}
	module, moduleErr := modules.ResolveGoModule(root, modulePath)
	if moduleErr == nil {
		return bindModule(root, module)
	}
	// A standard package is a single target, not a recursive pseudo-module.
	command := exec.Command("go", "list", "-json", modulePath)
	command.Dir = root
	data, err := command.Output()
	var pkg struct {
		ImportPath string
		Name       string
		Standard   bool
	}
	if err == nil {
		err = json.Unmarshal(data, &pkg)
	}
	if err != nil || !pkg.Standard || pkg.ImportPath != modulePath || !bindablePackage(pkg.ImportPath, pkg.Name) {
		return fmt.Errorf("gofx bind --module: %s is not a required Go module or standard package: %w", modulePath, moduleErr)
	}
	return bindPackages(root, []string{pkg.ImportPath})
}

// bindModule enumerates in the dependency directory, but loads export data in
// the project so replacements and selected dependency versions still apply.
func bindModule(root string, module modules.GoModule) error {
	command := exec.Command("go", "list", "-e", "-f", "{{.ImportPath}} {{.Name}}", "./...")
	command.Dir = module.Dir
	var diagnostics strings.Builder
	command.Stderr = &diagnostics
	data, err := command.Output()
	if err != nil {
		return fmt.Errorf("gofx bind: enumerate module %s: %w: %s", module.Path, err, strings.TrimSpace(diagnostics.String()))
	}
	var packages []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !bindablePackage(fields[0], fields[1]) {
			continue
		}
		if fields[0] == module.Path || strings.HasPrefix(fields[0], module.Path+"/") {
			packages = append(packages, fields[0])
		}
	}
	sort.Strings(packages)
	return bindPackages(root, packages)
}

func bindablePackage(importPath, name string) bool {
	if name == "main" || strings.HasSuffix(name, "_test") {
		return false
	}
	for _, part := range strings.Split(importPath, "/") {
		if part == "internal" || strings.HasSuffix(part, "_test") {
			return false
		}
	}
	return true
}

func bindPackages(root string, packages []string) error {
	loader := extern.Loader{Dir: root}
	options := extern.BindOptions{Prelude: make(map[string]bool), Float: true}
	for _, name := range types.PreludeExports {
		options.Prelude[name] = true
	}
	for _, importPath := range packages {
		pkg, err := loader.Load(importPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gofx bind: skipped package %s: %v\n", importPath, err)
			continue
		}
		dir := filepath.Join(root, "gofxbind", filepath.FromSlash(importPath))
		shimPath, shimPackage, err := prepareShimDir(dir)
		if err != nil {
			return diagnostic(dir, err)
		}
		options.Package = goIdentifier(path.Base(importPath))
		lexer.VisitFixedTokens(func(kind lexer.Kind, text string) {
			if kind == lexer.Keyword && text == options.Package {
				options.Package += "_"
			}
		})
		if err := writeBindings(pkg, options, filepath.Join(dir, "bindings.gfx"), dir, shimPath, shimPackage); err != nil {
			return err
		}
	}
	return nil
}
