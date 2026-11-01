package types

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gofx/ast"
)

// expandDeriving appends ordinary instances, so their fields and bodies use the
// same inference and dictionary resolution as handwritten instances. Consume
// the clauses only after the whole file expands successfully: checking the same
// AST again must not append another copy of its instances.
func expandDeriving(f *ast.File) error {
	var instances []ast.Decl
	for _, decl := range f.Decls {
		d, ok := decl.(*ast.TypeDecl)
		if !ok {
			continue
		}
		seen := make(map[string]bool, len(d.Deriving))
		for _, class := range d.Deriving {
			switch class {
			case "Eq", "Ord", "Debug", "Functor", "Foldable", "Traversable", "Encode", "Decode":
			case "Show":
				return &Error{File: f.Name, Pos: d.Pos, Msg: "deriving Show is not supported; derive Debug and write instance Show if a display form is needed"}
			default:
				return &Error{File: f.Name, Pos: d.Pos, Msg: fmt.Sprintf("cannot derive %s for %s: unknown class", class, d.Name)}
			}
			if seen[class] {
				return &Error{File: f.Name, Pos: d.Pos, Msg: fmt.Sprintf("duplicate deriving class %s for %s", class, d.Name)}
			}
			seen[class] = true
			if class == "Eq" || class == "Ord" {
				for _, ctor := range d.Ctors {
					for _, field := range ctor.Fields {
						if _, function := field.(*ast.FuncType); function {
							return &Error{File: f.Name, Pos: d.Pos, Msg: fmt.Sprintf("cannot derive %s for %s: field of function type", class, d.Name)}
						}
					}
				}
			}
			if class == "Encode" || class == "Decode" {
				for _, ctor := range d.Ctors {
					for _, field := range ctor.Fields {
						if deriveJSONFunction(field) {
							return &Error{File: f.Name, Pos: d.Pos, Msg: fmt.Sprintf("cannot derive %s for %s: field of function type", class, d.Name)}
						}
					}
				}
			}
			inst := &ast.InstanceDecl{Pos: d.Pos, Class: class, TypeCon: d.Name, Params: d.TParams}
			higher := class == "Functor" || class == "Foldable" || class == "Traversable"
			if higher {
				if err := validateDeriveLast(f.Name, d, class); err != nil {
					return err
				}
				inst.Params = nil
				inst.Fixed = d.TParams[:len(d.TParams)-1]
			} else if len(d.TParams) != 0 {
				inst.Context = make([]ast.Constraint, len(d.TParams))
				for i, param := range d.TParams {
					inst.Context[i] = ast.Constraint{Pos: d.Pos, Class: class, Param: param}
				}
			}
			method := ast.InstMethod{Pos: d.Pos, Params: []string{"a", "b"}, Body: &ast.Block{Pos: d.Pos}}
			switch class {
			case "Eq":
				method.Name, method.Body.Result = "eq", deriveEq(d)
			case "Ord":
				method.Name, method.Body.Result = "compare", deriveOrd(d)
			case "Debug":
				method.Name, method.Params, method.Body.Result = "debug", []string{"a"}, deriveDebug(d)
			case "Encode":
				method.Name, method.Params, method.Body.Result = "encode", []string{"a"}, deriveEncode(d)
			case "Decode":
				method.Name, method.Params, method.Body.Result = "decode", []string{"j"}, deriveDecode(d)
			case "Functor":
				method.Name, method.Params = "fmap", []string{"a", "f"}
				method.Body.Result = deriveLast(d, class)
			case "Foldable":
				method.Name, method.Params = "foldr", []string{"a", "init", "f"}
				method.Body.Result = deriveLast(d, class)
			case "Traversable":
				method.Name, method.Params = "traverse", []string{"a", "f"}
				method.Body.Result = deriveLast(d, class)
			}
			inst.Methods = []ast.InstMethod{method}
			instances = append(instances, inst)
		}
	}
	f.Decls = append(f.Decls, instances...)
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.TypeDecl); ok {
			d.Deriving = nil
		}
	}
	return nil
}

func deriveIdent(pos ast.Pos, name string) *ast.Ident {
	return &ast.Ident{ExprBase: ast.ExprBase{Pos: pos}, Name: name}
}

// Empty prefix ignores fields in constructor-tag ordering arms. Record
// constructors also support positional patterns, preserving declaration order.
func deriveCtorPattern(pos ast.Pos, ctor ast.Ctor, prefix string) *ast.PCtor {
	p := &ast.PCtor{Pos: pos, Name: ctor.Name, Args: make([]ast.Pattern, len(ctor.Fields))}
	for i := range p.Args {
		if prefix == "" {
			p.Args[i] = &ast.PWild{Pos: pos}
		} else {
			p.Args[i] = &ast.PVar{Pos: pos, Name: prefix + strconv.Itoa(i)}
		}
	}
	return p
}

func derivePairMatch(pos ast.Pos) *ast.Match {
	base := ast.ExprBase{Pos: pos}
	return &ast.Match{ExprBase: base, Scrut: &ast.TupleLit{
		ExprBase: base, Elems: []ast.Expr{deriveIdent(pos, "a"), deriveIdent(pos, "b")},
	}}
}

func deriveEq(d *ast.TypeDecl) ast.Expr {
	base := ast.ExprBase{Pos: d.Pos}
	m := derivePairMatch(d.Pos)
	for _, ctor := range d.Ctors {
		var body ast.Expr
		for i := range ctor.Fields {
			suffix := strconv.Itoa(i)
			eq := &ast.Binary{ExprBase: base, Op: "==", L: deriveIdent(d.Pos, "x"+suffix), R: deriveIdent(d.Pos, "y"+suffix)}
			if body == nil {
				body = eq
			} else {
				body = &ast.Binary{ExprBase: base, Op: "&&", L: body, R: eq}
			}
		}
		if body == nil {
			body = &ast.BoolLit{ExprBase: base, Value: true}
		}
		m.Arms = append(m.Arms, ast.Arm{Pos: d.Pos, Pat: &ast.PTuple{Pos: d.Pos, Elems: []ast.Pattern{
			deriveCtorPattern(d.Pos, ctor, "x"), deriveCtorPattern(d.Pos, ctor, "y"),
		}}, Body: body})
	}
	if len(d.Ctors) > 1 {
		m.Arms = append(m.Arms, ast.Arm{Pos: d.Pos, Pat: &ast.PWild{Pos: d.Pos}, Body: &ast.BoolLit{ExprBase: base, Value: false}})
	}
	return m
}

func deriveOrd(d *ast.TypeDecl) ast.Expr {
	base := ast.ExprBase{Pos: d.Pos}
	m := derivePairMatch(d.Pos)
	for index, ctor := range d.Ctors {
		var lex ast.Expr = deriveIdent(d.Pos, "EQ")
		for i := len(ctor.Fields) - 1; i >= 0; i-- {
			suffix := strconv.Itoa(i)
			lex = &ast.Match{ExprBase: base, Scrut: &ast.Call{
				ExprBase: base, Fn: deriveIdent(d.Pos, "compare"),
				Args: []ast.Expr{deriveIdent(d.Pos, "x"+suffix), deriveIdent(d.Pos, "y"+suffix)},
			}, Arms: []ast.Arm{
				{Pos: d.Pos, Pat: &ast.PCtor{Pos: d.Pos, Name: "EQ"}, Body: lex},
				{Pos: d.Pos, Pat: &ast.PVar{Pos: d.Pos, Name: "o"}, Body: deriveIdent(d.Pos, "o")},
			}}
		}
		m.Arms = append(m.Arms, ast.Arm{Pos: d.Pos, Pat: &ast.PTuple{Pos: d.Pos, Elems: []ast.Pattern{
			deriveCtorPattern(d.Pos, ctor, "x"), deriveCtorPattern(d.Pos, ctor, "y"),
		}}, Body: lex})
		// Once earlier tags have failed, a remaining occurrence of this tag
		// precedes every other constructor. The final tag needs no fallback.
		if index < len(d.Ctors)-1 {
			m.Arms = append(m.Arms,
				ast.Arm{Pos: d.Pos, Pat: &ast.PTuple{Pos: d.Pos, Elems: []ast.Pattern{
					deriveCtorPattern(d.Pos, ctor, ""), &ast.PWild{Pos: d.Pos},
				}}, Body: deriveIdent(d.Pos, "LT")},
				ast.Arm{Pos: d.Pos, Pat: &ast.PTuple{Pos: d.Pos, Elems: []ast.Pattern{
					&ast.PWild{Pos: d.Pos}, deriveCtorPattern(d.Pos, ctor, ""),
				}}, Body: deriveIdent(d.Pos, "GT")},
			)
		}
	}
	return m
}

func deriveDebug(d *ast.TypeDecl) ast.Expr {
	base := ast.ExprBase{Pos: d.Pos}
	m := &ast.Match{ExprBase: base, Scrut: deriveIdent(d.Pos, "a")}
	for _, ctor := range d.Ctors {
		var body ast.Expr
		if len(ctor.Fields) == 0 {
			body = &ast.StringLit{ExprBase: base, Value: ctor.Name}
		} else {
			interp := &ast.Interp{ExprBase: base, Texts: make([]string, len(ctor.Fields)+1), Exprs: make([]ast.Expr, len(ctor.Fields))}
			for i := range ctor.Fields {
				interp.Exprs[i] = deriveCall(d.Pos, "debug", deriveIdent(d.Pos, "x"+strconv.Itoa(i)))
				if i > 0 {
					interp.Texts[i] = ", "
				}
				if ctor.FieldNames != nil {
					interp.Texts[i] += ctor.FieldNames[i] + ": "
				}
			}
			if ctor.FieldNames == nil {
				interp.Texts[0] = ctor.Name + "(" + interp.Texts[0]
				interp.Texts[len(ctor.Fields)] = ")"
			} else {
				interp.Texts[0] = ctor.Name + "{" + interp.Texts[0]
				interp.Texts[len(ctor.Fields)] = "}"
			}
			body = interp
		}
		m.Arms = append(m.Arms, ast.Arm{Pos: d.Pos, Pat: deriveCtorPattern(d.Pos, ctor, "x"), Body: body})
	}
	return m
}

// Higher-kinded derivation transforms only occurrences of the last parameter.
// Leading parameters and fields not mentioning the last parameter are preserved.
func validateDeriveLast(file string, d *ast.TypeDecl, class string) error {
	if len(d.TParams) == 0 {
		return &Error{File: file, Pos: d.Pos, Msg: fmt.Sprintf("cannot derive %s for %s: requires a last type parameter", class, d.Name)}
	}
	last := d.TParams[len(d.TParams)-1]
	for _, ctor := range d.Ctors {
		for i, field := range ctor.Fields {
			if reason := derivePositionError(field, last); reason != "" {
				name := strconv.Itoa(i + 1)
				if ctor.FieldNames != nil {
					name = ctor.FieldNames[i]
				}
				return &Error{File: file, Pos: d.Pos, Msg: fmt.Sprintf("cannot derive %s for %s: constructor %s field %s: %s", class, d.Name, ctor.Name, name, reason)}
			}
		}
	}
	return nil
}

func deriveContains(t ast.Type, param string) bool {
	switch t := t.(type) {
	case *ast.TypeName:
		if t.Name == param {
			return true
		}
		for _, arg := range t.Args {
			if deriveContains(arg, param) {
				return true
			}
		}
	case *ast.TupleType:
		for _, elem := range t.Elems {
			if deriveContains(elem, param) {
				return true
			}
		}
	case *ast.FuncType:
		for _, arg := range t.Params {
			if deriveContains(arg, param) {
				return true
			}
		}
		return deriveContains(t.Ret, param)
	}
	return false
}

func derivePositionError(t ast.Type, param string) string {
	if !deriveContains(t, param) {
		return ""
	}
	switch t := t.(type) {
	case *ast.FuncType:
		return "type parameter " + param + " occurs in a function type"
	case *ast.TypeName:
		for i, arg := range t.Args {
			if i < len(t.Args)-1 && deriveContains(arg, param) {
				return "type parameter " + param + " occurs as a non-last argument of " + t.Name
			}
			if reason := derivePositionError(arg, param); reason != "" {
				return reason
			}
		}
	case *ast.TupleType:
		for _, elem := range t.Elems {
			if reason := derivePositionError(elem, param); reason != "" {
				return reason
			}
		}
	}
	return ""
}

func deriveCall(pos ast.Pos, name string, args ...ast.Expr) ast.Expr {
	return &ast.Call{ExprBase: ast.ExprBase{Pos: pos}, Fn: deriveIdent(pos, name), Args: args}
}

type lastDeriver struct {
	pos   ast.Pos
	param string
	next  int
}

func (g *lastDeriver) fresh() string {
	g.next++
	return "derived" + strconv.Itoa(g.next)
}

func (g *lastDeriver) lambda(names []string, body ast.Expr) ast.Expr {
	params := make([]ast.Param, len(names))
	for i, name := range names {
		params[i].Name = name
	}
	return &ast.Lambda{ExprBase: ast.ExprBase{Pos: g.pos}, Params: params, Body: body}
}

// tuple binds each component once, making nested reconstruction independent of
// field-access inference and avoiding repeated evaluation of the scrutinee.
func (g *lastDeriver) tuple(t *ast.TupleType, value ast.Expr, body func([]ast.Expr) ast.Expr) ast.Expr {
	pat := &ast.PTuple{Pos: g.pos, Elems: make([]ast.Pattern, len(t.Elems))}
	values := make([]ast.Expr, len(t.Elems))
	for i := range t.Elems {
		name := g.fresh()
		pat.Elems[i] = &ast.PVar{Pos: g.pos, Name: name}
		values[i] = deriveIdent(g.pos, name)
	}
	return &ast.Match{ExprBase: ast.ExprBase{Pos: g.pos}, Scrut: value, Arms: []ast.Arm{
		{Pos: g.pos, Pat: pat, Body: body(values)},
	}}
}

func (g *lastDeriver) mapped(t ast.Type, value ast.Expr) ast.Expr {
	if !deriveContains(t, g.param) {
		return value
	}
	switch t := t.(type) {
	case *ast.TypeName:
		if t.Name == g.param {
			return deriveCall(g.pos, "f", value)
		}
		name := g.fresh()
		return deriveCall(g.pos, "fmap", value, g.lambda([]string{name},
			g.mapped(t.Args[len(t.Args)-1], deriveIdent(g.pos, name))))
	case *ast.TupleType:
		return g.tuple(t, value, func(values []ast.Expr) ast.Expr {
			for i, elem := range t.Elems {
				values[i] = g.mapped(elem, values[i])
			}
			return &ast.TupleLit{ExprBase: ast.ExprBase{Pos: g.pos}, Elems: values}
		})
	default:
		panic("invalid derived Functor position")
	}
}

func (g *lastDeriver) folded(t ast.Type, value, init ast.Expr) ast.Expr {
	if !deriveContains(t, g.param) {
		return init
	}
	switch t := t.(type) {
	case *ast.TypeName:
		if t.Name == g.param {
			return deriveCall(g.pos, "f", value, init)
		}
		item, acc := g.fresh(), g.fresh()
		return deriveCall(g.pos, "foldr", value, init, g.lambda([]string{item, acc},
			g.folded(t.Args[len(t.Args)-1], deriveIdent(g.pos, item), deriveIdent(g.pos, acc))))
	case *ast.TupleType:
		return g.tuple(t, value, func(values []ast.Expr) ast.Expr {
			for i := len(t.Elems) - 1; i >= 0; i-- {
				init = g.folded(t.Elems[i], values[i], init)
			}
			return init
		})
	default:
		panic("invalid derived Foldable position")
	}
}

func (g *lastDeriver) traversed(t ast.Type, value ast.Expr) ast.Expr {
	switch t := t.(type) {
	case *ast.TypeName:
		if t.Name == g.param {
			return deriveCall(g.pos, "f", value)
		}
		name := g.fresh()
		return deriveCall(g.pos, "traverse", value, g.lambda([]string{name},
			g.traversed(t.Args[len(t.Args)-1], deriveIdent(g.pos, name))))
	case *ast.TupleType:
		return g.tuple(t, value, func(values []ast.Expr) ast.Expr {
			return g.sequence(t.Elems, values, func(elems []ast.Expr) ast.Expr {
				return &ast.TupleLit{ExprBase: ast.ExprBase{Pos: g.pos}, Elems: elems}
			})
		})
	default:
		panic("invalid derived Traversable position")
	}
}

// Sequence effects in constructor/tuple field order. Unmapped fields require
// neither an effect nor a typeclass dictionary.
func (g *lastDeriver) sequence(fields []ast.Type, values []ast.Expr, construct func([]ast.Expr) ast.Expr) ast.Expr {
	results := make([]ast.Expr, len(values))
	names := make([]string, len(values))
	for i, field := range fields {
		results[i] = values[i]
		if deriveContains(field, g.param) {
			names[i] = g.fresh()
			results[i] = deriveIdent(g.pos, names[i])
		}
	}
	body := deriveCall(g.pos, "pure", construct(results))
	for i := len(fields) - 1; i >= 0; i-- {
		if names[i] != "" {
			body = deriveCall(g.pos, "bind", g.traversed(fields[i], values[i]), g.lambda([]string{names[i]}, body))
		}
	}
	return body
}

func deriveLast(d *ast.TypeDecl, class string) ast.Expr {
	g := &lastDeriver{pos: d.Pos, param: d.TParams[len(d.TParams)-1]}
	m := &ast.Match{ExprBase: ast.ExprBase{Pos: d.Pos}, Scrut: deriveIdent(d.Pos, "a")}
	for _, ctor := range d.Ctors {
		values := make([]ast.Expr, len(ctor.Fields))
		for i := range values {
			values[i] = deriveIdent(d.Pos, "x"+strconv.Itoa(i))
		}
		construct := func(args []ast.Expr) ast.Expr {
			if len(args) == 0 {
				return deriveIdent(d.Pos, ctor.Name)
			}
			return deriveCall(d.Pos, ctor.Name, args...)
		}
		var body ast.Expr
		switch class {
		case "Functor":
			for i, field := range ctor.Fields {
				values[i] = g.mapped(field, values[i])
			}
			body = construct(values)
		case "Foldable":
			body = deriveIdent(d.Pos, "init")
			for i := len(ctor.Fields) - 1; i >= 0; i-- {
				body = g.folded(ctor.Fields[i], values[i], body)
			}
		case "Traversable":
			body = g.sequence(ctor.Fields, values, construct)
		}
		m.Arms = append(m.Arms, ast.Arm{Pos: d.Pos, Pat: deriveCtorPattern(d.Pos, ctor, "x"), Body: body})
	}
	return m
}

func deriveJSONFunction(field ast.Type) bool {
	found := false
	ast.Walk(field, func(node ast.Node) {
		if _, ok := node.(*ast.FuncType); ok {
			found = true
		}
	})
	return found
}

type jsonDeriver struct{ pos ast.Pos }

func (g jsonDeriver) str(value string) ast.Expr {
	return &ast.StringLit{ExprBase: ast.ExprBase{Pos: g.pos}, Value: value}
}

func (g jsonDeriver) call(name string, args ...ast.Expr) ast.Expr {
	return deriveCall(g.pos, name, args...)
}

func (g jsonDeriver) ident(name string) ast.Expr { return deriveIdent(g.pos, name) }

func (g jsonDeriver) bind(value ast.Expr, name string, body ast.Expr) ast.Expr {
	return g.call("bind", value, &ast.Lambda{ExprBase: ast.ExprBase{Pos: g.pos},
		Params: []ast.Param{{Pos: g.pos, Name: name}}, Body: body})
}

func (g jsonDeriver) expected(what, path string) ast.Expr {
	return g.call("Err", g.str("expected "+what+" at $"+path))
}

func (g jsonDeriver) list(values []ast.Expr) ast.Expr {
	return &ast.ListLit{ExprBase: ast.ExprBase{Pos: g.pos}, Elems: values}
}

func deriveEncode(d *ast.TypeDecl) ast.Expr {
	g := jsonDeriver{d.Pos}
	m := &ast.Match{ExprBase: ast.ExprBase{Pos: d.Pos}, Scrut: g.ident("a")}
	for _, ctor := range d.Ctors {
		fields := make([]ast.Expr, 0, len(ctor.Fields)+1)
		add := func(name string, value ast.Expr) {
			fields = append(fields, &ast.TupleLit{ExprBase: ast.ExprBase{Pos: d.Pos}, Elems: []ast.Expr{g.str(name), value}})
		}
		if len(d.Ctors) != 1 || ctor.FieldNames == nil {
			add("$tag", g.call("JStr", g.str(ctor.Name)))
		}
		if ctor.FieldNames != nil {
			for i, name := range ctor.FieldNames {
				add(name, g.call("encode", g.ident("x"+strconv.Itoa(i))))
			}
		} else if len(ctor.Fields) != 0 {
			values := make([]ast.Expr, len(ctor.Fields))
			for i := range values {
				values[i] = g.call("encode", g.ident("x"+strconv.Itoa(i)))
			}
			add("$values", g.call("JArr", g.list(values)))
		}
		m.Arms = append(m.Arms, ast.Arm{Pos: d.Pos, Pat: deriveCtorPattern(d.Pos, ctor, "x"),
			Body: g.call("_jsonObject", g.list(fields))})
	}
	return m
}

func (g jsonDeriver) decodedFields(ctor ast.Ctor, tuple bool) ast.Expr {
	values := make([]ast.Expr, len(ctor.Fields))
	for i := range values {
		values[i] = g.ident("v" + strconv.Itoa(i))
	}
	var value ast.Expr
	switch {
	case tuple:
		value = &ast.TupleLit{ExprBase: ast.ExprBase{Pos: g.pos}, Elems: values}
	case len(values) == 0:
		value = g.ident(ctor.Name)
	default:
		value = g.call(ctor.Name, values...)
	}
	body := g.call("Ok", value)
	for i := len(values) - 1; i >= 0; i-- {
		var field ast.Expr
		if ctor.FieldNames != nil {
			field = g.call("_jsonField", g.ident("fields"), g.str(ctor.FieldNames[i]))
		} else {
			path := "[" + strconv.Itoa(i) + "]"
			if !tuple {
				path = "[\"$values\"]" + path
			}
			field = g.call("_jsonDecodeAt", g.str(path), g.ident("x"+strconv.Itoa(i)))
		}
		body = g.bind(field, "v"+strconv.Itoa(i), body)
	}
	if ctor.FieldNames != nil || len(values) == 0 {
		return body
	}
	var pattern ast.Pattern = &ast.PCtor{Pos: g.pos, Name: "Nil"}
	for i := len(values) - 1; i >= 0; i-- {
		pattern = &ast.PCtor{Pos: g.pos, Name: "Cons", Args: []ast.Pattern{
			&ast.PVar{Pos: g.pos, Name: "x" + strconv.Itoa(i)}, pattern,
		}}
	}
	what, path := "array of length "+strconv.Itoa(len(values)), ""
	if !tuple {
		path = "[\"$values\"]"
	}
	m := &ast.Match{ExprBase: ast.ExprBase{Pos: g.pos}, Scrut: g.ident("values"),
		Arms: []ast.Arm{
			{Pos: g.pos, Pat: pattern, Body: body},
			{Pos: g.pos, Pat: &ast.PWild{Pos: g.pos}, Body: g.expected(what, path)},
		}}
	count := &ast.IntLit{ExprBase: ast.ExprBase{Pos: g.pos}, Value: int64(len(values))}
	array := g.call("_jsonValues", g.ident("fields"), count)
	if tuple {
		array = g.call("_jsonArray", g.ident("j"), count)
	}
	return g.bind(array, "values", m)
}

func deriveDecode(d *ast.TypeDecl) ast.Expr {
	g := jsonDeriver{d.Pos}
	var body ast.Expr
	if len(d.Ctors) == 1 && d.Ctors[0].FieldNames != nil {
		body = g.decodedFields(d.Ctors[0], false)
	} else {
		tags := &ast.Match{ExprBase: ast.ExprBase{Pos: d.Pos}, Scrut: g.ident("tag")}
		for _, ctor := range d.Ctors {
			tags.Arms = append(tags.Arms, ast.Arm{Pos: d.Pos,
				Pat: &ast.PLit{Pos: d.Pos, X: g.str(ctor.Name)}, Body: g.decodedFields(ctor, false)})
		}
		tags.Arms = append(tags.Arms, ast.Arm{Pos: d.Pos, Pat: &ast.PWild{Pos: d.Pos},
			Body: g.expected(d.Name+" constructor", "[\"$tag\"]")})
		body = g.bind(g.call("_jsonField", g.ident("fields"), g.str("$tag")), "tag", tags)
	}
	return &ast.Match{ExprBase: ast.ExprBase{Pos: d.Pos}, Scrut: g.ident("j"),
		Arms: []ast.Arm{
			{Pos: d.Pos, Pat: &ast.PCtor{Pos: d.Pos, Name: "JObj", Args: []ast.Pattern{&ast.PVar{Pos: d.Pos, Name: "fields"}}}, Body: body},
			{Pos: d.Pos, Pat: &ast.PWild{Pos: d.Pos}, Body: g.expected("object", "")},
		}}
}

// Tuples have unbounded arity. Materialize ordinary prelude instances only for
// arities occurring in this program; explicit instances take precedence.
func expandJSONTuples(files []*ast.File) {
	var owner *ast.File
	var pos ast.Pos
	arities := make(map[int]bool)
	existing := make(map[instanceKey]bool)
	classes := make(map[string]bool)
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.TypeDecl:
				if d.Name == "Json" {
					owner, pos = file, d.Pos
				}
			case *ast.ClassDecl:
				classes[d.Name] = true
			case *ast.InstanceDecl:
				existing[instanceKey{d.Class, d.TypeCon}] = true
			}
			ast.Walk(decl, func(node ast.Node) {
				switch n := node.(type) {
				case *ast.TupleType:
					arities[len(n.Elems)] = true
				case *ast.TupleLit:
					arities[len(n.Elems)] = true
				case *ast.PTuple:
					arities[len(n.Elems)] = true
				case *ast.TypeName:
					if suffix, ok := strings.CutPrefix(n.Name, "Tuple"); ok {
						if size, err := strconv.Atoi(suffix); err == nil && size >= 2 {
							arities[size] = true
						}
					}
				}
			})
		}
	}
	if owner == nil {
		return
	}
	sizes := make([]int, 0, len(arities))
	for size := range arities {
		sizes = append(sizes, size)
	}
	sort.Ints(sizes)
	g := jsonDeriver{pos}
	for _, size := range sizes {
		for _, class := range []string{"Encode", "Decode"} {
			name := "Tuple" + strconv.Itoa(size)
			if !classes[class] || existing[instanceKey{class, name}] {
				continue
			}
			inst := &ast.InstanceDecl{Pos: pos, Class: class, TypeCon: name}
			for i := range size {
				param := "JsonT" + strconv.Itoa(i)
				inst.Params = append(inst.Params, param)
				inst.Context = append(inst.Context, ast.Constraint{Pos: pos, Class: class, Param: param})
			}
			method := ast.InstMethod{Pos: pos, Name: "decode", Params: []string{"j"}, Body: &ast.Block{Pos: pos}}
			if class == "Encode" {
				method.Name, method.Params = "encode", []string{"a"}
				pat := &ast.PTuple{Pos: pos}
				values := make([]ast.Expr, size)
				for i := range values {
					binding := "x" + strconv.Itoa(i)
					pat.Elems = append(pat.Elems, &ast.PVar{Pos: pos, Name: binding})
					values[i] = g.call("encode", g.ident(binding))
				}
				method.Body.Result = &ast.Match{ExprBase: ast.ExprBase{Pos: pos}, Scrut: g.ident("a"),
					Arms: []ast.Arm{{Pos: pos, Pat: pat, Body: g.call("JArr", g.list(values))}}}
			} else {
				method.Body.Result = g.decodedFields(ast.Ctor{Fields: make([]ast.Type, size)}, true)
			}
			inst.Methods = []ast.InstMethod{method}
			owner.Decls = append(owner.Decls, inst)
		}
	}
}
