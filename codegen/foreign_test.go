package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestForeignRecordRegistrationKeys(t *testing.T) {
	const target = "go4t/extern/testdata/natural"
	dir := t.TempDir()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module recordkeys\n\ngo 1.23\n\nrequire go4t v0.0.0\nreplace go4t => "+filepath.ToSlash(root)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, module, source, metadata, want string
	}{
		{"legacy", "", "Status", "", target + ".Status"},
		{"module", "go:example.test/owner", "Status", "", "example.test/owner.Status"},
		{"metadata", "go:example.test/owner", "OptionGo", "example.test/owner.Option", "example.test/owner.Option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := fixture()
			data := &types.DataType{Name: "OwnedRecord"}
			data.Ctors = []*types.Ctor{{Name: "OwnedRecord", Data: data, Fields: []types.Type{types.Int}, FieldNames: []string{"count"}}}
			p.DataTypes[data.Name] = data
			p.ModuleNames = map[string]string{data.Name: tc.source}
			p.Owners = map[string]string{data.Name: "/owner"}
			p.Modules = []*ast.Module{{Path: "/owner", ImportPath: tc.module}}
			if tc.metadata != "" {
				p.ForeignRecordKeys = map[string]string{data.Name: tc.metadata}
			}
			extern(p, "echo", []string{"value"}, []types.Type{&types.TCon{Name: data.Name}}, target, "Echo")
			p.Funcs["echo"].Scheme.Type.Ret = &types.TCon{Name: data.Name}
			source, err := generate(p, "main")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(source), `rt.RegisterForeignRecord("`+tc.want+`",`) != 1 {
				t.Fatalf("expected one registration for %q:\n%s", tc.want, source)
			}
			path := filepath.Join(dir, tc.name)
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "main.go"), source, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
	command := exec.Command("go", "build", "./legacy", "./module", "./metadata")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("record key compilation: %v\n%s", err, output)
	}
}
