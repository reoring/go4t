package laws

import (
	"fmt"
	"strings"
)

// contradiction replays each fact with only its chronological predecessors.
// In particular assume must never rewrite a fact using that fact itself, nor a
// later fact whose own derivation depended on it. Solutions are case refinements
// and remain in scope throughout this replay.
func (e *evaluator) contradiction() (condition, value *term, err error) {
	parent := e.assumptions
	defer func() { e.assumptions = parent }()
	var facts []*assumption
	for a := parent; a != nil; a = a.parent {
		facts = append(facts, a)
	}
	for i := len(facts) - 1; i >= 0; i-- {
		a := facts[i]
		e.assumptions = a.parent
		actual, err := e.normal(e.instantiate(a.condition))
		if err != nil {
			return nil, nil, err
		}
		assumed, err := e.normal(e.instantiate(a.value))
		if err != nil {
			return nil, nil, err
		}
		if contradictoryValues(actual, assumed) {
			return a.condition, actual, nil
		}
		if assumed.kind == "bool" {
			comparison := actual
			if !assumed.flag {
				comparison = unary("!", comparison)
			}
			impossible, err := e.impossibleOrder(comparison)
			if err != nil {
				return nil, nil, err
			}
			if impossible {
				return a.condition, boolean(!assumed.flag), nil
			}
		}
	}
	return nil, nil, nil
}

// Different constructor heads (also inside tuples/constructors) and opposite
// Bool values are disjoint. A stuck field or a merely unequal pair of neutral
// terms is never a contradiction certificate.
func contradictoryValues(actual, assumed *term) bool {
	if actual.kind != assumed.kind {
		return false
	}
	switch actual.kind {
	case "bool":
		return actual.flag != assumed.flag
	case "ctor", "tuple":
		if actual.ctor != assumed.ctor {
			return true
		}
		for i, field := range actual.args {
			if contradictoryValues(field, assumed.args[i]) {
				return true
			}
		}
	}
	return false
}

func (e *evaluator) absurd() error {
	condition, _, err := e.contradiction()
	if err != nil || condition != nil {
		return err
	}
	var facts []*assumption
	var terms []*term
	for a := e.assumptions; a != nil; a = a.parent {
		facts = append(facts, a)
		terms = append(terms, a.condition, a.value)
	}
	d := e.displayTerms(terms...)
	var message strings.Builder
	message.WriteString("absurd: the assumptions in this case are not contradictory")
	for i := len(facts) - 1; i >= 0; i-- {
		a := facts[i]
		fmt.Fprintf(&message, "\nassumed: %s is %s", d.display(a.condition), d.display(a.value))
	}
	return fmt.Errorf("%s", message.String())
}
