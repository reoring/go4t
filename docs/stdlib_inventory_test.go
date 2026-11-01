package docs_test

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/parser"
	"go4t/std"
)

func moduleDocumentation(document, module string) (string, bool) {
	_, section, ok := strings.Cut(document, "## `std/"+module+"`\n")
	if !ok {
		return "", false
	}
	section, _, _ = strings.Cut(section, "\n## ")
	return section, true
}

// Source signatures come from the parser's declaration positions rather than
// a maintained required-name list. Qualified module sections prevent a same-name
// declaration elsewhere in the library from accidentally satisfying coverage.
func TestStdlibInventory(t *testing.T) {
	document, err := os.ReadFile("stdlib.md")
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	modulesSeen := make(map[string]bool)
	constructors, methods, derived := 0, 0, 0
	typeNames := make(map[string]bool)
	debugInstances := make(map[string]bool)
	err = fs.WalkDir(std.FS, ".", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(filename, ".gt") {
			return nil
		}
		source, err := std.FS.ReadFile(filename)
		if err != nil {
			return err
		}
		file, err := parser.Parse(string(source))
		if err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		module := path.Dir(filename)
		modulesSeen[module] = true
		section, ok := moduleDocumentation(string(document), module)
		if !ok {
			t.Errorf("%s: missing stdlib module section", filename)
			return nil
		}
		lines := strings.Split(string(source), "\n")
		for _, declaration := range file.Decls {
			header := strings.TrimSpace(lines[declaration.GetPos().Line-1])
			if strings.HasSuffix(header, "{") && !strings.HasSuffix(header, " {") {
				// Formatted multi-line record: join field lines until the closing brace.
				fields := []string{}
				for i := declaration.GetPos().Line; i < len(lines); i++ {
					line := strings.TrimSpace(lines[i])
					if strings.HasPrefix(line, "}") {
						header += strings.Join(fields, ", ") + line
						fields = nil
						break
					}
					fields = append(fields, strings.TrimSuffix(line, ","))
				}
				if fields != nil {
					header += strings.Join(fields, ", ") + "}"
				}
			}
			header = strings.TrimSuffix(header, " {")
			// Markdown escapes sum-constructor pipes, but signatures must
			// otherwise match source (including extern targets and context).
			header = strings.ReplaceAll(header, "|", "\\|")
			row := ""
			for _, candidate := range strings.Split(section, "\n") {
				if strings.HasPrefix(candidate, "| `"+header+"` | ") || strings.HasPrefix(candidate, "| `"+header+" { ") {
					row = candidate
					break
				}
			}
			if row == "" {
				t.Errorf("%s:%d: missing/stale source signature row: %s", filename, declaration.GetPos().Line, header)
				continue
			}
			_, descriptions, ok := strings.Cut(row, "` | ")
			semantics, boundary, columns := strings.Cut(descriptions, " | ")
			if !ok || !columns || strings.TrimSpace(semantics) == "" || strings.TrimSpace(strings.TrimSuffix(boundary, " |")) == "" {
				t.Errorf("%s: signature needs semantics and boundary: %s", filename, row)
			}
			containsName := func(name string) {
				t.Helper()
				if !regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(row) {
					t.Errorf("%s: declaration row omits %s", filename, name)
				}
			}
			switch decl := declaration.(type) {
			case *ast.FuncDecl:
				counts["functions/externs"]++
				containsName(decl.Name)
			case *ast.ClassDecl:
				counts["classes"]++
				containsName(decl.Name)
				for _, method := range decl.Methods {
					methods++
					containsName(method.Name)
					signature := strings.TrimSpace(lines[method.Pos.Line-1])
					if !strings.Contains(row, signature) {
						t.Errorf("%s: missing/stale class method signature %s", filename, signature)
					}
				}
			case *ast.TypeDecl:
				counts["types"]++
				key := module + "." + decl.Name
				typeNames[key] = true
				for _, class := range decl.Deriving {
					if class == "Debug" {
						debugInstances[key] = true
					}
				}
				containsName(decl.Name)
				for _, constructor := range decl.Ctors {
					constructors++
					containsName(constructor.Name)
				}
				for _, class := range decl.Deriving {
					derived++
					head := decl.Name
					var context []string
					if len(decl.TParams) != 0 {
						head += "[" + strings.Join(decl.TParams, ", ") + "]"
						for _, parameter := range decl.TParams {
							context = append(context, class+"["+parameter+"]")
						}
					}
					signature := "instance " + class + "[" + head + "]"
					if len(context) != 0 {
						signature += " given " + strings.Join(context, ", ")
					}
					if !strings.Contains(section, "| `"+signature+"` | ") {
						t.Errorf("%s: missing derived instance %s", filename, signature)
					}
				}
			case *ast.InstanceDecl:
				counts["instances"]++
				if decl.Class == "Debug" {
					debugInstances[module+"."+decl.TypeCon] = true
				}
				containsName(decl.Class)
				containsName(decl.TypeCon)
			case *ast.AliasDecl:
				counts["aliases"]++
				containsName(decl.Name)
			default:
				return fmt.Errorf("%s: unhandled top-level declaration %T", filename, decl)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name := range typeNames {
		if !debugInstances[name] {
			t.Errorf("%s: missing derived or explicit Debug instance", name)
		}
	}
	if len(modulesSeen) == 0 || len(counts) == 0 {
		t.Fatal("empty standard library inventory")
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	t.Logf("inventory: %d modules, %d declarations (%v), %d constructors, %d class methods, %d derived instances", len(modulesSeen), total, counts, constructors, methods, derived)
}
