package runtimefs

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddedRuntimeMatchesCheckout(t *testing.T) {
	files := func(tree fs.FS) []string {
		t.Helper()
		var names []string
		err := fs.WalkDir(tree, "rt", func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".go") {
				names = append(names, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return names
	}
	disk := os.DirFS(".")
	want, got := files(disk), files(sources)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("embedded Go files = %v, checkout = %v", got, want)
	}
	for _, path := range append(want, "go.mod") {
		embedded, err := sources.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		original, err := fs.ReadFile(disk, path)
		if err != nil || !bytes.Equal(embedded, original) {
			t.Errorf("embedded %s differs from checkout: %v", path, err)
		}
	}
}

func TestExtractedRuntimeIsSelfContained(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir); err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil || string(mod) != "module gofx\n\ngo "+GoVersion()+"\n" {
		t.Fatalf("extracted module = %q, %v", mod, err)
	}
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./rt/...")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("list extracted runtime: %v\n%s", err, out)
	}
	for _, path := range strings.Fields(string(out)) {
		if path != "gofx/rt" && !strings.HasPrefix(path, "gofx/rt/") {
			t.Errorf("runtime depends on nonstandard package %q", path)
		}
	}
}
