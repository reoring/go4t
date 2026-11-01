package main

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"gofx/ast"
	"gofx/extern"
	"gofx/parser"
	"gofx/prelude"
	"gofx/types"
)

const bindUsage = "gofx:1:1: usage: gofx bind IMPORT [-o FILE.gfx] [--prefix STR]"

func runBind(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("%s", bindUsage)
	}
	path := args[0]
	var output, prefix string
	seen := make(map[string]bool)
	for i := 1; i < len(args); i += 2 {
		flag := args[i]
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
	pkg, err := loader.Load(path)
	if err != nil {
		return diagnostic("gofx", err)
	}
	parsed, err := parser.Parse(prelude.Source)
	if err != nil {
		return diagnostic("prelude.gfx", err)
	}
	options := extern.BindOptions{Prefix: prefix, Prelude: make(map[string]bool)}
	for _, decl := range parsed.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			options.Prelude[decl.Name] = true
		case *ast.ClassDecl:
			for _, method := range decl.Methods {
				options.Prelude[method.Name] = true
			}
		}
	}
	// Probe the checker rather than requiring types.Float at compile time. This
	// keeps bind correct both before and after the Float builtin is available.
	probe, err := parser.Parse(`package main
extern func probeFloat(x Float) Float = "unused".Float
extern func main() IO[Unit] = "unused".Main
`)
	if err != nil {
		return diagnostic("gofx", err)
	}
	_, err = types.Check([]*ast.File{probe})
	options.Float = err == nil
	source := extern.Bind(pkg, options)
	if output == "" {
		_, err = fmt.Fprint(os.Stdout, source)
	} else {
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
