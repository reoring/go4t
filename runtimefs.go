// Package runtimefs carries the runtime sources needed by standalone go4t programs.
package runtimefs

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Embedding the directory includes every runtime subpackage, including future
// ones. Tests keep this file set and the module's Go version tied to the checkout.
//go:embed rt go.mod
var sources embed.FS

// GoVersion returns the toolchain language version required by the runtime.
func GoVersion() string {
	mod, _ := sources.ReadFile("go.mod")
	for _, line := range strings.Split(string(mod), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	panic("embedded go.mod has no go directive")
}

// Digest identifies the embedded file names and contents for persistent caches.
func Digest() (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(sources, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := sources.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%d:%s%d:", len(path), path, len(data))
		hash.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Write extracts a minimal go4t module into dir. The runtime depends only on
// Go's standard library and its own subpackages, so no dependency download is needed.
func Write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	mod := fmt.Sprintf("module github.com/reoring/go4t\n\ngo %s\n", GoVersion())
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		return err
	}
	return fs.WalkDir(sources, "rt", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := sources.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
