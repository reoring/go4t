package types_test

import (
	"strings"
	"testing"
)

func TestModuleNamesDoNotLeakOrCollide(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gt": `package main
import left "left/shape"
import right "right/shape"
func list_count() Int { 9 }
type Result[A] = Local(A)
func value(x Result[Int]) Int { match x { Local(n) => n } }
func main() IO[Unit] { println(show(left.size() + right.size() + list_count() + value(Local(2)))) }
`,
		"left/shape/a.gt":  "package shape\nfunc size() Int { 3 }",
		"right/shape/a.gt": "package shape\nfunc size() Int { 4 }",
	})
	program, err := checkModuleFixture(t, root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range program.Warnings {
		if w.Msg == "shadows prelude name Result" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing Result shadow warning")
	}
	if len(program.Modules) < 3 {
		t.Fatal("module graph lost dependencies")
	}
	root = moduleFixture(t, map[string]string{"main.gt": "package main\nfunc main() IO[Unit] { println(show(list_count())) }"})
	if _, err := checkModuleFixture(t, root); err == nil || !strings.Contains(err.Error(), "unknown identifier list_count") {
		t.Fatalf("canonical std name leaked: %v", err)
	}
}
