package laws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reoring/go4t/types"
)

const stuckRecordLaw = `
law independentModes(o Observed, other Observed) {
  (match o.mode { DryRun => decide(o); Live => Skip }) ==
  (match other.mode { DryRun => Skip; Live => Skip })
}
proof independentModes(o, other) { refl }
`

func TestReflDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, fixture, extra, law, stuck, differs string
	}{
		{
			name: "constructor mismatch", fixture: "cancel_bad1.gt", law: "cancelIdempotent",
			differs: "differs at: Pending(n) vs Cancelled(n)",
		},
		{
			name: "record field", fixture: "update.gt", extra: stuckRecordLaw, law: "independentModes",
			stuck:   "stuck on: o.mode\nstuck on: other.mode",
			differs: "differs at: o vs other",
		},
		{
			name: "recursive argument", fixture: "reentry.gt", law: "symbolicCall",
			extra: "\nlaw symbolicCall(xs List[Pod]) { _firstEvictable(xs) == None }\nproof symbolicCall(xs) { refl }\n",
			stuck: "stuck on: xs", differs: "differs at: _firstEvictable(xs) vs None",
		},
		{
			name: "recursive method argument", fixture: "update.gt", law: "symbolicEq",
			extra: "\nlaw symbolicEq(xs List[Int]) { eq(xs, Nil) == false }\nproof symbolicEq(xs) { refl }\n",
			stuck: "stuck on: xs", differs: "differs at: eq(xs, Nil) vs false",
		},
		{
			name: "canonical weak comparison", fixture: "compare.gt", law: "stuckComparison",
			extra: "\nlaw stuckComparison(c Int, s Int) { decide(c, s) == 0 }\nproof stuckComparison(c, s) { refl }\n",
			stuck: "stuck on: s <= 0", differs: "differs at: if s <= 0 { 1 } else { 2 } vs 0",
		},
		{
			name: "canonical strict comparison", fixture: "compare.gt", law: "stuckComparison",
			extra: "\nlaw stuckComparison(a Int, b Int) { (if a > b { 1 } else { 2 }) == 0 }\nproof stuckComparison(a, b) { refl }\n",
			stuck: "stuck on: b < a", differs: "differs at: if b < a { 1 } else { 2 } vs 0",
		},
		{
			name: "canonical equality", fixture: "compare.gt", law: "stuckComparison",
			extra: "\nlaw stuckComparison(a Int, b Int) { (if b == a { 1 } else { 2 }) == 0 }\nproof stuckComparison(a, b) { refl }\n",
			stuck: "stuck on: a == b", differs: "differs at: if a == b { 1 } else { 2 } vs 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			diagnostic := diagnosticsByLaw(t, loadSource(t, string(source)+tc.extra))[tc.law]
			assertNormalForms(t, diagnostic)
			var stuck, differs []string
			lines := strings.Split(diagnostic.Msg, "\n")
			for _, line := range lines {
				switch {
				case strings.HasPrefix(line, "stuck on: "):
					stuck = append(stuck, line)
				case strings.HasPrefix(line, "differs at: "):
					differs = append(differs, line)
				}
			}
			if got := strings.Join(stuck, "\n"); got != tc.stuck {
				t.Errorf("stuck lines = %q, want %q", got, tc.stuck)
			}
			if got := strings.Join(differs, "\n"); got != tc.differs {
				t.Errorf("difference lines = %q, want %q", got, tc.differs)
			}
			// Subterms must use the same spelling and collision suffixes as
			// the full forms, without independently renaming their variables.
			full := ""
			for _, line := range lines {
				if strings.HasPrefix(line, "LHS: ") || strings.HasPrefix(line, "RHS: ") {
					full += line + "\n"
				}
			}
			for _, line := range lines {
				if value, ok := strings.CutPrefix(line, "stuck on: "); ok && !strings.Contains(full, value) {
					t.Errorf("stuck term %q missing from full forms:\n%s", value, full)
				}
				if value, ok := strings.CutPrefix(line, "differs at: "); ok {
					left, right, found := strings.Cut(value, " vs ")
					if !found || !strings.Contains(full, left) || !strings.Contains(full, right) {
						t.Errorf("differing terms %q missing from full forms:\n%s", value, full)
					}
				}
			}
		})
	}
}

func TestDiagnosticStuckOn(t *testing.T) {
	x := &term{kind: "var", name: "x", id: 1}
	y := &term{kind: "var", name: "y", id: 2}
	choice := func(condition *term) *term {
		return &term{kind: "if", args: []*term{condition, integer(1), integer(2)}}
	}
	call := func(name string, args ...*term) *term {
		return &term{kind: "call", args: append([]*term{{kind: "global", name: name}}, args...)}
	}
	e := newEvaluator(&types.Program{})
	e.decreasing = map[string][]decreasingPosition{"recursive": {{position: 1}}}
	for _, tc := range []struct {
		name        string
		left, right *term
		want        string
	}{
		{"same sides", choice(x), choice(x), "stuck on: x"},
		{"distinct sides", choice(x), choice(y), "stuck on: x\nstuck on: y"},
		{"right only", integer(1), choice(y), "stuck on: y"},
		{"outermost", &term{kind: "if", args: []*term{x, choice(y), integer(0)}}, integer(0), "stuck on: x"},
		{"nested in data", &term{kind: "tuple", args: []*term{integer(0), choice(y)}}, integer(0), "stuck on: y"},
		{"decreasing position", call("recursive", x, y), integer(0), "stuck on: y"},
		{"nonrecursive", call("ordinary", x, y), integer(0), ""},
		{"constructor argument", call("recursive", x, &term{kind: "ctor"}), integer(0), ""},
		{"tuple argument", call("recursive", x, &term{kind: "tuple", args: []*term{y}}), integer(0), ""},
		{"neutral match", &term{kind: "match", args: []*term{x}}, integer(0), "stuck on: x"},
		{"known match head", &term{kind: "match", args: []*term{integer(1)}, arms: []normalArm{{guard: y, body: integer(0)}}}, integer(0), ""},
		{"decided if", choice(boolean(true)), integer(0), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left := e.stuckOn(tc.left)
			right := e.stuckOn(tc.right)
			got := e.displayStuck(left, right)
			if got != tc.want {
				t.Errorf("stuck = %q, want %q", got, tc.want)
			}
		})
	}
	other := call("recursive", x, y)
	other.args[0].dicts = []*dictionary{{}}
	if got := e.stuckOn(other); got != nil {
		t.Fatal("a termination certificate leaked to another dictionary environment")
	}
	e.fuel, e.nextID = 0, 7
	stuck, differs := e.diagnose(choice(x), choice(y))
	if stuck != "stuck on: x\nstuck on: y" || differs != "differs at: x vs y" {
		t.Fatalf("diagnostics with no evaluation fuel = %q, %q", stuck, differs)
	}
	if e.fuel != 0 || e.nextID != 7 {
		t.Fatal("diagnostics evaluated terms or freshened their binders")
	}
}

func TestDiagnosticFirstDifference(t *testing.T) {
	variable := func(name string, id int) *term { return &term{kind: "var", name: name, id: id} }
	binder := func(name string, id int) *term { return &term{kind: "bind", name: name, id: id} }
	tuple := func(args ...*term) *term { return &term{kind: "tuple", args: args} }
	lambda := func(name string, id int, last *term) *term {
		return &term{kind: "lambda", args: []*term{binder(name, id), tuple(variable(name, id), last)}}
	}
	match := func(name string, id int, last *term) *term {
		return &term{kind: "match", args: []*term{variable("s", 1)}, arms: []normalArm{{
			pattern: binder(name, id), guard: boolean(true), body: tuple(variable(name, id), last),
		}}}
	}
	one, two, three := integer(1), integer(2), integer(3)
	x, y := variable("same", 10), variable("same", 11)
	for _, tc := range []struct {
		name                             string
		left, right, wantLeft, wantRight *term
	}{
		{"equal", tuple(one, two), tuple(one, two), nil, nil},
		{"first minimal pair", tuple(tuple(one, two), one), tuple(tuple(one, three), two), two, three},
		{"free identities", tuple(x), tuple(y), x, y},
		{"equal lambdas", lambda("a", 20, one), lambda("b", 21, one), nil, nil},
		{"lambda scope", lambda("a", 20, one), lambda("b", 21, two), one, two},
		{"match scope", match("a", 20, one), match("b", 21, two), one, two},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, right := firstDifference(tc.left, tc.right, nil)
			if left != tc.wantLeft || right != tc.wantRight {
				t.Errorf("difference = (%v, %v), want (%v, %v)", left, right, tc.wantLeft, tc.wantRight)
			}
		})
	}
	left := &term{kind: "field", name: "first", index: 0, args: []*term{x}}
	right := &term{kind: "field", name: "second", index: 1, args: []*term{y}}
	if a, b := firstDifference(left, right, nil); a != left || b != right {
		t.Fatal("a field mismatch must not descend to the record bases")
	}
	// Readback sugar ignores field spelling just as alpha-equality does.
	right = &term{kind: "field", name: "alias", index: 0, args: []*term{x}}
	if a, b := firstDifference(left, right, nil); a != nil || b != nil {
		t.Fatal("field spelling changed alpha-equality")
	}
}
