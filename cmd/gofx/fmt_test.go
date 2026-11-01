package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFmtCommand(t *testing.T) {
	dir := t.TempDir()
	original := "package main;func main() IO[Unit]{println(\"hi\")}\n"
	canonical := "package main\n\nfunc main() IO[Unit] {\n  println(\"hi\")\n}\n"
	file := toolsWrite(t, filepath.Join(dir, "main.gfx"), original)
	for _, tc := range []struct {
		args   []string
		out    string
		status int
	}{
		{[]string{"fmt", file}, canonical, 0},
		{[]string{"fmt", "-l", dir}, file + "\n", 1},
		{[]string{"fmt", "-d", file}, "--- " + file + "\n+++ " + file + "\n@@ -1,1 +1,5 @@\n-" + original + "+package main\n+\n+func main() IO[Unit] {\n+  println(\"hi\")\n+}\n", 0},
		{[]string{"fmt", "-w", file}, "", 0},
		{[]string{"fmt", "-l", dir}, "", 0},
		{[]string{"fmt", "-d", file}, "", 0},
	} {
		out, stderr, status := toolsCommand(t, dir, "", tc.args...)
		if out != tc.out || stderr != "" || status != tc.status {
			t.Fatalf("%v: stdout=%q stderr=%q status=%d; want %q %d", tc.args, out, stderr, status, tc.out, tc.status)
		}
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != canonical {
		t.Fatalf("written %q", got)
	}
}

func TestFmtErrors(t *testing.T) {
	dir := t.TempDir()
	bad := toolsWrite(t, filepath.Join(dir, "bad.gfx"), "package main\nfunc")
	for _, args := range [][]string{{"fmt"}, {"fmt", "-x", bad}, {"fmt", "-w", "-l", bad}, {"fmt", "-d", "-w", bad}, {"fmt", filepath.Join(dir, "missing.gfx")}, {"fmt", "-w", bad}} {
		out, stderr, status := toolsCommand(t, dir, "", args...)
		if out != "" || status != 1 || !strings.Contains(stderr, ":") {
			t.Fatalf("%v: out=%q err=%q status=%d", args, out, stderr, status)
		}
	}
	got, err := os.ReadFile(bad)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package main\nfunc" {
		t.Fatal("invalid source overwritten")
	}
}

func TestFmtDirectoryAndPermissions(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	a := toolsWrite(t, filepath.Join(dir, "a.gfx"), "package main")
	b := toolsWrite(t, filepath.Join(nested, "b.gfx"), "package main")
	toolsWrite(t, filepath.Join(dir, "ignore.txt"), "not gofx")
	if err := os.Chmod(a, 0o640); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := runFmt([]string{"-l", dir, a}, &out)
	var status exitStatus
	if !errors.As(err, &status) || status != 1 || out.String() != a+"\n"+b+"\n" {
		t.Fatalf("list=%q error=%v", out.String(), err)
	}
	out.Reset()
	if err := runFmt([]string{"-w", dir}, &out); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions=%v", info.Mode())
	}
	if err := runFmt([]string{"-l", dir}, &out); err != nil || out.Len() != 0 {
		t.Fatalf("after write: %q %v", out.String(), err)
	}
}

func TestFmtDiffMissingNewline(t *testing.T) {
	var out bytes.Buffer
	if err := formatDiff(&out, "x.gfx", []byte("package x"), []byte("package x\n")); err != nil {
		t.Fatal(err)
	}
	want := "--- x.gfx\n+++ x.gfx\n@@ -1,1 +1,1 @@\n-package x\n\\ No newline at end of file\n+package x\n"
	if out.String() != want {
		t.Fatalf("diff=%q", out.String())
	}
}
