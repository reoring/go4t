package codegen

import (
	"bytes"
	"context"
	"fmt"
	goast "go/ast"
	"go/format"
	goparser "go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gofx/ast"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestSelfTailCallLoop(t *testing.T) {
	p := fixture()
	recur := call(function("swap", fnType(types.Int, types.Int)), local("b", types.Int), local("a", types.Int))
	f := addFunc(p, "swap", []string{"a", "b"}, []types.Type{types.Int, types.Int}, &ast.If{
		Cond: boolean(false), Then: integer(0), Else: &ast.BlockExpr{Block: &ast.Block{Result: recur}},
	})
	f.Decl.Pos = ast.Pos{Line: 3, Col: 1}
	p.Files = []*ast.File{{Name: "loop.gfx", Decls: []ast.Decl{f.Decl}}}
	out := assertSource(t, p, "for {", "continue", "//line loop.gfx:3")
	if strings.Contains(out, "return swap_(") {
		t.Fatalf("tail call still recurses:\n%s", out)
	}
	parsed, err := goparser.ParseFile(token.NewFileSet(), "generated.go", out, 0)
	if err != nil {
		t.Fatal(err)
	}
	var reassignment *goast.AssignStmt
	goast.Inspect(parsed, func(node goast.Node) bool {
		if assign, ok := node.(*goast.AssignStmt); ok && assign.Tok == token.ASSIGN && len(assign.Lhs) == 2 {
			reassignment = assign
		}
		return true
	})
	if reassignment == nil {
		t.Fatal("missing simultaneous parameter reassignment")
	}
	for _, rhs := range reassignment.Rhs {
		id, ok := rhs.(*goast.Ident)
		if !ok || !strings.HasPrefix(id.Name, "_gofx") {
			t.Fatalf("argument was not evaluated into a temporary: %#v", rhs)
		}
	}
}

func TestTailCallDictionariesPassThrough(t *testing.T) {
	p, showInt, _ := showFixture()
	a := &types.TVar{Name: "A", Kind: types.KStar{}}
	constraint := types.Constraint{Class: "Show", Arg: a}
	ref := types.DictRef{Kind: types.DictParam, Class: "Show", Param: a, ParamClass: "Show"}
	f := addFunc(p, "again", []string{"x"}, []types.Type{a}, call(function("again", fnType(a), ref), local("x", a)))
	f.Scheme.Constraints = []types.Constraint{constraint}
	out := assertSource(t, p, "func again_(dict_A Show,", "continue")
	if strings.Contains(out, "dict_A =") || strings.Contains(out, "return again_(") {
		t.Fatalf("pass-through dictionary changed or tail call retained:\n%s", out)
	}
	// A different dictionary changes the recursive instantiation. Such a call
	// must stay a call rather than silently using the first iteration's dict.
	types.AnnOf(f.Decl.Body.Result.(*ast.Call).Fn).Dicts = []types.DictRef{instanceDict(showInt)}
	out = assertSource(t, p, "return again_(Show_Int{}, x_)")
	if strings.Contains(out, "for {") {
		t.Fatalf("dictionary-changing recursion was incorrectly looped:\n%s", out)
	}
}

func TestNonTailAndShadowedCallsStayCalls(t *testing.T) {
	p := fixture()
	recur := call(function("nonTail", fnType(types.Int)), local("x", types.Int))
	addFunc(p, "nonTail", []string{"x"}, []types.Type{types.Int}, &ast.Binary{
		ExprBase: annotated(&types.Ann{Type: types.Int}), Op: "+", L: recur, R: integer(1),
	})
	out := assertSource(t, p, "nonTail_(x_)", "+ int64(1)")
	if strings.Contains(out, "for {") {
		t.Fatalf("non-tail recursion was looped:\n%s", out)
	}
	lambda := &ast.Lambda{Params: []ast.Param{{Name: "x"}}, Body: &ast.BlockExpr{Block: &ast.Block{
		Stmts:  []ast.Stmt{&ast.LetStmt{Name: "again", Value: identity()}},
		Result: call(local("again", fnType(types.Int)), local("x", types.Int)),
	}}}
	g := &generator{tuples: make(map[int]struct{})}
	if code := g.namedLambda("again", lambda); strings.Contains(code, "for {") {
		t.Fatalf("shadowed local call was looped:\n%s", code)
	}
	lambda.Params = []ast.Param{{Name: "again"}}
	lambda.Body = call(local("again", fnType(types.Int)), integer(0))
	if code := g.namedLambda("again", lambda); strings.Contains(code, "for {") {
		t.Fatalf("shadowing lambda parameter was looped:\n%s", code)
	}
}

func TestNamedLambdaTailCall(t *testing.T) {
	g := &generator{tuples: make(map[int]struct{})}
	lambda := &ast.Lambda{Params: []ast.Param{{Name: "x"}}, Body: &ast.Match{
		Scrut: local("x", types.Int), Arms: []ast.Arm{
			{Pat: &ast.PLit{X: integer(0)}, Body: integer(42)},
			{Pat: &ast.PVar{Name: "n"}, Body: call(local("again", fnType(types.Int)), integer(0))},
		},
	}}
	code := g.namedLambda("again", lambda)
	if !strings.Contains(code, "for {") || !strings.Contains(code, "continue") || strings.Contains(code, "any(again_)") {
		t.Fatalf("named lambda did not eliminate its tail call:\n%s", code)
	}
	output, err := runQualityGo(t, []byte("package main\nimport \"fmt\"\nfunc main() { f := "+code+"; fmt.Println(f(int64(1))) }"), false)
	if err != nil || output != "42\n" {
		t.Fatalf("named lambda output %q, err %v", output, err)
	}
}

func TestMillionElementFoldWithoutStackGrowth(t *testing.T) {
	source := qualityProgram(t, "million.gfx", `package main
import "std/result"
func millionList(n Int, xs List[Int]) List[Int] {
  if n == 0 { xs } else { millionList(n - 1, Cons(n, xs)) }
}
func main() IO[Unit] {
  let xs = millionList(1000000, Nil)
  let total = fold(xs, 0, func(acc, n) => acc + n)
  let size = length(xs)
  let reversed = reverse(xs)
  println("${total} ${size} ${result.fromOption(head(reversed), 0)} ${length(reversed)}")
}`)
	output, err := runQualityGo(t, source, true)
	if err != nil || output != "500000500000 1000000 1000000 1000000\n" {
		t.Fatalf("million-element run: %v\n%s", err, output)
	}
}

func TestTailCallEvaluationAndClosureSnapshots(t *testing.T) {
	source := qualityProgram(t, "snapshots.gfx", `package main
func swapValues(n Int, a Int, b Int) Int {
  if n == 0 { a * 10 + b } else {
    let remaining = n - 1
    swapValues(remaining, b, a)
  }
}
func snapshots(n Int, fs List[func(Unit) Int]) Int {
  match n {
    0 => fold(fs, 0, func(acc, f) => acc + f(unit()))
    remaining if remaining > 0 => snapshots(remaining - 1, Cons(func(_) => n, fs))
    _ => 0
  }
}
func shadowParameter(n Int, acc Int) Int {
  if n == 0 { acc } else {
    let n = n - 1
    shadowParameter(n, acc + 1)
  }
}
func main() IO[Unit] {
  println("${swapValues(3, 1, 2)} ${snapshots(3, Nil)} ${shadowParameter(5, 0)}")
}`)
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "21 6 5\n" {
		t.Fatalf("tail-call semantics: %v\n%s", err, output)
	}
}

func TestLargeResultMatchWithManyCallers(t *testing.T) {
	// A result-position match must remain practical to compile with Go's
	// default inliner, even when every constructor has its own caller.
	const count = 952
	var source strings.Builder
	source.WriteString("package main\ntype Configuration = ")
	for i := range count {
		if i != 0 {
			source.WriteString(" | ")
		}
		fmt.Fprintf(&source, "Value%d(Int)", i)
	}
	source.WriteString("\nfunc lookup(key (String,String,String)) Option[Configuration] { match key {\n")
	for i := range count {
		fmt.Fprintf(&source, "(\"group\",\"version\",\"kind%d\") => Some(Value%d(%d))\n", i, i, i)
	}
	source.WriteString("_ => None\n} }\n")
	for i := range count {
		fmt.Fprintf(&source, "func caller%d() Int { match lookup((\"group\",\"version\",\"kind%d\")) { Some(Value%d(n)) => n; _ => -1 } }\n", i, i, i)
	}
	var batches []string
	for start := 0; start < count; start += 32 {
		name := fmt.Sprintf("batch%d", start)
		batches = append(batches, name+"()")
		fmt.Fprintf(&source, "func %s() Int { ", name)
		for i := start; i < start+32 && i < count; i++ {
			if i != start {
				source.WriteString(" + ")
			}
			fmt.Fprintf(&source, "caller%d()", i)
		}
		source.WriteString(" }\n")
	}
	fmt.Fprintf(&source, "func main() IO[Unit] { println(show(%s)) }\n", strings.Join(batches, " + "))
	generated := qualityProgram(t, "large-result-match.gfx", source.String())
	output, err := runQualityGo(t, generated, false)
	want := fmt.Sprintf("%d\n", count*(count-1)/2)
	if err != nil || output != want {
		t.Fatalf("large result match: %v\nwant %q, got %q", err, want, output)
	}
}

func TestResultMatchEvaluationScopesAndReturns(t *testing.T) {
	source := qualityProgram(t, "result-match.gfx", `package main
type Value = Number(Int) | Text(String)
func evaluated(next func() Value) Int {
  match next() { Number(n) if n < 0 => -1; Number(n) => n; Text(_) => -2 }
}
func guarded(value Value, n Int) Int {
  match value {
    Number(n) if n < 0 => n
    Number(n) if n == 7 => n + 1
    Number(n) => n + 2
    Text(_) => n
  }
}
func erased[A](value Option[A], fallback A) A {
  match value { Some(n) => n; None => fallback }
}
func selected(value Value) Value {
  match value { Number(n) => Number(n + 1); Text(n) => Text(n) }
}
func closure(value Value) func(Unit) Int {
  match value {
    Number(n) => func(_) => n
    Text(_) => func(_) => -1
  }
}
func expression(value Value) Int {
  10 + (match value { Number(n) => n; Text(_) => 0 })
}
func main() IO[Unit] {
  let returned = match selected(Number(4)) { Number(n) => n; Text(_) => -1 }
  let textValue = Text("x")
  let fallback = erased(None,"fallback")
  println("${guarded(Number(-3),99)} ${guarded(Number(7),99)} ${guarded(Number(2),99)} ${guarded(textValue,99)} ${erased(Some(4),0)} ${fallback} ${returned} ${closure(Number(9))(unit())} ${expression(Number(5))}")
}
`)
	// The Go callback makes evaluation count observable without adding an
	// impure primitive to the source language.
	source = append(source, []byte(`
func init() {
	calls := 0
	got := evaluated_(func() any {
		calls++
		return &Value_Number{F0: int64(7)}
	})
	if any(got).(int64) != 7 || calls != 1 {
		panic("result match must evaluate its scrutinee exactly once")
	}
}
`)...)
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "-3 8 4 99 4 fallback 5 9 15\n" {
		t.Fatalf("result match semantics: %v\n%s", err, output)
	}
}

func TestSourceDirectivesAndFormatting(t *testing.T) {
	source := qualityProgram(t, "positions.gfx", `package main
func choose(n Int) Int {
  match n {
    0 => 2
    _ => 3
  }
}
func main() IO[Unit] { println(show(choose(0))) }
`)
	for _, line := range []string{"//line positions.gfx:2", "//line positions.gfx:4", "//line positions.gfx:5", "//line positions.gfx:8", "//line std/prelude/prelude.gfx:"} {
		if !bytes.Contains(source, []byte(line)) {
			t.Errorf("missing source directive %q", line)
		}
	}
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, "generated.go", source, goparser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	goast.Inspect(file, func(node goast.Node) bool {
		var pos token.Pos
		switch node := node.(type) {
		case *goast.FuncDecl:
			pos = node.Pos()
		case *goast.FuncLit:
			pos = node.Pos()
		default:
			return true
		}
		position := fset.Position(pos)
		if !strings.HasSuffix(position.Filename, ".gfx") {
			t.Errorf("function missing .gfx position: %s", position)
		}
		physical := fset.PositionFor(pos, false)
		lines := bytes.Split(source, []byte("\n"))
		if physical.Line < 2 || !bytes.HasPrefix(lines[physical.Line-2], []byte("//line ")) {
			t.Errorf("function lacks an immediately preceding directive: %s", position)
		}
		return true
	})
}

func TestFormattingFailureRetainsGeneratedSource(t *testing.T) {
	p := fixture()
	addFunc(p, "bad-name", nil, nil, integer(0))
	source, err := generate(p, "main")
	if err == nil || !strings.HasPrefix(err.Error(), "format generated Go:") || !bytes.Contains(source, []byte("func bad-name_")) {
		t.Fatalf("format failure lost its source or error: %q, %v", source, err)
	}
}

func TestSourceFilenameNewlineRejected(t *testing.T) {
	p := fixture()
	p.Main.Decl.Pos = ast.Pos{Line: 2, Col: 1}
	p.Files = []*ast.File{{Name: "bad\nname.gfx", Decls: []ast.Decl{p.Main.Decl}}}
	if _, err := generate(p, "main"); err == nil || err.Error() != "2:1: source filename contains a newline" {
		t.Fatalf("filename diagnostic = %v", err)
	}
}

func TestZeroArgumentTailCall(t *testing.T) {
	p := fixture()
	addFunc(p, "forever", nil, nil, call(function("forever", fnType())))
	out := assertSource(t, p, "func forever_() any", "for {", "continue")
	if strings.Contains(out, "return forever_()") {
		t.Fatalf("zero-argument tail call was not eliminated:\n%s", out)
	}
}

func TestNamedLambdaShadowedMatchArm(t *testing.T) {
	g := &generator{tuples: make(map[int]struct{})}
	lambda := &ast.Lambda{Params: []ast.Param{{Name: "x"}}, Body: &ast.Match{
		Scrut: local("x", types.Unit), Arms: []ast.Arm{{
			Pat:  &ast.PAs{Name: "again", Pat: &ast.PWild{}},
			Body: call(local("again", fnType(types.Int)), integer(0)),
		}},
	}}
	if code := g.namedLambda("again", lambda); strings.Contains(code, "for {") {
		t.Fatalf("match arm's shadowing binding was looped:\n%s", code)
	}
}

func TestRuntimePanicReportsGfxPosition(t *testing.T) {
	fixture, err := os.ReadFile("../examples/codegen-quality/panic.gfx")
	if err != nil {
		t.Fatal(err)
	}
	source := qualityProgram(t, "examples/codegen-quality/panic.gfx", string(fixture))
	output, err := runQualityGo(t, source, false)
	if err == nil || !strings.Contains(output, "integer divide by zero") || !strings.Contains(output, "examples/codegen-quality/panic.gfx:5") {
		t.Fatalf("panic did not retain the match-arm source position: %v\n%s", err, output)
	}
	t.Log(output)
}

func qualityProgram(t *testing.T, filename, source string) []byte {
	t.Helper()
	user, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	user.Name = filename
	modules, err := modules.LoadFiles([]*ast.File{user}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(modules)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := generate(program, "main")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, generated)
	}
	formatted, err := format.Source(generated)
	if err != nil || !bytes.Equal(generated, formatted) {
		t.Fatalf("generated source is not formatted: %v", err)
	}
	return generated
}

func runQualityGo(t *testing.T, source []byte, limitStack bool) (string, error) {
	t.Helper()
	dir := t.TempDir()
	main := filepath.Join(dir, "main.go")
	if err := os.WriteFile(main, source, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"run", main}
	if limitStack {
		// A million recursive calls exceed this by orders of magnitude. The
		// bound proves stack independence instead of relying on Go's growing stack.
		limit := filepath.Join(dir, "stack.go")
		if err := os.WriteFile(limit, []byte("package main\nimport \"runtime/debug\"\nfunc init() { debug.SetMaxStack(256 << 10) }\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, limit)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = ".."
	cmd.WaitDelay = 5 * time.Second
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("go run timed out: %w", ctx.Err())
	}
	return string(output), err
}
