package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gofx/ast"
	"gofx/codegen"
	"gofx/parser"
	"gofx/types"
)

// buildOptions accepts output flags before or after the input files.
func buildOptions(args []string) (inputs []string, outputDir, goPackage string, err error) {
	inputs = []string{"build"}
	var output, emit, pkg string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-o", "--emit-go", "--package":
			if i+1 == len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
				return nil, "", "", errors.New(usage)
			}
			i++
			var target *string
			switch arg {
			case "-o":
				target = &output
			case "--emit-go":
				target = &emit
			case "--package":
				target = &pkg
			}
			if *target != "" {
				return nil, "", "", errors.New(usage)
			}
			*target = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, "", "", errors.New(usage)
			}
			inputs = append(inputs, arg)
		}
	}
	if len(inputs) == 1 || (emit != "" && output != "") || (emit == "") != (pkg == "") || pkg == "main" {
		return nil, "", "", errors.New(usage)
	}
	if emit != "" {
		return inputs, emit, pkg, nil
	}
	return inputs, output, "main", nil
}

func testFiles(inputs []string) error {
	if err := sourceArguments(inputs); err != nil {
		return err
	}
	files, err := parseFiles(inputs)
	if err != nil {
		return err
	}
	entry := files[len(files)-1]
	removeMain(entry.Files)
	harness, err := testHarness(entry.Files)
	if err != nil {
		return err
	}
	harness.Package = entry.Name
	entry.Files = append(entry.Files, harness)
	program, err := checkFiles(files)
	if err != nil {
		return err
	}
	return runTemporary(program, inputs, "<test>")
}

func evalExpression(source string, inputs []string) error {
	if err := sourceArguments(inputs); err != nil {
		return err
	}
	expression, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		return diagnostic("<eval>", err)
	}
	files, err := parseFiles(inputs)
	if err != nil {
		return err
	}
	entry := files[len(files)-1]
	removeMain(entry.Files)
	wrapper, err := evalWrapper(expression, false)
	if err != nil {
		return err
	}
	wrapper.Package = entry.Name
	if len(entry.Files) != 0 {
		wrapper.Imports = entry.Files[0].Imports
	}
	entry.Files = append(entry.Files, wrapper)
	program, originalErr := checkFiles(files)
	if originalErr != nil {
		// Checking the IO wrapper supplies the IO context without relying on
		// diagnostic wording or on the failed check's partial annotations.
		wrapper, err = evalWrapper(expression, true)
		if err != nil {
			return err
		}
		wrapper.Package = entry.Name
		if len(entry.Files) > 1 {
			wrapper.Imports = entry.Files[0].Imports
		}
		entry.Files[len(entry.Files)-1] = wrapper
		program, err = checkFiles(files)
		if err != nil {
			return originalErr
		}
	}
	return runTemporary(program, inputs, "<eval>")
}

func sourceArguments(inputs []string) error {
	for _, input := range inputs {
		if strings.HasPrefix(input, "-") {
			return errors.New(usage)
		}
	}
	return nil
}

// Remove the user's entry before type checking; test/eval provide their own.
func removeMain(files []*ast.File) {
	for _, file := range files {
		decls := file.Decls[:0]
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok && f.Name == "main" {
				continue
			}
			decls = append(decls, decl)
		}
		file.Decls = decls
	}
}

// The wrapper is parsed independently, then the expression AST is injected.
// Parsing EXPR on its own rejects trailing declarations and preserves its positions.
func evalWrapper(expression ast.Expr, effectful bool) (*ast.File, error) {
	body := "println(show(gofxEvalExpression))"
	if effectful {
		body = "do { gofxEvalValue <- gofxEvalExpression; println(show(gofxEvalValue)) }"
	}
	file, err := parser.Parse("package main\nfunc main() IO[Unit] { " + body + " }\n")
	if err != nil {
		return nil, diagnostic("<eval>", err)
	}
	file.Name = "<eval>"
	main := file.Decls[0].(*ast.FuncDecl)
	if effectful {
		main.Body.Result.(*ast.Do).Stmts[0].(*ast.DoBind).X = expression
	} else {
		main.Body.Result.(*ast.Call).Args[0].(*ast.Call).Args[0] = expression
	}
	return file, nil
}

func testHarness(files []*ast.File) (*ast.File, error) {
	var tests []*ast.FuncDecl
	names := make(map[string]bool)
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				for _, method := range d.Methods {
					names[method.Name] = true
				}
			case *ast.FuncDecl:
				names[d.Name] = true
				if d.Extern != nil || len(d.Params) != 0 || len(d.TParams) != 0 ||
					!strings.HasPrefix(d.Name, "test") || len(d.Name) == len("test") {
					continue
				}
				if isType(d.Ret, "Bool") || isIOBool(d.Ret) {
					tests = append(tests, d)
				}
			}
		}
	}
	runner := "gofxTestRun"
	for names[runner] {
		runner += "_"
	}
	var source strings.Builder
	fmt.Fprintf(&source, "package main\nimport \"std/io\"\nextern func %s(name String, action func() IO[Bool]) IO[Bool] = %q.Run\n", runner, "gofx/rt/testrt")
	source.WriteString("func main() IO[Unit] { do {\n")
	for i, test := range tests {
		action := test.Name + "()"
		if !isIOBool(test.Ret) {
			action = "ioPure(" + action + ")"
		}
		fmt.Fprintf(&source, "gofxTestResult%d <- %s(%s, func() => %s)\n", i, runner, strconv.Quote(test.Name), action)
	}
	source.WriteString("let gofxTestPassed = 0")
	for i := range tests {
		fmt.Fprintf(&source, " + (if gofxTestResult%d { 1 } else { 0 })", i)
	}
	fmt.Fprintf(&source, "\nprintln(show(gofxTestPassed) + \" passed, \" + show(%d - gofxTestPassed) + \" failed\")\n", len(tests))
	fmt.Fprintf(&source, "if gofxTestPassed == %d { ioPure(unit()) } else { io.exit(1) }\n} }\n", len(tests))
	file, err := parser.Parse(source.String())
	if err != nil {
		return nil, diagnostic("<test>", err)
	}
	file.Name = "<test>"
	return file, nil
}

func isType(typ ast.Type, name string) bool {
	t, ok := typ.(*ast.TypeName)
	return ok && t.Name == name && len(t.Args) == 0
}

func isIOBool(typ ast.Type) bool {
	t, ok := typ.(*ast.TypeName)
	return ok && t.Name == "IO" && len(t.Args) == 1 && isType(t.Args[0], "Bool")
}

// Test/eval artifacts never replace a program's ordinary build output. A usable
// input module resolves externs; otherwise the compiler supplies its own runtime.
func runTemporary(program *types.Program, inputs []string, file string) error {
	base, err := inputDirectory(inputs)
	if err != nil {
		return diagnostic(file, err)
	}
	module, err := prepareBuildModule(buildModuleRoot(base), program)
	if err != nil {
		return err
	}
	defer module.close()
	sources, err := codegen.GenerateInDir(program, "main", module.dir)
	if err != nil {
		return diagnostic(file, err)
	}
	dir := filepath.Join(module.dir, "prog")
	if module.temporary {
		err = os.Mkdir(dir, 0o755)
	} else {
		dir, err = os.MkdirTemp(module.dir, ".gofx-source-")
	}
	if err != nil {
		return diagnostic(file, err)
	}
	defer os.RemoveAll(dir)
	if err := emitModules(dir, sources); err != nil {
		return diagnostic(file, err)
	}
	return execute(module.dir, "./"+filepath.Base(dir), file, nil)
}
