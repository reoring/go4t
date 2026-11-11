package laws

import "fmt"

// preconditionHolds filters closed parameter bindings without assuming any
// proof hypotheses, including the law's own precondition. Stuck is not true.
func (e *evaluator) preconditionHolds(law lawGoal, env *environment) (bool, error) {
	parent := e.assumptions
	e.assumptions = nil
	defer func() { e.assumptions = parent }()
	condition, err := e.precondition(law, env)
	return err == nil && condition.kind == "bool" && condition.flag, err
}

func (e *evaluator) precondition(law lawGoal, env *environment) (*term, error) {
	if law.Requires == nil {
		return boolean(true), nil
	}
	condition, err := e.eval(law.Requires, env, nil)
	if err != nil {
		return nil, err
	}
	return e.normal(condition)
}

func (e *evaluator) assumePrecondition(law lawGoal, env *environment) error {
	if law.Requires == nil {
		return nil
	}
	condition, err := e.precondition(law, env)
	if err != nil {
		return err
	}
	// Keep the original normalized facts: re-normalizing a later conjunct can
	// change its representation under earlier bounds, but must not erase a
	// fact that existing proofs use.
	previous := false
	var assume func(*term) error
	assume = func(conjunct *term) error {
		if conjunct.kind == "binary" && conjunct.name == "&&" {
			if err := assume(conjunct.args[0]); err != nil {
				return err
			}
			return assume(conjunct.args[1])
		}
		value, err := e.normal(conjunct)
		if err != nil {
			return err
		}
		impossible, err := e.impossibleOrder(value)
		if err != nil {
			return err
		}
		if value.kind == "bool" && !value.flag || impossible {
			message := fmt.Sprintf("requires can never hold: %s is false", e.display(conjunct))
			if previous {
				message += " given the conditions before it"
			}
			return &proofFailure{node: law.Requires, message: message}
		}
		e.addAssumption(conjunct, true)
		previous = true
		return nil
	}
	return assume(condition)
}

// Conjunction elimination is valid only in a true branch. A false conjunction
// does not tell us which conjunct failed. Negation reverses the branch truth.
func (e *evaluator) addAssumption(condition *term, truth bool) {
	if condition.kind == "binary" && condition.name == "&&" && truth {
		e.addAssumption(condition.args[0], true)
		e.addAssumption(condition.args[1], true)
	} else if condition.kind == "unary" && condition.name == "!" {
		e.addAssumption(condition.args[0], !truth)
	}
	e.assumptions = &assumption{condition: condition, value: boolean(truth), parent: e.assumptions}
}
