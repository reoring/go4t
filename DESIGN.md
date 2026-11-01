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

## Pass 7: layout rule, State monad, vet-clean codegen

### Layout rule (single definition)
A newline becomes `;` iff ALL of:
1. the previous token is one of: identifier, Int, String, interpolated string, `true`,
   `false`, `)`, `]`, `}`, `_`  (Go's rule), and
2. the innermost unclosed delimiter is `{` or there is none (`(`/`[` suppress), and
3. the next token (skipping whitespace, blank lines and `//` comments) is NOT one of the
   *leading continuation tokens*: `else`, `|` (not `||`), `.` (field access), `)`, `]`,
   `given`, `=>`, `<-`, or a binary operator (`+ - * / == != < <= > >= && ||`).

Rule 3 lists tokens that can never start a statement, so a line beginning with one of them
is always the continuation of the previous expression. `-` is included: a statement never
starts with unary minus in practice (write `0 - x` or parenthesise if you must).
All three checks live in `lexer.needSemi`/`lexer.continuationAhead`; no other place
decides insertion. `lexer_test.go` carries the full matrix.

### State monad
Prelude adds:
```
type State[S, A] = State(func(S) (A, S))
instance Functor[State[S]] { fmap(st, f) { State(func(s) => match runState(st, s) { (a, s2) => (f(a), s2) }) } }
instance Monad[State[S]] {
  pure(a) { State(func(s) => (a, s)) }
  bind(st, f) { State(func(s) => match runState(st, s) { (a, s2) => runState(f(a), s2) }) }
}
func runState[S, A](st State[S, A], s S) (A, S)
func evalState[S, A](st State[S, A], s S) A
func execState[S, A](st State[S, A], s S) S
func get[S]() State[S, S]
func put[S](s S) State[S, Unit]
func modify[S](f func(S) S) State[S, Unit]
func forEach[M[_] Monad, A](xs List[A], f func(A) M[Unit]) M[Unit]   // sequence_ / traverse_
func mapM[M[_] Monad, A, B](xs List[A], f func(A) M[B]) M[List[B]]
```
`examples/bank` rewrites `applyAll` with `State[(List[Account], Option[(Failure, Int)]), Unit]`
via `forEach` + `modify`, output unchanged.

### Vet-clean generated code
`match` lowering emits the final arm without a guard when its pattern is irrefutable at
every position (var, wildcard, tuple of irrefutables, or the sole constructor of a
single-constructor type): the checker's exhaustiveness proof makes it total once the earlier
arms have failed. A final arm whose pattern still contains a refutable position (a literal,
or a constructor of a multi-constructor type at a nested position) keeps its guard and the
lowering keeps a trailing `panic("non-exhaustive match")`; that panic is unreachable at
runtime but Go's compiler cannot see that, and `go vet` does not flag a `panic` tail. In
both forms no statement follows the last `return`/`panic`, so `go vet ./...` passes on a
tree containing every example's `main_gen/` directory; the `cmd/gofx` test vets a built
program.

## Pass 8: record patterns, tuple positional access

### Record patterns
`Ctor{f: p, g: q}` matches a record constructor by field name (any order); `Ctor{f: p, ..}`
ignores unlisted fields; without `..` every field must appear exactly once. Unknown field or
duplicate → error at the pattern. Positional `Ctor(p, q)` remains valid. Exhaustiveness
treats a record pattern as the equivalent positional pattern (unlisted = wildcard). Codegen:
same lowering as a positional ctor pattern after reordering by `Ctor.FieldNames`; the checker
records the positional order in a new `Ann`-free way: the parser keeps names, the checker
rewrites nothing but exposes `types.RecordPatternOrder(p *ast.PRecord, c *Ctor) []int`
(index per listed name) that codegen calls; both use `Program.DataTypes` to find the ctor.
Simplest: the checker stores the resolved `*Ctor` and index list in a side map
`Program.RecordPats map[*ast.PRecord]RecordPat{Ctor *Ctor; Indexes []int}`.

### Tuple positional access
`x.0`, `x.1`, … read tuple positions (`ast.Field` with `Index >= 0`). The lexer emits `.`
then an Int token; the parser builds `Field{Name: "0", Index: 0}`. The static type of `x`
must be `Tuple<n>[..]` with `Index < n` else `tuple index 2 out of range for (A, B)` /
`tuple index requires a tuple type, got T`. As with record fields, `x`'s type must be known at
that point (lambda params get it from the expected type as in pass 5). Codegen:
`x.(*TupleN).F<i>`. `fst`/`snd` stay in the prelude.

## Pass 9: let annotations, short-circuit, Eq/Ord, lambda patterns, match guards, Int bounds

### `let` annotations
`let x: T = e` (blocks and `do`). The checker unifies `T` (resolved in the enclosing
function's type-parameter scope) with `e`'s type before generalisation; an annotated let is
never generalised beyond the annotation (the annotation is the scheme; rigid tparams of the
enclosing function stay rigid). `ast.LetStmt.Type` / `ast.DoLet.Type`.

### Short-circuit `&&` / `||`
Codegen emits Go `(a).(bool) && (b).(bool)` boxed as `any`; `rt.And`/`rt.Or` are deleted
(callers: codegen only). Right operand is evaluated only when needed.

### `Eq` and `Ord`
Prelude:
```
class Eq[A]  { eq(a A, b A) Bool }
type Ordering = LT | EQ | GT
class Ord[A] : Eq[A] { compare(a A, b A) Ordering }
```
Instances: `Eq`/`Ord` for `Int`, `String`, `Bool` (Bool: false < true), `Ordering`,
`Option[A] given …`, `List[A] given …`, `(A, B) given …`; `Eq[Unit]`.
Builtin externs `eqInt/ltInt/eqString/ltString/eqBool` back the primitive instances
(`rt.EqInt` etc. remain; `compare` for primitives is implemented in prelude via `eq`/`lt`
externs).

Operators become class-method sugar: `a == b` ≡ `eq(a, b)`, `a != b` ≡ `!eq(a, b)`,
`a < b` ≡ `compare(a, b) == LT`, `a <= b` ≡ `compare(a, b) != GT`, `a > b` ≡ `… == GT`,
`a >= b` ≡ `… != LT`. The checker types `==`/`!=` as requiring `Eq[T]` on the (unified)
operand type and `< <= > >=` as requiring `Ord[T]`; the resolved dictionary is stored in
`Ann.Dicts[0]` of the `ast.Binary` node (one entry; `Ann.Inst` = `[T]`). Codegen emits
`dict.Eq(a, b)` / `rt.Not(dict.Eq(a, b))` and for ordering a comparison against the
`Ordering_LT`/`Ordering_GT` constructor values via a type switch helper emitted once per
program (`func ord_lt(o any) bool { _, ok := o.(*Ordering_LT); return ok }` etc.).
Primitive operand types still resolve to prelude instances, so `1 < 2` works unchanged.
`+ - * /` stay Int-only (`+` also String); numeric classes are out of scope.

Prelude additions: `head[A](xs List[A]) Option[A]`, `tail`, `take[A](n Int, xs)`, `drop`,
`sum(xs List[Int]) Int`, `concatMap[A, B](xs List[A], f func(A) List[B]) List[B]`,
`elem[A Eq](x A, xs List[A]) Bool`, `sortBy[A](xs List[A], cmp func(A, A) Ordering) List[A]`
(insertion sort), `sort[A Ord](xs List[A]) List[A]`, `maximum[A Ord](xs List[A]) Option[A]`,
`minimum`, `minInt() Int` (= -2^63), `maxInt() Int` (= 2^63-1) as externs.

### Lambda parameter patterns
`func(p1, p2) => e` where each `pi` is a `Pattern` (identifier, `_`, tuple, record or
single-constructor pattern, as-pattern). A refutable pattern (literal, constructor of a
multi-constructor type, including `Some`) is an error
`lambda parameter pattern must be irrefutable`. Parser: `ast.Param.Pat` set when the
parameter is not a bare identifier (a bare identifier keeps `Name`, so `func(x) =>` is
unchanged; `func(_) =>` yields `Pat = PWild`). Typed form: `func((a, b) (Int, String)) =>`
is NOT supported; patterns are untyped (use a call context or a `let` annotation). Checker
types the pattern against the fresh parameter meta and binds its variables in the body.
Codegen: parameter named `p<i>` and the pattern lowered as an irrefutable binding at the top
of the body (reuse the final-arm-unguarded match lowering).

### Match guards
`Arm := Pattern [ 'if' Expr ] '=>' Expr`. The guard is typed `Bool` with the pattern's
variables in scope. A guarded arm does not count toward exhaustiveness. Codegen: after the
pattern binds, `if !(guard) { fall through to the next arm }` — implement by making the
arm's body conditional and continuing to the next arm's test on failure (the existing
sequential-arm lowering supports this: arms are sequential `if`/type-switch blocks, so a
failed guard simply falls out of the block).

### Superclass context implication (pass 9 amendment to pass 3)
The superclass instance's context need not be identical to the subclass instance's. Rule:
for `instance Sub[T[P..]] given G1[Pi]..` the required `instance Super[T[P..]] given H1[Pj]..`
must have the same Params shape, and each `Hk[Pj]` must be *implied* by the sub context:
some `Gm[Pj]` with `Gm == Hk` or `Hk` on `Gm`'s superclass chain. The checker records, per
subclass instance, how to build the superclass dictionary: `Instance.SuperArgs []DictRef`,
one per entry of the superclass instance's Context, each a `DictParam` (naming the sub
context param) possibly wrapped in `DictSuper`. Codegen initialises the embedded superclass
struct field from `SuperArgs` instead of assuming identical names.
