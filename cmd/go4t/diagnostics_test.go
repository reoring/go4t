package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestCaretRendering(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		pos, end     ast.Pos
		want         string
	}{
		{"span", "let unused = 1", ast.Pos{Line: 1, Col: 5}, ast.Pos{Line: 1, Col: 11}, "\nlet unused = 1\n    ^~~~~~"},
		{"multiline", "match x {\n None => 0\n}", ast.Pos{Line: 1, Col: 1}, ast.Pos{Line: 3, Col: 2}, "\nmatch x {\n^~~~~~~~~"},
		{"clamp", "short", ast.Pos{Line: 1, Col: 2}, ast.Pos{Line: 1, Col: 100}, "\nshort\n ^~~~"},
		{"missing end", "name", ast.Pos{Line: 1, Col: 1}, ast.Pos{}, "\nname\n^"},
		{"unicode", "é hello", ast.Pos{Line: 1, Col: 3}, ast.Pos{Line: 1, Col: 8}, "\né hello\n  ^~~~~"},
		{"tab", "\tname", ast.Pos{Line: 1, Col: 2}, ast.Pos{Line: 1, Col: 6}, "\n\tname\n\t^~~~"},
		{"eof", "", ast.Pos{Line: 1, Col: 1}, ast.Pos{}, "\n\n^"},
		{"missing line", "line", ast.Pos{Line: 2, Col: 1}, ast.Pos{}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderExcerpt(tt.source, tt.pos, tt.end); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorsRenderSourceForEveryCommand(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.gt")
	source := "package main\nfunc main() IO[Unit] { println(1234) }\n"
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"check", "build", "run"} {
		err := run([]string{command, file})
		var checked *types.Error
		if !errors.As(err, &checked) {
			t.Fatalf("%s: %v", command, err)
		}
		want := file + ":2:32: type mismatch: expected String, got Int\nfunc main() IO[Unit] { println(1234) }\n                               ^~~~"
		if err.Error() != want {
			t.Errorf("%s: got %q, want %q", command, err, want)
		}
	}
}

func TestParseErrorRendersTokenSpan(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.gt")
	if err := os.WriteFile(file, []byte("package main\nfunc main() IO[Unit] { let x true }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"check", file})
	if err == nil || !strings.HasSuffix(err.Error(), "\n                             ^~~~") {
		t.Fatalf("diagnostic = %v", err)
	}
}
