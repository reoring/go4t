// Command gofx checks, builds, runs gofx sources, and binds Go packages.
// Several files form one program (same package, mutually visible declarations);
// the first file names the output directory.
// Successful checks report non-fatal warnings to stderr before continuing.
// Usage: gofx {check|build} FILE.gfx... | gofx run FILE.gfx... [-- args...]
// | gofx bind IMPORT [-o FILE.gfx] [--prefix STR].
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gofx/ast"
	"gofx/codegen"
	"gofx/parser"
	"gofx/prelude"
	"gofx/types"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "bind" {
		return runBind(args[1:])
	}
	usage := fmt.Errorf("gofx:1:1: usage: gofx {check|build} FILE.gfx... | gofx run FILE.gfx... [-- args...] | gofx bind IMPORT [-o FILE.gfx] [--prefix STR]")
	if len(args) < 2 || (args[0] != "check" && args[0] != "build" && args[0] != "run") {
		return usage
	}
	command := args[0]
	var inputs, progArgs []string
	rest := args[1:]
	for i, arg := range rest {
		if arg == "--" {
			if command != "run" {
				return usage
			}
			progArgs = rest[i+1:]
			break
		}
		inputs = append(inputs, arg)
	}
	if len(inputs) == 0 {
		return usage
	}
	file := inputs[0]
	preludeFile, err := parser.Parse(prelude.Source)
	if err != nil {
		return diagnostic("prelude.gfx", err)
	}
	preludeFile.Name = "prelude.gfx"
	files := []*ast.File{preludeFile}
	for _, input := range inputs {
		source, err := os.ReadFile(input)
		if err != nil {
			return diagnostic(input, err)
		}
		parsed, err := parser.Parse(string(source))
		if err != nil {
			return diagnostic(input, err)
		}
		parsed.Name = input
		if parsed.Package != files[len(files)-1].Package && len(files) > 1 {
			return fmt.Errorf("%s:1:1: package %s differs from %s", input, parsed.Package, files[1].Package)
		}
		files = append(files, parsed)
	}
	program, err := types.Check(files)
	if err != nil {
		return diagnostic(file, err)
	}
	for _, warning := range program.Warnings {
		warningFile := warning.File
		if warningFile == "" {
			warningFile = file
		}
		fmt.Fprintf(os.Stderr, "%s:%s: warning: %s\n", warningFile, warning.Pos, warning.Msg)
	}
	if command == "check" {
		return nil
	}
	source, err := codegen.Generate(program, "main")
	if err != nil {
		return diagnostic(file, err)
	}
	outputDir := strings.TrimSuffix(file, filepath.Ext(file)) + "_gen"
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
	// Build to a temporary binary and exec it directly: `go run` collapses
	// every non-zero program exit into status 1 and prints "exit status N",
	// which would hide the program's own exit code (see `exit`).
	tmp, err := os.MkdirTemp("", "gofx-run-")
	if err != nil {
		return diagnostic(file, err)
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "prog")
	build := exec.Command("go", "build", "-o", bin, "./"+filepath.ToSlash(relative))
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
			os.RemoveAll(tmp)
			os.Exit(exit.ExitCode())
		}
		return diagnostic(file, fmt.Errorf("run: %w", err))
	}
	return nil
}

var positionedError = regexp.MustCompile(`^\d+:\d+:`)

func diagnostic(file string, err error) error {
	var checked *types.Error
	if errors.As(err, &checked) && checked.File != "" {
		file = checked.File
	}
	if positionedError.MatchString(err.Error()) {
		return fmt.Errorf("%s:%w", file, err)
	}
	return fmt.Errorf("%s:1:1: %w", file, err)
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
