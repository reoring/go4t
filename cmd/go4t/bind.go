package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/reoring/go4t/extern"
	"github.com/reoring/go4t/types"
)

const bindUsage = "go4t:1:1: usage: go4t bind IMPORT [FILE.gt|DIR...] [-o FILE.gt|DIR] [--prefix STR] [--shim DIR]"

func runBind(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("%s", bindUsage)
	}
	path := args[0]
	var output, prefix, shimDir string
	var inputs []string
	seen := make(map[string]bool)
	for i := 1; i < len(args); i++ {
		flag := args[i]
		if !strings.HasPrefix(flag, "-") {
			inputs = append(inputs, flag)
			continue
		}
		if (flag != "-o" && flag != "--prefix" && flag != "--shim") || i+1 == len(args) || seen[flag] {
			return fmt.Errorf("%s", bindUsage)
		}
		seen[flag] = true
		switch flag {
		case "-o":
			output = args[i+1]
		case "--prefix":
			prefix = args[i+1]
		case "--shim":
			shimDir = args[i+1]
		}
		if flag != "--prefix" && args[i+1] == "" {
			return fmt.Errorf("%s", bindUsage)
		}
		i++
	}
	if prefix != "" {
		valid := !strings.HasPrefix(prefix, "ext_") && !strings.HasPrefix(prefix, "dict_")
		for i, r := range prefix {
			valid = valid && ((i == 0 && unicode.IsLower(r)) || (i != 0 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')))
		}
		if !valid {
			return fmt.Errorf("go4t:1:1: invalid binding prefix %q", prefix)
		}
	}
	var loader extern.Loader
	packageName := "main"
	if len(inputs) != 0 {
		loaded, err := parseFiles(inputs)
		if err != nil {
			return diagnostic(inputs[0], err)
		}
		entry := loaded[len(loaded)-1]
		packageName = entry.Name
		loader.Dir = entry.Path
	}
	var shimPath, shimPackage string
	if shimDir != "" {
		var err error
		shimPath, shimPackage, err = prepareShimDir(shimDir)
		if err != nil {
			return diagnostic(shimDir, err)
		}
		if loader.Dir == "" {
			loader.Dir = shimDir
		}
	}
	pkg, err := loader.Load(path)
	if err != nil {
		return diagnostic("go4t", err)
	}
	options := extern.BindOptions{Prefix: prefix, Prelude: make(map[string]bool), Float: true, Package: packageName}
	for _, name := range types.PreludeExports {
		options.Prelude[name] = true
	}
	return writeBindings(pkg, options, output, shimDir, shimPath, shimPackage)
}

// writeBindings writes the declarations and optional shim for one Go package.
func writeBindings(pkg *extern.Package, options extern.BindOptions, output, shimDir, shimPath, shimPackage string) error {
	var err error
	source := extern.Bind(pkg, options)
	if shimDir != "" {
		bindings, shim, _, shimErr := extern.BindWithShim(pkg, options, shimPath, shimPackage)
		if shimErr != nil {
			return diagnostic(shimDir, shimErr)
		}
		shimFile := filepath.Join(shimDir, "go4t_shim_"+goIdentifier(filepath.Base(pkg.Path))+".go")
		if err := os.WriteFile(shimFile, shim, 0o644); err != nil {
			return diagnostic(shimFile, err)
		}
		source = bindings
	}
	if output == "" {
		_, err = fmt.Fprint(os.Stdout, source)
	} else {
		if info, statErr := os.Stat(output); statErr == nil && info.IsDir() {
			output = filepath.Join(output, "bindings.gt")
		}
		err = os.WriteFile(output, []byte(source), 0o644)
	}
	if err != nil {
		if output == "" {
			output = "go4t"
		}
		return diagnostic(output, err)
	}
	return nil
}
