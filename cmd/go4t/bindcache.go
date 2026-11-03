package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"

	"go4t/ast"
	"go4t/extern"
	"go4t/parser"
)

// Development binaries have no module sum; fingerprint their executable once.
var bindingBinaryIdentity = sync.OnceValues(func() (string, error) {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" && info.Main.Sum != "" {
		return info.Main.Path + "@" + info.Main.Version + ":" + info.Main.Sum, nil
	}
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
})

// bindingCacheEntry returns an empty path when caching is disabled. GO4T_CACHE
// replaces the go4t cache root, not the bind subdirectory beneath it.
func bindingCacheEntry(loader *extern.Loader, importPath string, options extern.BindOptions, shimPath, shimName string) (string, error) {
	root := os.Getenv("GO4T_CACHE")
	if root == "off" {
		return "", nil
	}
	if root == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(cache, "go4t")
	}
	identity, err := bindingBinaryIdentity()
	if err != nil {
		return "", err
	}
	exportHash, err := loader.ExportHash(importPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "bind", bindingCacheKey(identity, runtime.Version(), exportHash, importPath, options, shimPath, shimName)), nil
}

func bindingCacheKey(identity, goVersion string, exportHash [sha256.Size]byte, importPath string, options extern.BindOptions, shimPath, shimName string) string {
	prelude := make([]string, 0, len(options.Prelude))
	for name, present := range options.Prelude {
		if present {
			prelude = append(prelude, name)
		}
	}
	sort.Strings(prelude)
	preludeHash := sha256.New()
	// JSON's string escaping and array framing keep adjacent names unambiguous.
	_ = json.NewEncoder(preludeHash).Encode(prelude)
	key := struct {
		Binary, GoVersion, ImportPath string
		Export                        [sha256.Size]byte
		Prefix, Package               string
		Float                         bool
		Prelude                       string
		ShimPath, ShimName            string
	}{identity, goVersion, importPath, exportHash, options.Prefix, options.Package, options.Float, hex.EncodeToString(preludeHash.Sum(nil)), shimPath, shimName}
	hash := sha256.New()
	_ = json.NewEncoder(hash).Encode(key)
	return hex.EncodeToString(hash.Sum(nil))
}

func readBindingCache(entry string) (*ast.File, []byte, error) {
	source, err := os.ReadFile(filepath.Join(entry, "bindings.gt"))
	if err != nil {
		return nil, nil, err
	}
	file, err := parser.Parse(string(source))
	if err != nil {
		return nil, nil, err
	}
	shim, err := os.ReadFile(filepath.Join(entry, "shim.go"))
	if err != nil {
		return nil, nil, err
	}
	return file, shim, nil
}

func writeBindingCache(entry, source string, shim []byte) error {
	if err := os.MkdirAll(entry, 0o755); err != nil {
		return err
	}
	if err := writeBindingCacheFile(filepath.Join(entry, "shim.go"), shim); err != nil {
		return err
	}
	return writeBindingCacheFile(filepath.Join(entry, "bindings.gt"), []byte(source))
}

// Classifier metadata shares the binding's content-addressed entry. Negative
// entries need no bindings or shim, and hits never load Go export data.
func readClassifierCache(entry string) (bool, error) {
	if entry == "" {
		return false, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(entry, "classifier"))
	if err != nil {
		return false, err
	}
	switch string(data) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid classifier cache metadata")
	}
}

func writeClassifierCache(entry string, present bool) error {
	if err := os.MkdirAll(entry, 0o755); err != nil {
		return err
	}
	data := "false"
	if present {
		data = "true"
	}
	return writeBindingCacheFile(filepath.Join(entry, "classifier"), []byte(data))
}

// Each key identifies exact bytes, so concurrent writers can safely replace
// either file. Publish bindings last; an incomplete entry is always a miss.
func writeBindingCacheFile(path string, content []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func traceBindingCache(importPath, status string, err error) {
	if os.Getenv("GO4T_TRACE") != "1" {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "go4t bind cache %s %s: %v\n", status, importPath, err)
	} else {
		fmt.Fprintf(os.Stderr, "go4t bind cache %s %s\n", status, importPath)
	}
}
