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
`show : Show method` (class `Show[A] { show(a A) String }`, instances `Int`/`String`/`Bool` backed by
externs `showInt`/`showString`/`showBool`), `unit : func() Unit`, `ioPure`, `ioBind`, `ioFail(msg String) IO[A]`.
Binary operators `+` on `Int`/`String` (concatenation), `- * /` on `Int`, `== !=` on
`Int`/`String`/`Bool`, `< <= > >=` on `Int`/`String` only (both sides same type), `&& ||` on Bool,
unary `-` on Int, `!` on Bool. No user operator overloading in PoC.

`match` arms are separated by newline, `;`, or `|`; an optional leading `|` is allowed.

Checker diagnostics: unification failure is `type mismatch: expected <T>, got <U>` (expected =
type demanded by context). Missing instance on a constrained type parameter appends
`(type parameter F is constrained only by Functor)`. Structural mismatches report the outermost
expected/actual pair. `+` operand validation is deferred until the body is zonked so operands may
be resolved through later context; an unresolved `+` defaults to `Int`; `Bool + Bool` reports
`addition requires Int or String, got Bool`.

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
| extern `= "pkg".Name` | import `pkg` as alias `ext_<n>`; the Go function uses the uniform ABI: every param `any`, result `any` (an `IO[...]` extern returns `rt.IO` boxed in `any`). Codegen emits a plain forwarding wrapper `func name_(a0 any, ...) any { return ext_n.Name(a0, ...) }`. |
| binary ops | `rt.AddInt(a, b any) any` / `rt.AddString` chosen by the Binary's zonked type, `rt.EqInt`, `rt.EqString`, `rt.LtInt`, …, `rt.And`, `rt.Or`, `rt.NegInt`, `rt.Not` (runtime package exports exactly the list in `rt/ops.go`) |
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
func ShowBool(b any) any            // "true" / "false"
func ShowString(s any) any          // identity
// ops.go: AddInt AddString SubInt MulInt DivInt NegInt EqInt NeInt LtInt LeInt GtInt GeInt
//         EqString NeString LtString LeString GtString GeString EqBool NeBool And Or Not
```

`rt/stdhttp`: `func GetStatus(url any) any` returning an `IO` whose result is `int64` status code
(uses `net/http`). This is the PoC's proof that real Go packages are reachable.

Prelude (`prelude/prelude.gfx`) declares: `Functor`, `Monad`, `Show`, `Option`, `instance
Functor[Option]`, `instance Monad[Option]`, `instance Functor[IO]`, `instance Monad[IO]` (via
externs `ioPure`/`ioBind`), `instance Show[Int|String|Bool]`, `println`, `showInt`, `showBool`,
`showString`, `unit`, `ioFail`, and `map`/`flatMap` helpers.
`IO` is a builtin TCon of kind `*→*`; `Unit` builtin TCon.

Prelude functions are emitted into the same generated file as user code (one Go file per
build; the prelude is re-emitted each time).

## Error handling

`do` in `IO` short-circuits on a Go `error` from any step (Bind does not call `f` when the
left side failed). `ioFail` constructs a failing IO. Panics inside IO are not recovered.

## CLI

```
gofx build DIR/NAME.gfx   # writes DIR/NAME_gen/main.go (package main); several programs may share DIR
gofx run DIR/NAME.gfx     # build then `go run ./DIR/NAME_gen` from the module root; exit code mirrored
gofx check FILE.gfx   # type-check only
```

Errors are printed `file:line:col: message` and exit 1.

## Deviations

The initial `unit : Unit` value is exposed as a zero-argument function because extern
declarations are functions: `extern func unit() Unit = "gofx/rt".UnitValue`.
Programs write `unit()` to obtain `Unit`; this is reflected in the contract above.

## Pass 3: if/else layout, List, constrained instances

### `if` / `else` layout
The lexer does not insert `;` after `}` when the next non-space token on the following line
is the keyword `else`. (Implementation: the lexer peeks ahead past whitespace/newlines; if the
next word is `else`, the newline is skipped silently.) `if a { x }\nelse { y }` is one expression.

### List and list literals
Prelude declares `type List[A] = Nil | Cons(A, List[A])`, `instance Functor[List]`,
`instance Monad[List]` (bind = flat map), `instance Show[List[A]] given Show[A]`
(renders `[1, 2, 3]`), and functions `fold[A, B](xs List[A], init B, f func(B, A) B) B`,
`length[A](xs List[A]) Int`, `append[A](xs List[A], ys List[A]) List[A]`,
`filter[A](xs List[A], p func(A) Bool) List[A]`, `reverse[A](xs List[A]) List[A]`.
Surface syntax `[e1, e2, ...]` (`ast.ListLit`) is typed as `List[A]` with all elements unified
to `A`; `[]` is `List[A]` with `A` a fresh meta (defaults to `Unit` if unconstrained, as any
other meta). Codegen desugars to nested `Cons`/`Nil` constructor values.
`[` after an expression is never indexing (no indexing in gofx); `[` in expression position
always starts a list literal.

### Constrained instances
```
instance Show[Option[A]] given Show[A] { show(o) { match o { | None => "None" | Some(a) => "Some(" + show(a) + ")" } } }
instance Show[Pair[A, B]] given Show[A], Show[B] { ... }
```
- Head: `Class[TypeCon]` (as before, TypeCon kind must equal the class param kind) or
  `Class[TypeCon[P1, .., Pn]]` where `Pi` are distinct fresh type variables of kind `*`, `n`
  equals the arity of `TypeCon`, and the class param has kind `*`. The `given` context lists
  constraints `D[Pi]` only on those variables (each `Pi` at most one constraint, PoC).
- Instance lookup for `C[T]`: zonk `T`; if head is a TCon, find instance for (C, TCon). If
  the instance has Params, bind them to T's args and recursively resolve each Context
  constraint, producing `DictRef{Kind: DictInstance, Instance, Args}`. Failure of any inner
  constraint fails the outer one with `no instance D[X] (required by instance C[TypeCon[..]])`.
- Inside the instance's method bodies, each context constraint `D[Pi]` is in scope as a
  dictionary param `DictParam{Param: Pi, ParamClass: D}`; the instance's own class for the
  head type is also resolvable (recursion through the instance itself, e.g. `show` on the
  tail inside `Show[List[A]]`).
- Codegen: `type Show_Option struct{ dict_A Show }` (one field per Context entry, named
  `dict_<Param>`), value `Show_Option{dict_A: <Args[0] expr>}`. Instances without context
  remain `Show_Int{}`. Superclass embedding is unchanged: `Monad_X struct{ Functor_X; ... }`,
  where the embedded `Functor_X` value is built from the *Functor* instance's own Args
  (resolved by the checker as a `DictSuper`-free lookup; codegen asks the checker-produced
  `Instance.Context` of the superclass instance: for PoC superclass instances of constrained
  instances must have the same Params/Context shape, checker enforces this).

## Pass 4: constructor namespace, tuples, as-patterns, partially applied instances

### Separate namespaces
Type names (data types, classes, builtins) live in one namespace; constructors live in a
second; functions/methods/locals in a third. `type Sku = Sku(String)` is legal. Lookup rules:
in type position only type names; in expression/pattern position a capitalised identifier is a
constructor; a lower-case identifier is a value. Duplicate detection is per namespace.
Reserved prefixes (`dict_`, `ext_`) still apply to all three.
Codegen: data type `T` keeps Go name `T`; constructor `C` of type `T` keeps `T_C` (already
unambiguous).

### Tuples
Builtin type constructors `Tuple2..TupleN` (`types.Tuple(n)`), surface type `(A, B)`,
expression `(a, b)`, pattern `(p, q)`. `(a)` is parenthesisation. Arity >= 2.
Prelude declares `instance Show[(A, B)] given Show[A], Show[B]` rendering `(a, b)` for arity 2
only (the instance head syntax `Show[(A, B)]` is sugar for TypeCon `Tuple2` with Params A, B).
Codegen: `rt.Tuple` = `[]any`-backed struct? No: a Go struct per arity is not needed; use
`type TupleN struct{ F0, F1, .. any }` emitted by codegen on demand into the generated file
(one per arity used), named `Tuple2` etc.; the Go interface for a class instance on tuples is
`Show_Tuple2`.

### As-patterns
`name @ pattern` binds `name` to the whole matched value. `@` is a new punct. Binds in the same
scope as the inner pattern's variables.

### Partially applied instance heads
`instance Functor[Result[E]]` where `Result : (*,*) -> *` and `Functor` expects `* -> *`:
the head lists `k` leading arguments (fresh rigid `*` vars, `Instance.Fixed`), and
`arity(TypeCon) - k` must equal the class parameter's kind arity. Dictionary resolution for
`C[T]` where zonked `T` is `TApp{Result, [X]}` (a partial application of kind `*->*`) or the
bare `Result` when `k == 0` selects the instance for (C, Result) and binds Fixed to the leading
args. Unification already handles `TApp{F, [a]} ~ TApp{TApp{Result,[E]}, [A]}`? No — the
checker must normalise applications: `TApp{TApp{Result,[E]},[A]}` and `TApp{Result,[E,A]}` are
the same type. Rule: a `TApp` whose Fun is a `TApp` is flattened on construction
(`types.App(fun, args...)` helper, exported), and unifying `TApp{F, [a]}` (F meta of kind
`*->*`) against `TApp{Result, [E, A]}` splits the right side as `F := TApp{Result,[E]}`,
`a := A`. Kinds: `TApp{Result,[E]}` has kind `*->*`. `Fixed` vars may appear in `Context`
(`instance Show[Result[E, A]] given Show[E], Show[A]` is the full-head form and is unchanged).
Codegen: no change in dictionary shape; `Fixed` vars are erased.

## Pass 5: defaulting warnings, let-polymorphism, records, string interpolation

### Defaulting warnings
When an unconstrained `*` meta is defaulted to `Unit`, the checker appends
`types.Error{Pos, "type of <what> defaulted to Unit; add an annotation or use the value"}` to
`Program.Warnings` instead of staying silent (`<what>` = `[]`, `None`, `let x`, or the
expression kind). Check still succeeds. CLI prints warnings to stderr as
`file:line:col: warning: ...` and continues; `gofx check` exits 0 with warnings.
Internal/prelude positions are reported with the prelude file name `prelude.gfx`.

### let-polymorphism
`let` (in blocks and `do`) generalises: after inferring the value, metas not free in the
environment become scheme TParams; class constraints on them become scheme Constraints.
Lambdas bound by `let` thus become reusable at several types
(`let twiceShow = func(x) => show(x) + show(x)` used at Int and Bool). Use sites instantiate
with fresh metas and resolve dictionaries exactly like top-level functions (`Ann.Dicts`).
Codegen contract is in `types.Ann.LetScheme`'s doc comment: constrained lets become local Go
functions `func(dict_A Show) any { return <value> }` and use sites call them.
Value restriction: only lambda values and literals/constructor applications are generalised;
other expressions (calls, `do`, `match`) stay monomorphic. Function parameters are never
generalised.

### Records
Constructor form `Name{a: T1, b: T2}` in a `type` declaration (`ast.Ctor.FieldNames`).
A record constructor is positional in patterns and calls (`Name(e1, e2)`, `Name(p, q)`) and
additionally supports: construction `Name{a: e1, b: e2}` (all fields, any order; missing or
unknown field is an error); field read `x.a` when the static type of `x` is a data type with
exactly one constructor and that constructor has field `a` (otherwise
`field a requires a single-constructor record type`); update `x{a: e}` with the same
single-constructor rule, producing a copy with the listed fields replaced.
`.` after an expression is field access only (there is no method call syntax).
Prelude: none. Codegen: records keep the `T_C{F0, F1}` Go struct; `x.a` → `x.(*T_C).F<i>`;
update → composite literal copying unlisted fields from a temp.
Field names share the value namespace rules (lower-case) but are scoped to their constructor;
two constructors may reuse a field name.

### String interpolation
A string literal containing `${expr}` is an `ast.Interp`. Lexer: inside a `"..."` literal,
`${` starts an interpolation and the matching `}` ends it; `$$` is a literal `$`; a bare `$`
not followed by `{` is a literal `$`; `{` and `}` outside `${...}` are ordinary characters
(no `{{` escape). Nested string literals inside `${}` are allowed (the lexer tracks brace
depth and quote state); the expression text is re-lexed and parsed as an expression by the
parser (positions offset to the original location). Each `expr` must have a `Show` instance
(resolved into `Ann.ShowDicts[i]`); String values use `Show[String]` (identity). Codegen:
`rt.Concat(parts ...any) any` taking strings; emit `rt.Concat("a ", <dict>.Show(x), " b")`.
`rt` adds `Concat`.

Rationale (pass 6): `{` already means block, record literal and record update; `"{x}"` next
to `else { ... }` was unreadable. `$` is unused elsewhere in the grammar.

### Match arm style
The leading `|` before the first arm remains legal but the examples and prelude write
one-line matches without it (`match p { (A(x), A(y)) => x == y }`) and multi-line matches
with one arm per line and no `|` unless arms span several lines.
