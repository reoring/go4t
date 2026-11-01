package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	runtimefs "go4t"
	"go4t/modules"
)

func runInit(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: go4t init <module-path>")
	}
	if !modules.ValidModulePath(args[0]) {
		return fmt.Errorf("go4t init: invalid module path %q", args[0])
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	for _, name := range []string{"go.mod", "go4t.mod"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return fmt.Errorf("go4t init: %s already exists", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	runtimeDir, err := initRuntime(root)
	if err != nil {
		return fmt.Errorf("go4t init: %w", err)
	}
	goMod := fmt.Sprintf("module %s\n\ngo 1.22\n\nrequire go4t v0.0.0\n\nreplace go4t => %s\n", args[0], strconv.Quote(runtimeDir))
	if err := createManifest(filepath.Join(root, "go.mod"), goMod); err != nil {
		return err
	}
	if err := createManifest(filepath.Join(root, "go4t.mod"), "module "+args[0]+"\n"); err != nil {
		os.Remove(filepath.Join(root, "go.mod"))
		return err
	}
	return nil
}

func createManifest(name, content string) error {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(name)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(name)
	}
	return closeErr
}

func initRuntime(root string) (string, error) {
	if module, err := modules.ResolveGoModule(root, "go4t"); err == nil {
		return module.Dir, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	cache, err = filepath.Abs(cache)
	if err != nil {
		return "", err
	}
	digest, err := runtimefs.Digest()
	if err != nil {
		return "", err
	}
	parent := filepath.Join(cache, "go4t", "runtime")
	dir := filepath.Join(parent, digest)
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return dir, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	temporary, err := os.MkdirTemp(parent, ".extract-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	if err := runtimefs.Write(temporary); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, dir); err != nil {
		// Another init may have published the same complete runtime concurrently.
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr != nil {
			return "", err
		}
	}
	return dir, nil
}

const getUsage = "usage: go4t get [--no-bind] <path>@<version>"

func runGet(args []string) error {
	var target string
	noBind := false
	for _, arg := range args {
		if arg == "--no-bind" && !noBind {
			noBind = true
		} else if target == "" && !strings.HasPrefix(arg, "-") {
			target = arg
		} else {
			return fmt.Errorf("%s", getUsage)
		}
	}
	modulePath, version, ok := strings.Cut(target, "@")
	if !ok || !modules.ValidModulePath(modulePath) || version == "" || version == "none" || strings.ContainsAny(version, "@ \t\r\n") {
		return fmt.Errorf("%s", getUsage)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, manifest, err := modules.FindRoot(cwd)
	if err != nil {
		return err
	}
	if root == "" {
		return fmt.Errorf("go4t get: no go4t.mod found above current directory; run go4t init <module-path>")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); os.IsNotExist(err) {
		return fmt.Errorf("go4t get: no go.mod in project root %s", root)
	} else if err != nil {
		return err
	}
	command := exec.Command("go", "get", target)
	command.Dir = root
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("go4t get: go get %s: %w", target, err)
	}
	selected, err := modules.ResolveGoModule(root, modulePath)
	if err != nil {
		return err
	}
	if selected.Version == "" {
		return fmt.Errorf("go4t get: module %s has no selected version", modulePath)
	}
	manifest.SetRequire(modulePath, selected.Version)
	if err := os.WriteFile(filepath.Join(root, "go4t.mod"), manifest.Bytes(), 0o644); err != nil {
		return err
	}
	if noBind {
		return nil
	}
	return bindModule(root, selected)
}
