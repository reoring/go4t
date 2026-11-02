package laws

import (
	"fmt"

	"go4t/ast"
)

type judgement uint8

const (
	unknown judgement = iota
	certain
	mismatch
)

// Combining all fields, rather than stopping at the first unknown, lets a
// later definite mismatch reject an arm on a partially known tuple.
func combine(a, b judgement) judgement {
	if a == mismatch || b == mismatch {
		return mismatch
	}
	if a == unknown || b == unknown {
		return unknown
	}
	return certain
}

func (e *evaluator) judge(pattern ast.Pattern, value *term, env *environment) judgement {
	switch p := pattern.(type) {
	case *ast.PWild:
		return certain
	case *ast.PVar:
		env.values[p.Name] = value
		return certain
	case *ast.PAs:
		env.values[p.Name] = value
		return e.judge(p.Pat, value, env)
	case *ast.PLit:
		literal := literalTerm(p.X)
		if value.kind != "int" && value.kind != "bool" && value.kind != "string" {
			return unknown
		}
		if alphaEqual(literal, value, nil) {
			return certain
		}
		return mismatch
	case *ast.PTuple:
		if value.kind != "tuple" {
			return unknown
		}
		result := certain
		for i, field := range p.Elems {
			result = combine(result, e.judge(field, value.args[i], env))
		}
		return result
	case *ast.PCtor:
		if value.kind != "ctor" {
			return unknown
		}
		if value.ctor != e.ctors[p.Name] {
			return mismatch
		}
		result := certain
		for i, field := range p.Args {
			result = combine(result, e.judge(field, value.args[i], env))
		}
		return result
	case *ast.PRecord:
		if value.kind != "ctor" {
			return unknown
		}
		record := e.p.RecordPats[p]
		if value.ctor != record.Ctor {
			return mismatch
		}
		result := certain
		for i, field := range p.Pats {
			result = combine(result, e.judge(field, value.args[record.Indexes[i]], env))
		}
		return result
	}
	return unknown
}

func (e *evaluator) match(arms []ast.Arm, value *term, env *environment, dicts dictionaryEnv) (*term, error) {
	for i, arm := range arms {
		branch := child(env)
		result := e.judge(arm.Pat, value, branch)
		if result == mismatch {
			continue
		}
		if result == unknown {
			return e.residualMatch(arms[i:], value, env, dicts)
		}
		if arm.Guard != nil {
			guard, err := e.eval(arm.Guard, branch, dicts)
			if err != nil {
				return nil, err
			}
			if guard.kind != "bool" {
				return e.residualMatch(arms[i:], value, env, dicts)
			}
			if !guard.flag {
				continue
			}
		}
		return e.eval(arm.Body, branch, dicts)
	}
	return nil, fmt.Errorf("normalization reached a match with no matching arm")
}

func (e *evaluator) residualMatch(arms []ast.Arm, value *term, env *environment, dicts dictionaryEnv) (*term, error) {
	// A recursive call beneath an unknown constructor remains neutral rather
	// than generating an infinite tree of hypothetical tails.
	e.residualDepth++
	defer func() { e.residualDepth-- }()
	result := &term{kind: "match", args: []*term{value}}
	for _, arm := range arms {
		status := e.judge(arm.Pat, value, child(env))
		if status == mismatch {
			continue
		}
		branch := child(env)
		pattern, err := e.pattern(arm.Pat, branch)
		if err != nil {
			return nil, err
		}
		guard := boolean(true)
		if arm.Guard != nil {
			guard, err = e.eval(arm.Guard, branch, dicts)
			if err != nil {
				return nil, err
			}
		}
		if guard.kind == "bool" && !guard.flag {
			continue
		}
		body, err := e.eval(arm.Body, branch, dicts)
		if err != nil {
			return nil, err
		}
		result.arms = append(result.arms, normalArm{pattern: pattern, guard: guard, body: body})
		if status == certain && guard.kind == "bool" && guard.flag {
			break
		}
	}
	return result, nil
}

// pattern freshens all variables before normalizing a residual arm. IDs are
// lexical identities, never source names, so substitution cannot capture them.
func (e *evaluator) pattern(pattern ast.Pattern, env *environment) (*term, error) {
	switch p := pattern.(type) {
	case *ast.PWild:
		return &term{kind: "wild"}, nil
	case *ast.PVar:
		value := e.fresh(p.Name)
		env.values[p.Name] = value
		return &term{kind: "bind", id: value.id, name: p.Name}, nil
	case *ast.PAs:
		value := e.fresh(p.Name)
		env.values[p.Name] = value
		nested, err := e.pattern(p.Pat, env)
		return &term{kind: "as", args: []*term{{kind: "bind", id: value.id, name: p.Name}, nested}}, err
	case *ast.PLit:
		return literalTerm(p.X), nil
	case *ast.PTuple:
		args := make([]*term, len(p.Elems))
		for i, field := range p.Elems {
			value, err := e.pattern(field, env)
			if err != nil {
				return nil, err
			}
			args[i] = value
		}
		return &term{kind: "tuple", args: args}, nil
	case *ast.PCtor:
		args := make([]*term, len(p.Args))
		for i, field := range p.Args {
			value, err := e.pattern(field, env)
			if err != nil {
				return nil, err
			}
			args[i] = value
		}
		return &term{kind: "ctor", ctor: e.ctors[p.Name], args: args}, nil
	case *ast.PRecord:
		record := e.p.RecordPats[p]
		args := make([]*term, len(record.Ctor.Fields))
		for i := range args {
			args[i] = &term{kind: "wild"}
		}
		for i, field := range p.Pats {
			value, err := e.pattern(field, env)
			if err != nil {
				return nil, err
			}
			args[record.Indexes[i]] = value
		}
		return &term{kind: "ctor", ctor: record.Ctor, args: args}, nil
	}
	return nil, fmt.Errorf("unsupported pattern %T", pattern)
}

func literalTerm(expr ast.Expr) *term {
	switch x := expr.(type) {
	case *ast.IntLit:
		return integer(x.Value)
	case *ast.BoolLit:
		return boolean(x.Value)
	case *ast.StringLit:
		return text(x.Value)
	}
	return &term{kind: "unsupported-literal"}
}

// Irrefutable lambda/let patterns may destructure a free value. Projections,
// rather than unrelated fresh variables, retain the dependency on that value.
func (e *evaluator) bindPattern(pattern ast.Pattern, value *term, env *environment) error {
	switch p := pattern.(type) {
	case *ast.PWild:
		return nil
	case *ast.PVar:
		env.values[p.Name] = value
		return nil
	case *ast.PAs:
		env.values[p.Name] = value
		return e.bindPattern(p.Pat, value, env)
	case *ast.PTuple:
		for i, field := range p.Elems {
			item := &term{kind: "field", index: i, args: []*term{value}}
			if value.kind == "tuple" {
				item = value.args[i]
			}
			if err := e.bindPattern(field, item, env); err != nil {
				return err
			}
		}
		return nil
	case *ast.PCtor:
		ctor := e.ctors[p.Name]
		for i, field := range p.Args {
			item := &term{kind: "field", ctor: ctor, index: i, args: []*term{value}}
			if value.kind == "ctor" && value.ctor == ctor {
				item = value.args[i]
			}
			if err := e.bindPattern(field, item, env); err != nil {
				return err
			}
		}
		return nil
	case *ast.PRecord:
		record := e.p.RecordPats[p]
		for i, field := range p.Pats {
			index := record.Indexes[i]
			item := &term{kind: "field", ctor: record.Ctor, index: index, args: []*term{value}}
			if value.kind == "ctor" && value.ctor == record.Ctor {
				item = value.args[index]
			}
			if err := e.bindPattern(field, item, env); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported irrefutable pattern %T", pattern)
}
