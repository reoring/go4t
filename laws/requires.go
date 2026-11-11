package laws

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
	e.addAssumption(condition, true)
	return nil
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
