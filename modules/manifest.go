package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Requirement names a Go module containing go4t sources.
type Requirement struct {
	Path, Version string
	line          int
}

// Manifest is a go4t.mod file. Comments and unrelated requirements survive edits.
type Manifest struct {
	Module   string
	Requires []Requirement
	lines    []string
}

// ValidModulePath reports whether name is a clean, unquoted module path.
func ValidModulePath(name string) bool {
	if name == "" || name == "." || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "-") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/.-_~", c)) {
			return false
		}
	}
	return true
}

// ReadManifest reads the module declaration and single-line require directives.
func ReadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go4t.mod"))
	if err != nil {
		return nil, err
	}
	m := &Manifest{lines: strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")}
	seen := make(map[string]bool)
	for i, line := range m.lines {
		line, _, _ = strings.Cut(line, "//")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if m.Module == "" {
			if len(fields) != 2 || fields[0] != "module" || !ValidModulePath(fields[1]) {
				return nil, fmt.Errorf("invalid go4t.mod in %s: expected module <name>", dir)
			}
			m.Module = fields[1]
			continue
		}
		if len(fields) != 3 || fields[0] != "require" || !ValidModulePath(fields[1]) || strings.ContainsAny(fields[2], "@\"`\\") {
			return nil, fmt.Errorf("invalid go4t.mod in %s: line %d: expected require <path> <version>", dir, i+1)
		}
		if seen[fields[1]] {
			return nil, fmt.Errorf("invalid go4t.mod in %s: duplicate require %s", dir, fields[1])
		}
		seen[fields[1]] = true
		m.Requires = append(m.Requires, Requirement{Path: fields[1], Version: fields[2], line: i})
	}
	if m.Module == "" {
		return nil, fmt.Errorf("invalid go4t.mod in %s: expected module <name>", dir)
	}
	return m, nil
}

// SetRequire adds a requirement or replaces its version without duplicating it.
func (m *Manifest) SetRequire(modulePath, version string) {
	line := "require " + modulePath + " " + version
	for i := range m.Requires {
		req := &m.Requires[i]
		if req.Path == modulePath {
			if _, comment, ok := strings.Cut(m.lines[req.line], "//"); ok {
				line += " //" + comment
			}
			m.lines[req.line] = line
			req.Version = version
			return
		}
	}
	m.Requires = append(m.Requires, Requirement{Path: modulePath, Version: version, line: len(m.lines)})
	m.lines = append(m.lines, line)
}

// Bytes returns the manifest with a terminating newline.
func (m *Manifest) Bytes() []byte { return []byte(strings.Join(m.lines, "\n") + "\n") }

// FindRoot finds the nearest go4t.mod, or returns an empty root and nil manifest.
func FindRoot(dir string) (string, *Manifest, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	for {
		m, err := ReadManifest(dir)
		if err == nil {
			return dir, m, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil, nil
		}
		dir = parent
	}
}

// GoModule is the selected module in the project's Go module graph. Dir already
// accounts for replace directives; Version is the selected, not requested, version.
type GoModule struct {
	Path, Version, Dir string
}

// ResolveGoModule queries Go in the project root, preserving its replacement graph.
func ResolveGoModule(root, modulePath string) (GoModule, error) {
	command := exec.Command("go", "list", "-m", "-json", modulePath)
	command.Dir = root
	var stderr strings.Builder
	command.Stderr = &stderr
	data, err := command.Output()
	if err != nil {
		return GoModule{}, fmt.Errorf("go list -m -json %s: %w: %s", modulePath, err, strings.TrimSpace(stderr.String()))
	}
	var module GoModule
	if err := json.Unmarshal(data, &module); err != nil {
		return GoModule{}, fmt.Errorf("go list -m -json %s: %w", modulePath, err)
	}
	if module.Dir == "" {
		return GoModule{}, fmt.Errorf("go list -m -json %s: no module directory", modulePath)
	}
	return module, nil
}
