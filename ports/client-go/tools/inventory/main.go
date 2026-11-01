// Command inventory records every file and Go declaration in the pinned client-go
// checkout. Run from the repository root with explicit -upstream and -output.
// Only the root .git entry is excluded; build tags never filter source files.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const version = "v0.37.2"
const revision = "72c52dd0cc3b9099b36af11290b0304045dd1ada"
const module = "k8s.io/client-go"

type declaration struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Line int `json:"line"`
	Exported bool `json:"exported"`
	Signature string `json:"signature"`
	Receiver string `json:"receiver,omitempty"`
	// ConstOrdinal preserves iota semantics, including inherited specs.
	ConstOrdinal *int `json:"const_ordinal,omitempty"`
	Inherited bool `json:"inherited,omitempty"`
}

type imported struct {
	Path string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

type fileRecord struct {
	Path string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes int `json:"bytes"`
	LOC int `json:"loc"`
	Domain string `json:"domain"`
	Category string `json:"category"`
	Fixture bool `json:"fixture"`
	License bool `json:"license"`
	Package string `json:"package,omitempty"`
	ImportPath string `json:"import_path,omitempty"`
	Role string `json:"role,omitempty"`
	Generated bool `json:"generated"`
	GeneratedMarkers []string `json:"generated_markers,omitempty"`
	BuildConstraints []string `json:"build_constraints,omitempty"`
	FilenameConstraints []string `json:"filename_constraints,omitempty"`
	Imports []imported `json:"imports,omitempty"`
	Declarations []declaration `json:"declarations,omitempty"`
}

type totals struct {
	Files int `json:"files"`
	GoFiles int `json:"go_files"`
	NonGoFiles int `json:"non_go_files"`
	SourceFiles int `json:"source_files"`
	TestFiles int `json:"test_files"`
	GeneratedFiles int `json:"generated_files"`
	FixtureFiles int `json:"fixture_files"`
	LicenseFiles int `json:"license_files"`
	LOC int `json:"loc"`
	Declarations int `json:"declarations"`
	ExportedDeclarations int `json:"exported_declarations"`
	DeclarationKinds map[string]int `json:"declaration_kinds"`
}

type inventory struct {
	SchemaVersion int `json:"schema_version"`
	Module string `json:"module"`
	Version string `json:"version"`
	Revision string `json:"revision"`
	Policy map[string]string `json:"policy"`
	Totals totals `json:"totals"`
	ByDomain map[string]*totals `json:"by_domain"`
	ByPackage map[string]*totals `json:"by_package"`
	ByCategory map[string]*totals `json:"by_category"`
	Files []fileRecord `json:"files"`
}

func main() {
	upstream := flag.String("upstream", "", "pinned upstream checkout (required)")
	output := flag.String("output", "", "JSON output file (required, outside upstream)")
	flag.Parse()
	if err := run(*upstream, *output); err != nil {
		fmt.Fprintln(os.Stderr, "inventory:", err)
		os.Exit(1)
	}
}

func run(upstream, output string) error {
	if upstream == "" || output == "" || flag.NArg() != 0 {
		return fmt.Errorf("require -upstream and -output, with no positional arguments")
	}
	root, err := filepath.EvalSymlinks(upstream)
	if err != nil { return err }
	root, err = filepath.Abs(root)
	if err != nil { return err }
	out, err := filepath.Abs(output)
	if err != nil { return err }
	parent, err := filepath.EvalSymlinks(filepath.Dir(out))
	if err != nil { return err }
	out = filepath.Join(parent, filepath.Base(out))
	rel, err := filepath.Rel(root, out)
	if err != nil { return err }
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("output must be outside upstream")
	}
	if _, err := os.Lstat(out); err == nil {
		info, err := os.Lstat(out)
		if err != nil { return err }
		if !info.Mode().IsRegular() { return fmt.Errorf("output must be a regular file") }
	} else if !os.IsNotExist(err) { return err }
	if err := checkRevision(root); err != nil { return err }
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil { return err }
	if !strings.Contains("\n"+string(mod)+"\n", "\nmodule "+module+"\n") {
		return fmt.Errorf("upstream module is not %s", module)
	}
	inv := inventory{
		SchemaVersion: 1, Module: module, Version: version, Revision: revision,
		Policy: map[string]string{
			"scope": "every regular file; only root .git metadata excluded; symlinks and other special files are errors",
			"parsing": "all .go files, including tests, internal, generated, fixtures and every platform; parser.AllErrors; any error aborts without writing output",
			"loc": "physical lines: newline count plus one for a nonempty unterminated final line; all files",
			"domain": "first path component; root files use (root)",
			"package": "import path followed by a space and package name; external test packages remain distinct",
			"category": "disjoint: go-test, go-generated, go-source, license, fixture, non-go (in that priority order)",
			"signatures": "syntax, not type inference; complete type definitions, body-free function/method declarations, and value specs; inherited const expressions expanded with their iota ordinal",
			"exported": "ast.IsExported on declaration name, including test declarations and methods on unexported receivers; full type signatures retain fields, methods and embedded types",
			"fixture": "testdata path component or .golden filename suffix; overlaps category and role",
			"license": "basename LICENSE, LICENCE, COPYING, NOTICE or their dot-suffixed variants, case insensitive",
		},
		ByDomain: map[string]*totals{}, ByPackage: map[string]*totals{}, ByCategory: map[string]*totals{},
	}
	err = filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		rel, err := filepath.Rel(root, filename)
		if err != nil { return err }
		if rel == ".git" {
			if entry.IsDir() { return filepath.SkipDir }
			return nil
		}
		if entry.IsDir() { return nil }
		info, err := entry.Info()
		if err != nil { return err }
		if !info.Mode().IsRegular() { return fmt.Errorf("%s: non-regular file", rel) }
		data, err := os.ReadFile(filename)
		if err != nil { return err }
		record, err := inspect(filepath.ToSlash(rel), data)
		if err != nil { return fmt.Errorf("%s: %w", rel, err) }
		inv.Files = append(inv.Files, record)
		return nil
	})
	if err != nil { return err }
	sort.Slice(inv.Files, func(i, j int) bool { return inv.Files[i].Path < inv.Files[j].Path })
	for _, f := range inv.Files {
		inv.Totals.add(f)
		addGroup(inv.ByDomain, f.Domain, f)
		addGroup(inv.ByCategory, f.Category, f)
		if f.Package != "" { addGroup(inv.ByPackage, f.ImportPath+" "+f.Package, f) }
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil { return err }
	data = append(data, '\n')
	if err := os.WriteFile(out, data, 0644); err != nil { return err }
	fmt.Printf("inventory %s %s: files=%d go=%d non-go=%d packages=%d declarations=%d exported=%d generated=%d tests=%d fixtures=%d licenses=%d\n", version, revision, inv.Totals.Files, inv.Totals.GoFiles, inv.Totals.NonGoFiles, len(inv.ByPackage), inv.Totals.Declarations, inv.Totals.ExportedDeclarations, inv.Totals.GeneratedFiles, inv.Totals.TestFiles, inv.Totals.FixtureFiles, inv.Totals.LicenseFiles)
	return nil
}

// Read Git metadata directly so regeneration needs only the Go standard library.
func checkRevision(root string) error {
	gitdir := filepath.Join(root, ".git")
	info, err := os.Stat(gitdir)
	if err != nil { return err }
	if !info.IsDir() {
		data, err := os.ReadFile(gitdir)
		if err != nil { return err }
		dir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
		if !ok { return fmt.Errorf("invalid .git file") }
		if !filepath.IsAbs(dir) { dir = filepath.Join(root, dir) }
		gitdir = dir
	}
	data, err := os.ReadFile(filepath.Join(gitdir, "HEAD"))
	if err != nil { return err }
	head := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(head, "ref: "); ok {
		common := gitdir
		if data, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
			common = strings.TrimSpace(string(data))
			if !filepath.IsAbs(common) { common = filepath.Join(gitdir, common) }
		} else if !os.IsNotExist(err) { return err }
		data, err := os.ReadFile(filepath.Join(common, filepath.FromSlash(ref)))
		if err == nil {
			head = strings.TrimSpace(string(data))
		} else if os.IsNotExist(err) {
			packed, err := os.ReadFile(filepath.Join(common, "packed-refs"))
			if err != nil { return err }
			head = ""
			for _, line := range strings.Split(string(packed), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[1] == ref { head = fields[0]; break }
			}
		} else { return err }
	}
	if head != revision { return fmt.Errorf("upstream revision %q, want %s", head, revision) }
	return nil
}

func inspect(filename string, data []byte) (fileRecord, error) {
	sum := sha256.Sum256(data)
	f := fileRecord{Path: filename, SHA256: hex.EncodeToString(sum[:]), Bytes: len(data), LOC: bytes.Count(data, []byte{'\n'}), Domain: "(root)", Category: "non-go"}
	if len(data) > 0 && data[len(data)-1] != '\n' { f.LOC++ }
	if first, _, ok := strings.Cut(filename, "/"); ok { f.Domain = first }
	f.Fixture = strings.Contains("/"+filename+"/", "/testdata/") || strings.HasSuffix(filename, ".golden")
	base := strings.ToUpper(path.Base(filename))
	for _, name := range []string{"LICENSE", "LICENCE", "COPYING", "NOTICE"} {
		if base == name || strings.HasPrefix(base, name+".") { f.License = true }
	}
	if f.Fixture { f.Category = "fixture" }
	if f.License { f.Category = "license" }
	if !strings.HasSuffix(filename, ".go") { return f, nil }
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, filename, data, parser.ParseComments|parser.AllErrors|parser.SkipObjectResolution)
	if err != nil { return f, err }
	f.Package = tree.Name.Name
	f.ImportPath = module
	if dir := path.Dir(filename); dir != "." { f.ImportPath += "/"+dir }
	f.Role, f.Category = "source", "go-source"
	f.Generated = ast.IsGenerated(tree)
	if f.Generated { f.Category = "go-generated" }
	if strings.HasSuffix(filename, "_test.go") { f.Role, f.Category = "test", "go-test" }
	for _, group := range tree.Comments {
		if group.Pos() > tree.Package { break }
		for _, comment := range group.List {
			text := comment.Text
			if strings.HasPrefix(text, "// Code generated ") && strings.HasSuffix(text, " DO NOT EDIT.") { f.GeneratedMarkers = append(f.GeneratedMarkers, text) }
			if constraint.IsGoBuild(text) || constraint.IsPlusBuild(text) {
				if _, err := constraint.Parse(text); err != nil { return f, err }
				f.BuildConstraints = append(f.BuildConstraints, text)
			}
		}
	}
	f.FilenameConstraints = filenameConstraints(filename)
	for _, spec := range tree.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil { return f, err }
		imp := imported{Path: value}
		if spec.Name != nil { imp.Alias = spec.Name.Name }
		f.Imports = append(f.Imports, imp)
	}
	for _, node := range tree.Decls {
		switch d := node.(type) {
		case *ast.FuncDecl:
			copy := *d
			copy.Doc, copy.Body = nil, nil
			sig, err := syntax(fset, &copy)
			if err != nil { return f, err }
			dcl := declaration{Kind: "func", Name: d.Name.Name, Line: fset.Position(d.Pos()).Line, Exported: ast.IsExported(d.Name.Name), Signature: sig}
			if d.Recv != nil {
				dcl.Kind = "method"
				dcl.Receiver, err = syntax(fset, d.Recv.List[0].Type)
				if err != nil { return f, err }
			}
			f.Declarations = append(f.Declarations, dcl)
		case *ast.GenDecl:
			var previous *ast.ValueSpec
			for ordinal, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					copy := *s
					copy.Doc, copy.Comment = nil, nil
					sig, err := syntax(fset, &copy)
					if err != nil { return f, err }
					f.Declarations = append(f.Declarations, declaration{Kind: "type", Name: s.Name.Name, Line: fset.Position(s.Pos()).Line, Exported: ast.IsExported(s.Name.Name), Signature: "type "+sig})
				case *ast.ValueSpec:
					copy := *s
					copy.Doc, copy.Comment = nil, nil
					inherited := d.Tok == token.CONST && len(s.Values) == 0
					if inherited {
						if previous == nil { return f, fmt.Errorf("const spec without preceding expression") }
						copy.Type, copy.Values = previous.Type, previous.Values
					} else { previous = s }
					sig, err := syntax(fset, &copy)
					if err != nil { return f, err }
					for _, name := range s.Names {
						dcl := declaration{Kind: d.Tok.String(), Name: name.Name, Line: fset.Position(name.Pos()).Line, Exported: ast.IsExported(name.Name), Signature: d.Tok.String()+" "+sig, Inherited: inherited}
						if d.Tok == token.CONST { n := ordinal; dcl.ConstOrdinal = &n }
						f.Declarations = append(f.Declarations, dcl)
					}
				}
			}
		}
	}
	return f, nil
}

func syntax(fset *token.FileSet, node any) (string, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, node); err != nil { return "", err }
	return buf.String(), nil
}

func filenameConstraints(filename string) []string {
	// These are Go's recognized filename suffixes, independent of the host platform.
	oses := " aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos "
	arches := " 386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm "
	stem := strings.TrimSuffix(strings.TrimSuffix(path.Base(filename), ".go"), "_test")
	parts := strings.Split(stem, "_")
	if len(parts) < 2 { return nil }
	last := parts[len(parts)-1]
	if strings.Contains(arches, " "+last+" ") {
		if len(parts) > 2 && strings.Contains(oses, " "+parts[len(parts)-2]+" ") { return parts[len(parts)-2:] }
		return []string{last}
	}
	if strings.Contains(oses, " "+last+" ") { return []string{last} }
	return nil
}

func addGroup(groups map[string]*totals, key string, f fileRecord) {
	t := groups[key]
	if t == nil { t = &totals{}; groups[key] = t }
	t.add(f)
}

func (t *totals) add(f fileRecord) {
	t.Files++
	t.LOC += f.LOC
	if f.Package == "" { t.NonGoFiles++ } else { t.GoFiles++ }
	if f.Role == "source" { t.SourceFiles++ }
	if f.Role == "test" { t.TestFiles++ }
	if f.Generated { t.GeneratedFiles++ }
	if f.Fixture { t.FixtureFiles++ }
	if f.License { t.LicenseFiles++ }
	if t.DeclarationKinds == nil { t.DeclarationKinds = map[string]int{} }
	for _, d := range f.Declarations {
		t.Declarations++
		t.DeclarationKinds[d.Kind]++
		if d.Exported { t.ExportedDeclarations++ }
	}
}
