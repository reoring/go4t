# gofx Language Specification

gofx is a small, statically typed, purely functional language with Go-flavoured syntax.
It adds higher-kinded types, type classes with dictionary passing, algebraic data types,
records, pattern matching and an `IO` type, and compiles to plain Go. This document
specifies the language as implemented; `DESIGN.md` records the implementation contracts
and the history of decisions, `examples/` are executable illustrations.

Normative words: *must*, *error* (a compile-time rejection), *warning* (reported, compile
continues).

## 1. Lexical structure

### 1.1 Source
UTF-8 text. One source file is one program; the prelude (`prelude/prelude.gfx`) is checked
before it in the same scope.

### 1.2 Comments
`//` to end of line. No block comments.

### 1.3 Identifiers
`Letter { Letter | Digit | "_" }`. A lone `_` is the wildcard pattern, not an identifier.
Identifiers whose first letter is upper-case name **types, classes and constructors**;
lower-case first letters name **values** (functions, methods, parameters, locals, fields).
This distinction is enforced by the checker (`constructor name must be capitalised`,
`value name must start with a lower-case letter`). Identifiers starting with `dict_` or
`ext_` are reserved (error).

Keywords: `package class instance type func extern let do match if else true false given`.

### 1.4 Literals
- Int: decimal digits, 64-bit signed.
- String: `"…"` with Go escape sequences (`\n`, `\"`, `\\`, …).
- Interpolated string: a string containing `${expr}`. `$$` is a literal `$`; a lone `$` and
  any `{`/`}` outside `${…}` are literal. Nested strings are allowed inside `${…}`.
- Bool: `true`, `false`.

### 1.5 Punctuation
`( ) [ ] { } , ; . .. = : | @ => <- == != <= >= && || + - * / < > ! _`

### 1.6 Layout (automatic semicolons)
A newline is converted to `;` iff all of:
1. the previous token is an identifier, Int, String, interpolated string, `true`, `false`,
   `)`, `]`, `}` or `_`;
2. the innermost unclosed bracket is `{` or there is none (newlines inside `(…)` and `[…]`
   never produce `;`);
3. the next token (after blank lines and comments) is not a *continuation token*:
   `else`, `|`, `||`, `.`, `)`, `]`, `given`, `=>`, `<-`, or a binary operator
   (`+ - * / == != < <= > >= &&`).

Consequences: `else` may start a line; match arms may be written one per line with or
without a leading `|`; constructor alternatives in `type` may start lines with `|`;
operators may lead a continuation line; a line starting with `-` is a continuation (write
`0 - x` to start a statement with negation).

## 2. Program structure

```
File := 'package' Ident { Decl }
Decl := Class | Instance | TypeDecl | Func | Extern
```

Declarations may appear in any order; all top-level names are mutually visible (recursion
between functions is allowed). The program must declare `func main() IO[Unit]`.

### 2.1 Namespaces
Three independent namespaces: **types** (data types, classes, builtins), **constructors**,
and **values** (functions, methods, externs, parameters, locals). `type Sku = Sku(String)`
is legal. Within a namespace a name may be declared once per program (prelude + user file);
a user file may not redeclare a prelude name. Record field names are scoped to their
constructor.

## 3. Types

### 3.1 Kinds
`*` is the kind of value types. `(*, …) -> *` is the kind of a type constructor with that
many parameters. Builtins: `Int String Bool Unit : *`; `IO : (*) -> *`;
`TupleN : (*,…,*) -> *` (one per arity ≥ 2, written `(A, B)`). Data types have kind
`(*,…) -> *` by their parameter count; all data-type parameters have kind `*`.

### 3.2 Type expressions
```
Type := Ident [ '[' Type {',' Type} ']' ]        // constructor application
      | 'func' '(' [Type {',' Type}] ')' Type     // function
      | '(' Type ')'                               // grouping
      | '(' Type ',' Type {',' Type} ')'           // tuple, arity >= 2
```
A type constructor may be partially applied where a type of arrow kind is expected
(`Result[E]` has kind `(*) -> *` when `Result : (*,*) -> *`). Application is flat:
`Result[E][A]` is not syntax; `Result[E, A]` is the fully applied form.

### 3.3 Type parameters and constraints
```
TParams := '[' TParam {',' TParam} ']'
TParam  := Ident [ '[' '_' {',' '_'} ']' ] [ Ident ]
```
`F[_]` declares a parameter of kind `(*) -> *`; `F[_, _]` of arity 2. A trailing class name
constrains the parameter (`F[_] Functor`, `A Show`). At most one constraint per parameter.

### 3.4 Data types
```
TypeDecl := 'type' Ident [ '[' Ident {',' Ident} ']' ] '=' Ctor { '|' Ctor }
Ctor     := Ident                                  // nullary
          | Ident '(' Type {',' Type} ')'          // positional fields
          | Ident '{' Ident ':' Type {',' Ident ':' Type} '}'   // record fields
```
A constructor is either positional or record-form, never both. Record constructors are
positional constructors with field names: they may be constructed and matched positionally
as well as by name. Types may be recursive and parameterised.

### 3.5 Functions
```
Func   := 'func' Ident [TParams] '(' [Param {',' Param}] ')' Type Block
Param  := Ident Type
Extern := 'extern' 'func' Ident [TParams] '(' Params ')' Type '=' String '.' Ident
```
Top-level functions are polymorphic over their declared type parameters (rank-1). The body
is a block whose final expression must have the declared return type.

An `extern` binds a Go function `"import/path".Name`. The Go function must use the erased
ABI: every parameter and the result are `any` (an `IO[T]` result is an `rt.IO`). Externs are
the only effect/FFI boundary; the checker treats them as opaque.

### 3.6 Classes and instances
```
Class     := 'class' Ident '[' TParam ']' [ ':' Ident '[' Ident ']' ] '{' { MethodSig } '}'
MethodSig := Ident [TParams] '(' Params ')' Type
Instance  := 'instance' Ident '[' Head ']' [ 'given' Constraint {',' Constraint} ]
             '{' { Ident '(' Idents ')' Block } '}'
Head      := Ident                        // bare type constructor
           | Ident '[' Ident {',' Ident} ']'   // applied / partially applied head
           | '(' Ident ',' Ident {',' Ident} ')' // tuple head (sugar for TupleN[A, B])
Constraint := Ident '[' Ident ']'
```
A class has exactly one parameter, of any kind, and optionally one superclass applied to the
same parameter (`class Monad[M[_]] : Functor[M]`). Methods may have their own type
parameters and constraints.

An instance head names a type constructor `T` and zero or more fresh type variables:
- `instance C[T]`: `T`'s kind must equal the class parameter's kind.
- `instance C[T[A, B]]` (full head): `C`'s parameter has kind `*`, `A, B` cover all of `T`'s
  parameters. Used with `given` to express *constrained instances*
  (`instance Show[List[A]] given Show[A]`).
- `instance C[T[E]]` (partial head): fewer variables than `T`'s arity; the remaining arity
  must equal the class parameter's kind arity (`instance Functor[Result[E]]`). `given` may
  constrain the fixed variables.

Each class method must be implemented exactly once with the declared parameter count; the
method bodies are checked against the method signature with the class parameter replaced by
the head. An instance for a subclass requires an instance of the superclass with the same
head shape and the same context. At most one instance per `(class, type constructor)`
(error `duplicate instance`).

Instance resolution for a constraint `C[T]` (after inference): if `T` is a rigid type
parameter, it must carry `C` or a subclass of `C` in its constraint; otherwise `T`'s head
constructor must have an instance of `C`, whose `given` context is resolved recursively on
`T`'s arguments. Failure is an error: `no instance C[T]`, with a hint naming the offending
type parameter and its actual constraint, or the instance that required the inner instance.

## 4. Expressions

```
Expr    := 'func' '(' [LParam {',' LParam}] ')' '=>' Expr    // lambda
         | 'do' '{' DoStmts '}'
         | 'match' Expr '{' Arms '}'
         | 'if' Expr Block 'else' (Block | IfExpr)
         | Binary
LParam  := Ident [Type]
Binary  := Unary { BinOp Unary }           // precedence below
Unary   := ['-' | '!'] Postfix
Postfix := Primary { '(' [Expr {',' Expr}] ')' | '.' Ident | '.' Int | RecordFields }
Primary := Ident | Int | String | Interp | 'true' | 'false'
         | UpperIdent RecordFields                // record construction
         | '(' Expr ')' | '(' Expr ',' Expr {',' Expr} ')'   // tuple
         | '[' [Expr {',' Expr}] ']'              // list literal
         | Block
Block   := '{' { Stmt ';' } Expr '}'
Stmt    := 'let' Ident '=' Expr | Expr
RecordFields := '{' Ident ':' Expr {',' Ident ':' Expr} '}'
```

Precedence, lowest to highest: `||`; `&&`; `== != < <= > >=`; `+ -`; `* /`; unary `- !`;
postfix. All binary operators are left-associative.

### 4.1 Operators
| operator | operand types | result |
|---|---|---|
| `+` | `Int`/`Int` or `String`/`String` | same |
| `- * /` | `Int` | `Int` (`/` truncates; division by zero panics at runtime) |
| `== !=` | any `T` with an `Eq[T]` instance | `Bool` (`a != b` ≡ `!eq(a, b)`) |
| `< <= > >=` | any `T` with an `Ord[T]` instance | `Bool` (via `compare(a, b)`) |
| `&& \|\|` | `Bool` | `Bool`, short-circuit |
| unary `-` | `Int` | `Int` |
| unary `!` | `Bool` | `Bool` |

`==`/`!=` and the ordering operators are sugar for the `Eq`/`Ord` class methods (§7), so
they work on any user type with an instance; the operand types must unify. `Bool` orders
`false < true`. Arithmetic is not class-based: `+ - * /` are `Int` only (`+` also `String`).

### 4.2 Blocks and `let`
A block's value is its final expression. `let x = e` or `let x: T = e` binds `x` for the
rest of the block; with an annotation `e` must have type `T` (type parameters of the
enclosing function may appear in `T`). Bindings are not recursive and shadow outer names.

`let` generalises (let-polymorphism) when the bound expression is a lambda, a literal, or a
constructor application; type variables not free in the environment become scheme
parameters and class constraints on them become scheme constraints. Other expressions
(calls, `do`, `match`, …) and annotated lets are bound monomorphically. Function parameters
are never generalised.

### 4.3 Lambdas
`func(x, y) => e`. Parameters are monomorphic; their types are inferred, or declared
(`func(u User) => …`). A parameter may be an irrefutable pattern instead of a name:
`func((name, score), _) => …`, `func(Point{x: x, ..}) => …`, `func(whole @ (a, b)) => …`.
Refutable patterns (literals, constructors of multi-constructor types) are an error. Pattern
parameters cannot carry a type annotation. When a lambda is passed directly as a call argument its parameter
types are taken from the callee's parameter type before the body is checked, so record and
tuple field access on the parameters needs no annotation in that position.

### 4.4 Calls
`f(a, b)`. Arity must match. Function-typed values (locals, parameters, fields) are called
the same way.

### 4.5 `if`
`if c { a } else { b }` is an expression; `c : Bool`; both branches have the same type.
`else if` chains are allowed. `else` is mandatory.

### 4.6 Tuples and lists
`(a, b)` constructs a tuple (arity ≥ 2); `(a)` is grouping. `t.0`, `t.1`, … read positions;
the static type of `t` must be a tuple type with enough positions. `[a, b, c]` is sugar for
`Cons(a, Cons(b, Cons(c, Nil)))` with all elements of one type; `[]` is `Nil`.

### 4.7 Records
`C{f: e, g: e2}` constructs a record constructor by name (all fields exactly once, any
order). `x.f` reads a field; `x{f: e}` produces a copy with the listed fields replaced. Both
require the static type of `x` to be a data type with exactly one constructor that declares
field `f` (error otherwise); if `x`'s type is not yet known the checker reports
`cannot infer record type of x.f; annotate`.

### 4.8 `match`
```
Arms   := [ '|' ] Arm { ( ';' | '|' ) Arm }
Arm    := Pattern [ 'if' Expr ] '=>' Expr
Pattern := '_' | Int | String | 'true' | 'false'
         | LowerIdent                              // binds
         | LowerIdent '@' Pattern                  // as-pattern
         | UpperIdent [ '(' Pattern {',' Pattern} ')' ]       // constructor, positional
         | UpperIdent '{' [ Ident ':' Pattern {',' Ident ':' Pattern} ] [',' '..'] '}' // record
         | '(' Pattern ',' Pattern {',' Pattern} ')'          // tuple
         | '(' Pattern ')'
```
Arms are tried in order; the first arm whose pattern matches **and** whose guard (if any)
is `true` yields the value. Guards see the pattern's variables. All arms have one type.
Guarded arms do not contribute to exhaustiveness. The arms **must be exhaustive** over the scrutinee's constructors (nested patterns are
analysed; literal patterns need a trailing catch-all); non-exhaustive matches are an error.
A record pattern must list every field or end with `..`. A variable may be bound once per
pattern.

### 4.9 Interpolated strings
`"a ${x} b ${f(y)}"` has type `String`. Each interpolated expression must have a `Show`
instance; its `show` result is spliced in.

### 4.10 `do`
```
DoStmts := { DoStmt ';' } Expr
DoStmt  := Ident '<-' Expr | 'let' Ident '=' Expr | Expr
```
`do` sequences a monad. Every `x <- e`, bare `e`, and the final expression must have type
`M[…]` for one and the same `M`, which must have a `Monad` instance (or be a type parameter
constrained by `Monad`). `x <- e` binds the result; a bare statement discards it; `let` is an
ordinary binding. The block's type is the final expression's type. Desugaring is
right-nested `bind`.

## 5. Type system

Hindley–Milner inference with:
- rank-1 polymorphism at top level and generalised `let` (§4.2);
- higher-kinded type variables (`F[_]`) and partial application of type constructors in
  unification (`F[a] ~ Result[E, A]` solves `F := Result[E]`, `a := A`);
- type classes resolved to dictionaries after inference (§3.6);
- rigid type parameters (a declared parameter unifies only with itself).

Unresolved type variables of kind `*` that carry no class constraint default to `Unit`
with a **warning** (`type of None defaulted to Unit; add an annotation or use the value`).
Unresolved variables of arrow kind or with constraints are an error.

Diagnostics: `file:line:col: message`. Mismatches read
`type mismatch: expected T, got U` where `expected` is the type demanded by context.

## 6. Evaluation

- Strict (call-by-value), left-to-right argument evaluation.
- All values are immutable. Data constructors allocate; record update copies.
- Effects exist only inside `IO[A]`. An `IO` value is a description; running `main`'s `IO`
  performs it. `IO` is a `Monad`; `bind` short-circuits on failure: a failed step (an extern
  returning a Go error, or `ioFail(msg)`) stops the sequence and the program prints
  `error: <msg>` to stderr and exits 1. Nothing is recovered inside the language.
- Functions, closures and dictionaries are first-class; there is no mutation, no `null`,
  no exceptions, no concurrency primitive.

## 7. Prelude

Always in scope. Types and classes:
```
class Functor[F[_]] { fmap[A, B](fa F[A], f func(A) B) F[B] }
class Monad[M[_]] : Functor[M] { pure[A](a A) M[A]; bind[A, B](ma M[A], f func(A) M[B]) M[B] }
class Show[A] { show(a A) String }
class Eq[A]   { eq(a A, b A) Bool }
class Ord[A] : Eq[A] { compare(a A, b A) Ordering }
type Ordering = LT | EQ | GT
type Option[A] = None | Some(A)
type List[A]   = Nil | Cons(A, List[A])
type State[S, A] = State(func(S) (A, S))
```
Instances: `Functor`/`Monad` for `Option`, `List`, `IO`, `State[S]`; `Show`, `Eq` and
`Ord` for `Int`, `String`, `Bool`, `Ordering`, `Option[A]`, `List[A]` and `(A, B)` (the
compound ones `given` the same class on the components); `Eq[Unit]`. User types get `Eq`/`Ord`
by writing an instance; there is no deriving.

Functions:
```
println(s String) IO[Unit]          unit() Unit
ioPure[A](a A) IO[A]                ioFail[A](msg String) IO[A]
map[F[_] Functor, A, B](fa F[A], f func(A) B) F[B]
flatMap[M[_] Monad, A, B](ma M[A], f func(A) M[B]) M[B]
forEach[M[_] Monad, A](xs List[A], f func(A) M[Unit]) M[Unit]
mapM[M[_] Monad, A, B](xs List[A], f func(A) M[B]) M[List[B]]
fold[A, B](xs List[A], init B, f func(B, A) B) B
length[A](xs List[A]) Int           append[A](xs List[A], ys List[A]) List[A]
filter[A](xs List[A], p func(A) Bool) List[A]      reverse[A](xs List[A]) List[A]
head[A](xs List[A]) Option[A]       tail[A](xs List[A]) List[A]
take[A](n Int, xs List[A]) List[A]  drop[A](n Int, xs List[A]) List[A]
sum(xs List[Int]) Int               concatMap[A, B](xs List[A], f func(A) List[B]) List[B]
elem[A Eq](x A, xs List[A]) Bool    sort[A Ord](xs List[A]) List[A]
sortBy[A](xs List[A], cmp func(A, A) Ordering) List[A]
maximum[A Ord](xs List[A]) Option[A]   minimum[A Ord](xs List[A]) Option[A]
minInt() Int   maxInt() Int
fst[A, B](p (A, B)) A               snd[A, B](p (A, B)) B
zip[A, B](xs List[A], ys List[B]) List[(A, B)]
runState[S, A](st State[S, A], s S) (A, S)   evalState …  A   execState … S
get[S]() State[S, S]   put[S](s S) State[S, Unit]   modify[S](f func(S) S) State[S, Unit]
```

## 8. Compilation model (informative)

Types and dictionaries are erased to Go `any`; classes become Go interfaces, instances
become structs (with one field per `given` constraint), constrained functions take
dictionary parameters, `do` becomes nested `Bind` calls, `match` becomes type switches,
tuples become `TupleN` structs. Generated code imports `gofx/rt` and the extern packages.
`gofx build FILE.gfx` writes `FILE_gen/main.go`; `gofx run` builds and executes it;
`gofx check` type-checks only.

## 9. Known limitations

- No floats, chars, arrays, maps, or mutable references.
- No type ascription on expressions (only on `let` and parameters).
- Field access on a lambda parameter requires the type to be known (annotation or call
  context).
- Prelude names cannot be shadowed by user declarations.
- No `deriving`: `Eq`/`Ord`/`Show` instances for user types are written by hand.
- `if` always requires `else` (it is an expression); use `match` guards for case analysis.
- Performance: uniform `any` representation, interface-dispatched dictionaries, linked
  lists.
