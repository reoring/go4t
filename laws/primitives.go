package laws

import (
	"fmt"
	"strconv"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

type dictionary struct {
	instance *types.Instance
	args     []*dictionary
}

type dictionaryEnv map[types.Constraint]*dictionary

func bindDictionaries(parent dictionaryEnv, constraints []types.Constraint, args []*dictionary) (dictionaryEnv, error) {
	if len(constraints) != len(args) {
		return nil, fmt.Errorf("dictionary arity mismatch: need %d, got %d", len(constraints), len(args))
	}
	if len(constraints) == 0 {
		return parent, nil
	}
	env := make(dictionaryEnv, len(parent)+len(constraints))
	for key, value := range parent {
		env[key] = value
	}
	for i, constraint := range constraints {
		env[constraint] = args[i]
	}
	return env, nil
}

func (e *evaluator) resolveAll(refs []types.DictRef, env dictionaryEnv) ([]*dictionary, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	result := make([]*dictionary, len(refs))
	for i, ref := range refs {
		dict, err := e.resolve(ref, env)
		if err != nil {
			return nil, err
		}
		result[i] = dict
	}
	return result, nil
}

func (e *evaluator) resolve(ref types.DictRef, env dictionaryEnv) (*dictionary, error) {
	if err := e.step(); err != nil {
		return nil, err
	}
	switch ref.Kind {
	case types.DictParam:
		if dict := env[types.Constraint{Class: ref.ParamClass, Arg: ref.Param}]; dict != nil {
			return dict, nil
		}
		return nil, fmt.Errorf("unresolved dictionary parameter %s", ref.ParamClass)
	case types.DictInstance:
		if ref.Instance == nil {
			return nil, fmt.Errorf("missing dictionary instance")
		}
		args, err := e.resolveAll(ref.Args, env)
		return &dictionary{instance: ref.Instance, args: args}, err
	case types.DictSuper:
		if ref.Super == nil {
			return nil, fmt.Errorf("missing superclass dictionary")
		}
		dict, err := e.resolve(*ref.Super, env)
		if err != nil {
			return nil, err
		}
		return e.superDictionary(dict, ref.Class)
	}
	return nil, fmt.Errorf("invalid dictionary annotation")
}

func (e *evaluator) superDictionary(dict *dictionary, class string) (*dictionary, error) {
	for dict.instance.Class.Name != class {
		inst := dict.instance
		var super *types.Instance
		for _, candidate := range e.p.Instances {
			if candidate.Class.Name == inst.Class.Super && candidate.TypeCon == inst.TypeCon {
				super = candidate
				break
			}
		}
		if super == nil {
			return nil, fmt.Errorf("missing superclass instance %s", class)
		}
		bound, err := bindDictionaries(nil, inst.Context, dict.args)
		if err != nil {
			return nil, err
		}
		args, err := e.resolveAll(inst.SuperArgs, bound)
		if err != nil {
			return nil, err
		}
		dict = &dictionary{instance: super, args: args}
	}
	return dict, nil
}

func (e *evaluator) methodBody(name string, dicts []*dictionary) (*ast.InstMethod, dictionaryEnv, error) {
	if len(dicts) == 0 {
		return nil, nil, fmt.Errorf("method %s has no dictionary", name)
	}
	receiver := dicts[0]
	inst := receiver.instance
	bound, err := bindDictionaries(nil, inst.Context, receiver.args)
	if err != nil {
		return nil, nil, err
	}
	for _, method := range inst.Class.Methods {
		if method.Name == name {
			bound, err = bindDictionaries(bound, method.Scheme.Constraints, dicts[1:])
			if err != nil {
				return nil, nil, err
			}
			for i := range inst.Decl.Methods {
				decl := &inst.Decl.Methods[i]
				if decl.Name == name {
					return decl, bound, nil
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("dictionary %s has no implementation of %s", inst.Class.Name, name)
}

func (e *evaluator) method(name string, dicts []*dictionary, args []*term) (*term, error) {
	if err := e.step(); err != nil {
		return nil, err
	}
	method, bound, err := e.methodBody(name, dicts)
	if err != nil {
		return nil, err
	}
	if !e.unfoldMethod(method, dicts, args) {
		fn := &term{kind: "method", name: name, dicts: dicts, arity: len(args)}
		return &term{kind: "call", args: append([]*term{fn}, args...)}, nil
	}
	env := child(nil)
	for i, param := range method.Params {
		env.values[param] = args[i]
	}
	return e.block(method.Body, env, bound)
}

func (e *evaluator) unfoldMethod(method *ast.InstMethod, dicts []*dictionary, args []*term) bool {
	for _, decrease := range e.decreasingMethods[method] {
		if sameDictionaries(decrease.dicts, dicts) {
			arg := args[decrease.position]
			return arg.kind == "ctor" || arg.kind == "tuple"
		}
	}
	return true
}

func primitive(ref *ast.ExternRef) bool {
	if ref.ImportPath != "github.com/reoring/go4t/rt" {
		return false
	}
	switch ref.Name {
	case "EqInt", "LtInt", "EqString", "LtString", "EqBool", "ShowInt", "ShowBool", "ShowString", "DebugString", "UnitValue":
		return true
	}
	return stringPrimitive(ref.Name) || modeledMap(ref.Name)
}

func externError(ref *ast.ExternRef) error {
	return fmt.Errorf("law depends on extern %s.%s", ref.ImportPath, ref.Name)
}

func (e *evaluator) extern(ref *ast.ExternRef, args []*term) (*term, error) {
	if !primitive(ref) {
		return nil, externError(ref)
	}
	if modeledMap(ref.Name) {
		return e.mapExtern(ref.Name, args)
	}
	if ref.Name == "UnitValue" {
		return &term{kind: "unit"}, nil
	}
	constant := true
	for _, arg := range args {
		if !constantTerm(arg) {
			constant = false
		}
	}
	if !constant {
		// Reflexive comparisons decide even on symbolic operands: Int and
		// String equality is total and antisymmetric at runtime (Float is
		// excluded from laws), so EqX(a, a) is true and LtX(a, a) is false.
		switch ref.Name {
		case "EqInt", "EqString", "EqBool":
			if alphaEqual(args[0], args[1], map[int]int{}) {
				return boolean(true), nil
			}
			if termKey(args[0]) > termKey(args[1]) {
				args = []*term{args[1], args[0]}
			}
		case "LtInt", "LtString":
			if alphaEqual(args[0], args[1], map[int]int{}) {
				return boolean(false), nil
			}
		}
		return &term{kind: "primitive", name: "rt." + ref.Name, args: args}, nil
	}
	if stringPrimitive(ref.Name) {
		return e.stringExtern(ref.Name, args), nil
	}
	switch ref.Name {
	case "EqInt":
		return boolean(args[0].number == args[1].number), nil
	case "LtInt":
		return boolean(args[0].number < args[1].number), nil
	case "EqString":
		return boolean(args[0].name == args[1].name), nil
	case "LtString":
		return boolean(args[0].name < args[1].name), nil
	case "EqBool":
		return boolean(args[0].flag == args[1].flag), nil
	case "ShowInt":
		return text(strconv.FormatInt(args[0].number, 10)), nil
	case "ShowBool":
		return text(strconv.FormatBool(args[0].flag)), nil
	case "ShowString":
		return args[0], nil
	case "DebugString":
		return text(strconv.Quote(args[0].name)), nil
	}
	return nil, externError(ref)
}

func unary(op string, value *term) *term {
	if op == "!" && value.kind == "bool" {
		return boolean(!value.flag)
	}
	if op == "-" && value.kind == "int" {
		return integer(-value.number)
	}
	return &term{kind: "unary", name: op, args: []*term{value}}
}

func addString(a, b *term) *term {
	if a.kind == "string" && b.kind == "string" {
		return text(a.name + b.name)
	}
	return &term{kind: "binary", name: "+", args: []*term{a, b}}
}

func (e *evaluator) binary(x *ast.Binary, env *environment, dicts dictionaryEnv) (*term, error) {
	left, err := e.eval(x.L, env, dicts)
	if err != nil {
		return nil, err
	}
	// Bool operators are short-circuiting, unlike ordinary function arguments.
	if left.kind == "bool" {
		if x.Op == "&&" && !left.flag || x.Op == "||" && left.flag {
			return left, nil
		}
	}
	right, err := e.eval(x.R, env, dicts)
	if err != nil {
		return nil, err
	}
	if x.Op == "&&" || x.Op == "||" {
		if left.kind == "bool" {
			return right, nil
		}
		return &term{kind: "binary", name: x.Op, args: []*term{left, right}}, nil
	}
	switch x.Op {
	case "==", "!=", "<", "<=", ">", ">=":
		ds, err := e.resolveAll(types.AnnOf(x).Dicts, dicts)
		if err != nil {
			return nil, err
		}
		// Only the trusted scalar dictionaries implement the mathematical
		// total orders used by the proof checker. Keep their comparisons
		// explicit instead of residualizing the prelude's compare branches.
		if len(ds) == 1 && e.trustedMapInstance(ds[0].instance) &&
			(ds[0].instance.TypeCon == types.Int || ds[0].instance.TypeCon == types.String) {
			domain := "Int"
			if ds[0].instance.TypeCon == types.String {
				domain = "String"
			}
			return scalarComparison(domain, x.Op, left, right), nil
		}
		op := x.Op
		method := "compare"
		if x.Op == "==" || x.Op == "!=" {
			method = "eq"
		}
		result, err := e.method(method, ds, []*term{left, right})
		if err != nil {
			return nil, err
		}
		if x.Op == "==" {
			return result, nil
		}
		if x.Op == "!=" {
			return unary("!", result), nil
		}
		if result.kind == "ctor" {
			order := e.sourceName(result.ctor.Name)
			switch op {
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
		return &term{kind: "order", name: op, args: []*term{result}}, nil
	}
	if x.Op == "/" || x.Op == "%" {
		if right.kind != "int" {
			return nil, fmt.Errorf("law depends on division by a nonconstant divisor")
		}
		if right.number == 0 {
			return nil, fmt.Errorf("law depends on division by zero")
		}
	}
	if left.kind == "int" && right.kind == "int" {
		switch x.Op {
		case "+":
			return integer(left.number + right.number), nil
		case "-":
			return integer(left.number - right.number), nil
		case "*":
			return integer(left.number * right.number), nil
		case "/":
			return integer(left.number / right.number), nil
		case "%":
			return integer(left.number % right.number), nil
		}
	}
	if x.Op == "+" && types.Unalias(types.AnnOf(x).Type) == types.String {
		return addString(left, right), nil
	}
	if x.Op == "+" || x.Op == "-" || x.Op == "*" {
		return &term{kind: "ring", name: x.Op, args: []*term{left, right}}, nil
	}
	return &term{kind: "binary", name: x.Op, args: []*term{left, right}}, nil
}
