package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	runtimefs "gofx"
	"gofx/types"
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
	command := exec.Command("go", "list", "-m", "gofx")
	command.Dir = root
	if err := command.Run(); err != nil {
		return ""
	}
	return root
}

// prepareBuildModule retains a usable surrounding module; otherwise it supplies
// the runtime carried by this compiler without changing the user's directory.
func prepareBuildModule(root string, program *types.Program) (buildModule, error) {
	if root != "" {
		return buildModule{dir: root}, nil
	}
	// Foreign dependencies belong to an application module, not to an implicit
	// download or a copy of the caller's unrelated module. Pick errors stably.
	foreign := ""
	for _, function := range program.Funcs {
		if ref := function.Decl.Extern; ref != nil {
			path := ref.ImportPath
			if path != "gofx/rt" && !strings.HasPrefix(path, "gofx/rt/") && (foreign == "" || path < foreign) {
				foreign = path
			}
		}
	}
	if foreign != "" {
		return buildModule{}, fmt.Errorf("extern %q requires a Go module: run inside a directory with go.mod that provides it", foreign)
	}
	dir, err := os.MkdirTemp("", "gofx-module-")
	if err != nil {
		return buildModule{}, err
	}
	module := buildModule{dir: dir, temporary: true}
	if err := runtimefs.Write(filepath.Join(dir, "gofx")); err != nil {
		module.close()
		return buildModule{}, err
	}
	mod := fmt.Sprintf("module gofxrun\n\ngo %s\n\nrequire gofx v0.0.0\n\nreplace gofx => ./gofx\n", runtimefs.GoVersion())
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		module.close()
		return buildModule{}, err
	}
	return module, nil
}

func (module buildModule) close() {
	if module.temporary {
		os.RemoveAll(module.dir)
	}
}

func printBuildModuleNote(dir string) {
	fmt.Fprintf(os.Stderr, "gofx: generated Go sources in %s; to compile, configure a Go module with require gofx v0.0.0 and replace gofx => /absolute/path/to/gofx, then run go build in that directory\n", dir)
}
