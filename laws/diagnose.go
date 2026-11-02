package laws

// diagnose only reads the already-normalized goal and termination certificates.
// In particular it neither consumes fuel nor freshens binders: subterm IDs must
// remain identical to the ones in the full LHS/RHS readback.
func (e *evaluator) diagnose(left, right *term) (stuck, differs string) {
	stuck = e.displayStuck(e.stuckOn(left), e.stuckOn(right))
	if a, b := firstDifference(left, right, nil); a != nil {
		differs = "differs at: " + e.display(a) + " vs " + e.display(b)
	}
	return stuck, differs
}

func (e *evaluator) displayStuck(left, right *term) string {
	message := ""
	if left != nil {
		message = "stuck on: " + e.display(left)
	}
	if right != nil && (left == nil || !alphaEqual(left, right, nil)) {
		if message != "" {
			message += "\n"
		}
		message += "stuck on: " + e.display(right)
	}
	return message
}

// stuckOn searches outside-in, then left-to-right. A residual match on a known
// value can be blocked by a guard or a nested pattern, not by its value head.
func (e *evaluator) stuckOn(t *term) *term {
	switch t.kind {
	case "if":
		if t.args[0].kind != "bool" {
			return t.args[0]
		}
	case "match":
		switch t.args[0].kind {
		case "ctor", "tuple", "int", "string", "bool", "unit", "lambda", "map":
		default:
			return t.args[0]
		}
	case "call":
		fn := t.args[0]
		var positions []decreasingPosition
		switch fn.kind {
		case "global":
			positions = e.decreasing[fn.name]
		case "method":
			// The call already has checked dictionaries. Locate its method
			// without allocating the bound environment used by evaluation.
			for i := range fn.dicts[0].instance.Decl.Methods {
				method := &fn.dicts[0].instance.Decl.Methods[i]
				if method.Name == fn.name {
					positions = e.decreasingMethods[method]
					break
				}
			}
		}
		for _, decrease := range positions {
			if sameDictionaries(decrease.dicts, fn.dicts) {
				arg := t.args[decrease.position+1]
				if arg.kind != "ctor" && arg.kind != "tuple" {
					return arg
				}
				break
			}
		}
	}
	for _, arg := range t.args {
		if stuck := e.stuckOn(arg); stuck != nil {
			return stuck
		}
	}
	for _, arm := range t.arms {
		if stuck := e.stuckOn(arm.guard); stuck != nil {
			return stuck
		}
		if stuck := e.stuckOn(arm.body); stuck != nil {
			return stuck
		}
	}
	return nil
}

// firstDifference descends through matching heads to the first unequal child.
// Head comparison uses alphaEqual too, so constructor, field, dictionary, and
// other metadata keep exactly the same meaning as in the proof checker.
func firstDifference(left, right *term, bound map[int]int) (*term, *term) {
	if alphaEqual(left, right, bound) {
		return nil, nil
	}
	if len(left.args) != len(right.args) || len(left.arms) != len(right.arms) {
		return left, right
	}
	a, b := *left, *right
	a.args, b.args, a.arms, b.arms = nil, nil, nil, nil
	if !alphaEqual(&a, &b, bound) {
		return left, right
	}
	if left.kind == "lambda" {
		bound = copyBindings(bound)
	}
	for i, arg := range left.args {
		if a, b := firstDifference(arg, right.args[i], bound); a != nil {
			return a, b
		}
	}
	for i, arm := range left.arms {
		other := right.arms[i]
		scope := copyBindings(bound)
		if a, b := firstDifference(arm.pattern, other.pattern, scope); a != nil {
			return a, b
		}
		if a, b := firstDifference(arm.guard, other.guard, scope); a != nil {
			return a, b
		}
		if a, b := firstDifference(arm.body, other.body, scope); a != nil {
			return a, b
		}
	}
	// A polynomial coefficient or exponent can differ without any unequal
	// child term; the polynomial itself is then the minimal differing pair.
	return left, right
}
