package extern

import (
	"crypto/sha256"
	"os"
	"path/filepath"
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
