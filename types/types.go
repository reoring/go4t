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
	String = &TCon{Name: "String", Kind: KStar{}}
	Bool   = &TCon{Name: "Bool", Kind: KStar{}}
	Unit   = &TCon{Name: "Unit", Kind: KStar{}}
	IO     = &TCon{Name: "IO", Kind: KArrow{Params: []Kind{KStar{}}, Ret: KStar{}}}
)

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

type Instance struct {
	Class   *Class
	TypeCon *TCon
	Decl    *ast.InstanceDecl
}

type DataType struct {
	Name    string
	TParams []*TVar
	Ctors   []*Ctor
}

type Ctor struct {
	Name   string
	Data   *DataType
	Fields []Type
	Index  int
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
	Param      string    // DictParam: constrained tparam name of the enclosing function
	ParamClass string    // DictParam: class bound on that tparam
	Super      *DictRef  // DictSuper: subclass dictionary to upcast from
}

// Ann is stored in ast.Expr.AnnSlot() for every expression.
type Ann struct {
	Type  Type
	Ref   RefKind
	Inst  []Type
	Dicts []DictRef
	Monad *DictRef // *ast.Do only
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
	Pos ast.Pos
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }
