package laws

import (
	"go4t/ast"
	"go4t/types"
)

// Modeled externs are pure but are not constant-only primitives. Unsupported
// comparators remain opaque; being eligible for a law does not make it provable.
func modeledMap(name string) bool {
	switch name {
	case "EmptyMap", "MapInsert", "MapLookup", "MapDelete", "MapMember", "MapSize", "MapFromList", "MapToList", "MapKeys", "MapValues", "MapUnion", "MapMap", "MapFold":
		return true
	}
	return false
}

func mapComparatorPosition(name string) int {
	switch name {
	case "MapInsert":
		return 3
	case "MapLookup", "MapDelete", "MapMember", "MapUnion":
		return 2
	case "MapFromList":
		return 1
	}
	return -1
}

// Inspect the checked lambda, not its extensional normal form: an arbitrary
// function which happens to return the same ordering is not an Ord witness.
func (e *evaluator) mapComparator(value *term) (*dictionary, error) {
	if value.kind != "closure" || value.arity != 2 || value.closure.block != nil {
		return nil, nil
	}
	cl := value.closure
	call, ok := cl.expr.(*ast.Call)
	if !ok || len(call.Args) != 2 {
		return nil, nil
	}
	fn, ok := call.Fn.(*ast.Ident)
	if !ok || fn.Name != "compare" || types.AnnOf(fn).Ref != types.RefMethod {
		return nil, nil
	}
	for i, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok || cl.params[i].Pat != nil || id.Name != cl.params[i].Name || types.AnnOf(id).Ref != types.RefLocal {
			return nil, nil
		}
	}
	dicts, err := e.resolveAll(types.AnnOf(fn).Dicts, cl.dicts)
	if err != nil || len(dicts) != 1 {
		return nil, err
	}
	if !e.eligibleMapDictionary(dicts[0], make(map[*types.Instance]bool)) {
		return nil, nil
	}
	return dicts[0], nil
}

// Deriving currently records provenance by sharing the owning TypeDecl's
// source position (generated InstanceDecls have no source End). Do not trust
// merely an Ord class name or a user implementation with a familiar body.
func (e *evaluator) trustedMapInstance(inst *types.Instance) bool {
	for _, module := range e.p.Modules {
		for _, file := range module.Files {
			owns := false
			for _, decl := range file.Decls {
				if decl == inst.Decl {
					owns = true
					break
				}
			}
			if !owns {
				continue
			}
			if module.ImportPath == "std/prelude" {
				if inst.TypeCon == types.Int || inst.TypeCon == types.String || inst.TypeCon == types.Bool {
					return true
				}
				if _, tuple := types.IsTuple(inst.TypeCon); tuple {
					return true
				}
			}
			if inst.Decl.End != (ast.Pos{}) {
				return false
			}
			for _, decl := range file.Decls {
				if data, ok := decl.(*ast.TypeDecl); ok && data.Name == inst.TypeCon.Name && data.Pos == inst.Decl.Pos {
					return true
				}
			}
			return false
		}
	}
	return false
}

// Follow concrete method dependencies as well as type-parameter dictionaries:
// a derived wrapper around a handwritten Ord/Eq is not a lawful key witness.
func (e *evaluator) eligibleMapDictionary(dict *dictionary, visiting map[*types.Instance]bool) bool {
	inst := dict.instance
	if inst.Class.Name != "Ord" && inst.Class.Name != "Eq" || !e.trustedMapInstance(inst) {
		return false
	}
	for _, arg := range dict.args {
		if !e.eligibleMapDictionary(arg, visiting) {
			return false
		}
	}
	if visiting[inst] {
		return true
	}
	visiting[inst] = true
	defer delete(visiting, inst)
	bound, err := bindDictionaries(nil, inst.Context, dict.args)
	if err != nil {
		return false
	}
	valid := true
	ast.Walk(inst.Decl, func(node ast.Node) {
		x, ok := node.(ast.Expr)
		if !ok || !valid {
			return
		}
		for _, ref := range types.AnnOf(x).Dicts {
			dep, err := e.resolve(ref, bound)
			if err != nil || !e.eligibleMapDictionary(dep, visiting) {
				valid = false
				return
			}
		}
	})
	if !valid {
		return false
	}
	if inst.Class.Name == "Ord" {
		eq, err := e.mapEqDictionary(dict)
		return err == nil && e.eligibleMapDictionary(eq, visiting)
	}
	return true
}

func (e *evaluator) mapEqDictionary(ord *dictionary) (*dictionary, error) {
	// Reuse exactly the superclass resolution used by checked == expressions.
	return e.superDictionary(ord, "Eq")
}

func (e *evaluator) mapEqual(ord *dictionary, a, b *term) (*term, error) {
	if alphaEqual(a, b, nil) {
		return boolean(true), nil
	}
	eq, err := e.mapEqDictionary(ord)
	if err != nil {
		return nil, err
	}
	value, err := e.method("eq", []*dictionary{eq}, []*term{a, b})
	if err != nil {
		return nil, err
	}
	return e.normal(value)
}

func literalMapKey(key *term) bool {
	switch key.kind {
	case "int", "string", "bool":
		return true
	case "tuple":
		for _, part := range key.args {
			if !literalMapKey(part) {
				return false
			}
		}
		return true
	}
	return false
}

// All keys in a well-typed map have the same type. This is rt's primitive and
// lexicographic tuple ordering, not a lexical ordering of printed terms.
func compareMapKeys(a, b *term) int {
	switch a.kind {
	case "int":
		if a.number < b.number {
			return -1
		}
		if a.number > b.number {
			return 1
		}
	case "string":
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
	case "bool":
		if !a.flag && b.flag {
			return -1
		}
		if a.flag && !b.flag {
			return 1
		}
	case "tuple":
		for i, part := range a.args {
			if order := compareMapKeys(part, b.args[i]); order != 0 {
				return order
			}
		}
	}
	return 0
}

// map.args alternates constant keys and arbitrary values, sorted and unique.
// insert.args is [base, key, value]; dictionaries retain its lawful Ord witness.
func (e *evaluator) mapInsert(base, key, value *term, ord *dictionary) *term {
	if base.kind == "map" && literalMapKey(key) {
		at := 0
		for at < len(base.args) && compareMapKeys(base.args[at], key) < 0 {
			at += 2
		}
		replace := at < len(base.args) && compareMapKeys(base.args[at], key) == 0
		n := len(base.args) + 2
		if replace {
			n -= 2
		}
		args := make([]*term, n)
		copy(args, base.args[:at])
		args[at], args[at+1] = key, value
		next := at
		if replace {
			next += 2
		}
		copy(args[at+2:], base.args[next:])
		return &term{kind: "map", args: args}
	}
	if base.kind == "insert" && sameDictionaries(base.dicts, []*dictionary{ord}) && alphaEqual(base.args[1], key, nil) {
		base = base.args[0]
	}
	return &term{kind: "insert", args: []*term{base, key, value}, dicts: []*dictionary{ord}}
}

func mapResidual(name string, args []*term, ord *dictionary) *term {
	value := &term{kind: "mapop", name: name, args: args}
	if ord != nil {
		value.dicts = []*dictionary{ord}
	}
	return value
}

func (e *evaluator) mapExtern(name string, args []*term) (*term, error) {
	var ord *dictionary
	if at := mapComparatorPosition(name); at >= 0 {
		var err error
		ord, err = e.mapComparator(args[at])
		if err != nil {
			return nil, err
		}
		if ord == nil {
			return &term{kind: "primitive", name: "rt." + name, args: args}, nil
		}
		args = args[:at]
	}
	return e.mapOperation(name, args, ord)
}

func (e *evaluator) mapLookup(base, key *term, ord *dictionary, member bool) (*term, error) {
	none := &term{kind: "ctor", ctor: e.ctors["None"]}
	some := func(value *term) *term { return &term{kind: "ctor", ctor: e.ctors["Some"], args: []*term{value}} }
	name := "MapLookup"
	if member {
		name = "MapMember"
		none = boolean(false)
		some = func(*term) *term { return boolean(true) }
	}
	if base.kind == "map" {
		result := none
		for i := len(base.args) - 2; i >= 0; i -= 2 {
			cond, err := e.mapEqual(ord, key, base.args[i])
			if err != nil {
				return nil, err
			}
			result = mapChoice(cond, some(base.args[i+1]), result)
		}
		return result, nil
	}
	if base.kind == "insert" && sameDictionaries(base.dicts, []*dictionary{ord}) {
		cond, err := e.mapEqual(ord, key, base.args[1])
		if err != nil {
			return nil, err
		}
		if cond.kind == "bool" && cond.flag {
			return some(base.args[2]), nil
		}
		rest, err := e.mapLookup(base.args[0], key, ord, member)
		if err != nil {
			return nil, err
		}
		return mapChoice(cond, some(base.args[2]), rest), nil
	}
	// A symbolic delete is observable through the same lawful dictionary,
	// even though its size and underlying map remain neutral.
	if base.kind == "mapop" && base.name == "MapDelete" && sameDictionaries(base.dicts, []*dictionary{ord}) {
		cond, err := e.mapEqual(ord, key, base.args[1])
		if err != nil {
			return nil, err
		}
		if cond.kind == "bool" && cond.flag {
			return none, nil
		}
		rest, err := e.mapLookup(base.args[0], key, ord, member)
		if err != nil {
			return nil, err
		}
		return mapChoice(cond, none, rest), nil
	}
	return mapResidual(name, []*term{base, key}, ord), nil
}

func mapChoice(condition, yes, no *term) *term {
	if condition.kind == "bool" {
		if condition.flag {
			return yes
		}
		return no
	}
	return &term{kind: "if", args: []*term{condition, yes, no}}
}

func (e *evaluator) mapOperation(name string, args []*term, ord *dictionary) (*term, error) {
	if err := e.step(); err != nil {
		return nil, err
	}
	if name == "EmptyMap" {
		return &term{kind: "map"}, nil
	}
	base := args[0]
	switch name {
	case "MapInsert":
		return e.mapInsert(base, args[1], args[2], ord), nil
	case "MapLookup", "MapMember":
		return e.mapLookup(base, args[1], ord, name == "MapMember")
	case "MapDelete":
		key := args[1]
		if base.kind == "map" && literalMapKey(key) {
			for i := 0; i < len(base.args); i += 2 {
				if compareMapKeys(base.args[i], key) == 0 {
					fields := make([]*term, len(base.args)-2)
					copy(fields, base.args[:i])
					copy(fields[i:], base.args[i+2:])
					return &term{kind: "map", args: fields}, nil
				}
			}
			return base, nil
		}
		if base.kind == "insert" && sameDictionaries(base.dicts, []*dictionary{ord}) {
			condition, err := e.mapEqual(ord, key, base.args[1])
			if err != nil {
				return nil, err
			}
			rest, err := e.mapOperation(name, []*term{base.args[0], key}, ord)
			if err != nil {
				return nil, err
			}
			return mapChoice(condition, rest, e.mapInsert(rest, base.args[1], base.args[2], ord)), nil
		}
		if base.kind == "mapop" && base.name == name && sameDictionaries(base.dicts, []*dictionary{ord}) && alphaEqual(base.args[1], key, nil) {
			return base, nil
		}
	case "MapSize":
		if base.kind == "map" {
			return integer(int64(len(base.args) / 2)), nil
		}
		if base.kind == "insert" {
			member, err := e.mapLookup(base.args[0], base.args[1], base.dicts[0], true)
			if err != nil {
				return nil, err
			}
			size, err := e.mapOperation(name, base.args[:1], nil)
			if err != nil {
				return nil, err
			}
			return mapChoice(member, size, &term{kind: "ring", name: "+", args: []*term{size, integer(1)}}), nil
		}
	case "MapFromList":
		return e.mapFromList(&term{kind: "map"}, base, ord)
	case "MapToList", "MapKeys", "MapValues":
		if base.kind == "map" {
			result := &term{kind: "ctor", ctor: e.ctors["Nil"]}
			for i := len(base.args) - 2; i >= 0; i -= 2 {
				value := base.args[i]
				if name == "MapValues" {
					value = base.args[i+1]
				}
				if name == "MapToList" {
					value = &term{kind: "tuple", args: base.args[i : i+2]}
				}
				result = &term{kind: "ctor", ctor: e.ctors["Cons"], args: []*term{value, result}}
			}
			return result, nil
		}
	case "MapUnion":
		if base.kind == "map" && args[1].kind == "map" {
			result := args[1]
			for i := 0; i < len(base.args); i += 2 {
				result = e.mapInsert(result, base.args[i], base.args[i+1], ord)
			}
			return result, nil
		}
	case "MapMap":
		if base.kind == "map" {
			fields := make([]*term, len(base.args))
			for i := 0; i < len(base.args); i += 2 {
				value, err := e.apply(args[1], base.args[i+1:i+2])
				if err != nil {
					return nil, err
				}
				fields[i], fields[i+1] = base.args[i], value
			}
			return &term{kind: "map", args: fields}, nil
		}
	case "MapFold":
		if base.kind == "map" {
			result := args[1]
			for i := 0; i < len(base.args); i += 2 {
				var err error
				result, err = e.apply(args[2], []*term{result, base.args[i], base.args[i+1]})
				if err != nil {
					return nil, err
				}
			}
			return result, nil
		}
	}
	return mapResidual(name, args, ord), nil
}

// A left fold preserves last-write-wins even when a known prefix has an open
// tail. The residual retains that accumulator; reversing inserts is unsound.
func (e *evaluator) mapFromList(base, list *term, ord *dictionary) (*term, error) {
	for list.kind == "ctor" && list.ctor == e.ctors["Cons"] && list.args[0].kind == "tuple" {
		if err := e.step(); err != nil {
			return nil, err
		}
		pair := list.args[0]
		base = e.mapInsert(base, pair.args[0], pair.args[1], ord)
		list = list.args[1]
	}
	if list.kind == "ctor" && list.ctor == e.ctors["Nil"] {
		return base, nil
	}
	return &term{kind: "mapfrom", args: []*term{base, list}, dicts: []*dictionary{ord}}, nil
}
