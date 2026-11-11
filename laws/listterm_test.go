package laws

import (
	"strings"
	"testing"
)

func TestListModels(t *testing.T) {
	for _, file := range []string{"list_models.gt", "list_models_sequences.gt", "list_models_pairs.gt", "list_models_indices.gt"} {
		t.Run(file, func(t *testing.T) {
			for _, err := range CheckProgram(loadFixture(t, file)) {
				t.Error(err)
			}
		})
	}
}

func TestListModelsRejectFalseAndForeign(t *testing.T) {
	errors := diagnosticsByLaw(t, loadFixture(t, "list_models_false.gt"))
	for _, name := range []string{"falseLengthAppend", "foreignLength"} {
		if err := errors[name]; err == nil || !strings.Contains(err.Msg, "stuck normal forms differ") {
			t.Errorf("%s: want unequal normal forms, got %v", name, err)
		}
	}
	if err := errors["libraryStillOwned"]; err != nil {
		t.Error(err)
	}
}

func TestListModelsKeepCallbackTaint(t *testing.T) {
	p := loadSource(t, `package main
import "std/list"
extern func bad(x Int) Bool = "example.invalid/predicate".Bad
law tainted(xs List[Int]) { list.count(xs, bad) == list.count(xs, bad) }
proof tainted(xs) { induction xs }
func main() IO[Unit] { ioPure(unit()) }
`)
	if err := diagnosticsByLaw(t, p)["tainted"]; err == nil || !strings.Contains(err.Msg, "extern") {
		t.Fatalf("callback taint escaped the model: %v", err)
	}
}
