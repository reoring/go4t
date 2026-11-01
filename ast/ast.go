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

func (*TypeName) typeNode() {}
func (*FuncType) typeNode() {}

type TypeParam struct {
	Name        string
	Arity       int      // 0 for ordinary params, n for F[_,...]
	Constraints []string // class names
}

type Param struct {
	Name string
	Type Type // nil for untyped lambda params
}

// ---- declarations ----

type File struct {
	Package string
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

type InstanceDecl struct {
	Pos     Pos
	Class   string
	TypeCon string // bare type constructor name
	Methods []InstMethod
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

type Ctor struct {
	Pos    Pos
	Name   string
	Fields []Type
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

type LetStmt struct {
	Pos   Pos
	Name  string
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

func (e *ExprBase) GetPos() Pos    { return e.Pos }
func (e *ExprBase) AnnSlot() *any  { return &e.Ann }
func (e *ExprBase) exprNode()      {}

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

type Arm struct {
	Pos  Pos
	Pat  Pattern
	Body Expr
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

func (*PCtor) patNode() {}
func (*PVar) patNode()  {}
func (*PWild) patNode() {}
func (*PLit) patNode()  {}
