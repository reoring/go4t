# gofx — a small Go-flavoured language with HKT, type classes and IO, transpiled to Go

PoC goal: `gofx build examples/http/main.gfx` → `examples/http/main_gen.go` → `go run` calls
`net/http` through `IO` and prints the status. Everything below is the contract between
packages; each package has one owner.

## Packages

| package      | owner  | role |
|--------------|--------|------|
| `ast`        | primary (done) | surface syntax tree, `Expr.AnnSlot()` filled by checker |
| `lexer`, `parser` | primary (done) | source → AST |
| `types`      | checker worker | kinds, type inference with HKT, class/instance resolution, annotation |
| `codegen`    | codegen worker | annotated AST → Go source text |
| `rt`         | runtime worker | Go runtime library imported by generated code (`gofx/rt`) |
| `prelude`    | runtime worker | `prelude/prelude.gfx` (embedded via `prelude.Source`), parsed + checked before every user file |
| `cmd/gofx`   | codegen worker | CLI: `gofx build <file.gfx>` writes `<name>_gen.go` next to it; `gofx run` builds then `go run` |

Module path: `gofx`. Go 1.22+. No third-party deps.

## Surface language (see parser.go grammar header)

```
package main

class Functor[F[_]] {
  fmap[A, B](fa F[A], f func(A) B) F[B]
}
class Monad[M[_]] : Functor[M] {
  pure[A](a A) M[A]
  bind[A, B](ma M[A], f func(A) M[B]) M[B]
}
type Option[A] = None | Some(A)
instance Functor[Option] { fmap(fa, f) { match fa { None => None | Some(a) => Some(f(a)) } } }   // arms separated by ';' or newline, not '|'
func twice[F[_] Functor, A](fa F[A], f func(A) A) F[A] { fmap(fmap(fa, f), f) }
extern func httpGet(url String) IO[Int] = "gofx/rt/stdhttp".GetStatus
func main() IO[Unit] {
  do {
    code <- httpGet("https://example.com")
    println(show(code))
  }
}
```

Builtin types: `Int` (int64), `String`, `Bool`, `Unit`, `IO[A]`, `func(A, B) C`.
Builtin values (declared in prelude as `extern`): `println : func(String) IO[Unit]`,
`show : func(Int) String`, `unit : Unit`, `ioPure`, `ioBind`, `ioFail(msg String) IO[A]`.
Binary operators `+ - * /` on `Int`, `== != < <= > >=` on `Int`/`String`/`Bool` (both sides same
type), `&& ||` on Bool, unary `-` on Int, `!` on Bool. No operator overloading in PoC.

Class method names and top-level function names share one global namespace. Constructors are
capitalised; variables/functions are lower-case. Names must be unique per file (prelude +
user file form one scope; user may not redeclare prelude names).

## Semantic types (`types` package, owned by checker worker, read by codegen)

```go
type Type interface{ isType() }
type TCon  struct{ Name string; Kind Kind }       // Int, String, Bool, Unit, IO, Option, user types
type TVar  struct{ Name string; Kind Kind }        // rigid (declared) type parameter, scoped by Name
type TMeta struct{ ID int; Kind Kind; Ref *Type }  // inference variable; after Zonk never appears in Ann
type TApp  struct{ Fun Type; Args []Type }         // Fun is TCon/TVar/TMeta of arrow kind; len(Args) >= 1
type TFunc struct{ Params []Type; Ret Type }

type Kind interface{ isKind() }
type KStar struct{}
type KArrow struct{ Params []Kind; Ret Kind }      // F[_,_] : KArrow{[*,*], *}
```

Scheme for a top-level function / class method:

```go
type Scheme struct {
    TParams []*TVar          // in declaration order
    Constraints []Constraint // from `F[_] Functor` in tparams; superclasses NOT expanded here
    Type *TFunc
}
type Constraint struct{ Class string; Arg Type } // Arg is a *TVar in schemes, concrete after instantiation
```

Class / instance tables (exported from `types.Program`):

```go
type Program struct {
    Classes   map[string]*Class     // by name
    Instances []*Instance
    DataTypes map[string]*DataType
    Funcs     map[string]*FuncInfo  // user + prelude functions, incl. externs
    Main      *FuncInfo             // must have type func() IO[Unit]
}
type Class    struct{ Name string; Param *TVar; Super string; Methods []*Method; Decl *ast.ClassDecl }
type Method   struct{ Name string; Class *Class; Scheme *Scheme } // Scheme.TParams excludes the class param; the class param is Class.Param
type Instance struct{ Class *Class; TypeCon *TCon; Decl *ast.InstanceDecl }
type DataType struct{ Name string; TParams []*TVar; Ctors []*Ctor }
type Ctor     struct{ Name string; Data *DataType; Fields []Type; Index int }
type FuncInfo struct{ Name string; Scheme *Scheme; Decl *ast.FuncDecl }
```

Annotation placed in every `ast.Expr.AnnSlot()` (as `*types.Ann`) after `Check`:

```go
type Ann struct {
    Type Type           // fully zonked; no TMeta. Unresolved metas default to Unit and are a checker error unless unconstrained (then Unit silently).
    // For *ast.Ident referring to a top-level function, class method, extern or constructor:
    Ref   RefKind       // RefLocal | RefFunc | RefMethod | RefCtor
    Inst  []Type        // instantiation of the referent's TParams, in order (zonked)
    Dicts []DictRef     // one per Constraint of the referent's Scheme, in order
    // For *ast.Do: the monad being sequenced.
    Monad DictRef
    // For *ast.Ident naming a dictionary-bound method (e.g. `fmap`) Dicts[0] is the class dict.
}
type DictRef struct {
    Kind    DictKind          // DictInstance | DictParam | DictSuper
    Class   string
    Instance *Instance        // DictInstance: the resolved instance (Class + TypeCon)
    Param   string            // DictParam: name of the enclosing function's constrained tparam ("F")
    ParamClass string         // DictParam: the class actually bound on that tparam (may be a subclass of Class)
    Super   *DictRef          // DictSuper: take Class from Super's dictionary via the superclass chain (Super.Class is a subclass of Class)
}
```

Dictionary resolution rules: a constraint `C[T]` after zonking is satisfied by (1) a tparam in
scope with constraint `D` where `D == C` or `D`'s superclass chain contains `C` → `DictParam`
(wrapped in `DictSuper` per hop), or (2) `T` is a `TCon` with a declared `instance C[T]` →
`DictInstance`. Otherwise checker error `no instance C[T]`. Instances for the same (class,
typecon) twice: error. An `instance Monad[X]` requires an `instance Functor[X]` in the file
(superclass instance must exist).

HKT unification: `TApp{F, [a]}` ~ `TApp{Option, [Int]}` unifies `F := Option`, `a := Int`.
`TApp{F,[a]}` ~ `Int` fails. Kinds are checked: a meta of kind `*` never unifies with `Option`
(kind `*→*`). Arity mismatch fails. `TApp{F,[a]}` ~ `IO[Int]` → `F := IO`.

`do { ... }` typing: all `x <- e` and bare `e` statements must have type `M[_]` with the same
`M` (a TCon or constrained TVar); `let` is a plain binding; the block's type is the type of the
final expression, which must also be `M[_]`. `Ann.Monad` is the Monad dictionary for `M`.

`match`: arms must cover every constructor or end with `_`/var (checker error otherwise).
Literal patterns require a trailing catch-all.

Local lambdas are monomorphic; `let` does not generalise (no let-polymorphism in PoC).
Recursion between top-level functions is allowed (all signatures are declared).

## Runtime representation (generated Go)

Uniform erased representation: every gofx value is `any` in generated Go.

| gofx | Go |
|------|----|
| `Int` | `int64` |
| `String` | `string` |
| `Bool` | `bool` |
| `Unit` | `rt.Unit` (`struct{}`) |
| `func(A,B) C` | `func(any, any) any` (arity preserved) |
| `IO[A]` | `rt.IO` = `func() (any, error)` |
| `type Option[A] = None \| Some(A)` | `type Option interface{ isOption() }`; `type Option_None struct{}`; `type Option_Some struct{ F0 any }`; both pointer receivers `(*Option_X) isOption()`; values are `*Option_Some{...}` |
| class `Functor` | `type Functor interface{ Fmap(fa any, f any) any }` (method names capitalised; all params/returns `any`); superclass: `type Monad interface{ Functor; Pure(a any) any; Bind(ma any, f any) any }` |
| `instance Functor[Option]` | `type Functor_Option struct{}` with the methods; `instance Monad[Option]` → `type Monad_Option struct{ Functor_Option }` (embeds the superclass instance struct so the Go interface is satisfied) |
| dictionary value for `C[T]` | `C_T{}` |
| function with constrained tparams `func twice[F[_] Functor, A](...)` | `func twice(dict_F Functor, fa any, f any) any` — one dict param per Constraint in Scheme order, named `dict_<TParam>`; a tparam with two constraints gets two dict params `dict_F_Functor`… (PoC: at most one constraint per tparam; checker rejects more) |
| `DictSuper` | the subclass interface embeds the superclass, so the value is passed as-is (Go interface upcast); no accessor call needed |
| extern `= "pkg".Name` | import `pkg` as alias `ext_<n>`; call wrapper `func(args ...) any` generated by codegen that converts each `any` arg to the declared gofx type's Go rep and boxes the result. Extern Go signature must exactly match: params in rep types, result in rep type (so `IO[Int]` extern returns `rt.IO`). |
| binary ops | `rt.AddInt(a, b any) any`, `rt.EqInt`, `rt.EqString`, `rt.LtInt`, …, `rt.And`, `rt.Or`, `rt.NegInt`, `rt.Not` (runtime package exports exactly the list in `rt/ops.go`) |
| `main` | generated `func main() { rt.RunMain(main_()) }` — user functions are emitted with a trailing `_` to avoid clashing with Go `main` and keywords: every gofx identifier `x` becomes Go `x_`. |

`rt` package exports (runtime worker implements; codegen emits exactly these names):

```go
type Unit struct{}
type IO func() (any, error)
func Pure(a any) any                // returns IO
func Bind(io any, f any) any        // f: func(any) any returning IO
func Fail(msg any) any
func RunMain(io any)                // runs, prints error to stderr and os.Exit(1) on failure
func Println(s any) any             // IO[Unit]
func ShowInt(n any) any             // string
// ops.go: AddInt SubInt MulInt DivInt NegInt EqInt NeInt LtInt LeInt GtInt GeInt
//         EqString NeString LtString LeString GtString GeString EqBool NeBool And Or Not
```

`rt/stdhttp`: `func GetStatus(url any) any` returning an `IO` whose result is `int64` status code
(uses `net/http`). This is the PoC's proof that real Go packages are reachable.

Prelude (`prelude/prelude.gfx`) declares: `Functor`, `Monad`, `Option`, `instance
Functor[Option]`, `instance Monad[Option]`, `instance Functor[IO]`, `instance Monad[IO]` (via
externs `ioPure`/`ioBind`), `println`, `show`, `unit`, `ioFail`, and `map`/`flatMap` helpers.
`IO` is a builtin TCon of kind `*→*`; `Unit` builtin TCon.

Prelude functions are emitted into the same generated file as user code (one Go file per
build; the prelude is re-emitted each time).

## Error handling

`do` in `IO` short-circuits on a Go `error` from any step (Bind does not call `f` when the
left side failed). `ioFail` constructs a failing IO. Panics inside IO are not recovered.

## CLI

```
gofx build FILE.gfx   # writes FILE_gen.go in the same directory, package main
gofx run FILE.gfx     # build then `go run` the directory
gofx check FILE.gfx   # type-check only
```

Errors are printed `file:line:col: message` and exit 1.
