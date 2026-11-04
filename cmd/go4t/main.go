// Command go4t checks, builds, runs, tests, evaluates sources, and binds Go packages.
// Several files form one program (same package, mutually visible declarations);
// the first input names the default binary and generated-source directory.
// Successful checks report non-fatal warnings to stderr before continuing.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go4t/ast"
	"go4t/codegen"
	"go4t/laws"
	"go4t/lsp"
	"go4t/modules"
	"go4t/types"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var status exitStatus
		if errors.As(err, &status) {
			os.Exit(int(status))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const usage = "go4t:1:1: usage: go4t init <module-path> | go4t get <path>@<version> | go4t check [--full-types] FILE.gt|DIR... | go4t build [--full-types] [-o FILE | --emit-go DIR [--package NAME]] FILE.gt|DIR... | go4t run [--full-types] FILE.gt|DIR... [-- args...] | go4t test FILE.gt|DIR... | go4t eval 'EXPR' [FILE.gt|DIR...] | go4t bind IMPORT [FILE.gt|DIR...] [-o FILE.gt|DIR] [--prefix STR] [--shim DIR] | go4t lsp | go4t fmt [-w|-d|-l] FILE...|DIR"

func run(args []string) error {
	if len(args) > 0 && args[0] == "init" {
		return runInit(args[1:])
	}
	if len(args) > 0 && args[0] == "get" {
		return runGet(args[1:])
	}
	if len(args) > 0 && args[0] == "lsp" {
		if len(args) != 1 {
			return errors.New(usage)
		}
		return lsp.Serve(os.Stdin, os.Stdout, lsp.Options{
			GoBinder: func(directory string) modules.GoBinder { return newGoImportBinder(directory) },
		})
	}
	if len(args) > 0 && args[0] == "bind" {
		return runBind(args[1:])
	}
	if len(args) > 0 && args[0] == "fmt" {
		return runFmt(args[1:], os.Stdout)
	}
	if len(args) < 2 {
		return errors.New(usage)
	}
	command := args[0]
	switch command {
	case "test":
		return testFiles(args[1:])
	case "eval":
		return evalExpression(args[1], args[2:])
	case "check", "build", "run":
	default:
		return errors.New(usage)
	}
	build := buildFlags{goPackage: "main"}
	if command == "build" {
		var err error
		args, build, err = buildOptions(args)
		if err != nil {
			return err
		}
	}
	var inputs, progArgs []string
	fullTypes := false
	rest := args[1:]
	for i, arg := range rest {
		if arg == "--" {
			if command != "run" {
				return errors.New(usage)
			}
			progArgs = rest[i+1:]
			break
		}
		if arg == "--full-types" {
			fullTypes = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return errors.New(usage)
		}
		inputs = append(inputs, arg)
	}
	if len(inputs) == 0 {
		return errors.New(usage)
	}
	file := inputs[0]
	if command == "build" && build.emitGo == "" {
		var err error
		build.outputFile, err = buildOutput(file, build.outputFile)
		if err != nil {
			return err
		}
	}
	var module buildModule
	var dir, sourceDir string
	var options []modules.Options
	var binder *goImportBinder
	if command != "check" {
		outputDir := build.emitGo
		if outputDir == "" {
			outputDir = strings.TrimSuffix(file, filepath.Ext(file)) + "_gen"
			if info, statErr := os.Stat(file); statErr == nil && info.IsDir() {
				outputDir = filepath.Join(file, "_gen")
			}
		}
		var err error
		dir, err = filepath.Abs(outputDir)
		if err != nil {
			return diagnostic(file, err)
		}
		root := buildModuleRoot(dir)
		if command == "build" && root == "" {
			base, err := inputDirectory(inputs)
			if err != nil {
				return diagnostic(file, err)
			}
			root = buildModuleRoot(base)
		}
		module, err = prepareBuildModule(root)
		if err != nil {
			return err
		}
		defer module.close()
		sourceDir = dir
		if module.temporary && (command == "run" || build.emitGo == "") {
			sourceDir = filepath.Join(module.dir, "prog")
			if command == "run" {
				dir = sourceDir
			}
		}
		binder = newGoImportBinder(module.dir)
		binder.outputDir = sourceDir
		options = append(options, modules.Options{GoBinder: binder})
	}
	files, err := parseFiles(inputs, options...)
	if err != nil {
		return err
	}
	program, err := checkFiles(files, fullTypes)
	if err != nil {
		return err
	}
	if command == "check" {
		failures := laws.CheckProgram(program)
		for _, notice := range laws.SampleNotices(program) {
			fmt.Fprintln(os.Stderr, notice)
		}
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, diagnostic("go4t", failure))
		}
		if len(failures) != 0 {
			return exitStatus(1)
		}
		return nil
	}

	if err := module.checkForeign(program); err != nil {
		return err
	}
	sources, err := codegen.GenerateInDir(program, build.goPackage, module.dir, codegen.Options{SideEffectImports: binder.sideEffectImports})
	if err != nil {
		var generated *types.Error
		if errors.As(err, &generated) {
			generated.Msg = generated.Message(fullTypes)
		}
		return diagnostic(file, err)
	}
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return diagnostic(file, err)
	}
	if err := emitModules(sourceDir, sources); err != nil {
		return diagnostic(file, err)
	}
	if sourceDir != dir {
		if err := retainBuildSources(sourceDir, dir, sources); err != nil {
			return diagnostic(file, err)
		}
	}
	if command == "build" && build.emitGo != "" {
		return nil
	}
	relative, err := filepath.Rel(module.dir, sourceDir)
	if err != nil {
		return diagnostic(file, err)
	}
	source := "./" + filepath.ToSlash(relative)
	if command == "build" {
		return buildBinary(module.dir, source, file, build.outputFile)
	}
	return execute(module.dir, source, file, progArgs)
}

func buildBinary(root, source, file, output string) error {
	build := exec.Command("go", "build", "-o", output, source)
	build.Dir = root
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return diagnostic(file, fmt.Errorf("go build: %w", err))
	}
	return nil
}

func execute(root, source, file string, progArgs []string) error {
	// Build to a temporary binary and exec it directly: `go run` collapses
	// every non-zero program exit into status 1 and prints "exit status N",
	// which would hide the program's own exit code (see `exit`).
	tmp, err := os.MkdirTemp("", "go4t-run-")
	if err != nil {
		return diagnostic(file, err)
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "prog")
	if err := buildBinary(root, source, file, bin); err != nil {
		return err
	}
	process := exec.Command(bin, progArgs...)
	process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := process.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exitStatus(exit.ExitCode())
		}
		return diagnostic(file, fmt.Errorf("run: %w", err))
	}
	return nil
}

type exitStatus int

func (s exitStatus) Error() string { return fmt.Sprintf("program exited with status %d", s) }

func parseFiles(inputs []string, options ...modules.Options) ([]*ast.Module, error) {
	if len(options) == 0 {
		base, err := inputDirectory(inputs)
		if err != nil {
			return nil, err
		}
		options = append(options, modules.Options{GoBinder: newGoImportBinder(base)})
	}
	modules, err := modules.Load(inputs, options...)
	if err != nil {
		file := "go4t"
		if len(inputs) != 0 {
			file = inputs[0]
		}
		return nil, diagnostic(file, err)
	}
	return modules, nil
}

func checkFiles(modules []*ast.Module, fullTypes bool) (*types.Program, error) {
	program, err := types.Check(modules)
	if err != nil {
		var checked *types.Error
		if errors.As(err, &checked) {
			checked.Msg = checked.Message(fullTypes)
		}
		return nil, diagnostic("go4t", err)
	}
	for _, warning := range program.Warnings {
		printWarning("go4t", warning)
	}
	return program, nil
}

func emitModules(dir string, sources map[string][]byte) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || sources[entry.Name()] != nil {
			continue
		}
		name := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(data), "// Code generated by go4t. DO NOT EDIT.") {
			if err := os.Remove(name); err != nil {
				return err
			}
		}
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(dir, name), source, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func moduleRoot(dir string) (string, error) {
	for {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !info.IsDir() {
			return dir, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("find module root: %w", err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above input directory")
		}
		dir = parent
	}
}
