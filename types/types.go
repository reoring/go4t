// Package types holds the semantic model of a go4t program: kinds, types,
// schemes, classes, instances and the per-expression annotations produced by
// Check. The definitions in this file are the contract consumed by codegen;
// see SPEC.md §5.
package types

import (
	"fmt"
	"strings"

	"github.com/reoring/go4t/ast"
)

// ---- kinds ----

type Kind interface{ isKind() }

type KStar struct{}
type KArrow struct {
	Params []Kind
	Ret    Kind
}

func (KStar) isKind()  {}
func (KArrow) isKind() {}

func KindString(k Kind) string {
	switch k := k.(type) {
	case KStar:
		return "*"
	case KArrow:
		ps := make([]string, len(k.Params))
		for i, p := range k.Params {
			ps[i] = KindString(p)
		}
		return "(" + strings.Join(ps, ", ") + ") -> " + KindString(k.Ret)
	}
	return "?"
}

// ---- types ----

type Type interface{ isType() }

// TCon is a declared type constructor: builtin, user data type, or IO.
type TCon struct {
	Name    string
	Display string // unmangled declaration spelling for context-free printing
	Kind    Kind
}

// TVar is a rigid type parameter. Its pointer, not Name, is its unique identity.
type TVar struct {
	Name string
	Kind Kind
}

// TMeta is an inference variable. After Check, no TMeta remains in any Ann.
type TMeta struct {
	ID   int
	Kind Kind
	Ref  *Type
}

// TApp applies an arrow-kinded Fun to Args. len(Args) >= 1.
type TApp struct {
	Fun  Type
	Args []Type
}

// TAlias records a source-written alias application. Body is its expanded
// semantic type; Name and Args are presentation provenance, never type identity.
// Unification and representation inspection must go through Unalias/prune.
type TAlias struct {
	Name    string // canonical declaration identity
	Display string // source spelling for context-free printing
	Args    []Type
	Body    Type
}

// App constructs a type application, flattening leading applications (including
// resolved inference variables). With no arguments it returns the constructor.
// Argument slices are shared when no flattening is needed and must not be mutated.
func App(fun Type, args ...Type) Type {
	fun = prune(fun)
	base, total := fun, len(args)
	for {
		app, ok := base.(*TApp)
		if !ok {
			break
		}
		total += len(app.Args)
		base = prune(app.Fun)
	}
	if base != fun {
		if len(args) == 0 {
			if app := fun.(*TApp); app.Fun == base {
				return fun
			}
		}
		flat := make([]Type, total)
		offset := total - len(args)
		copy(flat[offset:], args)
		for head := fun; head != base; {
			app := head.(*TApp)
			offset -= len(app.Args)
			copy(flat[offset:], app.Args)
			head = prune(app.Fun)
		}
		return &TApp{Fun: base, Args: flat}
	}
	if len(args) == 0 {
		return fun
	}
	return &TApp{Fun: fun, Args: args}
}

type TFunc struct {
	Params []Type
	Ret    Type
}

func (*TCon) isType()   {}
func (*TVar) isType()   {}
func (*TMeta) isType()  {}
func (*TApp) isType()   {}
func (*TFunc) isType()  {}
func (*TAlias) isType() {}

// Builtin type constructors.
var (
	Int      = &TCon{Name: "Int", Kind: KStar{}}
	Float    = &TCon{Name: "Float", Kind: KStar{}}
	String   = &TCon{Name: "String", Kind: KStar{}}
	Bool     = &TCon{Name: "Bool", Kind: KStar{}}
	Unit     = &TCon{Name: "Unit", Kind: KStar{}}
	GoError  = &TCon{Name: "GoError", Kind: KStar{}}
	IO       = &TCon{Name: "IO", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	Map      = &TCon{Name: "Map", Kind: KArrow{Params: []Kind{KStar{}, KStar{}}, Ret: KStar{}}}
	Set      = &TCon{Name: "Set", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	Array    = &TCon{Name: "Array", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	Fiber    = &TCon{Name: "Fiber", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	Ref      = &TCon{Name: "Ref", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	MutMap   = &TCon{Name: "MutMap", Kind: KArrow{Params: []Kind{KStar{}, KStar{}}, Ret: KStar{}}}
	Chan     = &TCon{Name: "Chan", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	RecvChan = &TCon{Name: "RecvChan", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	SendChan = &TCon{Name: "SendChan", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
)

// Tuple returns the builtin tuple constructor of arity n (>= 2), named
// "Tuple<n>" with kind (*,..,*) -> *. Instances are interned per arity.
func Tuple(n int) *TCon {
	if n < 2 {
		panic("tuple arity must be >= 2")
	}
	if c, ok := tuples[n]; ok {
		return c
	}
	params := make([]Kind, n)
	for i := range params {
		params[i] = KStar{}
	}
	c := &TCon{Name: fmt.Sprintf("Tuple%d", n), Kind: KArrow{Params: params, Ret: KStar{}}}
	tuples[n] = c
	return c
}

var tuples = map[int]*TCon{}

// IsTuple reports the arity when t is a tuple constructor.
func IsTuple(t *TCon) (int, bool) {
	var n int
	if _, err := fmt.Sscanf(t.Name, "Tuple%d", &n); err == nil && tuples[n] == t {
		return n, true
	}
	return 0, false
}

// ---- schemes, classes, instances, data ----

type Constraint struct {
	Class string
	Arg   Type // *TVar in a Scheme; concrete after instantiation
}

type Scheme struct {
	TParams     []*TVar
	Constraints []Constraint
	Type        *TFunc
}

type Class struct {
	Name    string
	Param   *TVar
	Super   string // "" if none
	Methods []*Method
	Decl    *ast.ClassDecl
}

// Method's Scheme.TParams exclude the class parameter; it is Class.Param.
type Method struct {
	Name   string
	Class  *Class
	Scheme *Scheme
}

// Instance represents a bare, fully applied, or partially applied instance head.
// Params (full heads) and Fixed (partial heads) are rigid variables of kind *.
// Context preserves declaration order; codegen emits one dict field per entry.
type Instance struct {
	Class   *Class
	TypeCon *TCon
	// Fixed are the leading type arguments held abstract by a partially
	// applied head (`instance Functor[Result[E]]`): the instance covers
	// TApp{TypeCon, Fixed ++ <class-kind args>}. Empty for full heads.
	Fixed  []*TVar
	Params []*TVar
	// HeadArgs preserves concrete nullary arguments and variable positions.
	// Coherence remains one instance per (class, type constructor).
	HeadArgs []Type
	Context  []Constraint
	// SuperArgs, for an instance of a class with a superclass, gives one
	// DictRef per Context entry of the superclass instance for the same
	// head, expressed in terms of this instance's Context (DictParam,
	// possibly wrapped in DictSuper). Nil when the class has no superclass
	// or the superclass instance has an empty Context.
	SuperArgs []DictRef
	Decl      *ast.InstanceDecl
}

type DataType struct {
	Name    string
	TParams []*TVar
	Ctors   []*Ctor
}

type Ctor struct {
	Name       string
	Data       *DataType
	Fields     []Type
	FieldNames []string // nil for positional constructors
	Index      int
}

type FuncInfo struct {
	Name   string
	Scheme *Scheme
	Decl   *ast.FuncDecl
}

// LawInfo describes a universally quantified equation outside the value namespace.
type LawInfo struct {
	Decl       *ast.LawDecl
	TParams    []*TVar
	ParamTypes []Type
	Proof      *ProofInfo
}

// ProofInfo retains the checked split bindings for the equational checker.
type ProofInfo struct {
	Decl    *ast.ProofDecl
	Law     *LawInfo
	PatVars map[*ast.PVar]Type
}

type Program struct {
	Modules []*ast.Module
	// ModuleNames maps canonical declarations to their source spelling.
	ModuleNames map[string]string
	// ForeignRecordKeys maps canonical record declarations to owner-package.GoType.
	ForeignRecordKeys map[string]string
	// ForeignHandleKeys maps canonical handle declarations to native Go types.
	ForeignHandleKeys map[string]ast.ForeignHandle
	// Owners maps canonical declarations to their owning module path.
	Owners    map[string]string
	Classes   map[string]*Class
	Instances []*Instance
	DataTypes map[string]*DataType
	Funcs     map[string]*FuncInfo
	Laws      map[string]*LawInfo
	Proofs    map[string]*ProofInfo
	Main      *FuncInfo
	// Files in check order: prelude first, then user file(s).
	Files []*ast.File
	// Warnings are non-fatal diagnostics (e.g. defaulted type variables).
	// Type checking adds them in source order; law checking appends its notices.
	Warnings []*Error
	// RecordPats resolves every record pattern to its constructor and the
	// positional index of each listed field name, in Names order.
	RecordPats map[*ast.PRecord]RecordPat
}

type RecordPat struct {
	Ctor    *Ctor
	Indexes []int
}

// RecordCase maps a common field operation to one constructor's field positions.
type RecordCase struct {
	Ctor    *Ctor
	Indexes []int
}

// ---- annotations ----

type RefKind int

const (
	RefLocal RefKind = iota
	RefFunc
	RefMethod
	RefCtor
	RefLaw // only a TRewrite target; never a callable expression
)

type DictKind int

const (
	DictInstance DictKind = iota
	DictParam
	DictSuper
)

type DictRef struct {
	Kind       DictKind
	Class      string
	Instance   *Instance // DictInstance
	Args       []DictRef // DictInstance: one per Instance.Context entry, in order
	Param      *TVar     // DictParam: the enclosing function or instance's rigid parameter
	ParamClass string    // DictParam: class bound on that tparam
	Super      *DictRef  // DictSuper: subclass dictionary to upcast from
}

// Ann is stored in ast.Expr.AnnSlot() for every expression.
// For comparison *ast.Binary nodes, Inst contains the unified operand type
// and Dicts contains its Eq (==, !=) or Ord (<, <=, >, >=) dictionary.
type Ann struct {
	Type  Type
	Ref   RefKind
	Inst  []Type
	Dicts []DictRef
	Monad *DictRef // *ast.Do only
	// Ctor is the resolved constructor for record literals and RefCtor idents,
	// or the first constructor for field access/update; nil for tuple fields.
	Ctor *Ctor
	// FieldIndex is the positional index for *ast.Field. For tuple access,
	// Type of X is Tuple<n>[...].
	FieldIndex int
	// FieldIndexes are the positional indexes of Names for *ast.RecordLit /
	// *ast.RecordUpdate, in Names order.
	FieldIndexes []int
	// RecordCases is set only for multi-constructor field access/updates, in
	// constructor declaration order. Indexes follows the source field order
	// (one entry for field access). Single-constructor operations use Ctor and
	// FieldIndex/FieldIndexes directly.
	RecordCases []RecordCase
	// ShowDicts holds, for *ast.Interp, one Show dictionary per Exprs entry.
	ShowDicts []DictRef
	// LetScheme is set on the value expression of a generalised `let` whose
	// scheme has class constraints; nil otherwise. Codegen emits such a value
	// as a Go function taking one dictionary parameter per Constraint
	// (dict_<TParam>, or dict_<TParam>_<Class> for multiple constraints on
	// that parameter) and returning the erased value; each use site (an
	// *ast.Ident with Ref == RefLocal and non-empty Ann.Dicts) calls it with
	// the resolved dictionaries. A let without constraints is a plain local
	// regardless of its TParams (erasure makes type generalisation free).
	LetScheme *Scheme
	// Recursive marks a let-bound lambda whose name occurs free in its body.
	// Its self references are monomorphic and do not apply LetScheme dictionaries.
	Recursive bool
	// ImplicitIO marks a synthetic prelude ioPure call. Its argument retains
	// the original annotation; tooling presents the call's lifted result type.
	ImplicitIO bool
}

// AnnOf returns the annotation of e, panicking if Check did not run.
func AnnOf(e ast.Expr) *Ann {
	a, ok := (*e.AnnSlot()).(*Ann)
	if !ok {
		panic(fmt.Sprintf("%s: expression has no annotation", e.GetPos()))
	}
	return a
}

// Error is a positioned compiler diagnostic.
type Error struct {
	Pos  ast.Pos
	End  ast.Pos // exclusive; zero when the source span is unavailable
	Msg  string
	File string // ast.File.Name of the file containing Pos ("" if unknown)
	// detail defers potentially large full-type rendering until requested.
	detail *diagnosticMessage
	names  nameContext
}

// NewError formats a diagnostic while retaining type arguments for full-detail
// rendering. Nested errors may implement Message(bool) to retain their detail.
func NewError(pos ast.Pos, format string, args ...any) *Error {
	message := messagef(format, args...)
	return &Error{Pos: pos, Msg: message.render(false, nameContext{}), detail: message}
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

func (e *Error) GetPos() ast.Pos { return e.Pos }
func (e *Error) GetEnd() ast.Pos { return e.End }

// Message selects the diagnostic's type detail without changing checker state.
func (e *Error) Message(fullTypes bool) string {
	if fullTypes && e.detail != nil {
		return e.detail.render(true, e.names)
	}
	return e.Msg
}
