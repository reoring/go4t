// Package ast defines the gofx surface syntax tree.
package ast

import "fmt"

type Pos struct{ Line, Col int }

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// ---- syntactic types ----

type Type interface{ typeNode() }

// TypeName is `Name` or `Name[Args]`; Name may be a type parameter.
type TypeName struct {
	Pos  Pos
	Name string
	Args []Type
}

type FuncType struct {
	Pos    Pos
	Params []Type
	Ret    Type
}

// TupleType is `(A, B)`; arity >= 2. Resolves to the builtin TCon `Tuple<n>`.
type TupleType struct {
	Pos   Pos
	Elems []Type
}

func (*TypeName) typeNode()  {}
func (*FuncType) typeNode()  {}
func (*TupleType) typeNode() {}

type TypeParam struct {
	Name        string
	Arity       int      // 0 for ordinary params, n for F[_,...]
	Constraints []string // class names
}

// Param is a function or lambda parameter. Lambda parameters may be a
// pattern instead of a name (`func((a, b), _) => ...`); then Pat is set and
// Name is "" . Function declarations always use Name.
type Param struct {
	Name string
	Type Type    // nil for untyped lambda params
	Pat  Pattern // lambda only; nil when the parameter is a plain name
}

// ---- declarations ----

type File struct {
	Package string
	Name    string // display name used in diagnostics (e.g. "prelude.gfx"); set by the caller
	Decls   []Decl
}

type Decl interface{ declNode() }

type ClassDecl struct {
	Pos     Pos
	Name    string
	Param   TypeParam
	Super   string // "" if none
	Methods []MethodSig
}

type MethodSig struct {
	Pos     Pos
	Name    string
	TParams []TypeParam
	Params  []Param
	Ret     Type
}

// InstanceDecl is `instance Class[TypeCon]`,
// `instance Class[TypeCon[A, B]] given Show[A], Show[B]`, or the partially
// applied head `instance Functor[Result[E]]` where Fixed lists the leading
// type arguments that are held abstract and the remaining arity is what the
// class parameter's kind demands. Params and Fixed are both fresh rigid
// variables of kind *; Context may constrain either. The parser records all head arguments in Params; the checker splits Fixed.
type InstanceDecl struct {
	Pos     Pos
	Class   string
	TypeCon string // bare type constructor name
	Params  []string
	Fixed   []string
	Context []Constraint
	Methods []InstMethod
}

type Constraint struct {
	Pos   Pos
	Class string
	Param string // one of InstanceDecl.Params
}

type InstMethod struct {
	Pos    Pos
	Name   string
	Params []string
	Body   *Block
}

type TypeDecl struct {
	Pos     Pos
	Name    string
	TParams []string
	Ctors   []Ctor
}

// Ctor is `Name(T1, T2)` (positional, FieldNames nil) or the record form
// `Name{a: T1, b: T2}` (FieldNames set, same length as Fields). A record
// constructor is still a positional constructor in every other respect.
type Ctor struct {
	Pos        Pos
	Name       string
	Fields     []Type
	FieldNames []string
}

type FuncDecl struct {
	Pos     Pos
	Name    string
	TParams []TypeParam
	Params  []Param
	Ret     Type
	Body    *Block     // nil when Extern != nil
	Extern  *ExternRef // nil for ordinary functions
}

type ExternRef struct {
	ImportPath string
	Name       string
}

func (*ClassDecl) declNode()    {}
func (*InstanceDecl) declNode() {}
func (*TypeDecl) declNode()     {}
func (*FuncDecl) declNode()     {}

// ---- statements / expressions ----

type Block struct {
	Pos    Pos
	Stmts  []Stmt
	Result Expr
}

type Stmt interface{ stmtNode() }

// LetStmt is `let name = e` or `let name: T = e` (Type nil when absent).
type LetStmt struct {
	Pos   Pos
	Name  string
	Type  Type
	Value Expr
}

type ExprStmt struct{ X Expr }

func (*LetStmt) stmtNode()  {}
func (*ExprStmt) stmtNode() {}

// Expr nodes carry an Ann slot the checker fills with *types.Ann.
type Expr interface {
	exprNode()
	GetPos() Pos
	AnnSlot() *any
}

type ExprBase struct {
	Pos Pos
	Ann any
}

func (e *ExprBase) GetPos() Pos   { return e.Pos }
func (e *ExprBase) AnnSlot() *any { return &e.Ann }
func (e *ExprBase) exprNode()     {}

type Ident struct {
	ExprBase
	Name string
}

type IntLit struct {
	ExprBase
	Value int64
}

type StringLit struct {
	ExprBase
	Value string
}

// Interp is an interpolated string `"a {x} b {y + 1}"`: Parts alternate
// literal text and expressions in source order (Texts[i] precedes Exprs[i];
// len(Texts) == len(Exprs)+1). Every expression must have a Show instance.
type Interp struct {
	ExprBase
	Texts []string
	Exprs []Expr
}

// Field is `x.name` (record field on a single-constructor record type) or
// `x.0`, `x.1`, ... (tuple position; Name is the decimal index text and
// Index >= 0). For record fields Index is -1.
type Field struct {
	ExprBase
	X     Expr
	Name  string
	Index int
}

// RecordLit is `Name{a: e1, b: e2}` (all fields, any order).
type RecordLit struct {
	ExprBase
	Ctor   string
	Names  []string
	Values []Expr
}

// RecordUpdate is `x{a: e1}`: copy of x with the named fields replaced.
type RecordUpdate struct {
	ExprBase
	X      Expr
	Names  []string
	Values []Expr
}

type BoolLit struct {
	ExprBase
	Value bool
}

type Call struct {
	ExprBase
	Fn   Expr
	Args []Expr
}

type Lambda struct {
	ExprBase
	Params []Param
	Body   Expr
}

type BlockExpr struct {
	ExprBase
	Block *Block
}

type Match struct {
	ExprBase
	Scrut Expr
	Arms  []Arm
}

// Arm is `pat => body` or `pat if guard => body`.
type Arm struct {
	Pos   Pos
	Pat   Pattern
	Guard Expr // nil when absent
	Body  Expr
}

type If struct {
	ExprBase
	Cond Expr
	Then Expr
	Else Expr
}

type Do struct {
	ExprBase
	Stmts  []DoStmt
	Result Expr
}

type DoStmt interface{ doStmtNode() }

type DoBind struct {
	Pos  Pos
	Name string
	X    Expr
}
type DoLet struct {
	Pos   Pos
	Name  string
	Type  Type // nil when absent
	Value Expr
}
type DoExpr struct{ X Expr }

func (*DoBind) doStmtNode() {}
func (*DoLet) doStmtNode()  {}
func (*DoExpr) doStmtNode() {}

type Binary struct {
	ExprBase
	Op   string
	L, R Expr
}

// ListLit is `[a, b, c]`; desugars to Cons(a, Cons(b, Cons(c, Nil))).
type ListLit struct {
	ExprBase
	Elems []Expr
}

// TupleLit is `(a, b, ...)`, arity >= 2. `(a)` is a parenthesised expression.
type TupleLit struct {
	ExprBase
	Elems []Expr
}

type Unary struct {
	ExprBase
	Op string
	X  Expr
}

// ---- patterns ----

type Pattern interface{ patNode() }

type PCtor struct {
	Pos  Pos
	Name string
	Args []Pattern
}
type PVar struct {
	Pos  Pos
	Name string
}
type PWild struct{ Pos Pos }
type PLit struct {
	Pos Pos
	X   Expr // IntLit, StringLit, BoolLit
}

// PTuple is `(p1, p2, ...)`, arity >= 2.
type PTuple struct {
	Pos   Pos
	Elems []Pattern
}

// PRecord is `Ctor{f: p, g: q}` or `Ctor{f: p, ..}`. Names are matched by
// field name on a record constructor; Rest reports a trailing `..` that
// ignores the unlisted fields. Without `..` every field must be listed.
type PRecord struct {
	Pos   Pos
	Ctor  string
	Names []string
	Pats  []Pattern
	Rest  bool
}

// PAs is `name @ pattern`.
type PAs struct {
	Pos  Pos
	Name string
	Pat  Pattern
}

func (*PCtor) patNode()   {}
func (*PVar) patNode()    {}
func (*PWild) patNode()   {}
func (*PLit) patNode()    {}
func (*PTuple) patNode()  {}
func (*PAs) patNode()     {}
func (*PRecord) patNode() {}
