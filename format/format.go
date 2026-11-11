// Package format prints canonical go4t source from its surface AST.
package format

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

// Source parses and formats a complete source file. Parse errors are unchanged.
func Source(src []byte) ([]byte, error) {
	f, err := parser.Parse(string(src))
	if err != nil {
		return nil, err
	}
	return File(f), nil
}

// File formats an unchecked parsed AST, retaining comments and surface sugar.
func File(f *ast.File) []byte {
	p := &printer{comments: f.Comments, source: f.Source}
	start := p.offset()
	p.text("package " + f.Package)
	p.anchor(f.PackagePos, f.PackageEnd, start, p.offset())
	for i := range f.Imports {
		imp := &f.Imports[i]
		p.newline()
		if i == 0 {
			p.newline()
		}
		end := p.mark(imp.Pos, imp.End)
		p.text("import ")
		if imp.Alias != "" {
			p.text(imp.Alias + " ")
		}
		p.text(quote(imp.Path))
		end()
	}
	for _, d := range f.Decls {
		p.newline()
		p.newline()
		p.decl(d)
	}
	p.newline()
	return []byte(p.withComments())
}

type span struct {
	pos, end            ast.Pos
	start, stop, indent int
}

type printer struct {
	b           *strings.Builder
	indent, col int
	spans       []span
	comments    []ast.Comment
	source      string
	lineOffsets []int // lazily indexed for comment anchors absent from the AST
	inline      bool  // interpolation expressions cannot introduce literal text newlines
}

func (p *printer) offset() int {
	if p.b == nil {
		return 0
	}
	return p.b.Len()
}
func (p *printer) pad() {
	if p.b == nil {
		p.b = &strings.Builder{}
	}
	if p.col == 0 {
		p.b.WriteString(strings.Repeat("  ", p.indent))
		p.col = 2 * p.indent
	}
}
func (p *printer) text(s string) {
	if s == "" {
		return
	}
	p.pad()
	p.b.WriteString(s)
	p.col += utf8.RuneCountInString(s)
}
func (p *printer) newline() {
	if p.inline {
		p.text("; ")
		return
	}
	p.b.WriteByte('\n')
	p.col = 0
}
func (p *printer) anchor(pos, end ast.Pos, start, stop int) {
	if pos.Line != 0 {
		p.spans = append(p.spans, span{pos, end, start, stop, p.indent})
	}
}
func (p *printer) mark(pos, end ast.Pos) func() {
	p.pad()
	start := p.offset()
	return func() { p.anchor(pos, end, start, p.offset()) }
}
func (p *printer) token(s string, pos ast.Pos) {
	p.pad()
	start := p.offset()
	p.text(s)
	end := pos
	end.Col += utf8.RuneCountInString(s)
	p.anchor(pos, end, start, p.offset())
}
func closePos(end ast.Pos) ast.Pos {
	if end.Col > 1 {
		end.Col--
	}
	return end
}
func (p *printer) open(pos ast.Pos)  { p.token("{", pos); p.indent++; p.newline() }
func (p *printer) close(end ast.Pos) { p.indent--; p.newline(); p.token("}", closePos(end)) }
func (p *printer) comma(i int) {
	if i > 0 {
		p.text(", ")
	}
}
func (p *printer) names(names []string) { p.text(strings.Join(names, ", ")) }
func (p *printer) typeNames(names []string) {
	if len(names) > 0 {
		p.text("[")
		p.names(names)
		p.text("]")
	}
}
func (p *printer) tparam(t ast.TypeParam) {
	p.text(t.Name)
	if t.Arity > 0 {
		p.text("[" + strings.TrimSuffix(strings.Repeat("_, ", t.Arity), ", ") + "]")
	}
	if len(t.Constraints) > 0 {
		p.text(" " + strings.Join(t.Constraints, " + "))
	}
}
func (p *printer) tparams(ts []ast.TypeParam) {
	if len(ts) == 0 {
		return
	}
	p.text("[")
	for i, t := range ts {
		p.comma(i)
		p.tparam(t)
	}
	p.text("]")
}
func (p *printer) params(params []ast.Param) {
	p.text("(")
	for i, a := range params {
		p.comma(i)
		end := p.mark(a.Pos, a.End)
		if a.Pat != nil {
			p.bindingPattern(a.Pat)
		} else {
			p.text(a.Name)
		}
		if a.Type != nil {
			p.text(" ")
			p.typ(a.Type)
		}
		end()
	}
	p.text(")")
}
func (p *printer) decl(decl ast.Decl) {
	defer p.mark(decl.GetPos(), decl.GetEnd())()
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Extern != nil {
			p.text("extern ")
		}
		p.text("func " + d.Name)
		p.tparams(d.TParams)
		p.params(d.Params)
		p.text(" ")
		p.typ(d.Ret)
		if d.Extern != nil {
			p.text(" = " + quote(d.Extern.ImportPath) + "." + d.Extern.Name)
		} else {
			p.text(" ")
			p.block(d.Body)
		}
	case *ast.LawDecl:
		p.text("law " + d.Name)
		p.tparams(d.TParams)
		p.params(d.Params)
		bodyStart := d.Pos
		if d.Requires != nil {
			p.text(" requires ")
			p.expr(d.Requires, 0)
			bodyStart = d.Requires.GetEnd()
		}
		p.text(" ")
		p.open(p.delimiterBetween('{', bodyStart, exprStart(d.LHS)))
		// Comparisons do not associate (ADR 0019): a comparison on either side
		// of the equation keeps its parentheses.
		p.expr(d.LHS, precedence["=="]+1)
		p.text(" == ")
		p.expr(d.RHS, precedence["=="]+1)
		p.close(d.End)
	case *ast.ProofDecl:
		p.text("proof " + d.Name)
		p.params(d.Params)
		p.text(" ")
		p.tacticBlock(p.delimiterBetween('{', d.Pos, d.Body.GetPos()), d.End, d.Body)
	case *ast.AliasDecl:
		p.text("alias " + d.Name)
		p.typeNames(d.TParams)
		p.text(" = ")
		p.typ(d.Type)
	case *ast.TypeDecl:
		p.text("type " + d.Name)
		p.typeNames(d.TParams)
		p.text(" = ")
		for i, c := range d.Ctors {
			if i > 0 {
				p.text(" | ")
			}
			end := p.mark(c.Pos, c.End)
			p.text(c.Name)
			if len(c.FieldNames) > 0 {
				p.record(c.Pos, c.End, len(c.Fields), func(i int) { p.text(c.FieldNames[i] + ": "); p.typ(c.Fields[i]) })
			} else if len(c.Fields) > 0 {
				p.text("(")
				for i, t := range c.Fields {
					p.comma(i)
					p.typ(t)
				}
				p.text(")")
			}
			end()
		}
		if len(d.Deriving) > 0 {
			p.text(" deriving ")
			p.names(d.Deriving)
		}
	case *ast.ClassDecl:
		p.text("class " + d.Name + "[")
		p.tparam(d.Param)
		p.text("]")
		if d.Super != "" {
			p.text(" : " + d.Super + "[" + d.Param.Name + "]")
		}
		p.text(" ")
		p.open(d.Pos)
		for i, m := range d.Methods {
			if i > 0 {
				p.newline()
			}
			end := p.mark(m.Pos, m.End)
			p.text(m.Name)
			p.tparams(m.TParams)
			p.params(m.Params)
			p.text(" ")
			p.typ(m.Ret)
			end()
		}
		p.close(d.End)
	case *ast.InstanceDecl:
		p.text("instance " + d.Class + "[" + d.TypeCon)
		p.typeNames(d.Params)
		p.text("]")
		if len(d.Context) > 0 {
			p.text(" given ")
			for i, c := range d.Context {
				p.comma(i)
				end := p.mark(c.Pos, c.End)
				p.text(c.Class + "[" + c.Param + "]")
				end()
			}
		}
		p.text(" ")
		p.open(d.Pos)
		for i, m := range d.Methods {
			if i > 0 {
				p.newline()
				p.newline()
			}
			end := p.mark(m.Pos, m.End)
			p.text(m.Name + "(")
			p.names(m.Params)
			p.text(") ")
			p.block(m.Body)
			end()
		}
		p.close(d.End)
	default:
		panic(fmt.Sprintf("format: unsupported declaration %T", d))
	}
}

func (p *printer) tacticBlock(pos, end ast.Pos, tactic ast.Tactic) {
	p.open(pos)
	p.tactic(tactic)
	p.close(end)
}

func (p *printer) tactic(tactic ast.Tactic) {
	defer p.mark(tactic.GetPos(), tactic.GetEnd())()
	switch t := tactic.(type) {
	case *ast.TRefl:
		p.text("refl")
	case *ast.TSample:
		p.text("sample(" + strconv.FormatInt(t.Count, 10) + ")")
	case *ast.TMatch:
		if t.Induction {
			p.text("induction ")
		} else {
			p.text("match ")
		}
		p.expr(t.Var, 0)
		if t.Bare {
			return
		}
		p.text(" ")
		p.open(p.delimiterBetween('{', t.Var.GetEnd(), t.End))
		for i, arm := range t.Arms {
			if i > 0 {
				p.newline()
			}
			end := p.mark(arm.Pos, arm.End)
			if p.multilineTacticArm(arm) {
				p.text("| ")
			}
			p.tacticArm(arm)
			end()
		}
		p.close(t.End)
	case *ast.TIf:
		p.text("if ")
		p.expr(t.Cond, 0)
		p.text(" ")
		thenEnd := p.delimiterBetween('}', t.Then.GetEnd(), t.Else.GetPos())
		if thenEnd != (ast.Pos{}) {
			thenEnd.Col++
		}
		p.tacticBlock(p.delimiterBetween('{', t.Cond.GetEnd(), t.Then.GetPos()), thenEnd, t.Then)
		p.text(" else ")
		p.tacticBlock(p.delimiterBetween('{', t.Then.GetEnd(), t.Else.GetPos()), t.End, t.Else)
	case *ast.TRewrite:
		p.text("rewrite ")
		p.expr(t.Law, 0)
		p.text("(")
		for i, arg := range t.Args {
			p.comma(i)
			p.expr(arg, 0)
		}
		p.text(") ")
		argsEnd := t.Law.GetEnd()
		if len(t.Args) > 0 {
			argsEnd = t.Args[len(t.Args)-1].GetEnd()
		}
		p.tacticBlock(p.delimiterBetween('{', argsEnd, t.Body.GetPos()), t.End, t.Body)
	default:
		panic(fmt.Sprintf("format: unsupported tactic %T", t))
	}
}

func (p *printer) tacticArm(arm ast.TacticArm) {
	p.pattern(arm.Pat)
	p.text(" => ")
	p.tactic(arm.Body)
}

func (p *printer) multilineTacticArm(arm ast.TacticArm) bool {
	if _, simple := arm.Body.(*ast.TRefl); !simple {
		return true
	}
	q := &printer{indent: p.indent, col: p.col, comments: p.comments}
	q.tacticArm(arm)
	return strings.Contains(q.b.String(), "\n")
}

func (p *printer) typ(t ast.Type) {
	defer p.mark(t.GetPos(), t.GetEnd())()
	switch t := t.(type) {
	case *ast.TypeName:
		p.text(t.Name)
		if len(t.Args) > 0 {
			p.text("[")
			for i, a := range t.Args {
				p.comma(i)
				p.typ(a)
			}
			p.text("]")
		}
	case *ast.FuncType:
		p.text("func(")
		for i, a := range t.Params {
			p.comma(i)
			p.typ(a)
		}
		p.text(") ")
		p.typ(t.Ret)
	case *ast.TupleType:
		p.text("(")
		for i, a := range t.Elems {
			p.comma(i)
			p.typ(a)
		}
		p.text(")")
	default:
		panic(fmt.Sprintf("format: unsupported type %T", t))
	}
}

func (p *printer) binding(name string, pat ast.Pattern, typ ast.Type, value ast.Expr) {
	p.text("let ")
	if pat == nil {
		p.text(name)
	} else {
		p.bindingPattern(pat)
	}
	if typ != nil {
		p.text(": ")
		p.typ(typ)
	}
	p.text(" = ")
	p.expr(value, 0)
}
func (p *printer) block(b *ast.Block) {
	defer p.mark(b.Pos, b.End)()
	p.open(b.Pos)
	for _, s := range b.Stmts {
		switch s := s.(type) {
		case *ast.LetStmt:
			end := p.mark(s.Pos, s.End)
			p.binding(s.Name, s.Pat, s.Type, s.Value)
			end()
		case *ast.ExprStmt:
			p.statementExpr(s.X)
		case *ast.FuncDecl:
			p.decl(s)
		default:
			panic(fmt.Sprintf("format: unsupported statement %T", s))
		}
		p.newline()
	}
	p.statementExpr(b.Result)
	p.close(b.End)
}

// A leading negative expression is a layout continuation, not a new statement.
// Parentheses separate it without adding a semantic node.
func (p *printer) statementExpr(e ast.Expr) {
	if negativeStart(e) {
		p.text("(")
		p.expr(e, 0)
		p.text(")")
		return
	}
	p.expr(e, 0)
}

func negativeStart(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Unary:
		return e.Op == "-"
	case *ast.Binary:
		return negativeStart(e.L)
	case *ast.Call:
		return e.Pipe != nil && negativeStart(e.Pipe.L)
	}
	return false
}

func (p *printer) bindingPattern(pat ast.Pattern) {
	_, variable := pat.(*ast.PVar)
	if variable {
		p.text("(")
	}
	p.pattern(pat)
	if variable {
		p.text(")")
	}
}

var precedence = map[string]int{"|>": 1, "||": 2, "&&": 3, "==": 4, "!=": 4, "<": 4, "<=": 4, ">": 4, ">=": 4, "+": 5, "-": 5, "*": 6, "/": 6, "%": 6}

func exprStart(e ast.Expr) ast.Pos {
	switch e := e.(type) {
	case *ast.Binary:
		return exprStart(e.L)
	case *ast.Call:
		if e.Pipe != nil {
			return exprStart(e.Pipe.L)
		}
		return exprStart(e.Fn)
	case *ast.Field:
		return exprStart(e.X)
	case *ast.RecordUpdate:
		return exprStart(e.X)
	case *ast.Lambda:
		if e.Partial && !e.Section {
			return exprStart(e.Body)
		}
	}
	return e.GetPos()
}
func (p *printer) expr(e ast.Expr, parent int) {
	defer p.mark(exprStart(e), e.GetEnd())()
	prec := 9
	switch e := e.(type) {
	case *ast.Binary:
		prec = precedence[e.Op]
	case *ast.Call:
		if e.Pipe != nil {
			prec = 1
		} else {
			prec = 8
		}
	case *ast.Field, *ast.RecordUpdate:
		prec = 8
	case *ast.Unary:
		prec = 7
	case *ast.Lambda:
		if !e.Partial {
			prec = 0
		}
	case *ast.If, *ast.Match, *ast.Do:
		prec = 0
	}
	parens := prec < parent
	if parens {
		p.text("(")
	}
	switch e := e.(type) {
	case *ast.Ident:
		if e.Placeholder {
			p.text("_")
		} else {
			p.text(e.Name)
		}
	case *ast.IntLit:
		p.text(strconv.FormatInt(e.Value, 10))
	case *ast.FloatLit:
		text := strconv.FormatFloat(e.Value, 'g', -1, 64)
		if !strings.ContainsAny(text, ".eE") {
			text += ".0"
		}
		p.text(text)
	case *ast.BoolLit:
		p.text(strconv.FormatBool(e.Value))
	case *ast.StringLit:
		p.text(quote(e.Value))
	case *ast.Interp:
		p.text("\"")
		for i, t := range e.Texts {
			q := quote(t)
			p.text(q[1 : len(q)-1])
			if i < len(e.Exprs) {
				pos := e.Pos
				if i < len(e.Starts) {
					pos = e.Starts[i]
					pos.Col -= 2
				}
				p.token("${", pos)
				old := p.inline
				p.inline = true
				p.expr(e.Exprs[i], 0)
				p.inline = old
				p.text("}")
			}
		}
		p.text("\"")
	case *ast.Unary:
		p.text(e.Op)
		p.expr(e.X, 7)
	case *ast.Binary:
		p.binary(e)
	case *ast.Call:
		if e.Pipe != nil {
			p.binary(e.Pipe)
		} else {
			p.expr(e.Fn, 8)
			p.token("(", e.Pos)
			for i, a := range e.Args {
				p.comma(i)
				p.expr(a, 0)
			}
			p.token(")", closePos(e.End))
		}
	case *ast.Field:
		p.expr(e.X, 8)
		p.text("." + e.Name)
	case *ast.RecordLit:
		p.text(e.Ctor)
		p.record(e.Pos, e.End, len(e.Values), func(i int) { p.field(e.Names[i], e.Values[i]) })
	case *ast.RecordUpdate:
		p.expr(e.X, 8)
		p.record(e.Pos, e.End, len(e.Values), func(i int) { p.field(e.Names[i], e.Values[i]) })
	case *ast.ListLit:
		p.text("[")
		for i, a := range e.Elems {
			p.comma(i)
			p.expr(a, 0)
		}
		p.text("]")
	case *ast.TupleLit:
		p.text("(")
		for i, a := range e.Elems {
			p.comma(i)
			p.expr(a, 0)
		}
		p.text(")")
	case *ast.Lambda:
		if e.Partial {
			if e.Section {
				p.text("(")
			}
			p.expr(e.Body, 0)
			if e.Section {
				p.text(")")
			}
		} else {
			p.text("func")
			p.params(e.Params)
			p.text(" => ")
			p.expr(e.Body, 0)
		}
	case *ast.BlockExpr:
		p.block(e.Block)
	case *ast.If:
		p.text("if ")
		p.expr(e.Cond, 0)
		p.text(" ")
		p.expr(e.Then, 0)
		p.text(" else ")
		p.expr(e.Else, 0)
	case *ast.Match:
		p.text("match ")
		p.expr(e.Scrut, 0)
		p.text(" ")
		p.open(e.Pos)
		arms := e.Arms
		if e.SourceArms != nil {
			arms = e.SourceArms
		}
		for i, a := range arms {
			if i > 0 {
				p.newline()
			}
			end := p.mark(a.Pos, a.End)
			if !p.inline && p.multilineArm(a) {
				p.text("| ")
			}
			p.arm(a)
			end()
		}
		p.close(e.End)
	case *ast.Do:
		p.text("do ")
		p.open(e.Pos)
		for _, s := range e.Stmts {
			switch s := s.(type) {
			case *ast.DoLet:
				end := p.mark(s.Pos, s.End)
				p.binding(s.Name, s.Pat, s.Type, s.Value)
				end()
			case *ast.DoBind:
				end := p.mark(s.Pos, s.End)
				if s.Pat != nil {
					p.bindingPattern(s.Pat)
				} else {
					p.text(s.Name)
				}
				p.text(" <- ")
				p.expr(s.X, 0)
				end()
			case *ast.DoExpr:
				p.statementExpr(s.X)
			case *ast.FuncDecl:
				p.decl(s)
			default:
				panic(fmt.Sprintf("format: unsupported do statement %T", s))
			}
			p.newline()
		}
		p.statementExpr(e.Result)
		p.close(e.End)
	default:
		panic(fmt.Sprintf("format: unsupported expression %T", e))
	}
	if parens {
		p.text(")")
	}
}
func quote(s string) string { return strconv.Quote(strings.ReplaceAll(s, "$", "$$")) }
func (p *printer) arm(a ast.Arm) {
	p.pattern(a.Pat)
	if a.Guard != nil {
		p.text(" if ")
		p.expr(a.Guard, 0)
	}
	p.text(" => ")
	p.expr(a.Body, 0)
}

func (p *printer) multilineArm(a ast.Arm) bool {
	q := &printer{indent: p.indent, col: p.col, comments: p.comments}
	q.arm(a)
	return strings.Contains(q.b.String(), "\n")
}
func (p *printer) binary(e *ast.Binary) {
	if e.Op != "|>" {
		p.expr(e.L, precedence[e.Op])
		p.text(" " + e.Op + " ")
		p.expr(e.R, precedence[e.Op]+1)
		return
	}
	// Measure the entire chain, so all steps agree on whether to break.
	var steps []ast.Expr
	var collect func(ast.Expr)
	collect = func(x ast.Expr) {
		if c, ok := x.(*ast.Call); ok && c.Pipe != nil {
			collect(c.Pipe.L)
			steps = append(steps, c.Pipe.R)
		} else {
			steps = append(steps, x)
		}
	}
	collect(e.L)
	steps = append(steps, e.R)
	q := &printer{inline: true}
	for i, s := range steps {
		if i > 0 {
			q.text(" |> ")
		}
		q.expr(s, 2)
	}
	wrap := !p.inline && p.col+utf8.RuneCountInString(q.b.String()) > 80
	p.expr(steps[0], 1)
	if wrap {
		p.indent++
	}
	for _, s := range steps[1:] {
		if wrap {
			p.newline()
			p.text("|> ")
		} else {
			p.text(" |> ")
		}
		p.expr(s, 2)
	}
	if wrap {
		p.indent--
	}
}
func (p *printer) field(name string, e ast.Expr) {
	defer p.mark(exprStart(e), e.GetEnd())()
	p.text(name + ": ")
	p.expr(e, 0)
}
func (p *printer) record(pos, end ast.Pos, n int, field func(int)) {
	// Render once into a temporary printer state, then keep it if it fits.
	oldB, oldCol, oldSpans := p.b, p.col, p.spans
	p.b = &strings.Builder{}
	p.spans = nil
	p.token("{", pos)
	for i := range n {
		p.comma(i)
		field(i)
	}
	p.text("}")
	text, spans := p.b.String(), p.spans
	p.b, p.col, p.spans = oldB, oldCol, oldSpans
	wrap := !p.inline && (oldCol+utf8.RuneCountInString(text) > 80 || strings.Contains(text, "\n") || p.hasComments(pos, end))
	if !wrap {
		offset := p.offset()
		p.text(text)
		for _, s := range spans {
			s.start += offset
			s.stop += offset
			p.spans = append(p.spans, s)
		}
		return
	}
	p.token("{", pos)
	p.indent++
	p.newline()
	for i := range n {
		if i > 0 {
			p.text(",")
			p.newline()
		}
		field(i)
	}
	p.close(end)
}
func (p *printer) pattern(pat ast.Pattern) {
	defer p.mark(pat.GetPos(), pat.GetEnd())()
	switch a := pat.(type) {
	case *ast.POr:
		for i, alt := range a.Alts {
			if i > 0 {
				p.text(" | ")
			}
			if _, nested := alt.(*ast.POr); nested {
				p.text("(")
				p.pattern(alt)
				p.text(")")
			} else {
				p.pattern(alt)
			}
		}
	case *ast.PVar:
		p.text(a.Name)
	case *ast.PWild:
		p.text("_")
	case *ast.PLit:
		p.expr(a.X, 0)
	case *ast.PCtor:
		if a.List {
			p.listPattern(a)
			break
		}
		p.text(a.Name)
		if len(a.Args) > 0 {
			p.text("(")
			for i, x := range a.Args {
				p.comma(i)
				p.pattern(x)
			}
			p.text(")")
		}
	case *ast.PTuple:
		p.text("(")
		for i, x := range a.Elems {
			p.comma(i)
			p.pattern(x)
		}
		p.text(")")
	case *ast.PAs:
		p.text(a.Name + " @ ")
		if _, or := a.Pat.(*ast.POr); or {
			p.text("(")
			p.pattern(a.Pat)
			p.text(")")
		} else {
			p.pattern(a.Pat)
		}
	case *ast.PRecord:
		p.text(a.Ctor)
		n := len(a.Names)
		if a.Rest {
			n++
		}
		p.record(a.Pos, a.End, n, func(i int) {
			if i == len(a.Names) {
				p.text("..")
			} else {
				p.text(a.Names[i] + ": ")
				p.pattern(a.Pats[i])
			}
		})
	default:
		panic(fmt.Sprintf("format: unsupported pattern %T", a))
	}
}

func (p *printer) listPattern(root *ast.PCtor) {
	p.text("[")
	var tail ast.Pattern = root
	for i := 0; ; i++ {
		if ctor, ok := tail.(*ast.PCtor); ok {
			if ctor.Name == "Nil" {
				break
			}
			p.comma(i)
			p.pattern(ctor.Args[0])
			tail = ctor.Args[1]
			continue
		}
		p.text(", ..")
		if _, wild := tail.(*ast.PWild); !wild {
			p.pattern(tail)
		}
		break
	}
	p.text("]")
}
