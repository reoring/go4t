package docs_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/types"
)

// go4tBinary builds the CLI once per test run. Documentation programs that
// import "go:…" need the Go binder, which lives in the CLI, not in a library
// package; checking them through the real binary is also the honest harness.
var go4tBinary = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "go4t-docs-cli-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "go4t")
	build := exec.Command("go", "build", "-o", path, "github.com/reoring/go4t/cmd/go4t")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build go4t: %v\n%s", err, out)
	}
	return path, nil
})

func usesGoImports(files map[string]string) bool {
	for _, text := range files {
		if strings.Contains(text, `"go:`) {
			return true
		}
	}
	return false
}

// checkWithCLI runs `go4t check` inside a throwaway Go module that points at
// this repository, exactly as a user project would.
func checkWithCLI(t *testing.T, directory string) error {
	t.Helper()
	binary, err := go4tBinary()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	module := "module docsexample\n\ngo 1.22\n\nrequire github.com/reoring/go4t v0.0.0\nreplace github.com/reoring/go4t => " + strconv.Quote(root) + "\n"
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "check", ".")
	command.Dir = directory
	out, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, out)
	}
	return nil
}

type docBlock struct {
	line int
	fail bool
	text string
}

type docExample struct {
	name   string
	line   int
	fail   bool
	blocks int
	files  map[string]string
}

// fencedBlocks tracks other fence languages too, so a go4t-looking line inside
// an unrelated code block is not accidentally treated as an executable example.
func fencedBlocks(source string) ([]docBlock, error) {
	var blocks []docBlock
	var body []string
	open, language, start := false, "", 0
	for i, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if !open && strings.HasPrefix(trimmed, "```") {
			open, language, start = true, strings.TrimSpace(strings.TrimPrefix(trimmed, "```")), i+1
			body = nil
			if strings.HasPrefix(language, "go4t") && language != "go4t" && language != "go4t error" {
				return nil, fmt.Errorf("line %d: unsupported go4t fence %q", start, language)
			}
			continue
		}
		if open && trimmed == "```" {
			if language == "go4t" || language == "go4t error" {
				blocks = append(blocks, docBlock{start, language == "go4t error", strings.Join(body, "\n") + "\n"})
			}
			open = false
			continue
		}
		if open {
			body = append(body, line)
		}
	}
	if open {
		return nil, fmt.Errorf("line %d: unclosed %s fence", start, language)
	}
	return blocks, nil
}

func localDocPath(name string) bool {
	return name != "." && filepath.IsLocal(name) && !strings.Contains(name, "\\") && filepath.ToSlash(filepath.Clean(name)) == name
}

// A dir directive groups fences in one document into a complete example root.
// File directives split a block into files relative to that root. Without dir,
// every fence is independent; without file, it supplies main.gt. No implicit
// main, package, prelude copy, or import is added to make examples pass.
func documentationExamples(blocks []docBlock) ([]*docExample, error) {
	var examples []*docExample
	groups := make(map[string]*docExample)
	for i, block := range blocks {
		group, filename := "", "main.gt"
		files := make(map[string]string)
		var body []string
		explicitFile, contentStarted := false, false
		flush := func() error {
			content := strings.Join(body, "\n")
			if !explicitFile && strings.TrimSpace(content) == "" {
				return nil
			}
			if !localDocPath(filename) {
				return fmt.Errorf("line %d: invalid file path %q", block.line, filename)
			}
			if _, exists := files[filename]; exists {
				return fmt.Errorf("line %d: repeated file %s", block.line, filename)
			}
			files[filename] = content + "\n"
			return nil
		}
		for _, line := range strings.Split(strings.TrimSuffix(block.text, "\n"), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "// dir:") {
				if group != "" || contentStarted || explicitFile {
					return nil, fmt.Errorf("line %d: dir must precede file directives and source", block.line)
				}
				group = strings.TrimSpace(strings.TrimPrefix(trimmed, "// dir:"))
				if !localDocPath(group) {
					return nil, fmt.Errorf("line %d: invalid dir %q", block.line, group)
				}
				continue
			}
			if strings.HasPrefix(trimmed, "// file:") {
				if err := flush(); err != nil {
					return nil, err
				}
				filename = strings.TrimSpace(strings.TrimPrefix(trimmed, "// file:"))
				explicitFile, body = true, nil
				continue
			}
			body = append(body, line)
			contentStarted = contentStarted || trimmed != ""
		}
		if err := flush(); err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("line %d: empty go4t fence", block.line)
		}
		example := groups[group]
		if group == "" || example == nil {
			name := group
			if name == "" {
				name = fmt.Sprintf("block_%02d_line_%d", i+1, block.line)
			}
			example = &docExample{name: name, line: block.line, fail: block.fail, files: make(map[string]string)}
			examples = append(examples, example)
			if group != "" {
				groups[group] = example
			}
		}
		if example.fail != block.fail {
			return nil, fmt.Errorf("line %d: mixed success/error fences in dir %s", block.line, group)
		}
		example.blocks++
		for name, source := range files {
			if _, exists := example.files[name]; exists {
				return nil, fmt.Errorf("line %d: duplicate grouped file %s", block.line, name)
			}
			example.files[name] = source
		}
	}
	return examples, nil
}

func TestDocumentationSnippets(t *testing.T) {
	// Documentation must use the embedded standard library, never an ambient
	// developer's GO4T_PATH override. Tests are intentionally sequential.
	t.Setenv("GO4T_PATH", "")
	passBlocks, failBlocks, passPrograms, failPrograms := 0, 0, 0, 0
	for _, name := range []string{"../SPEC.md", "tutorial.md", "stdlib.md", "laws.md", "../README.md"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		blocks, err := fencedBlocks(string(source))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(blocks) == 0 {
			t.Fatalf("%s: no checked go4t examples", name)
		}
		examples, err := documentationExamples(blocks)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, example := range examples {
			if example.fail {
				failBlocks += example.blocks
				failPrograms++
			} else {
				passBlocks += example.blocks
				passPrograms++
			}
			t.Run(filepath.Base(name)+"/"+example.name, func(t *testing.T) {
				directory := t.TempDir()
				var names []string
				for filename := range example.files {
					names = append(names, filename)
				}
				sort.Strings(names)
				for _, filename := range names {
					path := filepath.Join(directory, filepath.FromSlash(filename))
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(example.files[filename]), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				// These are exactly the loading/typechecking packages used by
				// go4t check. Like that command, this does not validate foreign
				// signatures, generate Go, or run effects. Warnings are nonfatal.
				// Programs with go: imports go through the CLI, which owns the binder.
				var err error
				if usesGoImports(example.files) {
					err = checkWithCLI(t, directory)
				} else {
					loaded, loadErr := modules.Load([]string{directory})
					err = loadErr
					if err == nil {
						_, err = types.Check(loaded)
					}
				}
				if (err != nil) != example.fail {
					t.Fatalf("%s:%d: expected failure=%v, check error=%v", name, example.line, example.fail, err)
				}
				if example.fail {
					t.Logf("expected rejection: %v", err)
				}
			})
		}
	}
	if failBlocks == 0 {
		t.Fatal("no expected-failure documentation examples")
	}
	t.Logf("snippets: %d pass / %d expected-fail fences; %d pass / %d expected-fail programs", passBlocks, failBlocks, passPrograms, failPrograms)
}

func TestDocumentationExtraction(t *testing.T) {
	source := "```go\n// not a go4t example\n```\n" +
		"```go4t\n// dir: demo\n// file: main.gt\npackage main\n```\n" +
		"```go4t\n// dir: demo\n// file: helper/helper.gt\npackage helper\n```\n" +
		"```go4t error\npackage bad\n```\n" +
		"```go4t\n// file: main.gt\npackage main\n// file: lib/lib.gt\npackage lib\n```\n"
	blocks, err := fencedBlocks(source)
	if err != nil {
		t.Fatal(err)
	}
	examples, err := documentationExamples(blocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 || len(examples) != 3 || examples[0].blocks != 2 || len(examples[0].files) != 2 || !examples[1].fail || len(examples[2].files) != 2 {
		t.Fatalf("incorrect extraction: blocks=%v examples=%+v", blocks, examples)
	}
	for _, source := range []string{"```go4t\npackage main", "```go4t skip\npackage main\n```"} {
		if _, err := fencedBlocks(source); err == nil {
			t.Errorf("accepted malformed fence %q", source)
		}
	}
	for _, blocks := range [][]docBlock{
		{{text: "// file: ../escape.gt\npackage main"}},
		{{text: "// dir: /absolute\npackage main"}},
		{{text: "// dir: demo\npackage main"}, {text: "// dir: demo\npackage main"}},
		{{text: "// dir: demo\npackage main"}, {fail: true, text: "// dir: demo\n// file: other.gt\npackage main"}},
	} {
		if _, err := documentationExamples(blocks); err == nil {
			t.Errorf("accepted invalid file directives: %+v", blocks)
		}
	}
}
