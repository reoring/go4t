package types

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
)

// SymbolKind describes a source declaration, independently of an editor protocol.
type SymbolKind int

const (
	FunctionSymbol SymbolKind = iota
	TypeSymbol
	ConstructorSymbol
	FieldSymbol
	ClassSymbol
	MethodSymbol
	AliasSymbol
	VariableSymbol
	TypeParameterSymbol
	InstanceSymbol
	LawSymbol
	ProofSymbol
)

// Symbol is a declaration and its exclusive, one-based rune-column source spans.
// Builtin types have no source declaration and therefore no Symbol.
type Symbol struct {
	Name, File, Doc  string
	Kind             SymbolKind
	Type             Type
	Pos, End         ast.Pos
	NamePos, NameEnd ast.Pos
	Children         []*Symbol
}

// Binding is one identifier occurrence. Type is the instantiated type at a use
// site, while Definition.Type is the declaration's (possibly polymorphic) type.
type Binding struct {
	Name       string
	Pos, End   ast.Pos
	Type       Type
	Definition *Symbol
}

// PositionIndex belongs to a successfully checked Program. It does not change
// annotations or checker behavior. Build a fresh index after each check.
type PositionIndex struct {
	files   map[string][]Binding
	symbols map[string][]*Symbol
	ioLifts map[string][]Binding
}

func (x *PositionIndex) At(file string, pos ast.Pos) *Binding {
	bs := x.files[file]
	i := sort.Search(len(bs), func(i int) bool { return less(pos, bs[i].Pos) }) - 1
	var binding *Binding
	if i >= 0 && !less(pos, bs[i].Pos) && less(pos, bs[i].End) {
		binding = &bs[i]
	}
	for _, lift := range x.ioLifts[file] {
		if less(pos, lift.Pos) || !less(pos, lift.End) {
			continue
		}
		if binding == nil {
			copy := lift
			binding = &copy
		} else if binding.Pos == lift.Pos {
			copy := *binding
			copy.Type = lift.Type
			binding = &copy
		}
	}
	return binding
}
func (x *PositionIndex) Symbols(file string) []*Symbol { return x.symbols[file] }

// PositionIndex builds tooling information from existing checked annotations.
// Source-free synthesized nodes are excluded. Ordinary compilation pays no index
// construction cost; clients explicitly request an index when they need it.
func (p *Program) PositionIndex() *PositionIndex {
	b := &indexBuilder{p: p, index: &PositionIndex{files: map[string][]Binding{}, symbols: map[string][]*Symbol{}, ioLifts: map[string][]Binding{}},
		tokens: map[*ast.File][]lexer.Token{}, values: map[string]*Symbol{}, types: map[string]*Symbol{}, ctors: map[string]*Symbol{}, laws: map[string]*Symbol{}, fields: map[*Ctor][]*Symbol{}}
	for _, f := range p.Files {
		toks, err := sourceTokens(f.Source, ast.Pos{Line: 1, Col: 1})
		if err != nil {
			continue
		}
		b.tokens[f] = toks
		for _, d := range f.Decls {
			if d.GetEnd() == (ast.Pos{}) {
				continue
			}
			if s := b.declaration(f, d); s != nil {
				b.index.symbols[f.Name] = append(b.index.symbols[f.Name], s)
			}
		}
	}
	for _, f := range p.Files {
		b.locals = nil
		for _, d := range f.Decls {
			if d.GetEnd() == (ast.Pos{}) {
				continue
			}
			b.collectLocals(f, d)
		}
		for _, d := range f.Decls {
			if d.GetEnd() == (ast.Pos{}) {
				continue
			}
			ast.Walk(d, func(n ast.Node) { b.reference(f, n) })
		}
	}
	for name, bs := range b.index.files {
		sort.SliceStable(bs, func(i, j int) bool { return less(bs[i].Pos, bs[j].Pos) })
		b.index.files[name] = bs
	}
	return b.index
}

// Interpolation expressions are lexed separately by the parser. Include their
// original-coordinate tokens as well, so fields and local declarations inside
// strings have the same definition support as ordinary expression syntax.
func sourceTokens(source string, base ast.Pos) ([]lexer.Token, error) {
	tokens, err := lexer.NewAt(source, base).All()
	if err != nil {
		return nil, err
	}
	result := tokens
	for _, token := range tokens {
		for _, part := range token.Parts {
			if part.Pos == (ast.Pos{}) {
				continue
			}
			nested, err := sourceTokens(part.Expr, part.Pos)
			if err != nil {
				return nil, err
			}
			result = append(result, nested...)
		}
	}
	if len(result) != len(tokens) {
		sort.SliceStable(result, func(i, j int) bool { return less(result[i].Pos, result[j].Pos) })
	}
	return result, nil
}

type scopedSymbol struct {
	symbol     *Symbol
	start, end ast.Pos
	typeName   bool
}
type indexBuilder struct {
	p                    *Program
	index                *PositionIndex
	tokens               map[*ast.File][]lexer.Token
	values, types, ctors map[string]*Symbol
	laws                 map[string]*Symbol
	fields               map[*Ctor][]*Symbol
	locals               []scopedSymbol
}

func less(a, b ast.Pos) bool { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }
func nameEnd(pos ast.Pos, name string) ast.Pos {
	return ast.Pos{Line: pos.Line, Col: pos.Col + utf8.RuneCountInString(name)}
}
func (b *indexBuilder) sourceName(name string) string {
	if original := b.p.ModuleNames[name]; original != "" {
		return original
	}
	return name
}

func (b *indexBuilder) globalRef(f *ast.File, name string, start, end ast.Pos, t Type, s *Symbol) {
	display := b.sourceName(name)
	b.add(f, b.token(f, display, start, end), display, t, s)
}

func (b *indexBuilder) token(f *ast.File, name string, start, end ast.Pos) ast.Pos {
	ts := b.tokens[f]
	i := sort.Search(len(ts), func(i int) bool { return !less(ts[i].Pos, start) })
	for ; i < len(ts) && less(ts[i].Pos, end); i++ {
		if ts[i].Kind == lexer.Ident && ts[i].Text == name {
			return ts[i].Pos
		}
	}
	return ast.Pos{}
}
func docComment(f *ast.File, line int) string {
	i := sort.Search(len(f.Comments), func(i int) bool { return f.Comments[i].Pos.Line >= line }) - 1
	last := i
	for i >= 0 && !f.Comments[i].Trailing && f.Comments[i].Pos.Line == line-1 {
		line--
		i--
	}
	if i == last {
		return ""
	}
	lines := make([]string, 0, last-i)
	for _, c := range f.Comments[i+1 : last+1] {
		lines = append(lines, strings.TrimSpace(strings.TrimPrefix(c.Text, "//")))
	}
	return strings.Join(lines, "\n")
}
func (b *indexBuilder) symbol(f *ast.File, name string, kind SymbolKind, n ast.Node, t Type) *Symbol {
	name = b.sourceName(name)
	pos := b.token(f, name, n.GetPos(), n.GetEnd())
	if pos == (ast.Pos{}) {
		return nil
	}
	s := &Symbol{Name: name, File: f.Name, Kind: kind, Type: t, Pos: n.GetPos(), End: n.GetEnd(), NamePos: pos, NameEnd: nameEnd(pos, name), Doc: docComment(f, n.GetPos().Line)}
	// extern's AST starts at func, possibly on a separate line from extern.
	if fn, ok := n.(*ast.FuncDecl); ok && fn.Extern != nil {
		ts := b.tokens[f]
		i := sort.Search(len(ts), func(i int) bool { return !less(ts[i].Pos, fn.Pos) })
		if i > 0 && ts[i-1].Text == "extern" {
			s.Pos = ts[i-1].Pos
			s.Doc = docComment(f, s.Pos.Line)
		}
	}
	b.add(f, pos, name, t, s)
	return s
}
func (b *indexBuilder) add(f *ast.File, pos ast.Pos, name string, t Type, s *Symbol) {
	if pos == (ast.Pos{}) {
		return
	}
	b.index.files[f.Name] = append(b.index.files[f.Name], Binding{Name: name, Pos: pos, End: nameEnd(pos, name), Type: t, Definition: s})
}
func (b *indexBuilder) ctorType(c *Ctor) Type {
	args := make([]Type, len(c.Data.TParams))
	for i, t := range c.Data.TParams {
		args[i] = t
	}
	ret := App(&TCon{Name: c.Data.Name, Display: b.p.ModuleNames[c.Data.Name], Kind: arityKind(len(args))}, args...)
	if len(c.Fields) == 0 {
		return ret
	}
	return &TFunc{Params: c.Fields, Ret: ret}
}
func (b *indexBuilder) declaration(f *ast.File, d ast.Decl) *Symbol {
	switch d := d.(type) {
	case *ast.ConstDecl:
		info := b.p.Funcs[d.Name]
		s := b.symbol(f, d.Name, VariableSymbol, d, info.Scheme.Type.Ret)
		b.values[d.Name] = s
		return s
	case *ast.FuncDecl:
		info := b.p.Funcs[d.Name]
		if info == nil {
			return nil
		}
		s := b.symbol(f, d.Name, FunctionSymbol, d, info.Scheme.Type)
		b.values[d.Name] = s
		return s
	case *ast.LawDecl:
		info := b.p.Laws[d.Name]
		s := b.symbol(f, d.Name, LawSymbol, d, &TFunc{Params: info.ParamTypes, Ret: exprType(d.LHS)})
		b.laws[d.Name] = s
		return s
	case *ast.ProofDecl:
		info := b.p.Proofs[d.Name]
		return b.symbol(f, d.Name, ProofSymbol, d, &TFunc{Params: info.Law.ParamTypes, Ret: exprType(info.Law.Decl.LHS)})
	case *ast.TypeDecl:
		data := b.p.DataTypes[d.Name]
		if data == nil {
			return nil
		}
		s := b.symbol(f, d.Name, TypeSymbol, d, &TCon{Name: d.Name, Display: b.p.ModuleNames[d.Name], Kind: arityKind(len(data.TParams))})
		b.types[d.Name] = s
		for i := range d.Ctors {
			c := &d.Ctors[i]
			cc := data.Ctors[i]
			var source ast.Node = c
			if d.Shorthand {
				source = d
			}
			child := b.symbol(f, c.Name, ConstructorSymbol, source, b.ctorType(cc))
			b.ctors[c.Name] = child
			if s != nil && child != nil {
				s.Children = append(s.Children, child)
			}
			start := c.Pos
			for j, name := range c.FieldNames {
				pos := b.token(f, name, start, c.Fields[j].GetPos())
				start = c.Fields[j].GetEnd()
				field := &Symbol{Name: name, File: f.Name, Kind: FieldSymbol, Type: cc.Fields[j], Pos: pos, End: c.Fields[j].GetEnd(), NamePos: pos, NameEnd: nameEnd(pos, name), Doc: docComment(f, pos.Line)}
				b.fields[cc] = append(b.fields[cc], field)
				b.add(f, pos, name, field.Type, field)
				if child != nil {
					child.Children = append(child.Children, field)
				}
			}
		}
		return s
	case *ast.AliasDecl:
		s := b.symbol(f, d.Name, AliasSymbol, d, &TCon{Name: d.Name, Display: b.p.ModuleNames[d.Name], Kind: arityKind(len(d.TParams))})
		b.types[d.Name] = s
		return s
	case *ast.ClassDecl:
		class := b.p.Classes[d.Name]
		if class == nil {
			return nil
		}
		s := b.symbol(f, d.Name, ClassSymbol, d, nil)
		b.types[d.Name] = s
		for i := range d.Methods {
			m := &d.Methods[i]
			child := b.symbol(f, m.Name, MethodSymbol, m, class.Methods[i].Scheme.Type)
			b.values[m.Name] = child
			if s != nil && child != nil {
				s.Children = append(s.Children, child)
			}
		}
		return s
	case *ast.InstanceDecl:
		// Instances are containers; method calls navigate to the class signature.
		class, con := b.sourceName(d.Class), b.sourceName(d.TypeCon)
		pos := b.token(f, class, d.Pos, d.End)
		if pos == (ast.Pos{}) {
			return nil
		}
		s := &Symbol{Name: class + "[" + con + "]", File: f.Name, Kind: InstanceSymbol, Pos: d.Pos, End: d.End, NamePos: pos, NameEnd: nameEnd(pos, class), Doc: docComment(f, d.Pos.Line)}
		for i := range d.Methods {
			m := &d.Methods[i]
			child := b.symbol(f, m.Name, MethodSymbol, m, nil)
			if child != nil {
				s.Children = append(s.Children, child)
			}
		}
		return s
	}
	return nil
}
func exprType(e ast.Expr) Type {
	if e == nil {
		return nil
	}
	a, ok := (*e.AnnSlot()).(*Ann)
	if !ok {
		return nil
	}
	return a.Type
}
func (b *indexBuilder) local(f *ast.File, name string, pos, start, end ast.Pos, t Type, typeName bool) *Symbol {
	if name == "" || pos == (ast.Pos{}) {
		return nil
	}
	kind := VariableSymbol
	if typeName {
		kind = TypeParameterSymbol
	}
	s := &Symbol{Name: name, File: f.Name, Kind: kind, Type: t, Pos: pos, End: nameEnd(pos, name), NamePos: pos, NameEnd: nameEnd(pos, name)}
	b.locals = append(b.locals, scopedSymbol{s, start, end, typeName})
	b.add(f, pos, name, t, s)
	return s
}
func (b *indexBuilder) lookup(name string, pos ast.Pos, typeName bool) *Symbol {
	var found *scopedSymbol
	for i := range b.locals {
		s := &b.locals[i]
		if s.symbol.Name != name || s.typeName != typeName || less(pos, s.start) || !less(pos, s.end) {
			continue
		}
		if found == nil || less(found.start, s.start) || found.start == s.start && less(s.end, found.end) {
			found = s
		}
	}
	if found != nil {
		return found.symbol
	}
	return nil
}
func (b *indexBuilder) params(f *ast.File, ps []ast.Param, t Type, start, end ast.Pos) {
	ft, _ := Unalias(t).(*TFunc)
	for i, p := range ps {
		var typ Type
		if ft != nil && i < len(ft.Params) {
			typ = ft.Params[i]
		}
		if p.Pat != nil {
			b.pattern(f, p.Pat, typ, start, end)
		} else {
			b.local(f, p.Name, p.Pos, start, end, typ, false)
		}
	}
}
func (b *indexBuilder) pattern(f *ast.File, p ast.Pattern, t Type, start, end ast.Pos) {
	if p == nil {
		return
	}
	switch p := p.(type) {
	case *ast.POr:
		for _, alt := range p.Alts {
			b.pattern(f, alt, t, start, end)
		}
	case *ast.PVar:
		b.local(f, p.Name, p.Pos, start, end, t, false)
	case *ast.PAs:
		b.local(f, p.Name, p.Pos, start, end, t, false)
		b.pattern(f, p.Pat, t, start, end)
	case *ast.PTuple:
		app, _ := Unalias(t).(*TApp)
		for i, p := range p.Elems {
			var sub Type
			if app != nil && i < len(app.Args) {
				sub = app.Args[i]
			}
			b.pattern(f, p, sub, start, end)
		}
	case *ast.PCtor:
		c := b.ctor(p.Name)
		fields := instFields(c, t)
		for i, p := range p.Args {
			var sub Type
			if i < len(fields) {
				sub = fields[i]
			}
			b.pattern(f, p, sub, start, end)
		}
	case *ast.PRecord:
		resolved := b.p.RecordPats[p]
		fields := instFields(resolved.Ctor, t)
		for i, p := range p.Pats {
			var sub Type
			if i < len(resolved.Indexes) && resolved.Indexes[i] < len(fields) {
				sub = fields[resolved.Indexes[i]]
			}
			b.pattern(f, p, sub, start, end)
		}
	}
}
func (b *indexBuilder) ctor(name string) *Ctor {
	for _, data := range b.p.DataTypes {
		for _, c := range data.Ctors {
			if c.Name == name {
				return c
			}
		}
	}
	return nil
}
func instFields(c *Ctor, t Type) []Type {
	if c == nil {
		return nil
	}
	app, _ := Unalias(t).(*TApp)
	if app == nil {
		return c.Fields
	}
	sub := map[*TVar]Type{}
	for i, p := range c.Data.TParams {
		if i < len(app.Args) {
			sub[p] = app.Args[i]
		}
	}
	return substituteList(c.Fields, sub)
}
func (b *indexBuilder) let(f *ast.File, name string, pos ast.Pos, pat ast.Pattern, value ast.Expr, end ast.Pos) {
	start := value.GetEnd()
	if a, ok := (*value.AnnSlot()).(*Ann); ok && a.Recursive {
		start = pos
	}
	if pat != nil {
		b.pattern(f, pat, exprType(value), start, end)
	} else {
		b.local(f, name, pos, start, end, exprType(value), false)
	}
}
func (b *indexBuilder) functions(f *ast.File, group []*ast.FuncDecl, end ast.Pos) {
	if len(group) == 0 {
		return
	}
	start := group[0].Pos
	for _, fn := range group {
		// Local functions have explicit parameter and return annotations.
		ft := &TFunc{Ret: b.syntaxType(fn.Ret, fn.Pos), Params: make([]Type, len(fn.Params))}
		for i, p := range fn.Params {
			ft.Params[i] = b.syntaxType(p.Type, fn.Pos)
		}
		pos := b.token(f, fn.Name, fn.Pos, fn.End)
		if symbol := b.local(f, fn.Name, pos, start, end, ft, false); symbol != nil {
			symbol.Kind, symbol.Pos, symbol.End = FunctionSymbol, fn.Pos, fn.End
			symbol.Doc = docComment(f, fn.Pos.Line)
		}
	}
}
func (b *indexBuilder) syntaxType(t ast.Type, pos ast.Pos) Type {
	switch t := t.(type) {
	case *ast.TypeName:
		var base Type = &TCon{Name: t.Name, Display: b.p.ModuleNames[t.Name]}
		if local := b.lookup(t.Name, pos, true); local != nil {
			base = local.Type
		}
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = b.syntaxType(a, pos)
		}
		return App(base, args...)
	case *ast.FuncType:
		ft := &TFunc{Ret: b.syntaxType(t.Ret, pos), Params: make([]Type, len(t.Params))}
		for i, p := range t.Params {
			ft.Params[i] = b.syntaxType(p, pos)
		}
		return ft
	case *ast.TupleType:
		args := make([]Type, len(t.Elems))
		for i, a := range t.Elems {
			args[i] = b.syntaxType(a, pos)
		}
		return App(Tuple(len(args)), args...)
	}
	return nil
}
func (b *indexBuilder) collectLocals(f *ast.File, d ast.Decl) {
	ast.Walk(d, func(n ast.Node) {
		var tparams []string
		switch n := n.(type) {
		case *ast.LawDecl:
			for _, tp := range n.TParams {
				tparams = append(tparams, tp.Name)
			}
			info := b.p.Laws[n.Name]
			b.params(f, n.Params, &TFunc{Params: info.ParamTypes}, n.Pos, n.End)
		case *ast.ProofDecl:
			info := b.p.Proofs[n.Name]
			b.params(f, n.Params, &TFunc{Params: info.Law.ParamTypes}, n.Pos, n.End)
		case *ast.TMatch:
			var scrutinee ast.Expr = n.Var
			if n.Scrut != nil {
				scrutinee = n.Scrut
			}
			for _, arm := range n.Arms {
				if or, ok := arm.Pat.(*ast.POr); ok {
					for _, alternative := range or.Alts {
						b.pattern(f, alternative, exprType(scrutinee), arm.Pos, arm.End)
					}
				} else {
					b.pattern(f, arm.Pat, exprType(scrutinee), arm.Pos, arm.End)
				}
			}
		case *ast.FuncDecl:
			for _, tp := range n.TParams {
				tparams = append(tparams, tp.Name)
			}
			var typ Type
			if info := b.p.Funcs[n.Name]; info != nil && info.Decl == n {
				typ = info.Scheme.Type
			} else {
				ft := &TFunc{Params: make([]Type, len(n.Params))}
				for i, p := range n.Params {
					ft.Params[i] = b.syntaxType(p.Type, n.Pos)
				}
				typ = ft
			}
			b.params(f, n.Params, typ, n.Pos, n.End)
		case *ast.MethodSig:
			for _, tp := range n.TParams {
				tparams = append(tparams, tp.Name)
			}
		case *ast.ClassDecl:
			tparams = []string{n.Param.Name}
		case *ast.TypeDecl:
			tparams = n.TParams
		case *ast.AliasDecl:
			tparams = n.TParams
		case *ast.InstanceDecl:
			tparams = n.Params
		case *ast.InstMethod:
			start := nameEnd(n.Pos, b.sourceName(n.Name))
			var signature *TFunc
			for _, instance := range b.p.Instances {
				if instance.Decl != d {
					continue
				}
				for _, method := range instance.Class.Methods {
					if method.Name == n.Name {
						signature = substitute(method.Scheme.Type, map[*TVar]Type{instance.Class.Param: instance.head()}).(*TFunc)
						break
					}
				}
				break
			}
			for i, name := range n.Params {
				pos := b.token(f, name, start, n.Body.Pos)
				var typ Type
				if signature != nil && i < len(signature.Params) {
					typ = signature.Params[i]
				}
				b.local(f, name, pos, n.Pos, n.End, typ, false)
				start = nameEnd(pos, name)
			}
		case *ast.Lambda:
			b.params(f, n.Params, exprType(n), n.Pos, n.End)
		case *ast.Match:
			arms := n.Arms
			if n.SourceArms != nil {
				arms = n.SourceArms
			}
			for _, a := range arms {
				b.pattern(f, a.Pat, exprType(n.Scrut), a.Pos, a.End)
			}
		case *ast.Block:
			var group []*ast.FuncDecl
			for _, s := range n.Stmts {
				if fn, ok := s.(*ast.FuncDecl); ok {
					group = append(group, fn)
					continue
				}
				b.functions(f, group, n.End)
				group = nil
				if s, ok := s.(*ast.LetStmt); ok {
					b.let(f, s.Name, s.NamePos, s.Pat, s.Value, n.End)
				}
			}
			b.functions(f, group, n.End)
		case *ast.Do:
			var group []*ast.FuncDecl
			for _, s := range n.Stmts {
				if fn, ok := s.(*ast.FuncDecl); ok {
					group = append(group, fn)
					continue
				}
				b.functions(f, group, n.End)
				group = nil
				switch s := s.(type) {
				case *ast.DoLet:
					b.let(f, s.Name, s.NamePos, s.Pat, s.Value, n.End)
				case *ast.DoBind:
					var typ Type
					if app, ok := Unalias(exprType(s.X)).(*TApp); ok && len(app.Args) > 0 {
						typ = app.Args[len(app.Args)-1]
					}
					if s.Pat != nil {
						b.pattern(f, s.Pat, typ, s.X.GetEnd(), n.End)
					} else {
						b.local(f, s.Name, s.Pos, s.X.GetEnd(), n.End, typ, false)
					}
				}
			}
			b.functions(f, group, n.End)
		}
		start := n.GetPos()
		for _, name := range tparams {
			pos := b.token(f, name, start, n.GetEnd())
			b.local(f, name, pos, n.GetPos(), n.GetEnd(), &TVar{Name: name, Kind: KStar{}}, true)
			start = nameEnd(pos, name)
		}
	})
}
func (b *indexBuilder) fieldRefs(f *ast.File, n ast.Node, names []string, values []ast.Expr, c *Ctor) {
	fields := b.fields[c]
	start := n.GetPos()
	if update, ok := n.(*ast.RecordUpdate); ok {
		start = update.X.GetEnd()
	}
	pattern, isPattern := n.(*ast.PRecord)
	for i, name := range names {
		end := n.GetEnd()
		if i < len(values) {
			end = values[i].GetPos()
		}
		if isPattern {
			end = pattern.Pats[i].GetPos()
		}
		pos := b.token(f, name, start, end)
		for _, field := range fields {
			if field.Name == name {
				b.add(f, pos, name, field.Type, field)
				break
			}
		}
		if i < len(values) {
			start = values[i].GetEnd()
		} else {
			if isPattern {
				start = pattern.Pats[i].GetEnd()
			} else {
				start = nameEnd(pos, name)
			}
		}
	}
}
func (b *indexBuilder) reference(f *ast.File, n ast.Node) {
	switch n := n.(type) {
	case *ast.Call:
		if a, ok := (*n.AnnSlot()).(*Ann); ok && a.ImplicitIO {
			// Hover the expression's own token, not its arguments or operands.
			tokens := b.tokens[f]
			i := sort.Search(len(tokens), func(i int) bool { return !less(tokens[i].Pos, n.Pos) })
			if i < len(tokens) && tokens[i].Pos == n.Pos {
				b.index.ioLifts[f.Name] = append(b.index.ioLifts[f.Name], Binding{
					Pos: n.Pos, End: tokens[i].End, Type: a.Type,
				})
			}
		}
	case *ast.Ident:
		a, ok := (*n.AnnSlot()).(*Ann)
		if !ok {
			return
		}
		var s *Symbol
		switch a.Ref {
		case RefFunc, RefMethod:
			s = b.values[n.Name]
		case RefCtor:
			s = b.ctors[n.Name]
		case RefLaw:
			s = b.laws[n.Name]
		case RefLocal:
			s = b.lookup(n.Name, n.Pos, false)
		}
		if a.Ref == RefLocal {
			b.add(f, n.Pos, n.Name, a.Type, s)
		} else {
			b.globalRef(f, n.Name, n.Pos, n.End, a.Type, s)
		}
	case *ast.TypeName:
		s := b.lookup(n.Name, n.Pos, true)
		if s == nil {
			s = b.types[n.Name]
		}
		if b.lookup(n.Name, n.Pos, true) != nil {
			b.add(f, n.Pos, n.Name, b.syntaxType(n, n.Pos), s)
		} else {
			b.globalRef(f, n.Name, n.Pos, n.End, b.syntaxType(n, n.Pos), s)
		}
	case *ast.Field:
		a, ok := (*n.AnnSlot()).(*Ann)
		if !ok {
			return
		}
		for _, s := range b.fields[a.Ctor] {
			if s.Name == n.Name {
				pos := b.token(f, n.Name, n.X.GetEnd(), n.End)
				b.add(f, pos, n.Name, a.Type, s)
				break
			}
		}
	case *ast.RecordLit:
		a, ok := (*n.AnnSlot()).(*Ann)
		if !ok {
			return
		}
		b.globalRef(f, n.Ctor, n.Pos, n.End, a.Type, b.ctors[n.Ctor])
		b.fieldRefs(f, n, n.Names, n.Values, a.Ctor)
	case *ast.RecordUpdate:
		a, ok := (*n.AnnSlot()).(*Ann)
		if !ok {
			return
		}
		b.fieldRefs(f, n, n.Names, n.Values, a.Ctor)
	case *ast.PCtor:
		b.globalRef(f, n.Name, n.Pos, n.End, nil, b.ctors[n.Name])
	case *ast.PRecord:
		b.globalRef(f, n.Ctor, n.Pos, n.End, nil, b.ctors[n.Ctor])
		b.fieldRefs(f, n, n.Names, nil, b.p.RecordPats[n].Ctor)
	case *ast.InstanceDecl:
		b.typeRef(f, n.Class, n.Pos, n.End)
		b.typeRef(f, n.TypeCon, nameEnd(n.Pos, "instance"), n.End)
	case *ast.Constraint:
		b.typeRef(f, n.Class, n.Pos, n.End)
	case *ast.ClassDecl:
		b.constraintRefs(f, []ast.TypeParam{n.Param}, n.Pos, n.End)
		if n.Super != "" {
			b.typeRef(f, n.Super, nameEnd(n.Pos, "class"), n.End)
		}
	case *ast.FuncDecl:
		b.constraintRefs(f, n.TParams, n.Pos, n.End)
	case *ast.MethodSig:
		b.constraintRefs(f, n.TParams, n.Pos, n.End)
	case *ast.TypeDecl:
		start := n.Pos
		if len(n.Ctors) > 0 {
			start = n.Ctors[len(n.Ctors)-1].End
		}
		for _, name := range n.Deriving {
			b.typeRef(f, name, start, n.End)
		}
	}
}
func (b *indexBuilder) typeRef(f *ast.File, name string, start, end ast.Pos) {
	if s := b.types[name]; s != nil {
		b.globalRef(f, name, start, end, s.Type, s)
	}
}

func (b *indexBuilder) constraintRefs(f *ast.File, params []ast.TypeParam, start, end ast.Pos) {
	for _, param := range params {
		pos := b.token(f, param.Name, start, end)
		start = nameEnd(pos, param.Name)
		for _, class := range param.Constraints {
			b.typeRef(f, class, start, end)
			display := b.sourceName(class)
			start = nameEnd(b.token(f, display, start, end), display)
		}
	}
}
