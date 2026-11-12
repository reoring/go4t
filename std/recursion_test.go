package std_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/codegen"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/std"
	"github.com/reoring/go4t/types"
)

func TestStandardLibraryRecursionWarnings(t *testing.T) {
	var source strings.Builder
	source.WriteString("package main\n")
	seen := make(map[string]bool)
	if err := fs.WalkDir(std.FS, ".", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(filename, ".gt") && !seen[path.Dir(filename)] {
			seen[path.Dir(filename)] = true
			fmt.Fprintf(&source, "import %q\n", "std/"+path.Dir(filename))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source.WriteString("func main() IO[Unit] { ioPure(unit()) }\n")
	p := recursionProgram(t, source.String())
	for _, warning := range p.Warnings {
		if strings.Contains(warning.Msg, "calls itself in a non-tail position") {
			t.Error(warning)
		}
	}
}

func recursionProgram(t *testing.T, source string) *types.Program {
	t.Helper()
	file, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	file.Name = "recursion_test.gt"
	mods, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check(mods)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMillionElementListFunctions(t *testing.T) {
	// Repeated values keep Eq-only nub linear while still visiting a million
	// cells. Searches and predicates deliberately traverse the entire input.
	checks := map[string]string{
		"foldl":         `list.foldl(xs, 0, add) == size`,
		"toList":        `length(list.toList(xs)) == size`,
		"foldMapList":   `length(list.foldMapList(xs, singleton)) == size`,
		"forEach":       `list.forEach(xs, func(_x Int) => Some(unit())) != None`,
		"mapM":          `match list.mapM(xs, func(x Int) => Some(x)) { Some(ys) => length(ys) == size; None => false }`,
		"fold":          `list.fold(xs, 0, add) == size`,
		"length":        `list.length(xs) == size`,
		"append":        `length(list.append(xs, xs)) == size * 2`,
		"filter":        `length(list.filter(xs, yes)) == size`,
		"reverse":       `length(list.reverse(xs)) == size`,
		"zip":           `length(list.zip(xs, xs)) == size`,
		"head":          `list.head(xs) == Some(1)`,
		"tail":          `length(list.tail(xs)) == size - 1`,
		"take":          `length(list.take(size, xs)) == size`,
		"drop":          `length(list.drop(size, xs)) == 0`,
		"sum":           `list.sum(xs) == size`,
		"concatMap":     `length(list.concatMap(xs, singleton)) == size`,
		"catOptions":    `length(list.catOptions(options)) == size`,
		"mapMaybe":      `length(list.mapMaybe(xs, func(x Int) => Some(x))) == size`,
		"elem":          `!list.elem(0, xs)`,
		"sortBy":        `length(list.sortBy(xs, compare)) == size`,
		"sort":          `length(list.sort(xs)) == size`,
		"maximum":       `list.maximum(xs) == Some(1)`,
		"minimum":       `list.minimum(xs) == Some(1)`,
		"zipWith":       `length(list.zipWith(xs, xs, add)) == size`,
		"zipWithAcc":    `length(list.zipWithAcc(xs, xs, add, xs)) == size * 2`,
		"unzip":         `length(fst(list.unzip(pairs))) == size`,
		"any":           `!list.any(xs, no)`,
		"all":           `list.all(xs, yes)`,
		"find":          `list.find(xs, no) == None`,
		"findIndex":     `list.findIndex(xs, no) == None`,
		"findIndexFrom": `list.findIndexFrom(xs, no, 0) == None`,
		"partition":     `length(fst(list.partition(xs, yes))) == size`,
		"nub":           `list.nub(xs) == [1]`,
		"rangeAcc":      `length(list.rangeAcc(0, 0, xs)) == size`,
		"replicateAcc":  `length(list.replicateAcc(0, 1, xs)) == size`,
		"intercalate":   `length(list.intercalate(xs, [[], []])) == size && length(list.intercalate([], lists)) == size`,
		"flatten":       `length(list.flatten(lists)) == size`,
		"last":          `list.last(xs) == Some(1)`,
		"init":          `length(list.init(xs)) == size - 1`,
		"initAcc":       `length(list.initAcc(xs, xs)) == size * 2 - 1`,
		"splitAt":       `length(fst(list.splitAt(size, xs))) == size`,
		"splitAtAcc":    `length(fst(list.splitAtAcc(size, xs, xs))) == size * 2`,
		"span":          `length(fst(list.span(xs, yes))) == size`,
		"spanAcc":       `length(fst(list.spanAcc(xs, yes, xs))) == size * 2`,
		"takeWhile":     `length(list.takeWhile(xs, yes)) == size`,
		"dropWhile":     `length(list.dropWhile(xs, yes)) == 0`,
		"lookup":        `list.lookup(0, pairs) == None`,
		"indexed":       `length(list.indexed(xs)) == size`,
		"indexedFrom":   `length(list.indexedFrom(xs, 0, pairs)) == size * 2`,
		"iterateAcc":    `length(list.iterateAcc(0, 1, func(x Int) => x, xs)) == size`,
		"product":       `list.product(xs) == 1`,
		"count":         `list.count(xs, yes) == size`,
		"unfoldAcc":     `length(list.unfoldAcc(0, func(_n Int) => None, xs)) == size`,
		"chunksOf":      `length(list.chunksOf(1, xs)) == size`,
		"chunksOfAcc":   `length(list.chunksOfAcc(1, xs, lists)) == size * 2`,
		"sortOn":        `length(list.sortOn(xs, func(x Int) => x)) == size`,
	}
	// Keep coverage tied to exports, including the historically public helpers
	// and higher-kinded Foldable entry points instantiated with List.
	data, err := std.FS.ReadFile("list/list.gt")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, decl := range file.Decls {
		f, ok := decl.(*ast.FuncDecl)
		if !ok || strings.HasPrefix(f.Name, "_") {
			continue
		}
		listParam := false
		for _, param := range f.Params {
			if typ, ok := param.Type.(*ast.TypeName); ok && (typ.Name == "List" || typ.Name == "F") {
				listParam = true
			}
		}
		if !listParam {
			continue
		}
		if checks[f.Name] == "" {
			t.Fatalf("missing million-element case for list.%s", f.Name)
		}
		names = append(names, f.Name)
	}
	if len(names) != len(checks) {
		t.Fatalf("list cases = %d, exported list functions = %d", len(checks), len(names))
	}
	sort.Strings(names)
	var source, want strings.Builder
	source.WriteString(`package main
import "std/list"
func main() IO[Unit] {
  let size = 1000000
  let xs = list.replicate(size, 1)
  let pairs = list.replicate(size, (1, 1))
  let lists = list.replicate(size, [1])
  let options = list.replicate(size, Some(1))
  let add = func(a Int, b Int) => a + b
  let singleton = func(x Int) => [x]
  let yes = func(_x Int) => true
  let no = func(_x Int) => false
  do {
`)
	for _, name := range names {
		fmt.Fprintf(&source, "println(%q + show(%s))\n", name+": ", checks[name])
		fmt.Fprintf(&want, "%s: true\n", name)
	}
	source.WriteString("}\n}\n")
	generated, err := codegen.Generate(recursionProgram(t, source.String()), "main")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	args := []string{"run"}
	for name, data := range generated {
		filename := filepath.Join(dir, name)
		if err := os.WriteFile(filename, data, 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, filename)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	start := time.Now()
	output, err := exec.CommandContext(ctx, "go", args...).CombinedOutput()
	t.Logf("%d list functions, compile and run: %s", len(names), time.Since(start))
	if err != nil || string(output) != want.String() {
		t.Fatalf("million-element list checks: %v\n%s", err, output)
	}
}
