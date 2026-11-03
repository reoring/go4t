package main

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go4t/extern"
)

func TestGoImportBindingCache(t *testing.T) {
	const importPath = "go4t/extern/testdata/mirrorfixture"
	cache := t.TempDir()
	t.Setenv("GO4T_CACHE", cache)
	dir := toolsModule(t)
	for _, build := range []bool{false, true} {
		name := "check"
		if build {
			name = "build"
		}
		t.Run(name, func(t *testing.T) {
			entry := ""
			bind := func() {
				t.Helper()
				binder := newGoImportBinder(dir)
				if build {
					binder.outputDir = filepath.Join(dir, "out")
				}
				module, err := binder.Bind(importPath)
				if err != nil {
					t.Fatal(err)
				}
				if module.Name != "mirrorfixture" || len(module.Files) != 1 || module.Files[0].Name != "go:"+importPath+"/bindings.gt" {
					t.Fatalf("unexpected binding module: %#v", module)
				}
				if module.ForeignRecordKeys["Point"] != importPath+".Point" {
					t.Fatalf("binding cache lost record ownership: %v", module.ForeignRecordKeys)
				}
				entry = binder.cacheEntries[importPath]
			}
			bind()
			if entry == "" {
				t.Fatal("binding cache entry was not recorded")
			}
			bindingFile := filepath.Join(entry, "bindings.gt")
			shimFile := filepath.Join(entry, "shim.go")
			source, err := os.ReadFile(bindingFile)
			if err != nil {
				t.Fatal(err)
			}
			shim, err := os.ReadFile(shimFile)
			if err != nil || len(shim) == 0 {
				t.Fatalf("cached shim: %v", err)
			}
			// A fixed old timestamp avoids clock-resolution races and sleeps.
			stamp := time.Unix(1234567890, 0)
			for _, file := range []string{bindingFile, shimFile, filepath.Join(entry, "records.json"), filepath.Join(entry, "classifier")} {
				if err := os.Chtimes(file, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			output := filepath.Join(dir, "out", "go4tbind", filepath.FromSlash(importPath), "go4t_shim_mirrorfixture.go")
			if build {
				if err := os.Remove(output); err != nil {
					t.Fatal(err)
				}
			}
			bind()
			for _, file := range []string{bindingFile, shimFile, filepath.Join(entry, "records.json"), filepath.Join(entry, "classifier")} {
				info, err := os.Stat(file)
				if err != nil || !info.ModTime().Equal(stamp) {
					t.Fatalf("cache hit rewrote %s: %v", file, err)
				}
			}
			if build {
				got, err := os.ReadFile(output)
				if err != nil || !bytes.Equal(got, shim) {
					t.Fatalf("cache hit did not restore exact shim bytes: %v", err)
				}
			}
			toolsWrite(t, bindingFile, "package !!!\n")
			bind()
			regenerated, err := os.ReadFile(bindingFile)
			if err != nil || !bytes.Equal(regenerated, source) {
				t.Fatalf("corrupt binding was not regenerated: %v", err)
			}
			if err := os.Remove(filepath.Join(entry, "records.json")); err != nil {
				t.Fatal(err)
			}
			bind()
			toolsWrite(t, filepath.Join(entry, "records.json"), "{")
			bind()
		})
	}
}

func TestGoImportBindingCacheOff(t *testing.T) {
	t.Setenv("GO4T_CACHE", "off")
	binder := newGoImportBinder(toolsModule(t))
	if _, err := binder.Bind("go4t/extern/testdata/mirrorfixture"); err != nil {
		t.Fatal(err)
	}
	entry, err := bindingCacheEntry(nil, "not-loaded", extern.BindOptions{}, "", "")
	if err != nil || entry != "" {
		t.Fatalf("disabled cache attempted a lookup: %q, %v", entry, err)
	}
}

func TestGoImportBindingCacheReusesDependencyExports(t *testing.T) {
	t.Setenv("GO4T_CACHE", t.TempDir())
	dir := toolsModule(t)
	cold := newGoImportBinder(dir)
	for _, path := range []string{"go4t/extern/testdata/mirrorfixture", "time"} {
		if _, err := cold.Bind(path); err != nil {
			t.Fatal(err)
		}
	}
	warm := newGoImportBinder(dir)
	if _, err := warm.Bind("go4t/extern/testdata/mirrorfixture"); err != nil {
		t.Fatal(err)
	}
	// The first go list already resolved time's export file. A transitive hit
	// must not invoke the Go tool again just to parse cached declarations.
	t.Setenv("PATH", t.TempDir())
	if _, err := warm.Bind("time"); err != nil {
		t.Fatalf("dependency cache hit tried to reload its Go package: %v", err)
	}
}

func TestBindingCacheKey(t *testing.T) {
	options := extern.BindOptions{Package: "fixture", Float: true, Prelude: map[string]bool{"show": true, "println": true}}
	exportHash := sha256.Sum256([]byte("export data"))
	key := bindingCacheKey("binary", "go-version", exportHash, "fixture", options, "shim/path", "shim")
	for _, name := range []string{"binary", "go-version", "export", "import", "prefix", "float", "prelude", "package", "shim-path", "shim-name"} {
		t.Run(name, func(t *testing.T) {
			binary, version, path, shimPath, shimName := "binary", "go-version", "fixture", "shim/path", "shim"
			hash, changed := exportHash, options
			switch name {
			case "binary":
				binary += "-new"
			case "go-version":
				version += "-new"
			case "export":
				hash = sha256.Sum256([]byte("new export data"))
			case "import":
				path += "/new"
			case "prefix":
				changed.Prefix = "native"
			case "float":
				changed.Float = false
			case "prelude":
				changed.Prelude = map[string]bool{"show": true}
			case "package":
				changed.Package += "_"
			case "shim-path":
				shimPath += "/new"
			case "shim-name":
				shimName += "_"
			}
			if got := bindingCacheKey(binary, version, hash, path, changed, shimPath, shimName); got == key {
				t.Fatalf("%s did not invalidate the cache", name)
			}
		})
	}
	options.Prelude = map[string]bool{"println": true, "absent": false, "show": true}
	if got := bindingCacheKey("binary", "go-version", exportHash, "fixture", options, "shim/path", "shim"); got != key {
		t.Fatal("prelude insertion order or absent names changed the set hash")
	}
}

func TestBindingExportHash(t *testing.T) {
	var loader extern.Loader
	if loader.ExportFile("missing") != "" {
		t.Fatal("unloaded package has export data")
	}
	if _, err := loader.ExportHash("missing"); err == nil || err.Error() != `no export data for "missing"` {
		t.Fatalf("missing export hash error: %v", err)
	}
	const path = "go4t/extern/testdata/mirrorfixture"
	if _, err := loader.Load(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(loader.ExportFile(path))
	if err != nil {
		t.Fatal(err)
	}
	got, err := loader.ExportHash(path)
	if err != nil || got != sha256.Sum256(data) {
		t.Fatalf("export hash did not match file content: %x, %v", got, err)
	}
}
