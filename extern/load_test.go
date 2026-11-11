package extern

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportHashReusesImmutableFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "export-a")
	if err := os.WriteFile(file, []byte("export a"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := Loader{exports: map[string]string{"p": file, "alias": file}}
	want := sha256.Sum256([]byte("export a"))
	if got, err := loader.ExportHash("p"); err != nil || got != want {
		t.Fatalf("initial export hash = %x, %v", got, err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if got, err := loader.ExportHash("alias"); err != nil || got != want {
		t.Fatalf("shared immutable file was re-read: %x, %v", got, err)
	}
	newFile := filepath.Join(dir, "export-b")
	if err := os.WriteFile(newFile, []byte("export b"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader.exports["p"] = newFile
	if got, err := loader.ExportHash("p"); err != nil || got != sha256.Sum256([]byte("export b")) {
		t.Fatalf("replacement export file retained old hash: %x, %v", got, err)
	}
}

func TestLoaderPreload(t *testing.T) {
	loader := Loader{}
	paths := []string{"encoding/hex", "net/url", "encoding/hex"}
	if err := loader.Preload(paths); err != nil {
		t.Fatal(err)
	}
	// No package may invoke Go again after the batch has resolved its exports,
	// including the JSON interfaces and dependencies required by Load.
	t.Setenv("PATH", t.TempDir())
	for _, path := range []string{"encoding/hex", "net/url", "encoding/json", "strings"} {
		pkg, err := loader.Load(path)
		if err != nil {
			t.Fatalf("load preloaded %s: %v", path, err)
		}
		if pkg.Path != path || len(pkg.Functions) == 0 {
			t.Fatalf("incomplete package %s: %#v", path, pkg)
		}
	}
	if err := loader.Preload(paths); err != nil {
		t.Fatalf("preloading known packages invoked Go again: %v", err)
	}
}

func TestLoaderPreloadEmptyAndInvalid(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	loader := Loader{}
	if err := loader.Preload(nil); err != nil {
		t.Fatalf("empty batch invoked Go: %v", err)
	}
	for _, path := range []string{"", "-e", "net/..."} {
		err := loader.Preload([]string{path})
		if err == nil || !strings.Contains(err.Error(), "invalid Go import path") {
			t.Errorf("Preload(%q) = %v", path, err)
		}
	}
}
