package laws

import (
	"strings"
	"testing"
)

func TestMapLaws(t *testing.T) {
	for _, file := range []string{"map.gt", "map_decide.gt", "map_quota.gt", "map_delete.gt"} {
		t.Run(file, func(t *testing.T) {
			if errors := CheckProgram(loadFixture(t, file)); len(errors) != 0 {
				for _, err := range errors { t.Error(err) }
			}
		})
	}
}

// placement.gt is a zone-placement decision core written against the public
// syntax; every law must close as written except insertCommutes, which states
// equality of two symbolic insert chains and is intentionally not provable.
func TestPlacementLaws(t *testing.T) {
	errors := diagnosticsByLaw(t, loadFixture(t, "placement.gt"))
	for name, err := range errors {
		if name != "insertCommutes" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if errors["insertCommutes"] == nil {
		t.Error("insertCommutes: symbolic insert chains must not be reordered")
	}
}

// assume_order.gt: an `if` assumption made before a `match` split must still
// apply after the split renames the variables it mentions.
func TestAssumptionSurvivesLaterSplit(t *testing.T) {
	for name, err := range diagnosticsByLaw(t, loadFixture(t, "assume_order.gt")) {
		t.Errorf("%s: %v", name, err)
	}
}

func TestMapNeutralComparators(t *testing.T) {
	p := loadFixture(t, "map_neutral.gt")
	errors := diagnosticsByLaw(t, p)
	for name := range p.Laws {
		err := errors[name]
		if err == nil || !strings.Contains(err.Msg, "stuck normal forms differ") {
			t.Errorf("%s: want stuck (not rejected), got %v", name, err)
		}
	}
}

func TestMapLiteralOrdering(t *testing.T) {
	e := newEvaluator(loadFixture(t, "map.gt"))
	for _, keys := range [][]*term{
		{integer(10), integer(-2), integer(1), integer(-2)},
		{text("z"), text("a"), text("日本"), text("")},
		{boolean(true), boolean(false), boolean(true)},
		{{kind: "tuple", args: []*term{integer(2), boolean(false)}}, {kind: "tuple", args: []*term{integer(1), boolean(true)}}, {kind: "tuple", args: []*term{integer(1), boolean(false)}}},
	} {
		value := &term{kind: "map"}
		for i, key := range keys { value = e.mapInsert(value, key, integer(int64(i)), nil) }
		for i := 2; i < len(value.args); i += 2 {
			if compareMapKeys(value.args[i-2], value.args[i]) >= 0 { t.Fatalf("unordered or duplicate keys: %s", e.display(value)) }
		}
		last := keys[len(keys)-1]
		for i := 0; i < len(value.args); i += 2 {
			if compareMapKeys(value.args[i], last) == 0 && value.args[i+1].number != int64(len(keys)-1) { t.Fatal("duplicate key did not retain last value") }
		}
	}
}
