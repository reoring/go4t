package ast

import (
	"strings"
	"testing"
)

func show(p Pattern) string {
	switch p := p.(type) {
	case *PVar:
		return p.Name
	case *PWild:
		return "_"
	case *PCtor:
		if len(p.Args) == 0 {
			return p.Name
		}
		return p.Name + "(" + showAll(p.Args) + ")"
	case *PTuple:
		return "(" + showAll(p.Elems) + ")"
	case *PRecord:
		fields := make([]string, len(p.Pats))
		for i, pat := range p.Pats {
			fields[i] = p.Names[i] + ": " + show(pat)
		}
		rest := ""
		if p.Rest {
			rest = ", .."
		}
		return p.Ctor + "{" + strings.Join(fields, ", ") + rest + "}"
	case *PAs:
		return p.Name + " @ " + show(p.Pat)
	case *POr:
		alts := make([]string, len(p.Alts))
		for i, alt := range p.Alts {
			alts[i] = show(alt)
		}
		return strings.Join(alts, " | ")
	}
	return "?"
}

func showAll(ps []Pattern) string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = show(p)
	}
	return strings.Join(out, ", ")
}

func ctor(name string, args ...Pattern) Pattern { return &PCtor{Name: name, Args: args} }
func or(alts ...Pattern) Pattern                { return &POr{Alts: alts} }

func TestAlternatives(t *testing.T) {
	x := &PVar{Name: "x"}
	for _, test := range []struct {
		pattern Pattern
		want    string
	}{
		{ctor("A"), "A"},
		{or(ctor("A"), ctor("B")), "A; B"},
		{or(ctor("A"), or(ctor("B"), ctor("C"))), "A; B; C"},
		{ctor("Some", or(ctor("A"), ctor("B"))), "Some(A); Some(B)"},
		// Order is the order of the equivalent consecutive arms: the leftmost
		// alternative varies slowest.
		{&PTuple{Elems: []Pattern{or(ctor("A"), ctor("B")), or(ctor("C"), ctor("D"))}}, "(A, C); (A, D); (B, C); (B, D)"},
		{&PRecord{Ctor: "R", Names: []string{"f", "g"}, Pats: []Pattern{x, or(ctor("A"), ctor("B"))}, Rest: true}, "R{f: x, g: A, ..}; R{f: x, g: B, ..}"},
		{&PAs{Name: "whole", Pat: ctor("Some", or(ctor("A"), ctor("B")))}, "whole @ Some(A); whole @ Some(B)"},
		{ctor("Cons", ctor("Some", or(x, x)), &PWild{}), "Cons(Some(x), _); Cons(Some(x), _)"},
	} {
		got := Alternatives(test.pattern)
		parts := make([]string, len(got))
		for i, p := range got {
			parts[i] = show(p)
		}
		if joined := strings.Join(parts, "; "); joined != test.want {
			t.Errorf("Alternatives(%s) = %s, want %s", show(test.pattern), joined, test.want)
		}
	}
}

// A pattern without alternatives is returned as the same node, and an
// expansion reuses the subpatterns that contain none.
func TestAlternativesSharesOrFreeNodes(t *testing.T) {
	plain := ctor("Cons", &PVar{Name: "x"}, &PWild{})
	if got := Alternatives(plain); len(got) != 1 || got[0] != plain {
		t.Fatalf("or-free pattern was copied: %v", got)
	}
	shared := ctor("Leaf", &PVar{Name: "n"})
	got := Alternatives(&PTuple{Elems: []Pattern{shared, or(ctor("A"), ctor("B"))}})
	if len(got) != 2 || got[0].(*PTuple).Elems[0] != shared || got[1].(*PTuple).Elems[0] != shared {
		t.Fatalf("or-free subpattern was not shared: %v", got)
	}
}
