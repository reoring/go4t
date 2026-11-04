package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	runtimefs "github.com/reoring/go4t"
	"github.com/reoring/go4t/types"
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
	command := exec.Command("go", "list", "-m", "github.com/reoring/go4t")
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
	mod := fmt.Sprintf("module go4trun\n\ngo %s\n\nrequire github.com/reoring/go4t v0.0.0\n\nreplace github.com/reoring/go4t => ./go4t\n", runtimefs.GoVersion())
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
			if path != "github.com/reoring/go4t/rt" && !strings.HasPrefix(path, "github.com/reoring/go4t/rt/") && (foreign == "" || path < foreign) {
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

// retainBuildSources keeps standalone build output beside the input while
// compiling it in the temporary runtime module. Go bindings are generated in
// that module so their import paths remain resolvable during compilation.
func retainBuildSources(stage, output string, sources map[string][]byte) error {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	if err := emitModules(output, sources); err != nil {
		return err
	}
	shims := filepath.Join(stage, "go4tbind")
	if _, err := os.Stat(shims); os.IsNotExist(err) {
		return nil
	}
	return copyBuildTree(shims, filepath.Join(output, "go4tbind"))
}

func copyBuildTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.Create(target)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		closeErr := output.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
