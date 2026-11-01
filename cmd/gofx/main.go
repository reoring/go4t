// Command gofx checks, builds, or runs a gofx source file and its prelude.
// Successful checks report non-fatal warnings to stderr before continuing.
// Usage: gofx {check|build} FILE.gfx | gofx run FILE.gfx [-- args...].
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
	if len(args) < 2 || (args[0] != "check" && args[0] != "build" && args[0] != "run") ||
		(len(args) > 2 && (args[0] != "run" || args[2] != "--")) {
		return fmt.Errorf("gofx:1:1: usage: gofx {check|build} FILE.gfx | gofx run FILE.gfx [-- args...]")
	}
	command, file := args[0], args[1]
	user, err := os.ReadFile(file)
	if err != nil {
		return diagnostic(file, err)
	}
	preludeFile, err := parser.Parse(prelude.Source)
	if err != nil {
		return diagnostic("prelude.gfx", err)
	}
	preludeFile.Name = "prelude.gfx"
	userFile, err := parser.Parse(string(user))
	if err != nil {
		return diagnostic(file, err)
	}
	userFile.Name = file
	program, err := types.Check([]*ast.File{preludeFile, userFile})
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
	var progArgs []string
	if len(args) > 2 {
		progArgs = args[3:]
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
