package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gofx/extern"
	"gofx/modules"
	"gofx/types"
)

const bindUsage = "gofx:1:1: usage: gofx bind IMPORT [FILE.gfx|DIR...] [-o FILE.gfx|DIR] [--prefix STR]"

func runBind(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("%s", bindUsage)
	}
	path := args[0]
	var output, prefix string
	var inputs []string
	seen := make(map[string]bool)
	for i := 1; i < len(args); i++ {
		flag := args[i]
		if !strings.HasPrefix(flag, "-") {
			inputs = append(inputs, flag)
			continue
		}
		if (flag != "-o" && flag != "--prefix") || i+1 == len(args) || seen[flag] {
			return fmt.Errorf("%s", bindUsage)
		}
		seen[flag] = true
		if flag == "-o" {
			output = args[i+1]
			if output == "" {
				return fmt.Errorf("%s", bindUsage)
			}
		} else {
			prefix = args[i+1]
		}
		i++
	}
	if prefix != "" {
		valid := !strings.HasPrefix(prefix, "ext_") && !strings.HasPrefix(prefix, "dict_")
		for i, r := range prefix {
			valid = valid && ((i == 0 && unicode.IsLower(r)) || (i != 0 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')))
		}
		if !valid {
			return fmt.Errorf("gofx:1:1: invalid binding prefix %q", prefix)
		}
	}
	var loader extern.Loader
	packageName := "main"
	if len(inputs) != 0 {
		loaded, err := modules.Load(inputs)
		if err != nil {
			return diagnostic(inputs[0], err)
		}
		entry := loaded[len(loaded)-1]
		packageName = entry.Name
		loader.Dir = entry.Path
	}
	pkg, err := loader.Load(path)
	if err != nil {
		return diagnostic("gofx", err)
	}
	options := extern.BindOptions{Prefix: prefix, Prelude: make(map[string]bool), Float: true, Package: packageName}
	for _, name := range types.PreludeExports {
		options.Prelude[name] = true
	}
	source := extern.Bind(pkg, options)
	if output == "" {
		_, err = fmt.Fprint(os.Stdout, source)
	} else {
		if info, statErr := os.Stat(output); statErr == nil && info.IsDir() {
			output = filepath.Join(output, "bindings.gfx")
		}
		err = os.WriteFile(output, []byte(source), 0o644)
	}
	if err != nil {
		if output == "" {
			output = "gofx"
		}
		return diagnostic(output, err)
	}
	return nil
}
