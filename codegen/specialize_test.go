package codegen

import (
	"fmt"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
)

func TestSpecializationNodeBudget(t *testing.T) {
	for _, size := range []int{40, 41} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			p := multiConstraintProgram(t, `func bounded[A Show](x A) String { show(x) }
func use() String { bounded(1) }`)
			decl := p.Funcs["bounded"].Decl
			count := 0
			ast.Walk(decl, func(ast.Node) { count++ })
			if count > size {
				t.Fatal("fixture larger than target")
			}
			for range size - count {
				decl.Body.Stmts = append(decl.Body.Stmts, &ast.ExprStmt{X: integer(0)})
			}
			g := &generator{specials: make(map[string][]*specialization)}
			g.planSpecializations(p)
			want := 0
			if size == 40 {
				want = 1
			}
			if got := len(g.specials["bounded"]); got != want {
				t.Fatalf("%d AST nodes: got %d copies, want %d", size, got, want)
			}
		})
	}
}

func TestSpecializationCopyBudget(t *testing.T) {
	var source strings.Builder
	source.WriteString("package main\nfunc render[A Show](x A) String { show(x) }\n")
	for i := range 9 {
		fmt.Fprintf(&source, "type Box%d = Box%d(Int)\n", i, i)
		fmt.Fprintf(&source, "instance Show[Box%d] { show(x) { match x { Box%d(n) => \"Box%d(\" + show(n) + \")\" } } }\n", i, i, i)
	}
	source.WriteString("func main() IO[Unit] { do {\n")
	var expected strings.Builder
	for i := range 9 {
		fmt.Fprintf(&source, "println(render(Box%d(%d)))\n", i, i)
		fmt.Fprintf(&expected, "Box%d(%d)\n", i, i)
	}
	source.WriteString("} }\n")
	generated := qualityProgram(t, "copies.gt", source.String())
	if n := strings.Count(string(generated), "func render__special"); n != 8 {
		t.Fatalf("got %d copies, want 8", n)
	}
	if !strings.Contains(string(generated), "render_(Show_Box8{},") {
		t.Fatal("ninth dictionary did not fall back")
	}
	output, err := runQualityGo(t, generated, false)
	if err != nil || output != expected.String() {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestSpecializationTailCallsAndUnconstrainedTypes(t *testing.T) {
	generated := qualityProgram(t, "special-tail.gt", `package main
func repeatValue[A Show](n Int, x A) String { if n == 0 { show(x) } else { repeatValue(n - 1, x) } }
func keepOther[A Show, B](x A, y B) B { let text = show(x); y }
func forward[A Show](x A) String { repeatValue(1000000, x) }
func main() IO[Unit] {
 let value = repeatValue
 println("${forward(7)} ${value(1, true)} ${keepOther(1, 2)} ${keepOther(1, true)}")
}`)
	output, err := runQualityGo(t, generated, true)
	if err != nil || output != "7 true 2 true\n" {
		t.Fatalf("output %q: %v", output, err)
	}
	if n := strings.Count(string(generated), "func keepOther__special"); n != 1 {
		t.Fatalf("same dictionary with different unconstrained B generated %d copies", n)
	}
	if !strings.Contains(string(generated), "for {") {
		t.Fatal("specialization lost tail loop")
	}
}
