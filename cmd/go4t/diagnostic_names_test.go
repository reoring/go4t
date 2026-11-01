package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go4t/types"
)

func TestNegativeFixtureDiagnosticNames(t *testing.T) {
	internal := regexp.MustCompile(`placeholder\d|\b[A-Z][a-z]+_[A-Z]\w*\b`)
	files, err := filepath.Glob("../../types/testdata/diagnostics/*.gt")
	if err != nil || len(files) == 0 {
		t.Fatalf("negative fixtures: %v (%d files)", err, len(files))
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			first, _, _ := strings.Cut(string(source), "\n")
			want, ok := strings.CutPrefix(first, "// want: ")
			if !ok || want == "" {
				t.Fatal("fixture must declare its expected diagnostic")
			}
			for _, args := range [][]string{{"check", file}, {"check", "--full-types", file}} {
				err := run(args)
				var diagnostic *types.Error
				if !errors.As(err, &diagnostic) {
					t.Fatalf("run(%q) = %v; want type diagnostic", args, err)
				}
				if internal.MatchString(err.Error()) {
					t.Errorf("internal name in CLI diagnostic: %v", err)
				}
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CLI diagnostic = %v, want %q", err, want)
				}
			}
		})
	}
}
