package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// prepareShimDir resolves the receiving import path even before a directory
// contains Go source. Module lookup happens before creating any directories.
func prepareShimDir(dir string) (string, string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	existing := abs
	for {
		info, err := os.Stat(existing)
		if err == nil {
			if !info.IsDir() {
				return "", "", fmt.Errorf("shim path %q is not a directory", existing)
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", "", err
		}
		existing = parent
	}
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = existing
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("resolve shim Go module: %s", strings.TrimSpace(string(data)))
	}
	module := strings.TrimSpace(string(data))
	if module == "" || module == os.DevNull {
		return "", "", fmt.Errorf("--shim requires DIR inside a Go module (with go.mod)")
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", "", err
	}
	cmd = exec.Command("go", "list", "-m", "-json")
	cmd.Dir = abs
	data, err = cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return "", "", fmt.Errorf("resolve shim import path: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return "", "", fmt.Errorf("resolve shim import path: %w", err)
	}
	var listed struct {
		Path string
		Dir  string
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		return "", "", fmt.Errorf("decode shim go list: %w", err)
	}
	if listed.Path == "" || listed.Dir == "" {
		return "", "", fmt.Errorf("--shim requires DIR inside a Go module (with go.mod)")
	}
	relative, err := filepath.Rel(listed.Dir, abs)
	if err != nil || !filepath.IsLocal(relative) {
		return "", "", fmt.Errorf("--shim requires DIR inside a Go module (with go.mod)")
	}
	importPath := listed.Path
	if relative != "." {
		importPath += "/" + filepath.ToSlash(relative)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", "", err
	}
	name := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(abs, entry.Name()), nil, parser.PackageClauseOnly)
		if err != nil {
			return "", "", fmt.Errorf("read shim package: %w", err)
		}
		if name != "" && name != file.Name.Name {
			return "", "", fmt.Errorf("shim directory contains multiple Go packages: %s and %s", name, file.Name.Name)
		}
		name = file.Name.Name
	}
	if name == "" {
		name = goIdentifier(filepath.Base(abs))
	}
	return importPath, name, nil
}

func goIdentifier(name string) string {
	var out strings.Builder
	for i, r := range name {
		if i == 0 && unicode.IsDigit(r) {
			out.WriteByte('_')
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	name = out.String()
	if name == "" || name == "_" {
		return "shim"
	}
	if token.Lookup(name).IsKeyword() {
		name += "_"
	}
	return name
}
