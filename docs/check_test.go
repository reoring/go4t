package docs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gofx/modules"
	"gofx/types"
)

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

// fencedBlocks tracks other fence languages too, so a gofx-looking line inside
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
			if strings.HasPrefix(language, "gofx") && language != "gofx" && language != "gofx error" {
				return nil, fmt.Errorf("line %d: unsupported gofx fence %q", start, language)
			}
			continue
		}
		if open && trimmed == "```" {
			if language == "gofx" || language == "gofx error" {
				blocks = append(blocks, docBlock{start, language == "gofx error", strings.Join(body, "\n") + "\n"})
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
// every fence is independent; without file, it supplies main.gfx. No implicit
// main, package, prelude copy, or import is added to make examples pass.
func documentationExamples(blocks []docBlock) ([]*docExample, error) {
	var examples []*docExample
	groups := make(map[string]*docExample)
	for i, block := range blocks {
		group, filename := "", "main.gfx"
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
			return nil, fmt.Errorf("line %d: empty gofx fence", block.line)
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
	// developer's GOFX_PATH override. Tests are intentionally sequential.
	t.Setenv("GOFX_PATH", "")
	passBlocks, failBlocks, passPrograms, failPrograms := 0, 0, 0, 0
	for _, name := range []string{"../SPEC.md", "tutorial.md", "stdlib.md", "../README.md"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		blocks, err := fencedBlocks(string(source))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(blocks) == 0 {
			t.Fatalf("%s: no checked gofx examples", name)
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
				// gofx check. Like that command, this does not validate foreign
				// signatures, generate Go, or run effects. Warnings are nonfatal.
				loaded, err := modules.Load([]string{directory})
				if err == nil {
					_, err = types.Check(loaded)
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
	source := "```go\n// not a gofx example\n```\n" +
		"```gofx\n// dir: demo\n// file: main.gfx\npackage main\n```\n" +
		"```gofx\n// dir: demo\n// file: helper/helper.gfx\npackage helper\n```\n" +
		"```gofx error\npackage bad\n```\n" +
		"```gofx\n// file: main.gfx\npackage main\n// file: lib/lib.gfx\npackage lib\n```\n"
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
	for _, source := range []string{"```gofx\npackage main", "```gofx skip\npackage main\n```"} {
		if _, err := fencedBlocks(source); err == nil {
			t.Errorf("accepted malformed fence %q", source)
		}
	}
	for _, blocks := range [][]docBlock{
		{{text: "// file: ../escape.gfx\npackage main"}},
		{{text: "// dir: /absolute\npackage main"}},
		{{text: "// dir: demo\npackage main"}, {text: "// dir: demo\npackage main"}},
		{{text: "// dir: demo\npackage main"}, {fail: true, text: "// dir: demo\n// file: other.gfx\npackage main"}},
	} {
		if _, err := documentationExamples(blocks); err == nil {
			t.Errorf("accepted invalid file directives: %+v", blocks)
		}
	}
}
