package docs_test

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/std"
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
	err = fs.WalkDir(std.FS, ".", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(filename, ".gfx") {
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
					if line == "}" {
						break
					}
					fields = append(fields, strings.TrimSuffix(line, ","))
				}
				header += strings.Join(fields, ", ") + "}"
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
	if len(modulesSeen) == 0 || len(counts) == 0 {
		t.Fatal("empty standard library inventory")
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	t.Logf("inventory: %d modules, %d declarations (%v), %d constructors, %d class methods, %d derived instances", len(modulesSeen), total, counts, constructors, methods, derived)
}
