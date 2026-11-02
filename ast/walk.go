package ast

// Walk visits a syntax node and its children in source order. Annotations are
// deliberately excluded: they contain semantic types and links back to syntax.
func Walk(n Node, visit func(Node)) {
	if n == nil {
		return
	}
	visit(n)
	walk := func(n Node) { Walk(n, visit) }
	params := func(ps []Param) {
		for i := range ps {
			walk(&ps[i])
		}
	}
	switch n := n.(type) {
	case *ClassDecl:
		for i := range n.Methods {
			walk(&n.Methods[i])
		}
	case *MethodSig:
		params(n.Params)
		walk(n.Ret)
	case *InstanceDecl:
		for i := range n.Context {
			walk(&n.Context[i])
		}
		for i := range n.Methods {
			walk(&n.Methods[i])
		}
	case *InstMethod:
		if n.Body != nil {
			walk(n.Body)
		}
	case *TypeDecl:
		for i := range n.Ctors {
			walk(&n.Ctors[i])
		}
	case *AliasDecl:
		walk(n.Type)
	case *Ctor:
		for _, t := range n.Fields {
			walk(t)
		}
	case *FuncDecl:
		params(n.Params)
		walk(n.Ret)
		if n.Body != nil {
			walk(n.Body)
		}
	case *LawDecl:
		params(n.Params)
		walk(n.LHS)
		walk(n.RHS)
	case *ProofDecl:
		params(n.Params)
		walk(n.Body)
	case *TMatch:
		walk(n.Var)
		for i := range n.Arms {
			walk(&n.Arms[i])
		}
	case *TacticArm:
		walk(n.Pat)
		walk(n.Body)
	case *TIf:
		walk(n.Cond)
		walk(n.Then)
		walk(n.Else)
	case *TRewrite:
		walk(n.Law)
		for _, arg := range n.Args {
			walk(arg)
		}
		walk(n.Body)
	case *Param:
		walk(n.Type)
		walk(n.Pat)
	case *TypeName:
		for _, t := range n.Args {
			walk(t)
		}
	case *FuncType:
		for _, t := range n.Params {
			walk(t)
		}
		walk(n.Ret)
	case *TupleType:
		for _, t := range n.Elems {
			walk(t)
		}
	case *Block:
		for _, s := range n.Stmts {
			switch s := s.(type) {
			case *LetStmt:
				walk(s)
			case *FuncDecl:
				walk(s)
			case *ExprStmt:
				walk(s.X)
			}
		}
		walk(n.Result)
	case *LetStmt:
		walk(n.Pat)
		walk(n.Type)
		walk(n.Value)
	case *DoLet:
		walk(n.Pat)
		walk(n.Type)
		walk(n.Value)
	case *DoBind:
		walk(n.Pat)
		walk(n.X)
	case *Do:
		for _, s := range n.Stmts {
			switch s := s.(type) {
			case *DoBind:
				walk(s)
			case *DoLet:
				walk(s)
			case *FuncDecl:
				walk(s)
			case *DoExpr:
				walk(s.X)
			}
		}
		walk(n.Result)
	case *Interp:
		for _, e := range n.Exprs {
			walk(e)
		}
	case *Field:
		walk(n.X)
	case *RecordLit:
		for _, e := range n.Values {
			walk(e)
		}
	case *RecordUpdate:
		walk(n.X)
		for _, e := range n.Values {
			walk(e)
		}
	case *Call:
		walk(n.Fn)
		for _, e := range n.Args {
			walk(e)
		}
	case *Lambda:
		params(n.Params)
		walk(n.Body)
	case *BlockExpr:
		if n.Block != nil {
			walk(n.Block)
		}
	case *Match:
		walk(n.Scrut)
		for i := range n.Arms {
			walk(&n.Arms[i])
		}
	case *Arm:
		walk(n.Pat)
		walk(n.Guard)
		walk(n.Body)
	case *If:
		walk(n.Cond)
		walk(n.Then)
		walk(n.Else)
	case *Binary:
		walk(n.L)
		walk(n.R)
	case *Unary:
		walk(n.X)
	case *ListLit:
		for _, e := range n.Elems {
			walk(e)
		}
	case *TupleLit:
		for _, e := range n.Elems {
			walk(e)
		}
	case *PCtor:
		for _, p := range n.Args {
			walk(p)
		}
	case *PLit:
		walk(n.X)
	case *PTuple:
		for _, p := range n.Elems {
			walk(p)
		}
	case *PRecord:
		for _, p := range n.Pats {
			walk(p)
		}
	case *PAs:
		walk(n.Pat)
	}
}

// EndAt finds the smallest source span starting at pos. This is used only when
// enriching an existing positioned diagnostic, not on the successful check path.
func EndAt(n Node, pos Pos) Pos {
	var end Pos
	Walk(n, func(n Node) {
		e := n.GetEnd()
		if n.GetPos() == pos && e != (Pos{}) && (end == (Pos{}) || e.Line < end.Line || e.Line == end.Line && e.Col < end.Col) {
			end = e
		}
	})
	return end
}
