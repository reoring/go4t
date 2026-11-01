// Command gofx checks, builds, runs, tests, evaluates sources, and binds Go packages.
// Several files form one program (same package, mutually visible declarations);
// the first file names the default build directory.
// Successful checks report non-fatal warnings to stderr before continuing.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gofx/ast"
	"gofx/codegen"
	"gofx/lsp"
	"gofx/modules"
	"gofx/types"
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

const usage = "gofx:1:1: usage: gofx init <module-path> | gofx get [--no-bind] <path>@<version> | gofx check [--full-types] FILE.gfx|DIR... | gofx build [--full-types] [-o DIR | --emit-go DIR --package NAME] FILE.gfx|DIR... | gofx run [--full-types] FILE.gfx|DIR... [-- args...] | gofx test FILE.gfx|DIR... | gofx eval 'EXPR' [FILE.gfx|DIR...] | gofx bind IMPORT [FILE.gfx|DIR...] [-o FILE.gfx|DIR] [--prefix STR] [--shim DIR] | gofx bind --module PATH | gofx lsp | gofx fmt [-w|-d|-l] FILE...|DIR"

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
		return lsp.Serve(os.Stdin, os.Stdout)
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
	outputDir, goPackage := "", "main"
	if command == "build" {
		var err error
		args, outputDir, goPackage, err = buildOptions(args)
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
	files, err := parseFiles(inputs)
	if err != nil {
		return err
	}
	program, err := checkFiles(files, fullTypes)
	if err != nil {
		return err
	}
	if command == "check" {
		return nil
	}

	if outputDir == "" {
		outputDir = strings.TrimSuffix(file, filepath.Ext(file)) + "_gen"
		if info, statErr := os.Stat(file); statErr == nil && info.IsDir() {
			outputDir = filepath.Join(file, "_gen")
		}
	}
	dir, err := filepath.Abs(outputDir)
	if err != nil {
		return diagnostic(file, err)
	}
	outputRoot := buildModuleRoot(dir)
	root := outputRoot
	if command == "build" && root == "" {
		base, err := inputDirectory(inputs)
		if err != nil {
			return diagnostic(file, err)
		}
		root = buildModuleRoot(base)
	}
	module, err := prepareBuildModule(root, program)
	if err != nil {
		return err
	}
	defer module.close()
	sources, err := codegen.GenerateInDir(program, goPackage, module.dir)
	if err != nil {
		var generated *types.Error
		if errors.As(err, &generated) {
			generated.Msg = generated.Message(fullTypes)
		}
		return diagnostic(file, err)
	}
	if command == "run" && module.temporary {
		dir = filepath.Join(module.dir, "prog")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return diagnostic(file, err)
	}
	if err := emitModules(dir, sources); err != nil {
		return diagnostic(file, err)
	}
	if command == "build" {
		if outputRoot == "" {
			printBuildModuleNote(dir)
		}
		return nil
	}
	relative, err := filepath.Rel(module.dir, dir)
	if err != nil {
		return diagnostic(file, err)
	}
	return execute(module.dir, "./"+filepath.ToSlash(relative), file, progArgs)
}

func execute(root, source, file string, progArgs []string) error {
	// Build to a temporary binary and exec it directly: `go run` collapses
	// every non-zero program exit into status 1 and prints "exit status N",
	// which would hide the program's own exit code (see `exit`).
	tmp, err := os.MkdirTemp("", "gofx-run-")
	if err != nil {
		return diagnostic(file, err)
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "prog")
	build := exec.Command("go", "build", "-o", bin, source)
	build.Dir = root
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return diagnostic(file, fmt.Errorf("go build: %w", err))
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

func parseFiles(inputs []string) ([]*ast.Module, error) {
	modules, err := modules.Load(inputs)
	if err != nil {
		file := "gofx"
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
		return nil, diagnostic("gofx", err)
	}
	for _, warning := range program.Warnings {
		printWarning("gofx", warning)
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
		if strings.HasPrefix(string(data), "// Code generated by gofx. DO NOT EDIT.") {
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
