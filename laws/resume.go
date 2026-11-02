package laws

import (
	"fmt"
	"strings"

	"go4t/ast"
)

// resume reduces eliminators whose operands became known during ring
// normalization or rewriting. Opaque recursive calls are unfolded only by
// replace when an operand actually changes, never merely by reading them back.
func (e *evaluator) resume(t *term) (*term, error) {
	switch t.kind {
	case "if":
		if t.args[0].kind == "bool" {
			if t.args[0].flag {
				return t.args[1], nil
			}
			return t.args[2], nil
		}
	case "unary":
		if t.name == "!" && t.args[0].kind == "bool" {
			return boolean(!t.args[0].flag), nil
		}
	case "binary":
		a, b := t.args[0], t.args[1]
		if a.kind == "bool" && (t.name == "&&" || t.name == "||") {
			if t.name == "&&" && !a.flag || t.name == "||" && a.flag {
				return a, nil
			}
			return b, nil
		}
		if t.name == "+" && a.kind == "string" && b.kind == "string" {
			return text(a.name+b.name), nil
		}
		if (t.name == "/" || t.name == "%") && b.kind == "int" {
			if b.number == 0 {
				return nil, fmt.Errorf("law depends on division by zero")
			}
			if a.kind == "int" {
				if t.name == "/" {
					return integer(a.number/b.number), nil
				}
				return integer(a.number%b.number), nil
			}
		}
	case "primitive":
		constant := true
		for _, arg := range t.args {
			if !constantTerm(arg) {
				constant = false
				break
			}
		}
		if constant {
			return e.extern(&ast.ExternRef{ImportPath: "go4t/rt", Name: strings.TrimPrefix(t.name, "rt.")}, t.args)
		}
	case "order":
		if t.args[0].kind == "ctor" {
			order := e.sourceName(t.args[0].ctor.Name)
			switch t.name {
			case "<":
				return boolean(order == "LT"), nil
			case "<=":
				return boolean(order != "GT"), nil
			case ">":
				return boolean(order == "GT"), nil
			case ">=":
				return boolean(order != "LT"), nil
			}
		}
	case "field":
		base := t.args[0]
		indexes := []int{t.index}
		if len(t.cases) != 0 {
			indexes = caseIndexes(base, t.cases)
		}
		if indexes != nil && (base.kind == "ctor" || base.kind == "tuple") {
			return base.args[indexes[0]], nil
		}
	case "update":
		base := t.args[0]
		indexes := t.indexes
		if len(t.cases) != 0 {
			indexes = caseIndexes(base, t.cases)
		}
		if base.kind == "ctor" && indexes != nil {
			fields := append([]*term(nil), base.args...)
			for i, value := range t.args[1:] {
				fields[indexes[i]] = value
			}
			return &term{kind: "ctor", ctor: base.ctor, args: fields}, nil
		}
	case "call":
		fn := t.args[0]
		if fn.kind == "lambda" {
			binders := fn.args[:len(fn.args)-1]
			for i, binder := range binders {
				e.solutions[binder.id] = t.args[i+1]
			}
			body, err := e.normal(fn.args[len(fn.args)-1])
			for _, binder := range binders {
				delete(e.solutions, binder.id)
			}
			return body, err
		}
	case "match":
		for _, arm := range t.arms {
			bindings := make(map[int]*term)
			switch matchNormal(arm.pattern, t.args[0], bindings) {
			case mismatch:
				continue
			case unknown:
				return t, nil
			}
			for id, value := range bindings {
				e.solutions[id] = value
			}
			guard, err := e.normal(arm.guard)
			var body *term
			if err == nil && guard.kind == "bool" && guard.flag {
				body, err = e.normal(arm.body)
			}
			for id := range bindings {
				delete(e.solutions, id)
			}
			if err != nil {
				return nil, err
			}
			if body != nil {
				return body, nil
			}
			if guard.kind != "bool" {
				return t, nil
			}
		}
		return nil, fmt.Errorf("normalization reached a match with no matching arm")
	}
	return e.assume(t), nil
}

func matchNormal(pattern, value *term, bindings map[int]*term) judgement {
	switch pattern.kind {
	case "wild":
		return certain
	case "bind":
		bindings[pattern.id] = value
		return certain
	case "as":
		bindings[pattern.args[0].id] = value
		return matchNormal(pattern.args[1], value, bindings)
	case "int", "bool", "string":
		if value.kind != pattern.kind {
			return unknown
		}
		if alphaEqual(pattern, value, nil) {
			return certain
		}
		return mismatch
	case "ctor", "tuple":
		if value.kind != pattern.kind {
			return unknown
		}
		if value.ctor != pattern.ctor {
			return mismatch
		}
		status := certain
		for i, field := range pattern.args {
			status = combine(status, matchNormal(field, value.args[i], bindings))
		}
		return status
	}
	return unknown
}

// Reading a closure back into a lambda changes its representation, not its
// arguments. Only substitution can justify reopening a neutral recursive call.
func (e *evaluator) hasSolution(t *term) bool {
	if t.kind == "var" && e.solutions[t.id] != nil {
		return true
	}
	for _, arg := range t.args {
		if e.hasSolution(arg) {
			return true
		}
	}
	for _, arm := range t.arms {
		if e.hasSolution(arm.guard) || e.hasSolution(arm.body) {
			return true
		}
	}
	return false
}
