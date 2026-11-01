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
	"gofx/parser"
	"gofx/prelude"
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

const usage = "gofx:1:1: usage: gofx check FILE.gfx... | gofx build [-o DIR | --emit-go DIR --package NAME] FILE.gfx... | gofx run FILE.gfx... [-- args...] | gofx test FILE.gfx... | gofx eval 'EXPR' [FILE.gfx...] | gofx bind IMPORT [-o FILE.gfx] [--prefix STR] | gofx lsp"

func run(args []string) error {
	if len(args) > 0 && args[0] == "lsp" {
		if len(args) != 1 {
			return errors.New(usage)
		}
		return lsp.Serve(os.Stdin, os.Stdout)
	}
	if len(args) > 0 && args[0] == "bind" {
		return runBind(args[1:])
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
	rest := args[1:]
	for i, arg := range rest {
		if arg == "--" {
			if command != "run" {
				return errors.New(usage)
			}
			progArgs = rest[i+1:]
			break
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
	program, err := checkFiles(files)
	if err != nil {
		return err
	}
	if command == "check" {
		return nil
	}
	source, err := codegen.Generate(program, goPackage)
	if err != nil {
		return diagnostic(file, err)
	}
	if outputDir == "" {
		outputDir = strings.TrimSuffix(file, filepath.Ext(file)) + "_gen"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return diagnostic(file, err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "main.go"), source, 0o644); err != nil {
		return diagnostic(file, err)
	}
	if command == "build" {
		return nil
	}
	dir, err := filepath.Abs(outputDir)
	if err != nil {
		return diagnostic(file, err)
	}
	root, err := moduleRoot(dir)
	if err != nil {
		return diagnostic(file, err)
	}
	relative, err := filepath.Rel(root, dir)
	if err != nil {
		return diagnostic(file, err)
	}
	return execute(root, "./"+filepath.ToSlash(relative), file, progArgs)
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

func parseFiles(inputs []string) ([]*ast.File, error) {
	preludeFile, err := parser.Parse(prelude.Source)
	if err != nil {
		return nil, diagnostic("prelude.gfx", err)
	}
	preludeFile.Name = "prelude.gfx"
	files := []*ast.File{preludeFile}
	for _, input := range inputs {
		source, err := os.ReadFile(input)
		if err != nil {
			return nil, diagnostic(input, err)
		}
		parsed, err := parser.Parse(string(source))
		if err != nil {
			return nil, diagnostic(input, err)
		}
		parsed.Name = input
		if len(files) > 1 && parsed.Package != files[1].Package {
			return nil, fmt.Errorf("%s:1:1: package %s differs from %s", input, parsed.Package, files[1].Package)
		}
		files = append(files, parsed)
	}
	return files, nil
}

func checkFiles(files []*ast.File) (*types.Program, error) {
	program, err := types.Check(files)
	if err != nil {
		return nil, diagnostic(files[1].Name, err)
	}
	for _, warning := range program.Warnings {
		printWarning(files[1].Name, warning)
	}
	return program, nil
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
