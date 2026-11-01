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
    Param   *TVar             // DictParam: canonical rigid parameter; pointer identity, never source name
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
| binary ops | `+ - * /` → `rt.AddInt`/`rt.AddString`/`rt.SubInt`/…; `== != < <= > >=` → `Eq`/`Ord` dictionary calls (pass 9); `&&`/`\|\|` → native Go short-circuit; unary → `rt.NegInt`, `rt.Not` |
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
// ops.go: AddInt AddString SubInt MulInt DivInt NegInt Not Concat MinInt MaxInt
//         EqInt LtInt EqString LtString EqBool   (backing the prelude Eq/Ord externs)
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
gofx run DIR/NAME.gfx [-- args]  # build to a temp binary, exec it with inherited stdio; its exit status is mirrored
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
decides insertion. The lexer cannot tell record braces from block braces, so the parser
skips inserted `;` inside record constructor declarations, record literals/updates and
record patterns (pass 11 fix; `parser/layout_record_test.go`). `lexer_test.go` carries the full matrix.

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

## Pass 10: let/bind patterns, deriving, Option helpers

### Patterns in `let` and `<-`
`let pat = e`, `let pat: T = e` (block and `do`), and `pat <- e` in `do` accept an
irrefutable pattern (same class as lambda parameters: variable, `_`, tuple, record or
single-constructor pattern, as-pattern). `ast.LetStmt.Pat`, `ast.DoLet.Pat`, `ast.DoBind.Pat`
are set when the left side is not a bare identifier. Refutable → error
`let pattern must be irrefutable` / `bind pattern must be irrefutable`. A pattern `let` is
never generalised (its variables are monomorphic). Codegen binds via the irrefutable-pattern
lowering used for lambda parameters; for `<-` the bind continuation takes `p any` and
destructures at the top of its body.

### deriving
`type T[A, B] = ... deriving Eq, Ord, Show` (`ast.TypeDecl.Deriving`; names must be among
`Eq`, `Ord`, `Show`; each at most once). Before checking, `types.Check` expands every
deriving into synthesized `*ast.InstanceDecl`s appended to the same file (positions = the
type declaration's), then checks normally; a hand-written instance for the same class and
type is the usual `duplicate instance` error. Generated heads: bare `C[T]` for
non-parameterised types; `C[T[A, B]] given C[A], C[B]` for parameterised types (every type
parameter gets the constraint; this over-approximates for phantom/function-typed parameters,
which is acceptable).

Generated bodies (as if written in source, using `==`, `compare`, `show` on fields, which
resolve to the field types' instances):
- `Eq`: `eq(a, b) { match (a, b) { (C1(x1..), C1(y1..)) => x1 == y1 && ... ; ... ; _ => false } }`
  (nullary pairs → `true`; single-constructor types omit the `_` arm). Field types must
  themselves be `Eq` (checker error otherwise, with the usual `required by instance` hint).
  Function-typed fields → error `cannot derive Eq for T: field of function type`.
- `Ord`: constructors ordered by declaration; `compare(a, b)` = `compare(tag(a), tag(b))`
  then, for equal constructors, lexicographic `compare` over fields using a generated helper
  `func <T>_tag(x T) Int`? No — avoid extra top-level names: generate the match directly:
  `match (a, b) { (C1(x..), C1(y..)) => <lex>; (C1(..), _) => LT; (_, C1(..)) => GT; (C2.. ) ... }`
  where `<lex>` is `match compare(x1, y1) { EQ => <lex of rest> | o => o }` and the empty lex
  is `EQ`. Deriving `Ord` requires `Eq` for the same type (derived or hand-written).
- `Show`: nullary → `"C"`; positional → `"C(${x1}, ${x2})"`; record → `"C{f1: ${x1}, f2: ${x2}}"`.
  `String` fields are shown via `Show[String]` (no quotes), consistent with the prelude.
The prelude derives `Eq, Ord, Show` for `Ordering` and `Option`, and `Eq, Ord` for `List`
and keeps its hand-written `Show[List]`/`Show[(A, B)]` (tuples are builtin; their instances
stay hand-written).

### Option helpers (prelude)
```
fromOption[A](o Option[A], d A) A
maybe[A, B](o Option[A], d B, f func(A) B) B
isSome[A](o Option[A]) Bool      isNone[A](o Option[A]) Bool
```

## Pass 11: strings, input, and program arguments

Goal: a gofx program can read input and process text, so small CLI tools are writable.
No new syntax; everything is prelude externs backed by `rt/str.go` and `rt/io.go`.

### String functions (prelude externs → `"gofx/rt"`)
```
strLength(s String) Int                     // rune count
strConcat(xs List[String]) String           // join without separator
join(sep String, xs List[String]) String
split(sep String, s String) List[String]    // Go strings.Split; sep "" splits into runes
lines(s String) List[String]                // split on "\n", drop a single trailing empty element
words(s String) List[String]                // strings.Fields
trim(s String) String                       // strings.TrimSpace
toUpper(s String) String    toLower(s String) String
startsWith(s String, prefix String) Bool    endsWith(s String, suffix String) Bool
contains(s String, sub String) Bool
replace(s String, old String, new String) String   // all occurrences
substring(s String, start Int, end Int) String     // rune indexes, clamped
chars(s String) List[String]                // one-rune strings
repeat(s String, n Int) String
parseInt(s String) Option[Int]              // strconv.ParseInt base 10; None on failure
showPadLeft(n Int, width Int) String        // right-aligned decimal
```
`List[String]`/`Option[Int]` cross the extern ABI as the generated Go representations
(`*List_Cons`/`*List_Nil`, `*Option_Some`/`*Option_None`). Those types are declared by the
generated program, not by `rt`; `rt` therefore cannot name them. Resolution: `rt` builds and
reads lists/options through a small **value-shape protocol** that codegen already emits for
prelude types: codegen emits, once per program, `func rt_cons(h, t any) any`,
`rt_nil() any`, `rt_some(a any) any`, `rt_none() any`, `rt_uncons(l any) (h, t any, ok bool)`
and registers them in `rt.Shapes` (a struct of func fields) at `init()` time. `rt` string
functions use `rt.Shapes` to construct/destructure. Codegen emits these helpers whenever the
program declares `List`/`Option` (always true with the prelude). Test with a fake `rt` that
has the same `Shapes` struct.

### Input and arguments (prelude externs)
```
readLine() IO[Option[String]]        // None at EOF; strips the trailing newline
readAll() IO[String]                 // rest of stdin
args() IO[List[String]]              // os.Args[1:]
exit(code Int) IO[A]                 // terminates; A unconstrained (defaults to Unit if unused)
getEnv(name String) IO[Option[String]]
```
`rt.RunMain` is unchanged; `exit` calls `os.Exit` from inside the IO action after flushing
stdout (stdout is unbuffered in rt today; keep it so).

### CLI: `gofx run FILE.gfx [-- args...]`
Arguments after `--` are passed to the program; stdin is inherited (already).

### Example
`examples/wc/main.gfx`: a word-count / frequency tool — reads stdin with `readAll`,
`lines`/`words`, counts with a `List[(String, Int)]` fold, `sortBy` count desc then word
asc, prints `showPadLeft(count, 5) + " " + word` for the top N where N comes from `args()`
(`parseInt`, default 10), and `exit(2)` with a usage message when the argument is not a
number. README records a run with a fixed input file `examples/wc/input.txt` via
`go run ./cmd/gofx run examples/wc/main.gfx -- 3 < examples/wc/input.txt`.

## Pass 12: towards practical programs (contract for parallel slices)

Goal: everything a person needs to write a small CLI/service in gofx. The work is split
into independent slices, each developed in its own git worktree/branch off `master`
(`.wt/<slice>`, branch `slice/<slice>`). The primary merges slices in order and resolves
conflicts. Rules for every slice:

- Do **not** edit `SPEC.md` or `DESIGN.md` (the primary integrates). Write the SPEC delta
  (new grammar, types, prelude signatures, limitations removed) to `docs/slices/<slice>.md`.
- Do not commit. Leave the worktree with the finished change; the primary commits it.
- Run only the slice's own package tests and its example (`go run ./cmd/gofx run
  examples/<slice>/main.gfx`), not the whole suite; the primary runs `go test ./...` after
  integration.
- Every slice adds `examples/<slice>/main.gfx` (+ `README.md` with the observed output)
  exercising the feature end to end, and unit tests in the owning package(s) for behaviour,
  boundaries and error messages.
- Keep diagnostics in the `file:line:col: message` form. Keep the uniform `any` ABI unless
  the slice is `typed-extern`.
- Names below are **fixed contracts**; other slices may rely on them without seeing the
  implementation. A slice that needs a name owned by another slice declares it as the
  owner will (same spelling, same signature) only if it is required to compile its tests,
  and notes the duplication in its `docs/slices/<slice>.md` so the primary deletes one copy.

### Slice contracts (wave A)

**float-num** — builtin `Float : *` (`types.Float`, Go `float64`), literal `1.5`, `2e3`,
`1.0e-3` (lexer token Float; `1.` and `.5` are not literals). `+ - * /` accept
`Float/Float` as well as `Int/Int` (resolved after inference like `+` today; `/` on Float
is IEEE division, no panic); `%` stays Int; unary `-` on Float. `Show/Eq/Ord[Float]`
(`show` uses Go `strconv.FormatFloat(f, 'g', -1, 64)`; `Ord` orders NaN below everything,
`Eq` is IEEE except NaN == NaN is false). Prelude: `toFloat(n Int) Float`,
`truncate(f Float) Int`, `floor`, `ceil`, `round(f Float) Int`, `sqrt`, `pow(x Float, y Float) Float`,
`powInt(x Int, y Int) Int`, `parseFloat(s String) Option[Float]`, `showFloatFixed(f Float, decimals Int) String`,
`min[A Ord](a A, b A) A`, `max[A Ord](a A, b A) A`, `absFloat`, `nan() Float`, `isNaN`.
Default for an unconstrained numeric literal remains Int (no numeric defaulting).

**collections** — builtin opaque type constructors `Map : (*,*) -> *`, `Set : (*) -> *`,
`Array : (*) -> *` (`types.Map/Set/Array`), implemented in `rt/coll.go` as persistent
structures (Map/Set: balanced tree ordered by the key's `compare`, passed from the prelude
as a `func(any, any) any` returning the generated `Ordering` value, read back through a new
`rt.Shapes.OrderingOf func(any) int` (-1/0/1) that codegen registers; Array: Go slice
copied on write). Prelude API (all pure):
```
emptyMap[K, V]() Map[K, V]        mapInsert[K Ord, V](m Map[K, V], k K, v V) Map[K, V]
mapLookup[K Ord, V](m Map[K, V], k K) Option[V]   mapDelete[K Ord, V](m, k) Map[K, V]
mapMember[K Ord, V](m, k) Bool    mapSize[K, V](m) Int
mapFromList[K Ord, V](xs List[(K, V)]) Map[K, V]  mapToList[K, V](m) List[(K, V)]   // ascending keys
mapKeys[K, V](m) List[K]   mapValues[K, V](m) List[V]
mapAdjust[K Ord, V](m, k, f func(V) V) Map[K, V]   // no-op when absent
mapAlter[K Ord, V](m, k, f func(Option[V]) Option[V]) Map[K, V]
mapUnion[K Ord, V](a, b) Map[K, V]  // left-biased
mapMap[K, V, W](m Map[K, V], f func(V) W) Map[K, W]   mapFold[K, V, B](m, init B, f func(B, K, V) B) B
groupBy[K Ord, A](xs List[A], key func(A) K) Map[K, List[A]]   // preserves element order
emptySet[A]() Set[A]  setInsert[A Ord](s, a) Set[A]  setDelete  setMember  setSize  setFromList  setToList  setUnion  setIntersection  setDifference
arrayFromList[A](xs List[A]) Array[A]  arrayToList  arrayLength  arrayGet[A](a Array[A], i Int) Option[A]
arraySet[A](a, i Int, v A) Array[A]  // copy; out of range returns a unchanged
arrayMap  arrayFold  arrayPush  arraySlice(a, start, end)  arraySortBy
```
Instances: `Show[Map[K, V]] given Show[K], Show[V]` renders `{k: v, k2: v2}`;
`Show[Set[A]]` renders `#{a, b}`; `Show[Array[A]]` renders `#[a, b]`; `Eq` for all three
given `Eq` on the components (Map/Set compare as sorted lists; this requires the keys to be
`Ord` to build, so `Eq[Map[K,V]] given Eq[K], Eq[V]` compares `mapToList`), `Functor[Array]`,
`Functor[Map[K]]`, `Foldable` is owned by **deriving** — do not add it here.

**io-effects** — prelude `type Result[E, A] = Err(E) | Ok(A) deriving Eq, Ord, Show` with
`instance Functor[Result[E]]`, `instance Monad[Result[E]]` (`do` works on Result; `Err`
short-circuits). Helpers `mapErr[E, F, A](r, f func(E) F) Result[F, A]`,
`fromResult[E, A](r, d A) A`, `okOr[E, A](o Option[A], e E) Result[E, A]`,
`toOption[E, A](r) Option[A]`, `isOk`, `isErr`, `either[E, A, B](r, onErr func(E) B, onOk func(A) B) B`,
`liftResult[E Show, A](r Result[E, A]) IO[A]` (Err → `ioFail(show(e))`),
`tryIO[A](io IO[A]) IO[Result[String, A]]` (never fails; captures `ioFail`, extern errors
**and Go panics** inside the action as `Err(message)`), `ioFailWith[E Show, A](e E) IO[A]`.
Runtime error policy: `rt.RunMain` recovers panics raised while running `main`'s IO and
reports them as `error: panic: <msg>` with exit status 2 (`ioFail`/extern errors keep
`error: <msg>`, exit 1). Files/time/random (all `IO`): `readFile(path) IO[String]`,
`writeFile(path, content) IO[Unit]`, `appendFile`, `fileExists(path) IO[Bool]`,
`listDir(path) IO[List[String]]` (sorted names), `removeFile`, `eprintln(s) IO[Unit]`,
`print(s) IO[Unit]` (no newline), `nowMillis() IO[Int]` (Unix ms), `monotonicMillis() IO[Int]`,
`formatTime(millis Int, layout String) String` (pure, UTC, Go layout), `randomInt(lo Int, hi Int) IO[Int]`
(inclusive), `randomFloat() IO[Float]` only if `Float` exists in the worktree — otherwise skip
it and note it, `sleep(millis Int) IO[Unit]`. `rt.ReadFile/WriteFile/Eprintln` already exist.
Codegen registers `rt.Shapes.Ok/Err` (`rt_ok`, `rt_err`) like `Some/None`.

**local-recursion** — `let f = func(...) => body` is recursive when `f` occurs free in the
lambda (checker: bind `f` monomorphically in the lambda's environment, generalise after).
New block statement `func name(params) RetType { body }` (same grammar as a top-level
function without type parameters; types required) — a maximal run of consecutive local
`func` statements is one mutually recursive group, checked monomorphically then
generalised together; they see the enclosing scope. Codegen: Go `var f_ func(any) any;
f_ = func(...) {...}` for recursive lets and groups. Also allowed inside `do`.

**pipe-partial** — `|>` binary operator, lowest precedence (below `||`), left-assoc:
`x |> f(a, b)` ≡ `f(x, a, b)`; `x |> f` ≡ `f(x)`; `x |> (func(y) => …)` applies. Placeholder
sections: a call whose arguments include `_` (`add(1, _)`, `map(_, f)`, `mapInsert(_, k, v)`)
is a lambda over the `_` positions in order (`func(a) => add(1, a)`); operator sections
`(_ + 1)`, `(2 * _)`, `(_ == x)` likewise (parenthesised, exactly one `_` per side allowed,
both sides `_` gives a 2-ary lambda). `_` elsewhere in expression position remains an
error (`_ is only valid as a placeholder in a call or operator section`). Patterns are
unaffected. Lexer: `|>` is a new punct and a continuation token for layout.

**deriving** — split `Show` (user-facing) from a new prelude `class Debug[A] { debug(a A) String }`.
`deriving Show` is removed (error `deriving Show is not supported; derive Debug and write
instance Show if a display form is needed`); `deriving Debug` produces the structural form
(`C`, `C(f1, f2)`, `C{a: f1, ..}`) via the fields' `Debug`. Prelude provides `Debug` for
`Int String Bool Unit Float? Ordering Option List Result? tuples` (String renders quoted
with Go escapes) and keeps today's `Show` instances (`Show[String]` is identity). String
interpolation uses `Show`. Examples currently deriving `Show` switch to `Debug` (and
interpolate with `${debug(x)}`) or gain a hand-written `Show`. New prelude classes
```
class Foldable[F[_]] { foldr[A, B](fa F[A], init B, f func(A, B) B) B }
class Traversable[F[_]] : Functor[F] { traverse[M[_] Monad, A, B](fa F[A], f func(A) M[B]) M[F[B]] }
```
with instances for `List`, `Option`, and prelude helpers `foldl`/`toList`/`foldMapList`
expressed via `Foldable`. `deriving Functor, Foldable, Traversable` for data types whose
**last** type parameter is the mapped one; the parameter may occur directly, inside
`List`/`Option`/other `Functor` instances' arguments, in tuples, and in the type's own
recursive occurrences; occurrences in function types or as a non-last argument are an
error naming the constructor and field.

**multi-constraint** — `TParam := Ident [ '[' '_' {',' '_'} ']' ] [ Ident { '+' Ident } ]`;
`ast.TypeParam.Constraints` already is a slice. Scheme constraints keep declaration order;
codegen dict parameter names become `dict_<T>` when a tparam has one constraint (unchanged)
and `dict_<T>_<Class>` when it has several. Duplicate classes on one parameter are an error;
a class and its superclass together is allowed (the subclass dict is used for both).
Resolution for rigid `C[T]` succeeds if any constraint of `T` is `C` or a subclass.

**alias-fields** — new top-level `alias Name[Params] = Type` (keyword `alias`); fully
transparent (expanded in `types/resolve.go`; must be fully applied; cycles are an error;
`alias` names live in the type namespace). Common-field access: `x.f` and `x{f: e}` on a
multi-constructor type are allowed when **every** constructor declares field `f` with the
same type (positions may differ); codegen emits a type switch. Error otherwise names the
constructor lacking the field.

**diagnostics** — `ast.Pos` gains `End Pos` tracking on declarations, expressions and
patterns (parser sets it); `types.Error` carries `Pos` and `End`; the CLI prints the source
line under the message with `^~~~` under the span (`gofx check`, `build`, `run`).
Non-exhaustive match errors list missing patterns (`missing: Some(_), None` /
`missing: (true, _)`), up to 5. `no instance Show[T]` for a user data type appends
`hint: add 'deriving Debug' (or 'Show') or write 'instance Show[T]'`. Unused `let`/lambda
parameter/local warning `x is unused; prefix with _ to silence` (identifiers starting with
`_` are exempt). Warnings keep the same `file:line:col: warning: msg` form.

**codegen-quality** — self tail calls (a top-level or local function whose body's result
position calls itself, through `if`/`match`/block tails) become a `for` loop with parameter
reassignment (test: `fold` over 1,000,000 elements runs without stack growth). Generated Go
carries `//line <file>.gfx:<n>` directives before each function and each `match` arm so Go
panics report `.gfx` positions; output is `go/format`-ed. Add `.github/workflows/ci.yml`
running `go vet ./...`, `go test ./...` and every `examples/*/main.gfx` via `gofx run`
(inputs from `examples/*/input.txt` when present, args from `examples/*/args.txt`).

**concurrency** — builtin `Fiber : (*) -> *` (`types.Fiber`). Prelude (`rt/conc.go`):
`fork[A](io IO[A]) IO[Fiber[A]]` (runs on a goroutine; failure/panic is captured),
`await[A](f Fiber[A]) IO[A]` (re-raises the fiber's failure), `parMapIO[A, B](xs List[A], f func(A) IO[B]) IO[List[B]]`
(all in parallel, first failure wins after all complete), `race[A](a IO[A], b IO[A]) IO[A]`,
`parBoth[A, B](a IO[A], b IO[B]) IO[(A, B)]`, `timeout[A](millis Int, io IO[A]) IO[Option[A]]`,
`sleep` is owned by io-effects (declare only if needed, see rule above). `rt.RunMain`
exits only after `main`'s IO completes; unawaited fibers are abandoned.

**typed-extern** — `extern func f(x Int, s String) Bool = "pkg".F` may bind a Go function
with the **natural** signature `func F(int64, string) bool` (`Int→int64`, `Float→float64`,
`String→string`, `Bool→bool`, `Unit→rt.Unit`, `IO[T]→rt.IO`, `List[String]→[]string`,
`List[Int]→[]int64`, `Option[T]→(T, bool)` for primitive `T`, everything else `any`).
Codegen emits an adapter wrapper that converts; existing `any`-ABI externs keep working
because the adapter is chosen by inspecting the Go package: new command `gofx bind
<go-import-path> [-o FILE.gfx]` parses the package (`go list -json` + `go/parser`,
`go/types` from the standard library, no third-party deps) and emits `extern func`
declarations for every exported function whose signature is representable, with a
`// skipped: Name (reason)` comment for the rest; the codegen adapter also uses that
analysis (shared package `extern/`) at build time so the ABI (natural vs `any`) is chosen
per Go function. `rt/stdhttp.GetStatus` is rewritten to the natural signature as the proof.

**cli-tools** — `gofx test FILE.gfx...`: compiles the program with every top-level
`func test<Name>() Bool` or `func test<Name>() IO[Bool]` collected into a generated harness
`main` (user `main` is not required for `gofx test`); prints `PASS testName` / `FAIL testName`
per test, a summary line, exit 1 on any failure. `gofx eval 'EXPR' [FILE.gfx...]` wraps the
expression in `func main() IO[Unit] { println(show(EXPR)) }` (if EXPR has type `IO[A]` with
`Show[A]`, it is run and its result shown; the checker's error decides). `gofx build -o
PATH` chooses the output directory; `gofx build --emit-go DIR --package NAME` writes a
non-main Go package exposing `func Main() rt.IO` for embedding in Go programs.

**stdlib-extras** — prelude additions (pure, `rt/str.go` where native):
List: `zipWith`, `unzip`, `any`, `all`, `find[A](xs, p) Option[A]`, `findIndex`, `partition`,
`nub[A Eq]`, `range(lo, hi) List[Int]` (half-open), `replicate`, `intercalate`,
`flatten`, `last`, `init`, `splitAt`, `span`, `takeWhile`, `dropWhile`, `lookup[K Eq, V](k, xs List[(K, V)]) Option[V]`,
`indexed[A](xs) List[(Int, A)]`, `iterate[A](n Int, x A, f func(A) A) List[A]`, `foldr`
(plain List version if deriving's Foldable is absent in the worktree — note the duplication), `product`,
`count[A](xs, p) Int`, `unfold`, `chunksOf`, `sortOn[A, K Ord](xs, key func(A) K)`.
String: `codePoint(s String) Option[Int]` (first rune), `fromCodePoint(n Int) String`,
`isDigit/isAlpha/isSpace/isUpper/isLower(s String) Bool` (all runes satisfy, non-empty),
`indexOf(s, sub) Option[Int]` (rune index), `padRight(s, width) String`, `padLeft`,
`reverseString`, `splitOnce(sep, s) Option[(String, String)]`, `charAt(s, i) Option[String]`,
`showQuoted(s) String`. Regex (Go `regexp`, pattern compile failure → `None`/`false`):
`regexMatch(pattern, s) Bool`, `regexFind(pattern, s) Option[String]`,
`regexFindAll(pattern, s) List[String]`, `regexReplace(pattern, s, repl) String`,
`regexSplit(pattern, s) List[String]`, `regexCaptures(pattern, s) Option[List[String]]`.
`type Lazy[A] = Lazy(func(Unit) A)` with `force[A](l Lazy[A]) A`, `delay`; a memoising
variant is not required (pure re-evaluation is acceptable; document it).
`docs/slices/stdlib-extras.md` must tabulate the boundary behaviour of **every** prelude
list/string function (`tail(Nil)`, `take(-1, …)`, `substring` clamping, `/ 0`, …).

### Wave B (after wave A is merged)
modules (`import`, qualified names, prelude shadowing, `std/*` split), lsp (`gofx lsp`),
fmt (`gofx fmt`), perf (Int/Bool unboxing, specialisation, benchmarks in CI),
json-deriving (`deriving Encode, Decode`), docs (SPEC rewrite, tutorial). Contracts are
written when wave A has landed.

### Wave B contracts

Same rules as wave A (worktree `.wt/<slice>`, branch `slice/<slice>`, no SPEC/DESIGN edits,
`docs/slices/<slice>.md`, `examples/<slice>/`). Base: master after wave A.

**modules** — `import "path"` / `import alias "path"` after `package`. A module is a
directory of `.gfx` files with the same `package` clause; the import path is resolved
relative to a root list: the importing file's module root (the directory containing the
nearest `gofx.mod` file, a one-line `module <name>` file; absent → the program's directory),
then `$GOFX_PATH` entries, then the compiler's embedded `std/`. Exported names are those
starting with an upper-case letter **for types/constructors/classes** and, for values, those
listed in an `export` block? — No: keep Go's rule but values are lower-case in gofx, so
visibility is: every top-level declaration is exported unless its name begins with `_`
(`func _helper()`); `_`-prefixed names are private to the module. Qualified access
`alias.name`, `alias.Type`, `alias.Ctor` (lexer: `Ident '.' Ident` in type position and
pattern position too). Unqualified import `import . "std/list"` is not provided. Instances
are global (no orphan rule; duplicate instance across modules is an error naming both
modules). The prelude becomes module `std/prelude`, implicitly imported unqualified into
every file; **user declarations may shadow prelude names** (local wins, warning
`shadows prelude name X`); std split: `std/list`, `std/string`, `std/regex`, `std/map`,
`std/set`, `std/array`, `std/io`, `std/conc`, `std/result`, `std/float`, with `std/prelude`
re-exporting only: `Functor Monad Show Debug Eq Ord Foldable Traversable Ordering Option
Result List Unit`, `map flatMap fold filter length append reverse head tail show debug
println print eprintln ioPure ioFail unit fst snd compare eq min max toFloat`, and the
operators' instances. Everything else moves to its std module and is reached qualified
(`import "std/string"` → `string.trim(s)`). Migrate all examples. `gofx run DIR` (a
directory) compiles the module in DIR as a program. Codegen: one Go file per gofx module
under `<out>/`, Go package name = last path element with `_` separators, names mangled
`pkg_name_`; dictionaries for cross-module instances resolve by (class, type) globally.
`types.Check` takes `[]*ast.Module`. CLI `check/build/run/test/eval/bind/fmt/lsp` all
accept files or a directory.

**lsp** — `gofx lsp` (stdio, JSON-RPC 2.0, LSP 3.17; no third-party deps): `initialize`,
`textDocument/didOpen|didChange|didClose` (full sync), `publishDiagnostics` (errors +
warnings from `types.Check` with spans), `textDocument/hover` (type of the identifier at the
position, rendered with the checker's `typeString`, plus the declaration's doc comment =
the `//` lines directly above it), `textDocument/definition` (identifiers, constructors,
types, fields, class methods; cross-file within the program/module), `textDocument/
documentSymbol`. A `lsp/` package holds the server; `types` exposes a position index
(`Program.Index(pos) *Binding` or equivalent built from annotations) — add to `types`
without changing existing behaviour. Tests drive the server through an in-memory pipe.
`editors/vscode/` contains a minimal client extension manifest (package.json + extension.js
that spawns `gofx lsp`) and `editors/README.md`.

**fmt** — `gofx fmt [-w] FILE...|DIR` prints/rewrites canonical formatting; `gofx fmt -d`
shows a diff; exit 1 when `-l` lists unformatted files. Canonical style: 2-space indent,
one declaration per blank-line group, `match` arms one per line without leading `|` unless
an arm spans lines, record fields one per line when the literal exceeds 80 columns,
trailing commas absent, operators spaced, comments preserved at their line (attach to the
following node; trailing comments stay trailing). Implementation: a `format/` package
printing from the AST (which must carry comments — extend the lexer/parser to collect
comments into `ast.File.Comments` with positions; do not break existing parsing). Property
tests: `fmt(fmt(x)) == fmt(x)` and `parse(fmt(x))` structurally equals `parse(x)` for every
`examples/**/*.gfx` and `prelude`/`std` files. Run `gofx fmt -w` over the whole repository
at the end and include that in the slice (expect large but mechanical diffs; keep them in a
separate commit message line "fmt: reformat repository" — the primary commits).

**perf** — in `codegen/`: (1) unboxed `Int`/`Bool`/`Float` for monomorphic locals and
parameters whose type is fully known after checking (emit `int64`/`bool`/`float64`
declarations; box only at `any` boundaries: polymorphic calls, data constructor fields,
dictionaries, externs); (2) specialisation of constrained functions called with concrete
dictionaries where the callee is in the same program and small (≤ 40 AST nodes) — emit a
monomorphic copy per (function, dict tuple) up to 8 copies, otherwise fall back; (3)
`bench/` directory with `go test -bench` harness compiling `bench/*.gfx` and timing
`sort` of 1,000,000 Ints, `Map` insert/lookup of 100,000 keys, `fold` over 10,000,000, and
the wc example on a 10 MB input; CI job runs benchmarks with `-benchtime=1x` and fails if
any exceeds a budget stored in `bench/budget.json` (set budgets at 2× the measured value
on the CI runner after the change; record before/after numbers in docs/slices/perf.md).
All existing tests and examples must keep passing; generated code must still be
`go vet`-clean.

**json-deriving** — prelude classes `class Encode[A] { encode(a A) Json }` and `class
Decode[A] { decode(j Json) Result[String, A] }` with `type Json = JNull | JBool(Bool) |
JNum(Float) | JStr(String) | JArr(List[Json]) | JObj(Map[String, Json])`, `toJson(j Json)
String` (compact), `toJsonPretty`, `parseJson(s String) Result[String, Json]` (rt, Go
`encoding/json` into an ordered representation; numbers → Float; object key order = source
order for encode, sorted for `Map`). `deriving Encode, Decode`: records → objects keyed by
field name; positional constructors → `{"$tag": "Ctor", "$values": [...]}`; nullary →
`{"$tag": "Ctor"}`; single-constructor records omit `$tag`; parameterised types get the
instances `given Encode[A]`…; `Option[A]` encodes `None` as `null`; `List`, `Map[String,
V]`, `Array`, tuples (as arrays), `Int`, `Float`, `String`, `Bool`, `Unit` (`null`) have
prelude instances; decode errors read `expected <what> at <path>` (`$.items[2].name`).
Lives in `std/json` once modules land — coordinate: if `modules` is not merged in your
worktree, put it in the prelude and note it; the primary moves it.

**docs** — rewrite `SPEC.md` from the merged implementation + every `docs/slices/*.md`
(then delete `docs/slices/`, folding deviations/boundary tables into SPEC §7 and a new §10
"Boundary behaviour"); fix §4 `Stmt` grammar and §7 deriving contradictions; add
`docs/tutorial.md` (Go developer oriented: type classes vs interfaces, IO vs side effects,
`do`, `Result`/`tryIO`, modules, `gofx test`, FFI via `bind`), `docs/stdlib.md` (every
std/prelude function with signature + one-line semantics + boundary row), and update
`README.md` with install/quick start and the examples table (each example's purpose and
run command). The docs slice must verify every code snippet by extracting fenced
` ```gofx ` blocks into `.gfx` files and running `gofx check` over them (`docs/check_test.go`).
Starts after modules merged (it documents the module system).

## Pass 13: service-grade gofx (contract for parallel slices)

Input: user acceptance review after Pass 12 (P0–P3 list). Same mechanics as Pass 12: one
worktree `.wt/<slice>` / branch `slice/<slice>` per slice, no commits by workers, the primary
merges in dependency order. Unlike Pass 12, slices **edit `SPEC.md` in place** (the sections
named in their contract) — `docs/slices/` is not used any more; `docs/check_test.go` and
`docs/stdlib_inventory_test.go` keep documentation honest.

### Domain model changes

- **Effect runtime.** `rt.IO` becomes `func(ctx context.Context) (any, error)`. A context is
  the *execution environment* of an effect: it carries cancellation and deadline only. Purity
  is unaffected (no user-visible context value). `RunMain` runs `main` with `context.Background()`
  wrapped so that SIGINT/SIGTERM cancel it. Every rt effect that blocks (`sleep`, `readLine`,
  fibers, HTTP) selects on `ctx.Done()`.
- **Failure.** An IO failure is `*rt.Failure{Kind Kind, Message string}` (implements `error`).
  Kinds are a closed set mirrored by the std ADT
  `type IoError = NotFound(String) | PermissionDenied(String) | AlreadyExists(String) | Timeout | Cancelled | Panic(String) | Network(String) | Failure(String)`
  (`deriving Eq, Debug`, hand-written `Show` = message text). `ioFail(msg)` raises `Failure(msg)`;
  `ioFailWith[E Show]` keeps raising `Failure(show(e))`; new `ioRaise(e IoError) IO[A]` raises a
  typed failure. `tryIO: IO[A] -> IO[Result[IoError, A]]` (breaking change from `Result[String, A]`;
  all std/examples/docs migrate; `ioErrorMessage(e IoError) String` gives the old text).
  Go errors from externs/natural-ABI adapters are classified in rt (`os.IsNotExist` → NotFound,
  `os.IsPermission` → PermissionDenied, `os.IsExist` → AlreadyExists, `context.DeadlineExceeded`
  → Timeout, `context.Canceled` → Cancelled, `net.Error` → Network, else Failure).
  Codegen registers `rt.Shapes.IoError func(kind string, msg string) any` building the ADT
  value (same mechanism as `rt_ok`/`rt_err`), and `rt.Shapes.IoErrorOf func(v any) (kind, msg string)`.
- **Fibers.** `Fiber` owns a child context; `cancel[A](f Fiber[A]) IO[Unit]` cancels it and
  returns immediately; awaiting a cancelled fiber fails with `Cancelled`. `timeout` and `race`
  cancel the loser and wait for it to finish before returning (bounded: a loser that ignores
  its context is abandoned after `timeout`'s own deadline + 100 ms, documented). `parMapIO`
  cancels siblings on first failure. New `withTimeout[A](millis Int, io IO[A]) IO[A]` (fails with
  `Timeout` instead of returning `Option`); `timeout` stays as the `Option` form implemented on top.
- **HTTP.** `std/http`: `type Request = Request{method: String, url: String, path: String, query: List[(String, String)], headers: List[(String, String)], body: String}`,
  `type Response = Response{status: Int, headers: List[(String, String)], body: String}`,
  `request(r Request) IO[Response]` (failures: Network/Timeout/Cancelled; non-2xx is a normal
  Response), `get(url) IO[Response]`, `post(url, contentType, body) IO[Response]`,
  `response(status, body) Response`, `jsonResponse[A Encode](status, a) Response`,
  `header(r Response|Request? , name) Option[String]` (two functions: `requestHeader`, `responseHeader`),
  `serve(port Int, handler func(Request) IO[Response]) IO[Unit]` — blocks until the enclosing
  context is cancelled (SIGINT or `cancel` on the fiber running it), each request handled on
  its own fiber with the request's context; handler failure → 500 with the message in the body
  and a line on stderr; `servePath` is not needed (route by `r.path` in the handler). Records
  cross the ABI via **new shapes** `rt.Shapes.Tuple3/Untuple3` (codegen `rt_tuple3`/`rt_untuple3`,
  same pattern as Tuple2) — rt returns `(status, headers, body)` tuples and the gofx side builds
  the records. Client has a default 30 s timeout; `requestWithTimeout(r, millis)`.
- **Dependencies.** A gofx library is a Go module containing `.gfx` files and a `gofx.mod`.
  `gofx init <module-path>` writes `go.mod` (module path, Go 1.22, `require gofx
  v0.0.0`) and `gofx.mod`. Its runtime replacement uses the `Dir` resolved by
  `go list -m -json gofx` from cwd, otherwise the embedded runtime extracted
  atomically to the user cache `gofx/runtime/<sha256>` (XDG_CACHE_HOME on Linux,
  default HOME/.cache); never a project-local runtime copy.
  `gofx get <path>@<version>` runs `go get` in the project root and adds/updates
  `require <path> <selected-version>` in `gofx.mod`. Import resolution order:
  nearest source module root → matching `gofx.mod` requires, longest prefix,
  resolved via `go list -m -json <path>` in the entry project's Go graph →
  `GOFX_PATH` → embedded `std`. Go owns replacements, version selection and
  checksums; dependency modules carry their own gofx manifests. Tests use a
  fixture library module wired with `replace` (no network).
- **Show/Float.** `show(Float)` always contains a decimal point or exponent (`1.0`, `1.5`,
  `1e+21`, `NaN`, `+Inf`) — same formatter as `Debug[Float]`. `showFloatFixed` is the display
  recommendation for money (SPEC §4.8 / §10.1 note).
- **Deriving Show** is allowed for enums *and* newtypes (single constructor, single field):
  `show(C(x)) = show(x)` (field type must have `Show`; type params get `Show` constraints).
  Error text for everything else: `deriving Show is only supported for enums and single-field newtypes; derive Debug and write instance Show[T] for a display form`.
  `Show` deriving does not require `Debug` (stated in §7.2 table).
- **Type printing.** Types resolved through an `alias` keep the alias name for printing
  (`types.TAlias{Name, Args, Body}`: transparent to unification/instances, opaque to the
  printer); diagnostics print alias names when the source used them. Diagnostic types longer
  than 160 characters are elided at depth ≥ 3 with `…`; `gofx check --full-types` disables elision.
- **Debug[List]** is the literal form `[a, b]` (Pass 13 wave 0, already in flight).
- **Run anywhere** (wave 0, in flight): the `gofx` binary embeds `rt/**` and synthesises a Go
  module in a temp dir for `run/test/eval` outside any go.mod; only programs with non-`gofx/rt`
  externs need a project go.mod.

### Slices

Wave 0 (running): `run-anywhere` (P0-1/2), `list-debug` (P2-7).
Wave 1 (independent, parallel): `show-rules` (P2-8/9/11/15), `ebnf-sync` (P2-10),
`io-error` (P1-5), `cancel` (P1-3), `compile-scale` (P3-12), `type-print` (P3-13),
`lsp-verify` (P3-14).
Wave 2 (after wave 1 merge): `http` (P1-4, on cancel + io-error), `deps` (P1-6, on run-anywhere).
Wave 3: an `examples/http-service` program (JSON GET → decode → aggregate → serve) written as the
acceptance program for P1, and a re-run of compile-scale/type-print/lsp-verify against it.

## Pass 14: inference direction and polish

Input: the user's 6-program retrospective after Pass 13. Language shape is frozen; this pass
changes inference *direction* and polishes diagnostics/std.

- **Expected types (bidirectional, bounded).** The checker gains an *expected type* parameter
  on expression inference (`inferExpr(e, expected *Type)`; `nil` = unknown). Propagation sites,
  and only these: function body tail ← declared return type; `let x: T = e` ← `T`; `if`
  branches ← the expected type of the `if`; `match` arms ← expected type of the `match`
  (first arm's inferred type when none); constructor and call arguments ← the parameter types
  after instantiating the callee's scheme (unify callee first, then check args with their
  parameter type as expected); record field values ← field types; lambda body ← result type of
  an expected function type, lambda parameters ← parameter types of an expected function type
  when unannotated; `do` final expression ← the block's expected type. Consumers of the
  expectation: placeholder/projection sections (`_.f` resolves the record from the expected
  `func(R) _`), unannotated lambda parameters used with field access, numeric literals are
  **not** affected (Int stays Int). Unification order is the only change: expected type is
  unified *before* inferring sub-expressions, so inference results are identical whenever the
  old algorithm succeeded (property test: every example/test program infers the same
  `TypeString` for every binding before and after). Acceptance: `Some(_.product)` with
  expected `Option[func(Sale) String]`, `map(xs, _.qty)` unchanged, `let f: func(Sale) String = _.product`,
  `match … { … => _.qty }` under a return type.
- **Diagnostics never show internal names.** Synthesized identifiers (`placeholderN_M`, dict
  params, generated lambdas) carry `ast.Ident.Display` (source spelling: `_`, `_.product`);
  the printer uses it. Data types print as `module.Name` (`http.Response`) using the import
  alias in the *diagnosing* file, or the bare name for same-module/prelude types, never the
  mangled `Http_Response`. Test: grep-style assertion over all diagnostics in the test suite
  that no message matches `placeholder\d|_[A-Z]\w*_[A-Z]`.
- **std Debug coverage.** Every std record/sum type derives `Debug` (`http.Request/Response`,
  `json.Json`, `io.IoError` already); `docs/stdlib_inventory_test.go` asserts that every std
  `type` has a `Debug` instance (derived or explicit).
- **HTTP server logging.** Handler outcomes classified `Cancelled` caused by client
  disconnect are not logged; other handler failures keep `http handler: <msg>`. `serve` logs
  nothing on normal shutdown.

## Pass 15: Go shell, gofx core (contract for parallel slices)

Input: a user review of a 3,000-line controller-runtime Reconcile. The decision logic
("observed state → next step") would read better as a gofx `match` over a sum type with
exhaustiveness and `deriving Eq`, but gofx cannot be embedded: `--emit-go` exposes only
`Main() rt.IO`, records cross the ABI erased, and externs cannot name methods. Porting
client-go/controller-runtime is rejected: they are mutable-cache/reflection designs, several
hundred thousand lines regenerated per Kubernetes release. The TypeScript analogy holds in
one direction (`bind` ≈ d.ts generation) and fails in the other: Go and gofx do not share a
value representation, so declarations need conversion code (closer to cgo than d.ts).

Same mechanics as Pass 13: worktree `.wt/<slice>` / branch `slice/<slice>`, workers do not
commit, slices edit `SPEC.md` sections named in their contract, `docs/check_test.go` and
`docs/stdlib_inventory_test.go` keep docs honest. Language shape stays frozen (no new syntax,
no new keywords): the existing `_` private prefix (§1.2) is the export gate.

### Domain model

- **Natural Go mirror** (`codegen/mirror.go`, new). A total function from a closed set of
  gofx types to Go types used at *both* ABI directions. Mirrorable types, compositionally:
  `Int`→`int64`, `Float`→`float64`, `String`→`string`, `Bool`→`bool`, `Unit`→`rt.Unit`,
  `List[T]`→`[]T`, `Option[T]`→`rt.Option[T]` (generic `struct{ Value T; Present bool }`,
  constructors `rt.Some(v)`, `rt.None[T]()`), `Result[E, A]`→`rt.Result[E, A]`
  (`struct{ Err E; Ok A; IsOk bool }`), `Map[K, V]`→`map[K]V` and `Set[A]`→`map[A]struct{}`
  only for scalar `K`/`A` (Int, Float, String, Bool), tuples →`rt.Tuple2[A, B]` …
  `rt.Tuple8` (`F0`…), `IO[T]`→`func(context.Context) (T, error)` **in result position
  only**, and non-parameterised user data types. Not mirrorable: functions, classes, type
  parameters, parameterised user data types (even fully applied), `IO` in parameter or
  nested position, `Map`/`Set` with non-scalar keys. The erased ABI of §8.3 ("Other
  types → any") is unchanged for anything outside this set.
- **Data type mirrors.** For a mirrorable user type `T` with constructors `C1..Cn`:
  - single constructor with field names (record): `type T struct { <Exported field> … }`,
    field names via `exported()` (first rune upper-cased); duplicate exported spellings
    are a generation error at the type declaration.
  - single positional constructor (newtype/tuple-like): `type T struct { F0 …; F1 … }`.
  - all constructors nullary (enum): `type T int`, `const ( TC1 T = iota; TC2; … )` in
    declaration order, plus `func (T) String() string` returning the constructor name.
  - otherwise (sum with payloads): `type T interface { isT() }`; each constructor is a
    **value** type `type TCi struct { … }` (record fields exported, positional `F0`…,
    nullary `struct{}`) with a value-receiver marker method. Go code type-switches on
    `TCi` values, never pointers.
  Mirror names: entry-module types use their source spelling (`Service`), constructor
  types/consts are `<Type><Ctor>` (`StepCleanup`); types from dependency modules get the
  module prefix from `codegen/modules.go` (`HttpResponse`, `StdIoIoError` style via
  `exported(prefix)+Name`), deterministic collision suffixes as for file prefixes. Internal
  erased names are always module-prefixed and lower-case-joined (`main_Service`,
  `main_Service_Service`), so they cannot collide with mirrors; a programmatic
  `Generate` with no modules and a mirror/internal name clash is a generation error.
- **Converters.** Per mirrorable type the generator emits unexported
  `toGo_<Mirror>(v any) <Mirror>` and `fromGo_<Mirror>(v <Mirror>) any`, structurally
  recursive, total on well-typed values (an unknown constructor panics with
  `invalid constructor` like `commonField`). Lists copy; maps copy (no aliasing in either
  direction); `Map` conversion from Go iterates sorted keys so the persistent tree is
  built deterministically.
- **Exported function.** With `--emit-go --package NAME`, every top-level function of the
  **entry module** that is non-extern, not `main`, not `_`-prefixed, has no type parameters
  or constraints, and whose parameter and result types are all mirrorable, is exported as
  `func <Exported name>(<params mirrored>) <result mirrored>` wrapping the internal
  function with converters. A result `IO[T]` becomes `func(ctx context.Context) (T, error)`:
  the wrapper runs the `rt.IO` with `ctx` and converts the value; failures are returned as
  the `*rt.Failure` error. Functions that fail a rule are listed once in
  `gofx_export.go` as `// not exported: <name> (<reason>)` (the `bind` skip convention,
  §8.3) so the user learns why; `main` and `_`-prefixed functions are silently omitted.
  `Main() rt.IO` stays as today. Nothing changes for `goPackage == "main"`.
- **Files.** Mirrors, converters and wrappers go in a new generated file
  `gofx_export.go` (same package); it is removed by the obsolete-file sweep like other
  compiler-generated files (§8.2). Mirrors are emitted only for types reachable from an
  exported signature (transitively), in sorted name order.
- **Method shims for `bind`.** `gofx bind IMPORT --shim DIR` writes, next to the `.gfx`
  declarations, a Go file `DIR/gofx_shim_<pkgbase>.go` (`package` = the receiving
  directory's Go package, read from an existing `.go` file, else the last path element)
  containing one package-level function per exported method of each exported named type:
  `func <Type>_<Method>(recv <Type or *Type>, args…) results` that calls the method. The
  `.gfx` output then declares `extern func <type><Method>(recv <Handle>, …) … =
  "<module>/<dir>".<Type>_<Method>` where `<Handle>` is a nullary data type
  `type <Type> = <Type>Handle` emitted once per receiver type in the same `.gfx` file.
  A handle is an **opaque** value: gofx passes it through the erased ABI and never
  inspects it (matching, `deriving`, `show` on a handle are undefined and documented so).
  Methods are skipped with the same reasons as functions (pointers other than the
  receiver, callbacks, variadics, generics, unsupported results). Without `--shim`,
  `bind` behaves as today. The shim is a convention over existing extern machinery: no
  change to `extern/match.go` rules.

### Slices

- `go-export` (deep): mirror, converters, exported wrappers, `gofx_export.go`,
  `rt.Option/Result/TupleN` generics in `rt/mirror.go`, SPEC §8.2 (`build` bullet),
  new §8.7 "Exported Go API", §10.7 rows, `examples/go-shell/` (a Go `main` that imports
  an emitted package and calls a controller-style `decide(observed) Step`, run by a
  `cmd/gofx` tools test like `TestEmitGoPackageCanBeImported`), README pointer.
- `bind-shim` (deep): `--shim`, handle types, Go shim file, SPEC §8.3 `bind` paragraph,
  §10.7 row, `examples/typed-extern` extended with one method binding
  (`strings.Builder` or `net/url.URL` method) proven by `gofx run`.

Deviations: the extern (Go→gofx call) direction keeps the §8.3 natural ABI table; using the
full mirror there is deferred until an example needs it. `IO[T]` is mirrored only as a
result. Enum mirrors are `int`-based, not `string`-based, so zero values are valid.
- Value-recursive single-constructor mirrors without slice/map/sum-interface indirection are rejected at their type declaration (`recursive Go value type <name>`): Go value structs cannot represent infinite size.
- Float Map/Set mirrors retain one NaN key but reject multiple Go NaN keys (`gofx: duplicate NaN map key` / `gofx: duplicate NaN set key`), since gofx Ord equates them and selecting a survivor would depend on Go map iteration order.
Method-shim deviation (approved): receivers use `any` plus a checked assertion to the original value/pointer type, rather than a typed Go parameter, to preserve `extern/match.go`'s erased-handle ABI; wrong receiver types panic with `gofx shim: <Type>_<Method>: receiver is <%T>`.


## Pass 16: parametric mirrors

Input: the approval-controller port (`examples/approval-controller`) had to write
`FetchedPlan`/`FetchedPolicy`/`FetchedWindow` by hand because Pass 15 mirrors exclude
parameterised user data types. Fully applied applications are mirrorable by monomorphisation.

- **Rule.** A type application `T[A1, …, An]` of a user data type is mirrorable iff every `Ai`
  is mirrorable and the resulting instantiated constructor fields are mirrorable (checked
  after substitution; recursion through `List`/`Map`/sum interfaces stays allowed, value
  recursion stays an error as in Pass 15). Bare `T` with parameters, or applications with
  type variables, remain not mirrorable.
- **Naming.** The Go mirror name is `<T><MirrorName(A1)>…<MirrorName(An)>` where
  `MirrorName` is the existing mirror spelling of each argument with non-identifier
  characters removed (`Fetched[Plan]` → `FetchedPlan`; `Fetched[List[String]]` →
  `FetchedListString`; `Pair[Int, Option[Bool]]` → `PairIntOptionBool`). Constructor mirrors
  are `<InstanceName><Ctor>` (`FetchedPlanFound`). Two distinct instantiations never share a
  name because mirror spellings are injective on the mirrorable set; a clash with a
  user-declared type of that spelling is a generation error at the user declaration.
- **Converters** are emitted per instantiation (`toGo_FetchedPlan` / `fromGo_FetchedPlan`),
  reachability-driven as today. The erased runtime representation of the instance is the
  shared generic constructor struct (`main_Fetched_Found{F0 any}`); converters substitute
  field types from the application's arguments.
- **String()** follows Pass 15 rules using the `Debug` dictionary of the instantiated type
  (instance resolution with concrete arguments, same as specialised calls).
- SPEC §8.7 mirror table gains the row and the naming rule; §10.7 gains the clash row.
- Acceptance: `examples/approval-controller/decide/decide.gfx` rewritten to
  `type Fetched[A] = NotReferenced | ReadError | Missing | Found(A)` used at `Plan`, `Policy`,
  `Window`; the controller's Go adapter uses `decide.FetchedPlanFound{F0: …}` etc.; all tests
  there unchanged and passing; `go generate` idempotent.


## Pass 17: interpreter loop for controllers that write mid-decision

Input: the approval controller proved read-only decisions; the reviewer asked how thick the Go
"shell" becomes when the decision requires a write (or a further read) in the middle. Target:
`examples/attachment-repair-controller` (verbatim copy of platform
`storage_attachment_repair_operation_controller.go`; its 973 lines include a compare-and-swap
`Delete` at the decision's centre and a dependent-read observation chain).

### Boundary

- **Observation stays in Go.** `observeBase`/`classifyInitialAttachments`/`observeAttachmentNodes`/
  `observeAuthorizedSafety` are dependent reads with I/O classification reasons
  (`PersistentVolumeNotFound`, `PodTargetNotUnique`, …); they are kept verbatim and project to a
  small result sum. Their reason strings cross as opaque `String` (I/O-layer facts, like error
  messages), not as `Reason` constructors.
- **The workflow state machine moves to gofx** (`decide/decide.gfx`): everything in
  `reconcileOperation` and `reconcileAuthorized`, plus `readBoundApproval`'s pure part
  (`validateAttachmentRepairApprovalBinding`, expiry, state derivation).

### Interpreter protocol (the design under test)

`decide(o Observed) Action` is pure and total. `Observed` carries every observation as an
`Option`/sum that starts empty; `Action` is

```
type Action = ObserveInitial | ObserveAuthorized | DeleteStale | Publish(Status)
```

Go runs a loop: call `Decide`; on `ObserveInitial`/`ObserveAuthorized` perform that observation,
fill the field, call again; on `DeleteStale` perform the compare-and-swap delete, fill
`deleteResult: Option[DeleteResult]`, call again; on `Publish` build the status map and stop. The
loop is generic (no controller-specific branching), bounded (each iteration fills one more field;
`decide` never asks for a field already filled — a `panic("decide asked twice")` guard in Go makes
that invariant observable), and reruns the pure function with strictly more information. No
continuations are encoded in `Action`: the next decision is simply `decide` again.

Projection types:

```
type Mode = DryRun | Live(ApprovalRead)
type ApprovalRead = NoRef | ReadFailed | Missing | Read(Approval)      // Go reads the object
type Approval = Approval{ deleting, refMatches, opIdentityMatches: Bool, generation: Option[Int],
                          targetBindingMatches, digestValid, evidenceComplete: Bool,
                          approvedAt, expiresAt: Timestamp, phase: String, digest: String }
type Timestamp = NoTimestamp | InvalidTimestamp | AtNanos(Int)          // from Pass 16 example
type Observation = ObsBlocked(String, String) | ObsOk(String)            // reason, message | digest
type AuthorizedObservation = AuthBlocked(String, String)
                           | AuthOk{ target: Target, digest: String, currentAttached: Bool }
type Target = Present | Deleting | Absent
type DeleteResult = Deleted | CasFailed | DeleteFailed
type Observed = Observed{ mode: Mode, featureGateEnabled, authorized, committed, requested: Bool,
                          approvedObservationDigest: String, nowNanos: Int,
                          initial: Option[Observation], authorized: Option[AuthorizedObservation],
                          deleteResult: Option[DeleteResult] }
type Status = Blocked(String, ReasonCode, String) | Running(String, String, Bool)
            | Succeeded(String, String, Bool) | AwaitingApproval | Authorized | Committed
type ReasonCode = Decided(Reason) | Observed(String)
```

`Status` names the status-builder the Go side already has (`blockedStatus`, `runningStatus`, …);
requeue is derived from the constructor (Blocked/Running/AwaitingApproval → interval, others → 0).
Go passes whichever observation has been performed (initial or authorized) to the builder, and the
approval object when it was read — this rule reproduces every `nil`/non-nil argument in the
original call sites (verified by reading L100-205). `Authorized` implies
`safety["approvalMatchesObservation"] = true` (the original sets it just before building).

### Measure

Report (a) Go lines of the interpreter loop and status mapping, (b) number of decision iterations
per path (max 3: approval → observation → delete), (c) oracle tests unchanged and passing, (d)
`go generate` idempotent. The interpreter is the cost; it must not grow with the number of
decision branches.

