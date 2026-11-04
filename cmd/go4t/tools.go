package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go4t/ast"
	"go4t/codegen"
	"go4t/modules"
	"go4t/parser"
	"go4t/types"
)

type buildFlags struct {
	outputFile string
	emitGo     string
	goPackage  string
}

// buildOptions accepts output flags before or after the input files.
func buildOptions(args []string) (inputs []string, options buildFlags, err error) {
	inputs = []string{"build"}
	var output, emit, pkg string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--full-types":
			inputs = append(inputs, arg)
		case "-o", "--emit-go", "--package":
			if i+1 == len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
				return nil, options, errors.New(usage)
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
				return nil, options, errors.New(usage)
			}
			*target = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, options, errors.New(usage)
			}
			inputs = append(inputs, arg)
		}
	}
	if len(inputs) == 1 || (emit != "" && output != "") || (emit == "" && pkg != "") {
		return nil, options, errors.New(usage)
	}
	if pkg == "" {
		pkg = "main"
	}
	return inputs, buildFlags{outputFile: output, emitGo: emit, goPackage: pkg}, nil
}

func buildOutput(file, output string) (string, error) {
	if output == "" {
		absolute, err := filepath.Abs(file)
		if err != nil {
			return "", diagnostic(file, err)
		}
		output = filepath.Base(absolute)
		if info, err := os.Stat(file); err != nil || !info.IsDir() {
			output = strings.TrimSuffix(output, ".gt")
		}
	}
	if info, err := os.Stat(output); err == nil && info.IsDir() {
		return "", diagnostic(file, fmt.Errorf("build output %q is a directory; use -o FILE or --emit-go DIR", output))
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return "", diagnostic(file, err)
	}
	return absolute, nil
}

func testFiles(inputs []string) error {
	if err := sourceArguments(inputs); err != nil {
		return err
	}
	scratch, err := prepareTemporary(inputs, "<test>")
	if err != nil {
		return err
	}
	defer scratch.close()
	files, err := parseFiles(inputs, scratch.options())
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
	program, err := checkFiles(files, false)
	if err != nil {
		return err
	}
	return scratch.run(program, "<test>")
}

func evalExpression(source string, inputs []string) error {
	if err := sourceArguments(inputs); err != nil {
		return err
	}
	expression, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		return diagnostic("<eval>", err)
	}
	scratch, err := prepareTemporary(inputs, "<eval>")
	if err != nil {
		return err
	}
	defer scratch.close()
	files, err := parseFiles(inputs, scratch.options())
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
	program, originalErr := checkFiles(files, false)
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
		program, err = checkFiles(files, false)
		if err != nil {
			return originalErr
		}
	}
	return scratch.run(program, "<eval>")
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
	body := "println(show(go4tEvalExpression))"
	if effectful {
		body = "do { go4tEvalValue <- go4tEvalExpression; println(show(go4tEvalValue)) }"
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
			case *ast.TypeDecl:
				for _, ctor := range d.Ctors {
					names[ctor.Name] = true
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
	fresh := func(name string) string {
		for names[name] {
			name += "_"
		}
		names[name] = true
		return name
	}
	runner := fresh("go4tTestRun")
	// Bound both do-bind scope depth and sum expression depth, including when
	// there are more leaves than a single aggregation helper can call.
	const batchSize = 64
	var results [batchSize]string
	for i := range results {
		results[i] = fresh("go4tTestResult" + strconv.Itoa(i))
	}
	passed := fresh("go4tTestPassed")
	var source strings.Builder
	fmt.Fprintf(&source, "package main\nimport \"std/io\"\nextern func %s(name String, action func() IO[Bool]) IO[Bool] = %q.Run\n", runner, "go4t/rt/testrt")
	helpers := make([]string, 0, (len(tests)+batchSize-1)/batchSize)
	for start := 0; start < len(tests); start += batchSize {
		end := min(start+batchSize, len(tests))
		helper := fresh("go4tTestBatch" + strconv.Itoa(len(helpers)))
		helpers = append(helpers, helper)
		fmt.Fprintf(&source, "func %s() IO[Int] { do {\n", helper)
		for i, test := range tests[start:end] {
			action := test.Name + "()"
			if !isIOBool(test.Ret) {
				action = "ioPure(" + action + ")"
			}
			fmt.Fprintf(&source, "%s <- %s(%s, func() => %s)\n", results[i], runner, strconv.Quote(test.Name), action)
		}
		source.WriteString("ioPure(0")
		for i := range end - start {
			fmt.Fprintf(&source, " + (if %s { 1 } else { 0 })", results[i])
		}
		source.WriteString(")\n} }\n")
	}
	for level := 0; len(helpers) > 1; level++ {
		parents := make([]string, 0, (len(helpers)+batchSize-1)/batchSize)
		for start := 0; start < len(helpers); start += batchSize {
			end := min(start+batchSize, len(helpers))
			helper := fresh(fmt.Sprintf("go4tTestGroup%d_%d", level, len(parents)))
			parents = append(parents, helper)
			fmt.Fprintf(&source, "func %s() IO[Int] { do {\n", helper)
			for i, child := range helpers[start:end] {
				fmt.Fprintf(&source, "%s <- %s()\n", results[i], child)
			}
			source.WriteString("ioPure(0")
			for i := range end - start {
				fmt.Fprintf(&source, " + %s", results[i])
			}
			source.WriteString(")\n} }\n")
		}
		helpers = parents
	}
	source.WriteString("func main() IO[Unit] { do {\n")
	action := "ioPure(0)"
	if len(helpers) != 0 {
		action = helpers[0] + "()"
	}
	fmt.Fprintf(&source, "%s <- %s\n", passed, action)
	fmt.Fprintf(&source, "println(show(%s) + \" passed, \" + show(%d - %s) + \" failed\")\n", passed, len(tests), passed)
	fmt.Fprintf(&source, "if %s == %d { ioPure(unit()) } else { io.exit(1) }\n} }\n", passed, len(tests))
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

// temporaryBuild holds the module and source directory for test/eval runs.
// Both exist before imports are loaded so Go bindings can write their shims
// beneath the source directory; neither replaces a program's build output.
type temporaryBuild struct {
	module buildModule
	dir    string
	binder *goImportBinder
}

// prepareTemporary selects a usable input module to resolve externs; otherwise
// the compiler supplies its own runtime.
func prepareTemporary(inputs []string, file string) (*temporaryBuild, error) {
	base, err := inputDirectory(inputs)
	if err != nil {
		return nil, diagnostic(file, err)
	}
	module, err := prepareBuildModule(buildModuleRoot(base))
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(module.dir, "prog")
	if module.temporary {
		err = os.Mkdir(dir, 0o755)
	} else {
		dir, err = os.MkdirTemp(module.dir, ".go4t-source-")
	}
	if err != nil {
		module.close()
		return nil, diagnostic(file, err)
	}
	binder := newGoImportBinder(module.dir)
	binder.outputDir = dir
	return &temporaryBuild{module: module, dir: dir, binder: binder}, nil
}

func (t *temporaryBuild) options() modules.Options { return modules.Options{GoBinder: t.binder} }

func (t *temporaryBuild) close() {
	os.RemoveAll(t.dir)
	t.module.close()
}

func (t *temporaryBuild) run(program *types.Program, file string) error {
	if err := t.module.checkForeign(program); err != nil {
		return err
	}
	sources, err := codegen.GenerateInDir(program, "main", t.module.dir, codegen.Options{SideEffectImports: t.binder.sideEffectImports})
	if err != nil {
		return diagnostic(file, err)
	}
	if err := emitModules(t.dir, sources); err != nil {
		return diagnostic(file, err)
	}
	return execute(t.module.dir, "./"+filepath.Base(t.dir), file, nil)
}
