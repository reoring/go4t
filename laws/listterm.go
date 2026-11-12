package laws

// These are replacements for the owning std/list bodies, not name-based
// rewrite rules. Keep the equations and their runtime oracle in sync with
// ADR 0014. Source dependency/termination checking still visits the real bodies.
func modeledList(name string) bool {
	switch name {
	case "length", "sum", "product", "count", "reverse", "zipWith", "unzip", "partition", "flatten", "intercalate", "init", "span", "findIndex", "indexed":
		return true
	}
	return false
}

func (e *evaluator) listModel(name string) string {
	local := e.p.ModuleNames[name]
	if local == "" {
		local = name
	}
	if modeledList(local) && e.listFunctions[local] == name {
		return local
	}
	return ""
}

func listPosition(name string) int {
	if name == "intercalate" {
		return 1
	}
	return 0
}

func (e *evaluator) unfoldList(name string, args []*term) bool {
	xs := args[listPosition(name)]
	if xs.kind != "ctor" {
		return false
	}
	if xs.ctor == e.ctors["Nil"] {
		return true
	}
	if xs.ctor != e.ctors["Cons"] {
		return false
	}
	return name != "zipWith" || args[1].kind == "ctor"
}

func termCall(fn *term, args ...*term) *term {
	return &term{kind: "call", args: append([]*term{fn}, args...)}
}

func (e *evaluator) listCall(name string, args ...*term) *term {
	f := e.p.Funcs[e.listFunctions[name]]
	return termCall(&term{kind: "global", name: f.Name, arity: len(f.Decl.Params)}, args...)
}

func (e *evaluator) listNil() *term {
	return &term{kind: "ctor", ctor: e.ctors["Nil"]}
}

func (e *evaluator) listCons(x, xs *term) *term {
	return &term{kind: "ctor", ctor: e.ctors["Cons"], args: []*term{x, xs}}
}

func termPair(a, b *term) *term { return &term{kind: "tuple", args: []*term{a, b}} }
func termField(value *term, index int) *term {
	return &term{kind: "field", index: index, args: []*term{value}}
}
func termAdd(a, b *term) *term { return &term{kind: "ring", name: "+", args: []*term{a, b}} }

// Bind a recursive pair once. Duplicating its call beneath two projections
// would make normalization exponential on concrete lists.
func termPairCase(value, a, b, body *term) *term {
	pattern := termPair(&term{kind: "bind", id: a.id, name: a.name}, &term{kind: "bind", id: b.id, name: b.name})
	return &term{kind: "match", args: []*term{value}, arms: []normalArm{
		{pattern: pattern, guard: boolean(true), body: body},
	}}
}

// Keep the tag test separate from its branches. Refining xs to Cons while
// normalizing a branch that calls the model on xs would unfold an open tail
// forever. The Boolean test preserves the same equation without that expansion.
func (e *evaluator) listCase(xs, nilValue, consValue *term) *term {
	empty := &term{kind: "match", args: []*term{xs}, arms: []normalArm{
		{pattern: e.listNil(), guard: boolean(true), body: boolean(true)},
		{pattern: e.listCons(&term{kind: "wild"}, &term{kind: "wild"}), guard: boolean(true), body: boolean(false)},
	}}
	return mapChoice(empty, nilValue, consValue)
}

func (e *evaluator) listOperation(name string, args []*term) (*term, error) {
	xs := args[listPosition(name)]
	if xs.ctor == e.ctors["Nil"] {
		switch name {
		case "length", "sum", "count":
			return integer(0), nil
		case "product":
			return integer(1), nil
		case "unzip", "partition", "span":
			return termPair(e.listNil(), e.listNil()), nil
		case "findIndex":
			return &term{kind: "ctor", ctor: e.ctors["None"]}, nil
		default:
			return e.listNil(), nil
		}
	}
	x, rest := xs.args[0], xs.args[1]
	switch name {
	case "length":
		return termAdd(integer(1), e.listCall(name, rest)), nil
	case "sum":
		return termAdd(x, e.listCall(name, rest)), nil
	case "product":
		return &term{kind: "ring", name: "*", args: []*term{x, e.listCall(name, rest)}}, nil
	case "count":
		// Writing the indicator as an addend avoids duplicating the recursive
		// call under a symbolic predicate; addition is modulo 2^64.
		return termAdd(mapChoice(termCall(args[1], x), integer(1), integer(0)), e.listCall(name, rest, args[1])), nil
	case "reverse":
		return e.listCall("append", e.listCall(name, rest), e.listCons(x, e.listNil())), nil
	case "zipWith":
		ys := args[1]
		if ys.ctor == e.ctors["Nil"] {
			return e.listNil(), nil
		}
		return e.listCons(termCall(args[2], x, ys.args[0]), e.listCall(name, rest, ys.args[1], args[2])), nil
	case "unzip":
		tail := e.listCall(name, rest)
		a, b := e.fresh("left"), e.fresh("right")
		return termPairCase(tail, a, b, termPair(e.listCons(termField(x, 0), a), e.listCons(termField(x, 1), b))), nil
	case "partition":
		tail := e.listCall(name, rest, args[1])
		yes, no := e.fresh("yes"), e.fresh("no")
		body := mapChoice(termCall(args[1], x), termPair(e.listCons(x, yes), no), termPair(yes, e.listCons(x, no)))
		return termPairCase(tail, yes, no, body), nil
	case "flatten":
		return e.listCall("append", x, e.listCall(name, rest)), nil
	case "intercalate":
		tail := e.listCall("append", args[0], e.listCall(name, args[0], rest))
		return e.listCall("append", x, e.listCase(rest, e.listNil(), tail)), nil
	case "init":
		return e.listCase(rest, e.listNil(), e.listCons(x, e.listCall(name, rest))), nil
	case "span":
		tail := e.listCall(name, rest, args[1])
		a, b := e.fresh("prefix"), e.fresh("suffix")
		body := termPairCase(tail, a, b, termPair(e.listCons(x, a), b))
		return mapChoice(termCall(args[1], x), body, termPair(e.listNil(), xs)), nil
	case "findIndex":
		none := &term{kind: "ctor", ctor: e.ctors["None"]}
		some := func(value *term) *term { return &term{kind: "ctor", ctor: e.ctors["Some"], args: []*term{value}} }
		i := e.fresh("index")
		tail := &term{kind: "match", args: []*term{e.listCall(name, rest, args[1])}, arms: []normalArm{
			{pattern: none, guard: boolean(true), body: none},
			{pattern: some(&term{kind: "bind", id: i.id, name: i.name}), guard: boolean(true), body: some(termAdd(i, integer(1)))},
		}}
		return mapChoice(termCall(args[1], x), some(integer(0)), tail), nil
	case "indexed":
		pair := e.fresh("pair")
		shift := &term{kind: "lambda", args: []*term{
			{kind: "bind", id: pair.id, name: pair.name},
			termPair(termAdd(termField(pair, 0), integer(1)), termField(pair, 1)),
		}}
		fmap := &term{kind: "method", name: "fmap", arity: 2, dicts: []*dictionary{e.listFunctor}}
		return e.listCons(termPair(integer(0), x), termCall(fmap, e.listCall(name, rest), shift)), nil
	}
	panic("unregistered list model")
}

// Resolve by module ownership once, rather than trusting an unqualified name
// (or searching the whole program at each recursive reduction).
func (e *evaluator) initListModels() {
	var owner string
	for _, module := range e.p.Modules {
		if module.ImportPath == "std/list" {
			owner = module.Path
			break
		}
	}
	e.listFunctions = make(map[string]string)
	if owner == "" {
		return
	}
	for name := range e.p.Funcs {
		if e.p.Owners[name] == owner {
			local := e.p.ModuleNames[name]
			if local == "" {
				local = name
			}
			e.listFunctions[local] = name
		}
	}
	for _, inst := range e.p.Instances {
		if inst.Class.Name == "Functor" && inst.TypeCon.Name == "List" {
			e.listFunctor = &dictionary{instance: inst}
			break
		}
	}
}
