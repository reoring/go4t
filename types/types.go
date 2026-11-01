// Package types holds the semantic model of a gofx program: kinds, types,
// schemes, classes, instances and the per-expression annotations produced by
// Check. The definitions in this file are the contract consumed by codegen;
// see DESIGN.md.
package types

import (
	"fmt"
	"strings"

	"gofx/ast"
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
	Name string
	Kind Kind
}

// TVar is a rigid (declared) type parameter.
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

func (*TCon) isType()  {}
func (*TVar) isType()  {}
func (*TMeta) isType() {}
func (*TApp) isType()  {}
func (*TFunc) isType() {}

// Builtin type constructors.
var (
	Int    = &TCon{Name: "Int", Kind: KStar{}}
	Float  = &TCon{Name: "Float", Kind: KStar{}}
	String = &TCon{Name: "String", Kind: KStar{}}
	Bool   = &TCon{Name: "Bool", Kind: KStar{}}
	Unit   = &TCon{Name: "Unit", Kind: KStar{}}
	IO     = &TCon{Name: "IO", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	Map    = &TCon{Name: "Map", Kind: KArrow{Params: []Kind{KStar{}, KStar{}}, Ret: KStar{}}}
	Set    = &TCon{Name: "Set", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
	Array  = &TCon{Name: "Array", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
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

func TypeString(t Type) string {
	switch t := t.(type) {
	case *TCon:
		return t.Name
	case *TVar:
		return t.Name
	case *TMeta:
		if t.Ref != nil {
			return TypeString(*t.Ref)
		}
		return fmt.Sprintf("?%d", t.ID)
	case *TApp:
		as := make([]string, len(t.Args))
		for i, a := range t.Args {
			as[i] = TypeString(a)
		}
		return TypeString(t.Fun) + "[" + strings.Join(as, ", ") + "]"
	case *TFunc:
		ps := make([]string, len(t.Params))
		for i, p := range t.Params {
			ps[i] = TypeString(p)
		}
		return "func(" + strings.Join(ps, ", ") + ") " + TypeString(t.Ret)
	}
	return "?"
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
	Fixed   []*TVar
	Params  []*TVar
	Context []Constraint
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

type Program struct {
	Classes   map[string]*Class
	Instances []*Instance
	DataTypes map[string]*DataType
	Funcs     map[string]*FuncInfo
	Main      *FuncInfo
	// Files in check order: prelude first, then user file(s).
	Files []*ast.File
	// Warnings are non-fatal diagnostics (e.g. defaulted type variables),
	// in source order.
	Warnings []*Error
	// RecordPats resolves every record pattern to its constructor and the
	// positional index of each listed field name, in Names order.
	RecordPats map[*ast.PRecord]RecordPat
}

type RecordPat struct {
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
	Param      string    // DictParam: constrained tparam name of the enclosing function or instance
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
	// Ctor is the resolved constructor for record *ast.Field, *ast.RecordLit,
	// *ast.RecordUpdate (and for RefCtor idents); nil for tuple *ast.Field.
	Ctor *Ctor
	// FieldIndex is the positional index for *ast.Field. For tuple access,
	// Type of X is Tuple<n>[...].
	FieldIndex int
	// FieldIndexes are the positional indexes of Names for *ast.RecordLit /
	// *ast.RecordUpdate, in Names order.
	FieldIndexes []int
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
}

// AnnOf returns the annotation of e, panicking if Check did not run.
func AnnOf(e ast.Expr) *Ann {
	a, ok := (*e.AnnSlot()).(*Ann)
	if !ok {
		panic(fmt.Sprintf("%s: expression has no annotation", e.GetPos()))
	}
	return a
}

// Error is a positioned checker error.
type Error struct {
	Pos  ast.Pos
	Msg  string
	File string // ast.File.Name of the file containing Pos ("" if unknown)
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }
