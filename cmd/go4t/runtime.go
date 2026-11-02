package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	runtimefs "go4t"
	"go4t/types"
)

type buildModule struct {
	dir       string
	temporary bool
}

func inputDirectory(inputs []string) (string, error) {
	base := "."
	if len(inputs) != 0 {
		base = inputs[0]
		if info, err := os.Stat(base); err != nil || !info.IsDir() {
			base = filepath.Dir(base)
		}
	}
	return filepath.Abs(base)
}

func buildModuleRoot(base string) string {
	root, err := moduleRoot(base)
	if err != nil {
		return ""
	}
	command := exec.Command("go", "list", "-m", "go4t")
	command.Dir = root
	if err := command.Run(); err != nil {
		return ""
	}
	return root
}

// prepareBuildModule retains a usable surrounding module; otherwise it supplies
// the runtime carried by this compiler without changing the user's directory.
func prepareBuildModule(root string) (buildModule, error) {
	if root != "" {
		return buildModule{dir: root}, nil
	}
	dir, err := os.MkdirTemp("", "go4t-module-")
	if err != nil {
		return buildModule{}, err
	}
	module := buildModule{dir: dir, temporary: true}
	if err := runtimefs.Write(filepath.Join(dir, "go4t")); err != nil {
		module.close()
		return buildModule{}, err
	}
	mod := fmt.Sprintf("module go4trun\n\ngo %s\n\nrequire go4t v0.0.0\n\nreplace go4t => ./go4t\n", runtimefs.GoVersion())
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		module.close()
		return buildModule{}, err
	}
	return module, nil
}

// checkForeign retains the explicit-extern contract outside application modules.
// go: bindings have already been resolved in the selected module by the adapter.
func (module buildModule) checkForeign(program *types.Program) error {
	if !module.temporary {
		return nil
	}
	foreign := ""
	for name, function := range program.Funcs {
		if strings.HasPrefix(program.Owners[name], "go:") {
			continue
		}
		if ref := function.Decl.Extern; ref != nil {
			path := ref.ImportPath
			if path != "go4t/rt" && !strings.HasPrefix(path, "go4t/rt/") && (foreign == "" || path < foreign) {
				foreign = path
			}
		}
	}
	if foreign != "" {
		return fmt.Errorf("extern %q requires a Go module: run inside a directory with go.mod that provides it", foreign)
	}
	return nil
}

func (module buildModule) close() {
	if module.temporary {
		os.RemoveAll(module.dir)
	}
}

func printBuildModuleNote(dir string) {
	fmt.Fprintf(os.Stderr, "go4t: generated Go sources in %s; to compile, configure a Go module with require go4t v0.0.0 and replace go4t => /absolute/path/to/go4t, then run go build in that directory\n", dir)
}
