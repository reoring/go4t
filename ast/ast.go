// Package ast defines the go4t surface syntax tree.
package ast

import "fmt"

type Pos struct{ Line, Col int }

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// ---- syntactic types ----

type Type interface {
	Node
	typeNode()
}

// TypeName is `Name` or `Name[Args]`; Name may be a type parameter.
type TypeName struct {
	Pos  Pos
	End  Pos
	Name string
	Args []Type
}

type FuncType struct {
	Pos    Pos
	End    Pos
	Params []Type
	Ret    Type
}

// TupleType is `(A, B)`; arity >= 2. Resolves to the builtin TCon `Tuple<n>`.
type TupleType struct {
	Pos   Pos
	End   Pos
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
	Pos     Pos
	End     Pos
	Name    string
	Display string  // source spelling for synthesized parameters; empty for source names
	Type    Type    // nil for untyped lambda params
	Pat     Pattern // lambda only; nil when the parameter is a plain name
}

func (p Param) DisplayName() string {
	if p.Display != "" {
		return p.Display
	}
	return p.Name
}

// ---- declarations ----

// Module is one directory of same-package source files. Path is its resolved
// identity; the entry module is last in a compilation's module list.
type Module struct {
	Path       string
	ImportPath string // logical import path; std/... identifies standard declarations
	Name       string
	Files      []*File
	// ForeignRecordKeys maps binder-declared source names to owning Go
	// package/type keys. It is semantic metadata, not part of source comments.
	ForeignRecordKeys map[string]string
	// ForeignHandleKeys records the native Go identity of opaque handles.
	ForeignHandleKeys map[string]ForeignHandle
	// ForeignBindings resolves caller-dependent externs after type inference.
	ForeignBindings ForeignBindings
}

// ForeignHandle identifies the Go value stored by an opaque binding type.
// Struct handles use a pointer box; other named types retain their Go value.
type ForeignHandle struct {
	PackagePath string
	TypeName    string
	Struct      bool
}

// ForeignBindings is the import binder's instantiation port. Signature contains
// concrete types; foreign TypeNames use their "go:owner.Type" identity.
// Implementations validate in check-only mode and emit companions in build mode.
type ForeignBindings interface {
	Instantiate(symbol string, signature *FuncType) (*ExternRef, error)
}

type Import struct {
	Pos, End Pos
	Alias    string // empty selects the last import-path component
	Path     string
	Resolved string // loader-assigned target identity; empty uses Path
}

func (i *Import) GetPos() Pos    { return i.Pos }
func (i *Import) GetEnd() Pos    { return i.End }
func (i *Import) SetEnd(end Pos) { i.End = end }

type File struct {
	Package                string
	Name                   string // display name used in diagnostics; set by the caller
	Imports                []Import
	Decls                  []Decl
	Source                 string // original source for position-based tooling; empty for synthesized files
	PackagePos, PackageEnd Pos
	Comments               []Comment
}

// Comment retains a line comment, including // but excluding its newline.
// Trailing reports whether a source token precedes it on the same line.
type Comment struct {
	Pos, End Pos
	Text     string
	Trailing bool
}

// Node spans use one-based rune columns and an exclusive End position.
type Node interface {
	GetPos() Pos
	GetEnd() Pos
	SetEnd(Pos)
}

type Decl interface {
	Node
	declNode()
}

type ClassDecl struct {
	Pos     Pos
	End     Pos
	Name    string
	Param   TypeParam
	Super   string // "" if none
	Methods []MethodSig
}

type MethodSig struct {
	Pos     Pos
	End     Pos
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
	End     Pos
	Class   string
	TypeCon string // bare type constructor name
	Params  []string
	Fixed   []string
	Context []Constraint
	Methods []InstMethod
}

type Constraint struct {
	Pos   Pos
	End   Pos
	Class string
	Param string // one of InstanceDecl.Params
}

type InstMethod struct {
	Pos    Pos
	End    Pos
	Name   string
	Params []string
	Body   *Block
}

// TypeDecl is `type Name[Params] = Ctors [deriving Class, ...]`.
type TypeDecl struct {
	Pos      Pos
	End      Pos
	Name     string
	TParams  []string
	Ctors    []Ctor
	Deriving []string
	// Shorthand marks `type Name = {fields}` (ADR 0022): the single record
	// constructor is named after the type. Formatting hint only.
	Shorthand bool
}

// AliasDecl is `alias Name[Params] = Type`; aliases have no runtime representation.
type AliasDecl struct {
	Pos     Pos
	End     Pos
	Name    string
	TParams []string
	Type    Type
}

func (d *AliasDecl) GetPos() Pos    { return d.Pos }
func (d *AliasDecl) GetEnd() Pos    { return d.End }
func (d *AliasDecl) SetEnd(end Pos) { d.End = end }

// Ctor is `Name(T1, T2)` (positional, FieldNames nil) or the record form
// `Name{a: T1, b: T2}` (FieldNames set, same length as Fields). A record
// constructor is still a positional constructor in every other respect.
type Ctor struct {
	Pos        Pos
	End        Pos
	Name       string
	Fields     []Type
	FieldNames []string
}

// FuncDecl also represents a local function statement in a block or do.
// Local functions have no type parameters or extern binding.
type FuncDecl struct {
	Pos     Pos
	End     Pos
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
func (*AliasDecl) declNode()    {}
func (*FuncDecl) declNode()     {}

// ConstDecl is a top-level `let name [: Type] = value` (ADR 0022): a nullary
// pure function that is referenced without parentheses. Type is nil when the
// value is a literal that determines it.
type ConstDecl struct {
	Pos     Pos
	End     Pos
	NamePos Pos
	Name    string
	Type    Type
	Value   Expr
}

func (*ConstDecl) declNode()        {}
func (d *ConstDecl) GetPos() Pos    { return d.Pos }
func (d *ConstDecl) GetEnd() Pos    { return d.End }
func (d *ConstDecl) SetEnd(end Pos) { d.End = end }

// ---- laws and proofs (ADR 0001) ----

// LawDecl is `law Name[TParams](params) {requires Expr} { body } [proof {…}]`
// (ADR 0001, 0017, 0021). The equality is definitional: both sides must
// normalise to alpha-equal terms; the Eq instance of the type is not used.
type LawDecl struct {
	Pos     Pos
	End     Pos
	Name    string
	TParams []TypeParam // plain kind-* names; constraints and arities are rejected by the checker
	Params  []Param
	// Requires is the whole Bool precondition, erased with the law: the
	// left-associated && of the written clauses. Every checker reads this.
	Requires Expr
	// RequiresClauses lists the written `requires` clauses, the operands of
	// that chain, when there are several. Formatting hint only, like Call.Pipe.
	RequiresClauses []Expr
	LHS             Expr
	RHS             Expr
	// Predicate marks a body written as one Bool expression: LHS is that
	// expression and RHS a synthesized `true`. Formatting hint only.
	Predicate bool
}

// ProofDecl is `proof Name(x, y) { tactic }` for the law of the same name.
// Params are plain names, positionally rebinding the law's parameters.
type ProofDecl struct {
	Pos    Pos
	End    Pos
	Name   string
	Params []Param // Name only; Type nil
	Body   Tactic
	// Inline marks a proof written as `} proof { tactic }` after its law
	// (ADR 0017); the parser places it directly after that LawDecl.
	Inline bool
	// Implicit marks the `refl` proof the checker supplies for a law that has
	// none. It never appears in File.Decls and is not a declaration symbol.
	Implicit bool
}

type Tactic interface {
	Node
	tacticNode()
}

// TRefl is `refl`: both goal sides normalise to the same term.
type TRefl struct{ Pos, End Pos }

// TAbsurd is `absurd` (ADR 0018): the assumptions of the current case are
// contradictory, so the case holds vacuously.
type TAbsurd struct{ Pos, End Pos }

// TSample is `sample(N)`: runtime testing only, never proof evidence.
type TSample struct {
	Pos, End Pos
	Count    int64
}

// TMatch is `match x { pat => tactic ... }`: splits proof variable x by
// constructor. Patterns are PCtor/PRecord over PVar/PWild/nested PCtor,
// PTuple for tuple-typed variables, Bool literals, or POr alternatives;
// no other literals, guards or as-patterns.
type TMatch struct {
	Pos Pos
	End Pos
	Var *Ident
	// Scrut is the scrutinee when it is not a bare proof variable (ADR 0018);
	// Var is nil then. Exactly one of Var and Scrut is set.
	Scrut Expr
	Arms  []TacticArm
	// Induction adds structural self-rewrites; Bare supplies every constructor.
	Induction bool
	Bare      bool
}

type TacticArm struct {
	Pos  Pos
	End  Pos
	Pat  Pattern
	Body Tactic
	// Alts is Alternatives(Pat) as type-checked: the or-free patterns the
	// prover splits on. Filled by the checker; nil before checking.
	Alts []Pattern
}

// TIf is `if cond { tactic } else { tactic }`: cond is a Bool expression over
// proof variables; each branch assumes cond normalises to true/false.
type TIf struct {
	Pos  Pos
	End  Pos
	Cond Expr
	Then Tactic
	Else Tactic
}

// TRewrite is `rewrite Law(args) { tactic }`: instantiates law Law with
// args (expressions over proof variables), rewrites every occurrence of its
// left side with its right side in both goal sides, then continues. Law may
// be the enclosing proof's own law (induction hypothesis).
type TRewrite struct {
	Pos  Pos
	End  Pos
	Law  *Ident
	Args []Expr
	Body Tactic
}

func (*LawDecl) declNode()   {}
func (*ProofDecl) declNode() {}

func (d *LawDecl) GetPos() Pos      { return d.Pos }
func (d *LawDecl) GetEnd() Pos      { return d.End }
func (d *LawDecl) SetEnd(end Pos)   { d.End = end }
func (d *ProofDecl) GetPos() Pos    { return d.Pos }
func (d *ProofDecl) GetEnd() Pos    { return d.End }
func (d *ProofDecl) SetEnd(end Pos) { d.End = end }

func (*TRefl) tacticNode()    {}
func (*TAbsurd) tacticNode()  {}
func (*TSample) tacticNode()  {}
func (*TMatch) tacticNode()   {}
func (*TIf) tacticNode()      {}
func (*TRewrite) tacticNode() {}

func (t *TRefl) GetPos() Pos       { return t.Pos }
func (t *TRefl) GetEnd() Pos       { return t.End }
func (t *TRefl) SetEnd(end Pos)    { t.End = end }
func (t *TAbsurd) GetPos() Pos     { return t.Pos }
func (t *TAbsurd) GetEnd() Pos     { return t.End }
func (t *TAbsurd) SetEnd(end Pos)  { t.End = end }
func (t *TSample) GetPos() Pos     { return t.Pos }
func (t *TSample) GetEnd() Pos     { return t.End }
func (t *TSample) SetEnd(end Pos)  { t.End = end }
func (t *TMatch) GetPos() Pos      { return t.Pos }
func (t *TMatch) GetEnd() Pos      { return t.End }
func (t *TMatch) SetEnd(end Pos)   { t.End = end }
func (t *TIf) GetPos() Pos         { return t.Pos }
func (t *TIf) GetEnd() Pos         { return t.End }
func (t *TIf) SetEnd(end Pos)      { t.End = end }
func (t *TRewrite) GetPos() Pos    { return t.Pos }
func (t *TRewrite) GetEnd() Pos    { return t.End }
func (t *TRewrite) SetEnd(end Pos) { t.End = end }

// ---- statements / expressions ----

type Block struct {
	Pos    Pos
	End    Pos
	Stmts  []Stmt
	Result Expr
}

type Stmt interface{ stmtNode() }

// LetStmt is `let name = e`, `let name: T = e`, or `let pat = e` /
// `let pat: T = e` with an irrefutable pattern (Pat set, Name "").
type LetStmt struct {
	Pos     Pos
	End     Pos
	NamePos Pos // start of the bare binding name, when Pat is nil
	Name    string
	Pat     Pattern
	Type    Type
	Value   Expr
}

type ExprStmt struct{ X Expr }

func (*LetStmt) stmtNode()  {}
func (*ExprStmt) stmtNode() {}
func (*FuncDecl) stmtNode() {}

// Expr nodes carry an Ann slot the checker fills with *types.Ann.
type Expr interface {
	exprNode()
	Node
	AnnSlot() *any
}

type ExprBase struct {
	Pos Pos
	End Pos
	Ann any
}

func (e *ExprBase) GetPos() Pos   { return e.Pos }
func (e *ExprBase) GetEnd() Pos   { return e.End }
func (e *ExprBase) SetEnd(p Pos)  { e.End = p }
func (e *ExprBase) AnnSlot() *any { return &e.Ann }
func (e *ExprBase) exprNode()     {}

type Ident struct {
	ExprBase
	Name        string
	Display     string // source spelling when Name is synthesized or qualified
	Placeholder bool   // synthesized section/call parameter reference
}

// DisplayName returns the source spelling without exposing compiler identities.
func (e *Ident) DisplayName() string {
	if e.Display != "" {
		return e.Display
	}
	return e.Name
}

type IntLit struct {
	ExprBase
	Value int64
	// Text is the source spelling when it is not the plain decimal form of
	// Value (separators, a base prefix, the smallest Int); empty otherwise and
	// for synthesized literals. Formatting hint only (ADR 0019).
	Text string
}

type FloatLit struct {
	ExprBase
	Value float64
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
	Texts  []string
	Exprs  []Expr
	Starts []Pos // expression starts immediately after each ${
}

// Field is `x.name` (a field common to every constructor of a data type) or
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
	Pipe *Binary // original |> surface tree; checking uses Fn and Args
}

type Lambda struct {
	ExprBase
	Params  []Param
	Body    Expr
	Partial bool // synthesized placeholder lambda
	Section bool // parenthesised operator/projection section, not a partial call or bare projection argument
}

type BlockExpr struct {
	ExprBase
	Block *Block
}

type Match struct {
	ExprBase
	Scrut Expr
	Arms  []Arm
	// SourceArms retains written or-patterns when checking expands Arms. It is
	// nil otherwise. Formatting, hover and rechecking use these source arms;
	// semantic passes and Walk visit only Arms, never both copies of the body.
	SourceArms []Arm
}

// Arm is `pat => body` or `pat if guard => body`.
type Arm struct {
	Pos   Pos
	End   Pos
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

// DoBind is `name <- e` or `pat <- e` (irrefutable pattern; Pat set, Name "").
type DoBind struct {
	Pos  Pos
	End  Pos
	Name string
	Pat  Pattern
	X    Expr
}
type DoLet struct {
	Pos     Pos
	End     Pos
	NamePos Pos // start of the bare binding name, when Pat is nil
	Name    string
	Pat     Pattern // irrefutable pattern when not a plain name
	Type    Type    // nil when absent
	Value   Expr
}
type DoExpr struct{ X Expr }

func (*DoBind) doStmtNode()   {}
func (*DoLet) doStmtNode()    {}
func (*DoExpr) doStmtNode()   {}
func (*FuncDecl) doStmtNode() {}

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

type Pattern interface {
	Node
	patNode()
}

type PCtor struct {
	Pos  Pos
	End  Pos
	Name string
	Args []Pattern
	// List marks the root of a parser-desugared list pattern for formatting.
	// The semantic pattern remains Nil/Cons; user-written constructors leave it false.
	List bool
}
type PVar struct {
	Pos  Pos
	End  Pos
	Name string
}
type PWild struct{ Pos, End Pos }
type PLit struct {
	Pos Pos
	End Pos
	X   Expr // IntLit, StringLit, BoolLit
}

// PTuple is `(p1, p2, ...)`, arity >= 2.
type PTuple struct {
	Pos   Pos
	End   Pos
	Elems []Pattern
}

// PRecord is `Ctor{f: p, g: q}` or `Ctor{f: p, ..}`. Names are matched by
// field name on a record constructor; Rest reports a trailing `..` that
// ignores the unlisted fields. Without `..` every field must be listed.
type PRecord struct {
	Pos   Pos
	End   Pos
	Ctor  string
	Names []string
	Pats  []Pattern
	Rest  bool
}

// PAs is `name @ pattern`.
type PAs struct {
	Pos  Pos
	End  Pos
	Name string
	Pat  Pattern
}

// POr is `p1 | p2 | ...` (ADR 0020), at least two alternatives. Every
// alternative binds the same variables. It means the same as writing one arm
// per alternative, in order; see Alternatives.
type POr struct {
	Pos  Pos
	End  Pos
	Alts []Pattern
}

func (*PCtor) patNode()   {}
func (*PVar) patNode()    {}
func (*PWild) patNode()   {}
func (*PLit) patNode()    {}
func (*PTuple) patNode()  {}
func (*PAs) patNode()     {}
func (*PRecord) patNode() {}
func (*POr) patNode()     {}
