// Package parser builds the go4t AST.
//
// Grammar (';' is inserted at newlines, Go-style, except before 'else'):
//
//	File     := 'package' Ident ';' { Decl ';' }
//	Decl     := Class | Instance | TypeDecl | Alias | Func | Extern | Law | Proof
//	Class    := 'class' Ident '[' TParam ']' [ ':' Ident '[' Ident ']' ] '{' { MethodSig ';' } '}'
//	Instance := 'instance' Ident '[' InstHead ']' [ 'given' Context ] '{' { Ident '(' Idents ')' Block ';' } '}'
//	InstHead := Ident [ '[' Idents ']' ] | '(' Ident ',' Ident { ',' Ident } ')'
//	          // head arguments remain in Params; the checker splits Fixed for partial applications
//	Context  := Constraint { ',' Constraint }
//	Constraint := Ident '[' Ident ']' // parameter declared in the instance head
//	TypeDecl := 'type' Ident [ '[' Idents ']' ] '=' Ctor { '|' Ctor } [ 'deriving' Idents ]
//	Alias    := 'alias' Ident [ '[' Idents ']' ] '=' Type
//	Ctor     := Ident [ '(' [ Type { ',' Type } ] ')' | '{' NamedType { ',' NamedType } [ ',' ] '}' ]
//	NamedType := Ident ':' Type // records need at least one field; no positional/named mixing
//	Func     := 'func' Ident [ TParams ] '(' Params ')' Type Block
//	Extern   := 'extern' 'func' Ident [ TParams ] '(' Params ')' Type '=' String '.' Ident
//	Law      := 'law' Ident '(' Params ')' '{' Expr '==' Expr '}'
//	          // parse a single expression with a top-level ==, then retain its operands;
//	          // normal precedence applies, so lower-precedence sides need parentheses
//	Proof    := 'proof' Ident '(' [ Ident { ',' Ident } [ ',' ] ] ')' TacticBlock
//	TacticBlock := '{' Tactic [ ';' ] '}'
//	Tactic   := 'refl' | 'match' Ident '{' [ TacticArms ] '}'
//	          | 'if' Expr TacticBlock 'else' TacticBlock
//	          | 'rewrite' QualifiedIdent '(' [ Expr { ',' Expr } [ ',' ] ] ')' TacticBlock
//	          | 'sample' '(' Int ')' | 'induction' Ident [ '{' [ TacticArms ] '}' ]
//	TacticArms := { ';' } [ '|' ] { ';' } TacticArm { ArmSep TacticArm } [ ArmSep ]
//	TacticArm := Pattern '=>' Tactic // constructors, records, tuples, variables and _ only
//	TParam   := Ident [ '[' '_' { ',' '_' } ']' ] { Ident }
//	Type     := Ident [ '[' Type { ',' Type } ']' ] | 'func' '(' [ Type { ',' Type } ] ')' Type
//	          | '(' Type ')' | '(' Type ',' Type { ',' Type } ')'
//	Block    := '{' { Stmt ';' } Expr [ ';' ] '}'
//	Stmt     := 'let' Pattern [ ':' Type ] '=' Expr | LocalFunc | Expr
//	LocalFunc := 'func' Ident '(' Params ')' Type Block
//	Expr     := InferExpr | Projection
//	InferExpr := 'func' '(' LParams ')' '=>' Expr | 'do' DoBlock | 'match' Expr '{' Arms '}'
//	          | 'if' Expr Block 'else' ( Block | IfExpr ) | Binary
//	LParams  := [ LParam { ',' LParam } [ ',' ] ]
//	LParam   := LowerIdent [ Type ] | Pattern // only bare non-constructor names may be annotated
//	DoBlock  := '{' { DoStmt ';' } Expr [ ';' ] '}'
//	DoStmt   := Pattern '<-' Expr | 'let' Pattern [ ':' Type ] '=' Expr | LocalFunc | Expr
//	Arms     := { ';' } [ '|' ] { ';' } Arm { ArmSep Arm } [ ArmSep ]
//	Arm      := Pattern [ 'if' Expr ] '=>' Expr
//	ArmSep   := ( ';' | '|' ) { ';' | '|' }
//	Pattern  := '_' | Lit | Ident [ '(' [ Pattern { ',' Pattern } ] ')' ]
//	          | UpperIdent RecordPatternFields
//	          | LowerIdent '@' Pattern | '(' Pattern ')' | '(' Pattern ',' Pattern { ',' Pattern } ')'
//	          | '[' [ Pattern { ',' Pattern } [ ',' '..' [ LowerIdent | '_' ] ] [ ',' ] ] ']'
//	RecordPatternFields := '{' ( '..' [ ',' ] | NamedPattern { ',' NamedPattern } [ ',' '..' ] [ ',' ] ) '}'
//	NamedPattern := Ident ':' Pattern // names must be unique; '..' must be last; no field punning
//	LowerIdent := Ident // starts with a lower-case letter; '@' only follows a bare variable name
//	Postfix  := Primary { '(' [ CallArg { ',' CallArg } [ ',' ] ] ')' | '.' ( Ident | Int ) | RecordFields }
//	CallArg  := Expr | '_'
//	Primary  := Ident | Lit | Interp | UpperIdent RecordFields | '(' InferExpr ')' | '(' InferExpr ',' InferExpr { ',' InferExpr } ')' | Section | Block | List
//	Section  := '(' ( '_' BinOp Expr | Expr BinOp '_' | '_' BinOp '_' | Projection ) ')'
//	Projection := '_' '.' ( Ident | Int ) { '.' ( Ident | Int ) }
//	          // bare placeholders occupy a whole argument or operator operand;
//	          // projections form independent unary lambdas in expected-type contexts;
//	          // calls/sections lower to lambdas, and lowest-precedence |> lowers to calls
//	RecordFields := '{' Ident ':' Expr { ',' Ident ':' Expr } [ ',' ] '}'
//	          // record literals/updates require same-line '{' and lookahead '{' Ident ':';
//	          // other braces begin blocks, including if bodies and match arms
//	Interp   := '"' Text '${' Expr '}' { Text '${' Expr '}' } Text '"'
//	          // '$$' in Text is '$'; lone '$' and braces are literal
//	          // nested strings are allowed in Expr
//	List     := '[' [ InferExpr { ',' InferExpr } [ ',' ] ] ']'
package parser

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
)

type Parser struct {
	toks   []lexer.Token
	i      int
	source string // retained for capture-free placeholder names, including interpolation source
}

func Parse(src string) (f *ast.File, err error) {
	l := lexer.New(src)
	toks, err := l.All()
	if err != nil {
		return nil, err
	}
	p := &Parser{toks: toks, source: src}
	defer recoverParseError(&err)
	f = p.file()
	f.Source = src
	f.Comments = l.Comments
	return f, nil
}

// ParseExpr parses exactly one expression, starting at the one-based position
// base in the original source. Trailing semicolons (including inserted ones) are
// accepted; other trailing tokens are errors.
func ParseExpr(src string, base ast.Pos) (expr ast.Expr, err error) {
	toks, err := lexer.NewAt(src, base).All()
	if err != nil {
		return nil, err
	}
	p := &Parser{toks: toks, source: src}
	defer recoverParseError(&err)
	parsed := p.expr()
	p.skipSemis()
	if p.tok().Kind != lexer.EOF {
		p.fail(p.pos(), "expected end of expression, found %s", p.tok())
	}
	return parsed, nil
}

func recoverParseError(err *error) {
	if r := recover(); r != nil {
		pe, ok := r.(parseError)
		if !ok {
			panic(r)
		}
		*err = pe
	}
}

type parseError struct {
	Pos, End ast.Pos
	msg      string
}

func (e parseError) Error() string {
	if e.Pos == (ast.Pos{}) {
		return e.msg
	}
	return fmt.Sprintf("%s: %s", e.Pos, e.msg)
}
func (e parseError) GetPos() ast.Pos { return e.Pos }
func (e parseError) GetEnd() ast.Pos { return e.End }

func (p *Parser) fail(pos ast.Pos, format string, args ...any) {
	end := ast.Pos{Line: pos.Line, Col: pos.Col + 1}
	if p.tok().Pos == pos {
		end = p.tok().End
	}
	panic(parseError{Pos: pos, End: end, msg: fmt.Sprintf(format, args...)})
}

func (p *Parser) tok() lexer.Token { return p.toks[p.i] }
func (p *Parser) pos() ast.Pos     { return p.tok().Pos }

func (p *Parser) end() ast.Pos { return p.toks[p.i-1].End }

func (p *Parser) finish(n ast.Node) {
	if n != nil {
		n.SetEnd(p.end())
	}
}

func (p *Parser) is(kind lexer.Kind, text string) bool {
	t := p.tok()
	return t.Kind == kind && t.Text == text
}
func (p *Parser) isP(text string) bool { return p.is(lexer.Punct, text) }
func (p *Parser) isK(text string) bool { return p.is(lexer.Keyword, text) }

func (p *Parser) next() lexer.Token {
	t := p.toks[p.i]
	if t.Kind != lexer.EOF {
		p.i++
	}
	return t
}

func (p *Parser) expectP(text string) lexer.Token {
	if !p.isP(text) {
		p.fail(p.pos(), "expected %q, found %s", text, p.tok())
	}
	return p.next()
}

func (p *Parser) expectK(text string) lexer.Token {
	if !p.isK(text) {
		p.fail(p.pos(), "expected %q, found %s", text, p.tok())
	}
	return p.next()
}

func (p *Parser) ident() string {
	t := p.tok()
	if t.Kind != lexer.Ident {
		p.fail(t.Pos, "expected identifier, found %s", t)
	}
	p.next()
	return t.Text
}

func (p *Parser) qualified() string {
	name := p.ident()
	if p.acceptP(".") {
		name += "." + p.ident()
	}
	return name
}

func (p *Parser) skipSemis() {
	for p.isP(";") {
		p.next()
	}
}

func (p *Parser) acceptP(text string) bool {
	if p.isP(text) {
		p.next()
		return true
	}
	return false
}

// ---- declarations ----

func (p *Parser) file() *ast.File {
	p.skipSemis()
	pos := p.expectK("package").Pos
	f := &ast.File{Package: p.ident(), PackagePos: pos, PackageEnd: p.end()}
	p.skipSemis()
	for p.isK("import") {
		i := ast.Import{Pos: p.next().Pos}
		if p.tok().Kind == lexer.Ident {
			i.Alias = p.ident()
		}
		if p.tok().Kind != lexer.String {
			p.fail(p.pos(), "expected import path string")
		}
		i.Path = p.next().Text
		i.End = p.end()
		f.Imports = append(f.Imports, i)
		p.skipSemis()
	}
	for p.tok().Kind != lexer.EOF {
		f.Decls = append(f.Decls, p.decl())
		p.skipSemis()
	}
	return f
}

func (p *Parser) decl() (decl ast.Decl) {
	defer func() { p.finish(decl) }()
	t := p.tok()
	if t.Kind != lexer.Keyword {
		p.fail(t.Pos, "expected declaration, found %s", t)
	}
	switch t.Text {
	case "class":
		return p.classDecl()
	case "instance":
		return p.instanceDecl()
	case "type":
		return p.typeDecl()
	case "alias":
		return p.aliasDecl()
	case "let":
		pos := p.next().Pos
		d := &ast.ConstDecl{Pos: pos, Name: p.ident()}
		if p.acceptP(":") {
			d.Type = p.typ()
		} else if p.tok().Kind == lexer.Ident || p.isK("func") {
			p.fail(p.pos(), `let annotations are written "let x: Int = …"`)
		}
		p.expectP("=")
		d.Value = p.expr()
		return d
	case "func":
		return p.funcDecl(false)
	case "extern":
		p.next()
		return p.funcDecl(true)
	case "law":
		return p.lawDecl()
	case "proof":
		return p.proofDecl()
	}
	p.fail(t.Pos, "expected declaration, found %s", t)
	return nil
}

func (p *Parser) tparam() ast.TypeParam {
	tp := ast.TypeParam{Name: p.ident()}
	if p.acceptP("[") {
		p.expectP("_")
		tp.Arity = 1
		for p.acceptP(",") {
			p.expectP("_")
			tp.Arity++
		}
		p.expectP("]")
	}
	if p.tok().Kind == lexer.Ident {
		tp.Constraints = append(tp.Constraints, p.qualified())
		for p.acceptP("+") {
			tp.Constraints = append(tp.Constraints, p.qualified())
		}
	}
	return tp
}

func (p *Parser) tparams() []ast.TypeParam {
	if !p.acceptP("[") {
		return nil
	}
	var out []ast.TypeParam
	for {
		out = append(out, p.tparam())
		if !p.acceptP(",") || p.isP("]") {
			break
		}
	}
	p.expectP("]")
	return out
}

func (p *Parser) params() []ast.Param {
	p.expectP("(")
	var out []ast.Param
	for !p.isP(")") {
		pos := p.pos()
		name := p.ident()
		if p.isP(":") {
			p.fail(p.pos(), `parameters are written "x Int", without ":"`)
		}
		typ := p.typ()
		out = append(out, ast.Param{Pos: pos, End: p.end(), Name: name, Type: typ})
		if !p.acceptP(",") {
			break
		}
	}
	p.expectP(")")
	return out
}

func (p *Parser) classDecl() ast.Decl {
	pos := p.next().Pos
	c := &ast.ClassDecl{Pos: pos, Name: p.ident()}
	p.expectP("[")
	c.Param = p.tparam()
	p.acceptP(",")
	p.expectP("]")
	if p.acceptP(":") {
		c.Super = p.qualified()
		p.expectP("[")
		if n := p.ident(); n != c.Param.Name {
			p.fail(pos, "superclass must be applied to %s", c.Param.Name)
		}
		p.acceptP(",")
		p.expectP("]")
	}
	p.expectP("{")
	p.skipSemis()
	for !p.isP("}") {
		m := ast.MethodSig{Pos: p.pos(), Name: p.ident()}
		m.TParams = p.tparams()
		m.Params = p.params()
		m.Ret = p.typ()
		m.End = p.end()
		c.Methods = append(c.Methods, m)
		p.skipSemis()
	}
	p.expectP("}")
	return c
}

func (p *Parser) instanceDecl() ast.Decl {
	pos := p.next().Pos
	d := &ast.InstanceDecl{Pos: pos, Class: p.qualified()}
	p.expectP("[")
	headPos := p.pos()
	tupleHead := p.acceptP("(")
	if !tupleHead {
		d.TypeCon = p.qualified()
	}
	if tupleHead || p.acceptP("[") {
		for {
			paramPos := p.pos()
			name := p.ident()
			if slices.Contains(d.Params, name) {
				p.fail(paramPos, "duplicate instance type parameter %s", name)
			}
			d.Params = append(d.Params, name)
			if !p.acceptP(",") || tupleHead && p.isP(")") || !tupleHead && p.isP("]") {
				break
			}
		}
		if tupleHead {
			p.expectP(")")
			if len(d.Params) < 2 {
				p.fail(headPos, "tuple instance head needs at least two elements")
			}
			d.TypeCon = "Tuple" + strconv.Itoa(len(d.Params))
		} else {
			p.expectP("]")
		}
	}
	p.acceptP(",")
	p.expectP("]")
	if p.isK("given") {
		givenPos := p.next().Pos
		if len(d.Params) == 0 {
			p.fail(givenPos, "given requires instance type parameters")
		}
		for {
			c := ast.Constraint{Pos: p.pos(), Class: p.qualified()}
			p.expectP("[")
			paramPos := p.pos()
			c.Param = p.ident()
			if !slices.Contains(d.Params, c.Param) {
				p.fail(paramPos, "undeclared instance type parameter %s", c.Param)
			}
			p.acceptP(",")
			p.expectP("]")
			c.End = p.end()
			d.Context = append(d.Context, c)
			if !p.acceptP(",") {
				break
			}
		}
	}
	p.expectP("{")
	p.skipSemis()
	for !p.isP("}") {
		m := ast.InstMethod{Pos: p.pos(), Name: p.ident()}
		p.expectP("(")
		for !p.isP(")") {
			m.Params = append(m.Params, p.ident())
			if !p.acceptP(",") {
				break
			}
		}
		p.expectP(")")
		m.Body = p.block()
		m.End = p.end()
		d.Methods = append(d.Methods, m)
		p.skipSemis()
	}
	p.expectP("}")
	return d
}

func (p *Parser) aliasDecl() ast.Decl {
	pos := p.next().Pos
	d := &ast.AliasDecl{Pos: pos, Name: p.ident()}
	if p.acceptP("[") {
		for {
			d.TParams = append(d.TParams, p.ident())
			if !p.acceptP(",") || p.isP("]") {
				break
			}
		}
		p.expectP("]")
	}
	p.expectP("=")
	d.Type = p.typ()
	return d
}

func (p *Parser) typeDecl() ast.Decl {
	pos := p.next().Pos
	d := &ast.TypeDecl{Pos: pos, Name: p.ident()}
	if p.acceptP("[") {
		for {
			d.TParams = append(d.TParams, p.ident())
			if !p.acceptP(",") || p.isP("]") {
				break
			}
		}
		p.expectP("]")
	}
	p.expectP("=")
	for {
		c := ast.Ctor{Pos: p.pos(), Name: d.Name}
		if p.isP("{") {
			if len(d.Ctors) != 0 {
				p.fail(p.pos(), "the record shorthand declares the only constructor of %s; name the constructors to add alternatives", d.Name)
			}
			d.Shorthand = true
		} else {
			c.Name = p.ident()
		}
		if p.acceptP("(") {
			for !p.isP(")") {
				c.Fields = append(c.Fields, p.typ())
				if p.tok().Kind == lexer.Ident {
					p.fail(p.pos(), "positional constructor fields have no names; write %s{n: Int} to name them", c.Name)
				}
				if !p.acceptP(",") {
					break
				}
			}
			p.expectP(")")
		} else if p.acceptP("{") {
			// Record braces are not blocks: newline-inserted ';' are layout noise.
			p.skipSemis()
			if p.isP("}") {
				p.fail(c.Pos, "record constructor needs at least one field")
			}
			for {
				c.FieldNames = append(c.FieldNames, p.ident())
				if p.tok().Kind == lexer.Ident || p.isK("func") {
					p.fail(p.pos(), `record fields are written "n: Int"`)
				}
				p.expectP(":")
				c.Fields = append(c.Fields, p.typ())
				p.skipSemis()
				if !p.acceptP(",") {
					break
				}
				p.skipSemis()
				if p.isP("}") {
					break
				}
			}
			p.expectP("}")
		}
		c.End = p.end()
		d.Ctors = append(d.Ctors, c)
		if d.Shorthand && p.isP("|") {
			p.fail(p.pos(), "the record shorthand declares the only constructor of %s; name the constructors to add alternatives", d.Name)
		}
		if !p.acceptP("|") {
			break
		}
	}
	if p.isK("deriving") {
		p.next()
		for {
			d.Deriving = append(d.Deriving, p.qualified())
			if !p.acceptP(",") {
				break
			}
		}
	}
	return d
}

func (p *Parser) funcDecl(extern bool) (decl ast.Decl) {
	defer func() { p.finish(decl) }()
	pos := p.expectK("func").Pos
	d := &ast.FuncDecl{Pos: pos, Name: p.ident()}
	d.TParams = p.tparams()
	d.Params = p.params()
	d.Ret = p.typ()
	if extern {
		p.expectP("=")
		t := p.tok()
		if t.Kind != lexer.String {
			p.fail(t.Pos, "expected import path string, found %s", t)
		}
		p.next()
		p.expectP(".")
		d.Extern = &ast.ExternRef{ImportPath: t.Text, Name: p.ident()}
		return d
	}
	d.Body = p.block()
	return d
}

func (p *Parser) lawDecl() ast.Decl {
	pos := p.next().Pos
	d := &ast.LawDecl{Pos: pos, Name: p.ident()}
	d.TParams = p.tparams()
	d.Params = p.params()
	p.skipSemis()
	if p.is(lexer.Ident, "requires") {
		p.next()
		d.Requires = p.expr()
		p.skipSemis()
	}
	p.expectP("{")
	p.skipSemis()
	bodyPos := p.pos()
	body := p.expr()
	equality, ok := body.(*ast.Binary)
	if !ok || equality.Op != "==" {
		p.fail(bodyPos, "law body must be an equality (lhs == rhs)")
	}
	d.LHS, d.RHS = equality.L, equality.R
	p.skipSemis()
	p.expectP("}")
	return d
}

func (p *Parser) proofDecl() ast.Decl {
	pos := p.next().Pos
	d := &ast.ProofDecl{Pos: pos, Name: p.ident()}
	p.expectP("(")
	for !p.isP(")") {
		param := ast.Param{Pos: p.pos(), Name: p.ident()}
		param.End = p.end()
		d.Params = append(d.Params, param)
		if !p.acceptP(",") {
			break
		}
	}
	p.expectP(")")
	d.Body = p.tacticBlock()
	return d
}

func (p *Parser) tacticBlock() ast.Tactic {
	p.expectP("{")
	p.skipSemis()
	tactic := p.tactic()
	p.skipSemis()
	p.expectP("}")
	return tactic
}

func (p *Parser) tactic() (tactic ast.Tactic) {
	defer func() { p.finish(tactic) }()
	pos := p.pos()
	switch {
	case p.isK("refl"):
		p.next()
		return &ast.TRefl{Pos: pos}
	case p.is(lexer.Ident, "sample"):
		p.next()
		p.expectP("(")
		count := p.tok()
		if count.Kind != lexer.Int {
			p.fail(count.Pos, "sample count must be an Int literal in 1..100000")
		}
		p.next()
		n, err := strconv.ParseInt(count.Text, 10, 64)
		if err != nil || n < 1 || n > 100000 {
			p.fail(count.Pos, "sample count must be an Int literal in 1..100000")
		}
		p.expectP(")")
		return &ast.TSample{Pos: pos, Count: n}
	case p.isK("match"), p.is(lexer.Ident, "induction"):
		induction := p.tok().Text == "induction"
		p.next()
		variable := &ast.Ident{ExprBase: ast.ExprBase{Pos: p.pos()}, Name: p.ident()}
		p.finish(variable)
		m := &ast.TMatch{Pos: pos, Var: variable, Induction: induction}
		if induction && !p.isP("{") {
			m.Bare = true
			return m
		}
		p.expectP("{")
		p.skipSemis()
		p.acceptP("|")
		p.skipSemis()
		for !p.isP("}") {
			arm := ast.TacticArm{Pos: p.pos(), Pat: p.pattern()}
			p.checkTacticPattern(arm.Pat)
			if p.isK("if") {
				p.fail(p.pos(), "tactic match does not support guards")
			}
			p.expectP("=>")
			arm.Body = p.tactic()
			arm.End = p.end()
			m.Arms = append(m.Arms, arm)
			for p.isP(";") || p.isP("|") {
				p.next()
			}
		}
		p.expectP("}")
		return m
	case p.isK("if"):
		p.next()
		branch := &ast.TIf{Pos: pos, Cond: p.expr()}
		branch.Then = p.tacticBlock()
		p.expectK("else")
		branch.Else = p.tacticBlock()
		return branch
	case p.isK("rewrite"):
		p.next()
		law := &ast.Ident{ExprBase: ast.ExprBase{Pos: p.pos()}, Name: p.qualified()}
		p.finish(law)
		rewrite := &ast.TRewrite{Pos: pos, Law: law}
		p.expectP("(")
		for !p.isP(")") {
			rewrite.Args = append(rewrite.Args, p.expr())
			if !p.acceptP(",") {
				break
			}
		}
		p.expectP(")")
		rewrite.Body = p.tacticBlock()
		return rewrite
	}
	p.fail(pos, "expected tactic, found %s", p.tok())
	return nil
}

func (p *Parser) checkTacticPattern(pat ast.Pattern) {
	switch pat := pat.(type) {
	case *ast.PVar, *ast.PWild:
	case *ast.PCtor:
		if pat.List {
			p.fail(pat.Pos, "tactic match does not support list patterns")
		}
		for _, arg := range pat.Args {
			p.checkTacticPattern(arg)
		}
	case *ast.PRecord:
		for _, field := range pat.Pats {
			p.checkTacticPattern(field)
		}
	case *ast.PTuple:
		for _, elem := range pat.Elems {
			p.checkTacticPattern(elem)
		}
	case *ast.PLit:
		p.fail(pat.Pos, "tactic match does not support literal patterns")
	case *ast.PAs:
		p.fail(pat.Pos, "tactic match does not support as-patterns")
	default:
		p.fail(pat.GetPos(), "unsupported tactic match pattern")
	}
}

// ---- types ----

func (p *Parser) typ() (typ ast.Type) {
	defer func() { p.finish(typ) }()
	pos := p.pos()
	if p.isK("func") {
		p.next()
		ft := &ast.FuncType{Pos: pos}
		p.expectP("(")
		for !p.isP(")") {
			ft.Params = append(ft.Params, p.typ())
			if !p.acceptP(",") {
				break
			}
		}
		p.expectP(")")
		ft.Ret = p.typ()
		return ft
	}
	if p.acceptP("(") {
		first := p.typ()
		if !p.acceptP(",") {
			p.expectP(")")
			return first
		}
		if p.isP(")") {
			p.fail(p.pos(), "a tuple needs at least two elements")
		}
		tuple := &ast.TupleType{Pos: pos, Elems: []ast.Type{first, p.typ()}}
		for p.acceptP(",") && !p.isP(")") {
			tuple.Elems = append(tuple.Elems, p.typ())
		}
		p.expectP(")")
		return tuple
	}
	tn := &ast.TypeName{Pos: pos, Name: p.qualified()}
	if p.acceptP("[") {
		for {
			tn.Args = append(tn.Args, p.typ())
			if !p.acceptP(",") || p.isP("]") {
				break
			}
		}
		p.expectP("]")
	}
	return tn
}

// ---- blocks / statements ----

func (p *Parser) block() *ast.Block {
	b := &ast.Block{Pos: p.expectP("{").Pos}
	p.skipSemis()
	for {
		if p.isP("}") {
			p.fail(p.pos(), "block must end with an expression")
		}
		if p.localFuncAhead() {
			b.Stmts = append(b.Stmts, p.localFunc())
			p.skipSemis()
			continue
		}
		if p.isK("let") {
			pos := p.next().Pos
			namePos := p.pos()
			name, pat := p.bindingPattern()
			var typ ast.Type
			if p.acceptP(":") {
				typ = p.typ()
			}
			p.expectP("=")
			value := p.expr()
			b.Stmts = append(b.Stmts, &ast.LetStmt{Pos: pos, End: p.end(), NamePos: namePos, Name: name, Pat: pat, Type: typ, Value: value})
			p.skipSemis()
			continue
		}
		e := p.expr()
		p.skipSemis()
		if p.isP("}") {
			b.Result = e
			break
		}
		b.Stmts = append(b.Stmts, &ast.ExprStmt{X: e})
	}
	p.expectP("}")
	b.End = p.end()
	return b
}

func (p *Parser) localFuncAhead() bool {
	return p.isK("func") && p.toks[p.i+1].Kind == lexer.Ident
}

func (p *Parser) localFunc() *ast.FuncDecl {
	// Reject the only signature form that differs from a top-level function
	// before parsing its body, retaining the ordinary parameter/type grammar.
	if next := p.toks[p.i+2]; next.Kind == lexer.Punct && next.Text == "[" {
		p.fail(next.Pos, "local functions cannot have type parameters")
	}
	return p.funcDecl(false).(*ast.FuncDecl)
}

// bindingPattern preserves the compact Name representation for a bare local.
func (p *Parser) bindingPattern() (string, ast.Pattern) {
	t := p.tok()
	if t.Kind == lexer.Ident {
		first, _ := utf8.DecodeRuneInString(t.Text)
		next := p.toks[p.i+1]
		if (unicode.IsLower(first) || first == '_') && (next.Kind == lexer.Ident || next.Kind == lexer.Keyword && next.Text == "func") {
			p.fail(next.Pos, `let annotations are written "let x: Int = …"`)
		}
		if (unicode.IsLower(first) || first == '_') && next.Kind == lexer.Punct && (next.Text == "=" || next.Text == ":" || next.Text == "<-") {
			return p.ident(), nil
		}
	}
	return "", p.pattern()
}

// tryBind backtracks both failed patterns and patterns not followed by <-.
// Only the left side is speculative; errors in a bind's expression propagate.
func (p *Parser) tryBind() (name string, pat ast.Pattern, ok bool) {
	start := p.i
	defer func() {
		if r := recover(); r != nil {
			if _, parseFailure := r.(parseError); !parseFailure {
				panic(r)
			}
		}
		if !ok {
			p.i = start
			name, pat = "", nil
		}
	}()
	name, pat = p.bindingPattern()
	ok = p.acceptP("<-")
	return
}

// ---- expressions ----

func (p *Parser) expr() ast.Expr {
	if p.isP("_") && p.toks[p.i+1].Text == "." {
		pos := p.pos()
		var holes []*ast.Ident
		body := p.projection(p.placeholder(&holes))
		return partialLambda(pos, body, holes, false)
	}
	return p.sectionExpr(nil)
}

// sectionExpr permits bare placeholders only at this expression's binary
// level. Calls, nested parentheses and lambda bodies establish their own scope.
func (p *Parser) sectionExpr(holes *[]*ast.Ident) (expr ast.Expr) {
	defer func() { p.finish(expr) }()
	pos := p.pos()
	switch {
	case p.isK("func"):
		p.next()
		l := &ast.Lambda{ExprBase: ast.ExprBase{Pos: pos}}
		p.expectP("(")
		for !p.isP(")") {
			prm := ast.Param{Pos: p.pos()}
			if p.tok().Kind == lexer.Ident && !isUpper(p.tok().Text) && p.toks[p.i+1].Text != "@" && p.toks[p.i+1].Text != "|" {
				prm.Name = p.ident()
				if p.isP(":") {
					p.fail(p.pos(), `parameters are written "x Int", without ":"`)
				}
				if !p.isP(",") && !p.isP(")") {
					prm.Type = p.typ()
				}
			} else {
				prm.Pat = p.pattern()
				if !p.isP(",") && !p.isP(")") {
					p.fail(p.pos(), "pattern parameters cannot be annotated")
				}
			}
			prm.End = p.end()
			l.Params = append(l.Params, prm)
			if !p.acceptP(",") {
				break
			}
		}
		p.expectP(")")
		p.expectP("=>")
		l.Body = p.expr()
		return l
	case p.isK("do"):
		p.next()
		return p.doExpr(pos)
	case p.isK("match"):
		p.next()
		return p.matchExpr(pos)
	case p.isK("if"):
		return p.ifExpr()
	}
	return p.binary(0, holes)
}

func (p *Parser) ifExpr() (expr ast.Expr) {
	defer func() { p.finish(expr) }()
	pos := p.expectK("if").Pos
	e := &ast.If{ExprBase: ast.ExprBase{Pos: pos}}
	e.Cond = p.expr()
	e.Then = p.blockExpr()
	p.expectK("else")
	if p.isK("if") {
		e.Else = p.ifExpr()
	} else {
		e.Else = p.blockExpr()
	}
	return e
}

func (p *Parser) blockExpr() (expr ast.Expr) {
	defer func() { p.finish(expr) }()
	pos := p.pos()
	return &ast.BlockExpr{ExprBase: ast.ExprBase{Pos: pos}, Block: p.block()}
}

func (p *Parser) doExpr(pos ast.Pos) (expr ast.Expr) {
	defer func() { p.finish(expr) }()
	d := &ast.Do{ExprBase: ast.ExprBase{Pos: pos}}
	p.expectP("{")
	p.skipSemis()
	for {
		if p.isP("}") {
			p.fail(p.pos(), "do block must end with an expression")
		}
		if p.localFuncAhead() {
			d.Stmts = append(d.Stmts, p.localFunc())
			p.skipSemis()
			continue
		}
		spos := p.pos()
		if p.isK("let") {
			p.next()
			namePos := p.pos()
			name, pat := p.bindingPattern()
			var typ ast.Type
			if p.acceptP(":") {
				typ = p.typ()
			}
			p.expectP("=")
			value := p.expr()
			d.Stmts = append(d.Stmts, &ast.DoLet{Pos: spos, End: p.end(), NamePos: namePos, Name: name, Pat: pat, Type: typ, Value: value})
			p.skipSemis()
			continue
		}
		if name, pat, ok := p.tryBind(); ok {
			value := p.expr()
			d.Stmts = append(d.Stmts, &ast.DoBind{Pos: spos, End: p.end(), Name: name, Pat: pat, X: value})
			p.skipSemis()
			continue
		}
		e := p.expr()
		p.skipSemis()
		if p.isP("}") {
			d.Result = e
			break
		}
		d.Stmts = append(d.Stmts, &ast.DoExpr{X: e})
	}
	p.expectP("}")
	return d
}

func (p *Parser) matchExpr(pos ast.Pos) (expr ast.Expr) {
	defer func() { p.finish(expr) }()
	m := &ast.Match{ExprBase: ast.ExprBase{Pos: pos}}
	m.Scrut = p.expr()
	p.expectP("{")
	p.skipSemis()
	p.acceptP("|")
	p.skipSemis()
	for !p.isP("}") {
		arm := ast.Arm{Pos: p.pos(), Pat: p.pattern()}
		if p.isK("if") {
			p.next()
			arm.Guard = p.expr()
		}
		p.expectP("=>")
		arm.Body = p.expr()
		arm.End = p.end()
		m.Arms = append(m.Arms, arm)
		for p.isP(";") || p.isP("|") {
			p.next()
		}
	}
	p.expectP("}")
	if len(m.Arms) == 0 {
		p.fail(pos, "match needs at least one arm")
	}
	return m
}

func (p *Parser) pattern() (pat ast.Pattern) {
	defer func() { p.finish(pat) }()
	t := p.tok()
	switch t.Kind {
	case lexer.Punct:
		if t.Text == "_" {
			p.next()
			return &ast.PWild{Pos: t.Pos}
		}
		if t.Text == "[" {
			return p.listPattern()
		}
		if t.Text == "(" {
			p.next()
			first := p.pattern()
			if !p.acceptP(",") {
				p.expectP(")")
				return first
			}
			tuple := &ast.PTuple{Pos: t.Pos, Elems: []ast.Pattern{first, p.pattern()}}
			for p.acceptP(",") {
				tuple.Elems = append(tuple.Elems, p.pattern())
			}
			p.expectP(")")
			return tuple
		}
	case lexer.Int, lexer.Float, lexer.String:
		return &ast.PLit{Pos: t.Pos, X: p.primary()}
	case lexer.Keyword:
		if t.Text == "true" || t.Text == "false" {
			return &ast.PLit{Pos: t.Pos, X: p.primary()}
		}
	case lexer.Ident:
		t.Text = p.qualified()
		if p.acceptP("@") {
			first, _ := utf8.DecodeRuneInString(t.Text)
			if !unicode.IsLower(first) && first != '_' {
				p.fail(t.Pos, "as-pattern name must be a lower-case identifier")
			}
			return &ast.PAs{Pos: t.Pos, Name: t.Text, Pat: p.pattern()}
		}
		if p.isP("(") {
			p.next()
			c := &ast.PCtor{Pos: t.Pos, Name: t.Text}
			for !p.isP(")") {
				c.Args = append(c.Args, p.pattern())
				if !p.acceptP(",") {
					break
				}
			}
			p.expectP(")")
			return c
		}
		if isUpper(t.Text) || strings.Contains(t.Text, ".") {
			if p.acceptP("{") {
				record := &ast.PRecord{Pos: t.Pos, Ctor: t.Text}
				p.skipSemis()
				for !p.isP("}") {
					if p.isP("..") {
						restPos := p.next().Pos
						record.Rest = true
						p.acceptP(",")
						p.skipSemis()
						if !p.isP("}") {
							p.fail(restPos, ".. must be last in record pattern")
						}
						break
					}
					fieldPos := p.pos()
					name := p.ident()
					if slices.Contains(record.Names, name) {
						p.fail(fieldPos, "duplicate field %s in record pattern", name)
					}
					p.expectP(":")
					record.Names = append(record.Names, name)
					record.Pats = append(record.Pats, p.pattern())
					p.skipSemis()
					if !p.acceptP(",") {
						break
					}
					p.skipSemis()
				}
				p.expectP("}")
				if len(record.Names) == 0 && !record.Rest {
					p.fail(t.Pos, "record pattern needs fields or ..")
				}
				return record
			}
			return &ast.PCtor{Pos: t.Pos, Name: t.Text}
		}
		return &ast.PVar{Pos: t.Pos, Name: t.Text}
	}
	p.fail(t.Pos, "expected pattern, found %s", t)
	return nil
}

func (p *Parser) listPattern() ast.Pattern {
	pos := p.expectP("[").Pos
	var elems []ast.Pattern
	var tail ast.Pattern
	for !p.isP("]") {
		if p.isP("..") {
			restPos := p.next().Pos
			if len(elems) == 0 {
				p.fail(restPos, "list rest pattern needs a preceding element")
			}
			switch {
			case p.isP("_"):
				t := p.next()
				tail = &ast.PWild{Pos: t.Pos, End: p.end()}
			case p.tok().Kind == lexer.Ident && !isUpper(p.tok().Text):
				t := p.next()
				tail = &ast.PVar{Pos: t.Pos, End: p.end(), Name: t.Text}
			case p.isP("]") || p.isP(","):
				tail = &ast.PWild{Pos: restPos, End: p.end()}
			default:
				p.fail(p.pos(), "list rest must be a lower-case name or _")
			}
			p.acceptP(",")
			if !p.isP("]") {
				p.fail(restPos, ".. must be last in list pattern")
			}
			break
		}
		elems = append(elems, p.pattern())
		if !p.acceptP(",") {
			break
		}
	}
	closePos := p.expectP("]").Pos
	end := p.end()
	if tail == nil {
		tail = &ast.PCtor{Pos: closePos, End: end, Name: "Nil"}
	}
	for i := len(elems) - 1; i >= 0; i-- {
		tail = &ast.PCtor{Pos: elems[i].GetPos(), End: end, Name: "Cons", Args: []ast.Pattern{elems[i], tail}}
	}
	root := tail.(*ast.PCtor)
	root.Pos = pos
	root.List = true
	return root
}

func isUpper(s string) bool {
	s = strings.TrimLeft(s, "_")
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

var binPrec = map[string]int{
	"|>": 1, "||": 2, "&&": 3,
	"==": 4, "!=": 4, "<": 4, "<=": 4, ">": 4, ">=": 4,
	"+": 5, "-": 5, "*": 6, "/": 6, "%": 6,
}

func (p *Parser) binary(minPrec int, holes *[]*ast.Ident) ast.Expr {
	start := 0
	if holes != nil {
		start = len(*holes)
	}
	var left ast.Expr
	if holes != nil && p.isP("_") {
		left = p.projection(p.placeholder(holes))
	} else {
		left = p.unary()
	}
	for {
		t := p.tok()
		prec, ok := binPrec[t.Text]
		if t.Kind != lexer.Punct || !ok || prec <= minPrec {
			return left
		}
		p.next()
		right := p.binary(prec, holes)
		if holes != nil {
			// A section's holes are whole operands, not subexpressions of
			// either side. Nested sections have already become lambdas.
			for _, hole := range (*holes)[start:] {
				if hole != left && hole != right {
					p.fail(hole.Pos, placeholderError)
				}
			}
		}
		if t.Text == "|>" {
			left = pipeCall(t.Pos, left, right)
		} else {
			left = &ast.Binary{ExprBase: ast.ExprBase{Pos: t.Pos, End: p.end()}, Op: t.Text, L: left, R: right}
		}
	}
}

const placeholderError = "_ is only valid as a placeholder in a call or operator section"

func (p *Parser) placeholder(holes *[]*ast.Ident) *ast.Ident {
	pos := p.expectP("_").Pos
	// Positions distinguish synthesized bindings even inside interpolations,
	// whose parsers start at the original source position. Rejecting names
	// appearing anywhere in the source also protects free interpolated names.
	name := fmt.Sprintf("placeholder%d_%d", pos.Line, pos.Col)
	for strings.Contains(p.source, name) {
		name += "_"
	}
	hole := &ast.Ident{ExprBase: ast.ExprBase{Pos: pos, End: p.end()}, Name: name, Display: "_", Placeholder: true}
	*holes = append(*holes, hole)
	return hole
}

func partialLambda(pos ast.Pos, body ast.Expr, holes []*ast.Ident, section bool) ast.Expr {
	if len(holes) == 0 {
		return body
	}
	params := make([]ast.Param, len(holes))
	for i, hole := range holes {
		params[i] = ast.Param{Pos: hole.Pos, End: hole.End, Name: hole.Name, Display: hole.Display}
	}
	return &ast.Lambda{ExprBase: ast.ExprBase{Pos: pos, End: body.GetEnd()}, Params: params, Body: body, Partial: true, Section: section}
}

func pipeCall(pos ast.Pos, left, right ast.Expr) ast.Expr {
	surface := &ast.Binary{ExprBase: ast.ExprBase{Pos: pos, End: right.GetEnd()}, Op: "|>", L: left, R: right}
	if call, ok := right.(*ast.Call); ok {
		args := make([]ast.Expr, len(call.Args)+1)
		args[0] = left
		copy(args[1:], call.Args)
		return &ast.Call{ExprBase: call.ExprBase, Fn: call.Fn, Args: args, Pipe: surface}
	}
	return &ast.Call{ExprBase: ast.ExprBase{Pos: pos, End: right.GetEnd()}, Fn: right, Args: []ast.Expr{left}, Pipe: surface}
}

func (p *Parser) unary() (expr ast.Expr) {
	defer func() { p.finish(expr) }()
	t := p.tok()
	if t.Kind == lexer.Punct && (t.Text == "-" || t.Text == "!") {
		p.next()
		return &ast.Unary{ExprBase: ast.ExprBase{Pos: t.Pos}, Op: t.Text, X: p.unary()}
	}
	return p.postfix()
}

func (p *Parser) postfix() ast.Expr {
	e := p.primary()
	for {
		switch {
		case p.isP("("):
			pos := p.next().Pos
			c := &ast.Call{ExprBase: ast.ExprBase{Pos: pos}, Fn: e}
			var holes []*ast.Ident
			for !p.isP(")") {
				pos := p.pos()
				var argHoles []*ast.Ident
				arg := p.sectionExpr(&argHoles)
				if len(argHoles) > 0 {
					if !p.isP(",") && !p.isP(")") {
						p.fail(argHoles[0].Pos, placeholderError)
					}
					if arg == argHoles[0] {
						holes = append(holes, argHoles...)
					} else {
						_, projection := arg.(*ast.Field)
						arg = partialLambda(pos, arg, argHoles, !projection)
					}
				}
				c.Args = append(c.Args, arg)
				if !p.acceptP(",") {
					break
				}
			}
			p.expectP(")")
			c.End = p.end()
			e = partialLambda(pos, c, holes, false)
		case p.isP("."):
			e = p.projection(e)
		case p.recordAhead():
			update := &ast.RecordUpdate{ExprBase: ast.ExprBase{Pos: p.pos()}, X: e}
			update.Names, update.Values = p.recordFields()
			update.End = p.end()
			e = update
		default:
			return e
		}
	}
}

// projection consumes only selectors, so a projection placeholder cannot
// silently grow into a method call, record update or another expression.
func (p *Parser) projection(e ast.Expr) ast.Expr {
	for p.isP(".") {
		pos := p.next().Pos
		field := &ast.Field{ExprBase: ast.ExprBase{Pos: pos}, X: e, Index: -1}
		if t := p.tok(); t.Kind == lexer.Int {
			p.next()
			index, err := strconv.Atoi(t.Text)
			if err != nil {
				p.fail(t.Pos, "bad tuple index %s", t.Text)
			}
			field.Name, field.Index = t.Text, index
		} else {
			field.Name = p.ident()
		}
		field.End = p.end()
		e = field
	}
	return e
}

// recordAhead reports `{ Ident :` on the same line as the preceding token;
// a newline directly after `{` (inserted `;`) is skipped for the lookahead.
func (p *Parser) recordAhead() bool {
	if !p.isP("{") || p.i == 0 || p.toks[p.i-1].Pos.Line != p.pos().Line {
		return false
	}
	j := p.i + 1
	for j < len(p.toks) && p.toks[j].Kind == lexer.Punct && p.toks[j].Text == ";" {
		j++
	}
	return j+1 < len(p.toks) && p.toks[j].Kind == lexer.Ident &&
		p.toks[j+1].Kind == lexer.Punct && p.toks[j+1].Text == ":"
}

func (p *Parser) recordFields() (names []string, values []ast.Expr) {
	p.expectP("{")
	p.skipSemis()
	for {
		names = append(names, p.ident())
		p.expectP(":")
		values = append(values, p.expr())
		p.skipSemis()
		if !p.acceptP(",") {
			break
		}
		p.skipSemis()
		if p.isP("}") {
			break
		}
	}
	p.expectP("}")
	return names, values
}

func (p *Parser) primary() (expr ast.Expr) {
	defer func() { p.finish(expr) }()
	t := p.tok()
	base := ast.ExprBase{Pos: t.Pos}
	switch t.Kind {
	case lexer.Ident:
		p.next()
		if isUpper(t.Text) && p.recordAhead() {
			record := &ast.RecordLit{ExprBase: base, Ctor: t.Text}
			record.Names, record.Values = p.recordFields()
			return record
		}
		return &ast.Ident{ExprBase: base, Name: t.Text}
	case lexer.Int:
		p.next()
		v, err := strconv.ParseInt(t.Text, 10, 64)
		if err != nil {
			p.fail(t.Pos, "bad integer literal %s", t.Text)
		}
		return &ast.IntLit{ExprBase: base, Value: v}
	case lexer.Float:
		p.next()
		v, err := strconv.ParseFloat(t.Text, 64)
		if err != nil {
			p.fail(t.Pos, "bad float literal %s", t.Text)
		}
		return &ast.FloatLit{ExprBase: base, Value: v}
	case lexer.String:
		p.next()
		return &ast.StringLit{ExprBase: base, Value: t.Text}
	case lexer.InterpTok:
		p.next()
		interp := &ast.Interp{
			ExprBase: base,
			Texts:    make([]string, 0, len(t.Parts)/2+1),
			Exprs:    make([]ast.Expr, 0, len(t.Parts)/2),
		}
		for _, part := range t.Parts {
			if part.Pos == (ast.Pos{}) {
				interp.Texts = append(interp.Texts, part.Text)
				continue
			}
			expr, err := ParseExpr(part.Expr, part.Pos)
			if err != nil {
				if pe, ok := err.(parseError); ok {
					panic(pe)
				}
				panic(parseError{msg: err.Error()})
			}
			interp.Exprs = append(interp.Exprs, expr)
			interp.Starts = append(interp.Starts, part.Pos)
		}
		return interp
	case lexer.Keyword:
		if t.Text == "true" || t.Text == "false" {
			p.next()
			return &ast.BoolLit{ExprBase: base, Value: t.Text == "true"}
		}
	case lexer.Punct:
		if t.Text == "_" {
			p.fail(t.Pos, placeholderError)
		}
		if t.Text == "(" {
			p.next()
			var holes []*ast.Ident
			first := p.sectionExpr(&holes)
			if len(holes) > 0 {
				if first == holes[0] || !p.isP(")") {
					p.fail(holes[0].Pos, placeholderError)
				}
				p.next()
				return partialLambda(t.Pos, first, holes, true)
			}
			if !p.acceptP(",") {
				p.expectP(")")
				return first
			}
			if p.isP(")") {
				p.fail(p.pos(), "a tuple needs at least two elements")
			}
			tuple := &ast.TupleLit{ExprBase: base, Elems: []ast.Expr{first, p.sectionExpr(nil)}}
			for p.acceptP(",") && !p.isP(")") {
				tuple.Elems = append(tuple.Elems, p.sectionExpr(nil))
			}
			p.expectP(")")
			return tuple
		}
		if t.Text == "{" {
			return p.blockExpr()
		}
		if t.Text == "[" {
			p.next()
			list := &ast.ListLit{ExprBase: base}
			for !p.isP("]") {
				list.Elems = append(list.Elems, p.sectionExpr(nil))
				if !p.acceptP(",") {
					break
				}
			}
			p.expectP("]")
			return list
		}
	}
	p.fail(t.Pos, "expected expression, found %s", t)
	return nil
}
