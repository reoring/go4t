package types

import (
	"fmt"
	"strconv"

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
		var seen [3]bool
		for _, class := range d.Deriving {
			var index int
			switch class {
			case "Eq":
				index = 0
			case "Ord":
				index = 1
			case "Show":
				index = 2
			default:
				return &Error{File: f.Name, Pos: d.Pos, Msg: fmt.Sprintf("cannot derive %s for %s: unknown class", class, d.Name)}
			}
			if seen[index] {
				return &Error{File: f.Name, Pos: d.Pos, Msg: fmt.Sprintf("duplicate deriving class %s for %s", class, d.Name)}
			}
			seen[index] = true
			if class == "Eq" || class == "Ord" {
				for _, ctor := range d.Ctors {
					for _, field := range ctor.Fields {
						if _, function := field.(*ast.FuncType); function {
							return &Error{File: f.Name, Pos: d.Pos, Msg: fmt.Sprintf("cannot derive %s for %s: field of function type", class, d.Name)}
						}
					}
				}
			}
			inst := &ast.InstanceDecl{Pos: d.Pos, Class: class, TypeCon: d.Name, Params: d.TParams}
			if len(d.TParams) != 0 {
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
			case "Show":
				method.Name, method.Params, method.Body.Result = "show", []string{"a"}, deriveShow(d)
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

func deriveShow(d *ast.TypeDecl) ast.Expr {
	base := ast.ExprBase{Pos: d.Pos}
	m := &ast.Match{ExprBase: base, Scrut: deriveIdent(d.Pos, "a")}
	for _, ctor := range d.Ctors {
		var body ast.Expr
		if len(ctor.Fields) == 0 {
			body = &ast.StringLit{ExprBase: base, Value: ctor.Name}
		} else {
			interp := &ast.Interp{ExprBase: base, Texts: make([]string, len(ctor.Fields)+1), Exprs: make([]ast.Expr, len(ctor.Fields))}
			for i := range ctor.Fields {
				interp.Exprs[i] = deriveIdent(d.Pos, "x"+strconv.Itoa(i))
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
