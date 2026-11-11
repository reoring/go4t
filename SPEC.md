# go4t Language Specification

go4t is a statically typed, strict, functional language with Go-flavoured syntax,
higher-kinded types, type classes, immutable algebraic data, and explicit `IO`.
It compiles to Go. This document describes the current implementation;
[the tutorial](docs/tutorial.md) teaches it and [the standard library](docs/stdlib.md)
lists every source declaration. `docs/history/DESIGN.md` records historical design contracts,
not an alternative definition of current behaviour.

*Error* means compile-time rejection; *warning* is nonfatal. Grammar below uses
EBNF: `:=` defines a production, `|` chooses, `{...}` repeats zero or more times,
`[...]` is optional, parentheses group, and quoted strings are fixed lexer
tokens. `? ... ?` names a lexical token class defined in §1. Commas and
separators are explicit in the grammar; a `Separator` may be written or inserted
by layout. `Name` is optionally qualified by a file-local import alias.
`File` is the grammar's only unreferenced root. `docs/grammar_test.go` checks
all `ebnf` fences, reference/terminal coverage, and bounded parser-only generation.
Complete `go4t` fences are checked by `docs/check_test.go`; `go4t error` fences
must fail.

## 1. Lexical structure

### 1.1 Source and comments

Source is UTF-8. A module consists of `.gt` files sharing a `package` spelling
(§2). `//` comments run to end of line; there are no block comments. Adjacent
non-trailing comment lines immediately above a declaration supply editor hover
documentation; a blank line breaks attachment.

### 1.2 Identifiers and keywords

Identifiers contain letters, digits, and underscores, and begin with a letter or
underscore. Ignoring a leading private underscore, types, classes and constructors
start upper-case; values, methods and fields start lower-case. `_` alone is a
pattern wildcard or a restricted expression placeholder (§4.5), not a value name.
Prefixes `dict_` and `ext_` are reserved. `_name` suppresses unused-local warnings
and makes a top-level declaration private to its module.

Keywords: `package import alias class instance type func extern let do match if
else true false given deriving law proof refl rewrite`.

### 1.3 Literals and punctuation

- `Int`: decimal digits, signed 64-bit values; a negative number uses unary `-`.
- `Float`: `Digits '.' Digits [Exponent]` or `Digits Exponent`, where
  `Exponent := ('e' | 'E') ['+' | '-'] Digits`. Examples: `1.5`, `2e3`, `1.0e-3`.
  Neither `1.` nor `.5` is a source Float literal. Overflow is a parse error.
- `String`: double-quoted text with Go escapes, e.g. `"a\n"`.
- Interpolation: `${expr}` inside a string uses `Show`; `$$` means literal `$`.
  Lone dollars and braces outside an interpolation are literal; nested strings
  are permitted inside the interpolated expression.
- `Bool`: `true`, `false`.

Punctuation includes `( ) [ ] { } , ; . .. = : | |> @ => <- == != <= >= && || + - * / % < > ! _`.

The lexical leaves of the syntactic grammar are token classes, not literal
spellings. `LowerIdent` includes private lower-case names; `UpperIdent` includes
private upper-case names (ignoring leading underscores). Neither includes `_`.
`String` excludes interpolation; `Interp` contains at least one `${...}`.

```ebnf
Ident      := ? identifier ?
LowerIdent := ? lower-case identifier ?
UpperIdent := ? upper-case identifier ?
Int        := ? integer literal ?
Float      := ? float literal ?
String     := ? string literal ?
Interp     := ? interpolated string literal ?
```

### 1.4 Layout

A newline inserts `;` when the previous token can end an expression (identifier,
Int, Float, String/interpolated string, Bool, `refl`, `)`, `]`, `}`, `_`), the innermost
unclosed delimiter is a block brace or there is none, and the next token is not
a continuation: `else`, `|`, `||`, `|>`, `.`, `)`, `]`, `given`, `deriving`,
`=>`, `<-`, or a binary operator. Blank lines/comments do not break continuation.
Newlines inside parentheses, brackets, record declarations/literals/updates and
record patterns do not separate statements. Fields still need commas.

Thus `else`, match alternatives, operators and pipeline steps may lead a line.
A leading `-` continues the previous expression: use grouping or an explicit
semicolon to start a separate negative expression. Explicit semicolons always
separate statements. Tuple access `t.0.1` is not lexed as a Float literal.

## 2. Program structure

### 2.1 Files, declarations and namespaces

```ebnf
File       := {Separator} 'package' Ident {Separator} {Import} {Decl {Separator}}
Import     := 'import' [Ident] String {Separator}
Decl       := Class | Instance | TypeDecl | AliasDecl | Func | Extern | LawDecl | ProofDecl
Name       := Ident ['.' Ident]
Separator  := ';'
```

Imports precede declarations. Top-level declarations in one module are mutually
visible, including recursion across files. `check`, `build` and `run` require an
entry `func main() IO[Unit]`; `test` and `eval` replace the entry (§8).
Types/classes, constructors and values have separate namespaces, so
`type Sku = Sku(String)` is legal. Duplicate declarations within a namespace are
errors. Record field names belong to their constructors. Builtin types cannot
be redeclared.

### 2.2 Modules and resolution

A directory input loads its immediate `.gt` files in lexical filename order,
not recursively. Explicit filenames select only those entry files, all in one
directory and with one package spelling. Imported modules load every immediate
`.gt` file. Module identities are absolute directories (embedded std identities
are `std/<module>`), independent of alias. Dependencies are deduplicated before
traversal; recursive import graphs are supported.

`import "geometry"` uses the last path component as its alias;
`import g "geometry"` uses `g`. Imports are **file-scoped**. They never introduce
unqualified declarations. Dot, wildcard and blank imports are not supported;
duplicate/invalid aliases are errors. The same module may have several aliases.
Qualified values, types, classes, constructors, instance heads, constraints,
deriving classes and constructor patterns are supported.

Import paths are nonempty clean relative slash paths: no absolute paths,
backslashes, `..` or leading `../`. Search in this order:

1. The importing module's nearest ancestor `go4t.mod`; absent a file, the entry
   program's directory is the root. An exact module name refers to the root;
   a `<name>/` prefix is stripped.
2. Matching `require <path> <version>` declarations in that `go4t.mod`.
   Imports use `<path>` for the dependency root or `<path>/<subdir>` for a
   subdirectory. Matching requires use the longest slash-delimited prefix.
   `go list -m -json <path>` runs in the entry project's root, and its `Dir`
   supplies the dependency root, including local `go.mod` replacements.
3. Nonempty `GO4T_PATH` entries in platform path-list order.
4. The compiler's embedded `std/` sources for `std/...` imports.

The first existing directory wins, even if it is empty or invalid; errors do
not fall through. A filesystem std override retains its logical std identity
for intrinsic types and reexports, alongside its absolute-directory identity.

`import "go:<path>"` resolves a Go package through the import-time binder
(ADR 0003), separate from the source-directory search above. Generated bindings
are not committed project sources; build shims stay beneath the build output.

A `go4t.mod` begins with `module <name>`, followed by zero or more single-line
`require <path> <version>` directives. Blank lines and `//` comments are allowed;
duplicate requires, extra module directives and other directives are errors.
Module paths are clean relative slash paths using ASCII letters, digits,
`-`, `.`, `_`, `~`; `.`/`..` components and a leading `-` are not allowed.
Go module path/version rules additionally apply when invoking Go tooling.
`replace` directives belong in **go.mod**, not go4t.mod.

A dependency is a Go module containing `go.mod`, `go4t.mod` and `.gt` sources.
Its own imports use its nearest go4t root and requires, but Go queries always
use the entry project's selected Go graph, so application replacements and Go's
version selection apply transitively. `go4t.mod` declares language dependencies;
`go.mod`/`go.sum` remain authoritative for selected versions and checksums.
`go4t get` synchronizes the requested requirement with Go's selected version.
A declared dependency that cannot be resolved by Go is an error, not a
GO4T_PATH fallback. A resolved dependency lacking the requested subdirectory
can fall through; an existing empty/invalid directory cannot. Unused requires
are not resolved eagerly.

Every top-level name is public unless it begins `_`; private names remain usable
throughout their owning module but not through another module's qualification.
Instances are global over the loaded graph, with no orphan rule. A duplicate
class/type instance across modules reports both module paths.

### 2.3 Implicit prelude and shadowing

`std/prelude` is implicitly available with exactly these unqualified names:

```text
Functor Monad Show Debug Eq Ord Foldable Traversable Ordering Option Result List Unit
map flatMap fold filter length append reverse head tail show debug
println print eprintln ioPure ioFail unit fst snd compare eq min max toFloat
```

Reexported types also expose `LT EQ GT None Some Err Ok Nil Cons`. Builtins
`Int Float String Bool GoError IO Ref Fiber Chan RecvChan SendChan Map Set Array` and tuple
types are independent of imports. All instances in loaded modules participate in resolution.

Module declarations and lexical bindings win over implicit names. Top-level
shadowing warns `shadows prelude name X`; it is legal. Explicitly import
`std/prelude` for `prelude.fmap`, `prelude.pure`, `prelude.bind`,
`prelude.foldr`, and `prelude.traverse`: those methods are **not** implicit.
Other library operations require their owning module, e.g. `string.trim` or
`list.sort`. §7 lists the distribution.

```go4t
package main
import "std/string"
import p "std/prelude"
func main() IO[Unit] {
  println(string.trim(p.fmap(Some(" hello "), string.trim) |> show))
}
```

## 3. Types

### 3.1 Kinds and builtins

`*` is the kind of values; `(*, ...) -> *` is a constructor kind.

| Types | Kind / meaning |
| --- | --- |
| `Int`, `Float`, `String`, `Bool`, `Unit` | `*`; int64, binary64, text, Boolean, unit |
| `GoError` | `*`; opaque Go `error` handle, also available as `prelude.GoError`; no constructors or class instances |
| `IO[A]` | `(*) -> *`; deferred action producing A or failing |
| `Fiber[A]` | `(*) -> *`; opaque concurrent result handle |
| `Ref[A]` | `(*) -> *`; opaque mutable cell accessed only through IO |
| `Chan[A]`, `RecvChan[A]`, `SendChan[A]` | `(*) -> *`; opaque bidirectional, receive-only and send-only channel handles |
| `Map[K, V]` | `(*, *) -> *`; persistent comparator-ordered map |
| `Set[A]` | `(*) -> *`; persistent comparator-ordered set |
| `Array[A]` | `(*) -> *`; immutable random-access sequence |
| `TupleN` / `(A, B, ...)` | N parameters, N ≥ 2 |

Opaque builtins have no source constructors to pattern-match. Data-type and
alias parameters have kind `*`; function/class parameters can have higher kinds.

### 3.2 Type expressions and parameters

```ebnf
Type    := Name ['[' Type {',' Type} ']']
         | 'func' '(' [Type {',' Type} [',']] ')' Type
         | '(' Type ')' | '(' Type ',' Type {',' Type} ')'
TParams := '[' TParam {',' TParam} ']'
TParam  := Ident ['[' '_' {',' '_'} ']'] [Name {'+' Name}]
DataParams := '[' Ident {',' Ident} ']'
```

A constructor may be partially applied where a higher kind is expected:
`Result[E]`, `Map[K]`, `state.State[S]`. Application is flat: use `Result[E, A]`,
not `Result[E][A]`. `F[_]` is unary; `F[_, _]` is binary.
`A Show + Ord` requires both classes. Class constraints must match the parameter's
kind; duplicate constraints on a parameter or in a `given` context are errors.
Declaring both subclass and superclass is legal; the most-derived dictionary
supplies both, regardless of their order. Unrelated constraints remain separate.
Explicit schemes preserve parameter and constraint order. A class's own parameter
uses superclass syntax, not a trailing constraint list.

Trailing commas are accepted in function-type parameter lists, but not in
type applications, tuple types, type-parameter lists, or higher-kind arities.

### 3.3 Algebraic data and aliases

```ebnf
TypeDecl  := 'type' Ident [DataParams] '=' Ctor {'|' Ctor}
             ['deriving' Name {',' Name}]
Ctor      := Ident ['(' [Type {',' Type} [',']] ')'
             | '{' Ident ':' Type {',' Ident ':' Type} [','] '}']
AliasDecl := 'alias' Ident [DataParams] '=' Type
```

Constructors are nullary (bare or `C()`), positional, or record-form. Records
also support positional construction and patterns. Recursive and parameterised
data are allowed; deriving rules are in §7.2. There are no zero-field record
constructors. Data and alias parameters are plain names, not constrained or
higher-kinded function parameters.

Aliases share the type namespace, introduce no constructors, and expand before
inference, field checking and instance resolution. They are transparent, not
Go-style distinct named types. Forward references/chains work; all aliases,
including unused ones, are validated, and cycles are errors. Every alias use must
supply exactly its parameters: aliases cannot themselves be partially applied,
although a fully applied alias may expand to a higher-kinded constructor. Expansion
uses declaration-scope parameter identity. Diagnostics and LSP hover preserve
source-written alias names and their arguments, including aliases nested in other
types: `alias Rates = Map[Currency, Float]` prints as `Rates` where used. This is
presentation provenance, not a new type identity. Type variables and independently
inferred types are never renamed merely because they equal an alias's body.

### 3.4 Functions and extern declarations

```ebnf
Func   := 'func' Ident [TParams] '(' [Param {',' Param} [',']] ')' Type Block
Param  := Ident Type
Extern := 'extern' 'func' Ident [TParams] '(' [Param {',' Param} [',']] ')' Type
          '=' String '.' Ident
```

Top-level functions have explicitly declared rank-1 type parameters and return
types. Their final expression must match the return type. Extern declarations
bind exported package-level Go functions; `check` validates only the go4t type.
`build`/`run` inspect foreign signatures and generate adapters (§8.3).

### 3.5 Classes and instances

```ebnf
Class      := 'class' Ident '[' TParam ']' [':' Name '[' Ident ']']
              '{' {Separator} {MethodSig {Separator}} '}'
MethodSig  := Ident [TParams] '(' [Param {',' Param} [',']] ')' Type
Instance   := 'instance' Name '[' Head ']' ['given' Constraint {',' Constraint}]
              '{' {Separator} {InstMethod {Separator}} '}'
InstMethod := Ident '(' [Ident {',' Ident} [',']] ')' Block
Head       := Name ['[' HeadArg {',' HeadArg} ']']
            | '(' Ident ',' Ident {',' Ident} ')'
HeadArg    := Ident
Constraint := Name '[' Ident ']'
```

Classes have one parameter, possibly higher-kinded, and at most one superclass
on that parameter. Methods may add their own parameters/constraints. Each method
must be implemented exactly once with matching value arity. A subclass instance
requires its superclass instance with compatible head and context.

The superclass argument must be the class parameter. Instance head arguments
must be distinct, and `given` arguments must name parameters of that head;
these name-dependent restrictions are enforced by the parser as well.

A head can be bare (`Functor[List]`), fully applied (`Show[List[A]]`) or partially
applied (`Monad[Result[E]]`), with remaining arity matching the class parameter's
kind. Normally head arguments are distinct fresh variables. Direct heads also
accept concrete nullary builtins `Int Float String Bool Unit`, e.g.
`Encode[Map[String, V]] given Encode[V]`. Arbitrary concrete user types and nested
specializations are not supported. Coherence is still one instance per
(class, outer type constructor), not general overlapping-instance matching.

After alias expansion a head must be a constructor with every written variable
occurring exactly once as a direct argument. Permutations are legal; erased,
repeated, nested or specialized alias-head arguments and function heads are not.
An instance through an alias is an instance of the expanded constructor.

Rigid type parameters satisfy a class if any declared constraint is that class
or a subclass. Concrete resolution selects the head's instance and recursively
resolves its `given` constraints. Missing instances are errors, including context
and available constraints where relevant. Resolution is global, not per call-site
import alias.

## 4. Expressions

### 4.1 Grammar and precedence

```ebnf
Expr       := InferExpr | Projection
InferExpr  := 'func' '(' [LParam {',' LParam} [',']] ')' '=>' Expr
            | 'do' '{' {Separator} {DoStmt Separator} Expr {Separator} '}'
            | 'match' Expr '{' Arms '}'
            | IfExpr | Binary
IfExpr     := 'if' Expr Block 'else' (Block | IfExpr)
LParam     := LowerIdent [Type] | Pattern
Binary     := Unary {BinOp Unary}                    // precedence below
BinOp      := '|>' | '||' | '&&' | '==' | '!=' | '<' | '<=' | '>' | '>='
            | '+' | '-' | '*' | '/' | '%'
Unary      := {'-' | '!'} Postfix
Postfix    := Primary {'(' [CallArg {',' CallArg} [',']] ')' | '.' Ident | '.' Int | RecordFields}
CallArg    := Expr | '_'
Primary    := Name | Int | Float | String | Interp | 'true' | 'false'
            | ConstructorName RecordFields | '(' InferExpr ')' | '(' InferExpr ',' InferExpr {',' InferExpr} ')'
            | '[' [InferExpr {',' InferExpr} [',']] ']' | Block | Section
Block      := '{' {Separator} {Stmt Separator} Expr {Separator} '}'
Stmt       := LetStmt | LocalFunc | Expr
LetStmt    := 'let' Pattern [':' Type] '=' Expr
LocalFunc  := 'func' Ident '(' [Param {',' Param} [',']] ')' Type Block
DoStmt     := Pattern '<-' Expr | LetStmt | LocalFunc | Expr
Section    := '(' '_' BinOp Unary ')' | '(' Unary BinOp '_' ')' | '(' '_' BinOp '_' ')'
            | '(' Projection ')'
Projection := '_' '.' (Ident | Int) {'.' (Ident | Int)}
RecordFields := '{' Ident ':' Expr {',' Ident ':' Expr} [','] '}'
```

Precedence, low to high: `|>`; `||`; `&&`; `== != < <= > >=`; `+ -`;
`* / %`; unary `- !`; postfix. Binary operators associate left. The grammar's
`Name` in expression position is resolved against imports versus record fields;
not every `x.f` is a qualified module name.

Unary operators may repeat. Sections have a whole unary operand on each
non-placeholder side; parenthesise a compound expression, e.g. `(_ + (a + b))`.
Pattern irrefutability in lets, lambdas and do binds is checked after parsing,
not a separate syntactic production. Only a bare lower-case lambda parameter
may carry a type annotation. Statement separators shown here are the
unambiguous form; the parser also accepts adjacent statements when their token
boundaries suffice (e.g. a local function followed by the block result).

### 4.2 Operators

| Operator | Operands and result |
| --- | --- |
| `+` | Int/Int → Int, Float/Float → Float, String/String → String |
| `- * /` | Int/Int → Int or Float/Float → Float |
| `%` | Int only |
| `== !=` | Same type with `Eq`; result Bool |
| `< <= > >=` | Same type with `Ord`; result Bool |
| `&&`, `||` | Bool, short-circuit left to right |
| Unary `-`, `!` | Int/Float negation; Bool negation |
| `x |> f(args)` | Insert x as the first argument, except placeholder lambdas (§4.5) |

Arithmetic is monomorphic, not a numeric class. There is no Int/Float coercion;
integer literals remain Int. Context may determine arithmetic operands as Float;
otherwise unresolved arithmetic operands default to Int. Float division is IEEE
754, including infinities/NaN on zero divisors. Int overflow wraps; Int division
truncates toward zero and zero division/remainder panics. `Eq`/`Ord` operators
use the corresponding class semantics (§10.1).

### 4.3 Blocks, lets and local recursion

A block's value is its final expression. `let x = e` or `let x: T = e` binds
for the remaining block and shadows an outer binding. Pattern lets must be
irrefutable, e.g. `let (a, b) = pair` or a single-constructor record pattern.

A bare-name lambda let, including an annotated lambda, binds its own name
monomorphically within its body. Free self references are recursive; parameters
and nested bindings can shadow them. After inference, unannotated lambda,
literal and constructor lets generalise variables not free in the environment,
including inferred independent class constraints. Calls, `do`, `match`, annotated
lets and pattern lets remain monomorphic. Non-lambda/pattern initializers are
nonrecursive: `let x = x + 1` refers to an outer x or fails. Polymorphic recursion
is not inferred.

Consecutive local `func` statements form a maximal mutually recursive group.
All signatures are available monomorphically to the group, which sees enclosing
bindings and earlier statements, but not later groups separated by other
statements. Local functions require parameter/return types and have no own type
parameters or extern form; enclosing type parameters are usable. Names are
published/generalised together after checking. Duplicate names in a group fail.
In `do`, lets and local functions are pure bindings and can capture earlier binds.
Neither declaration replaces the required final expression.

```go4t
package main
func main() IO[Unit] {
  func even(n Int) Bool { if n == 0 { true } else { odd(n - 1) } }
  func odd(n Int) Bool { if n == 0 { false } else { even(n - 1) } }
  let sumTo = func(n Int, acc Int) => if n == 0 { acc } else { sumTo(n - 1, acc + n) }
  println("${even(10)} ${sumTo(5, 0)}")
}
```

### 4.4 Lambdas and calls

`func(x, y) => e` infers monomorphic parameter types; ordinary parameters may
be annotated. Irrefutable pattern parameters are allowed but cannot carry a
type annotation: `func((x, y), _) => x + y`. Refutable parameters are errors.
A lambda passed directly to a call gets its parameter types from that argument
context before checking its body, enabling unannotated field selection there.
Calls require exact arity; function-valued locals, arguments and fields are callable.
There is no automatic currying.

### 4.5 Pipelines and sections

`x |> f` is `f(x)`; `x |> f(a, b)` is `f(x, a, b)`;
`x |> (func(y) => e)` applies the lambda. Direct `_` call arguments instead
produce a lambda over holes in left-to-right order: `f(_, x, _)` is a two-argument
function. A placeholder call on a pipe's right is already a lambda:
`xs |> string.join(",", _)` puts xs in the **second** argument.

Parenthesised operator sections permit whole operands `_`: `(_ + 1)`, `(2 * _)`,
`(_ < _)`, `(_ |> f)`. Two holes mean two parameters, not currying. Nested calls
have independent placeholder scopes. Fixed arguments are evaluated inside the
synthesized lambda body, on each application, not when constructing the section.

Projection sections `(_.field)` and `(_.0)` desugar to `func(x) => x.field`
and `func(x) => x.0`. Selectors may chain: `(_.a.b)` is one unary lambda,
not nested lambdas. A projection may occupy a whole call argument without
parentheses: `map(xs, _.qty)` is `map(xs, func(x) => x.qty)`, so
`xs |> map(_.qty)` works too. Each projection has its own parameter scope:
`map(_, _.qty)` has one outer hole and an independent projection lambda.
Bare projections also work as function return tails, annotated-let initializers,
`if` branches, `match` arms, lambda bodies, record field values and `do` tails
when the expected function type reaches them (§5.1). For example,
`func key() Option[func(Sale) String] { Some(_.product) }` uses its declared
return type to determine the projection's record type. An annotation such as
`let key: func(Sale) String = _.product` does the same.
Record types and tuple arity must be known from an expected function type or an
explicit lambda parameter annotation; projection sections use ordinary field-access
rules, including common fields across constructors (§4.6). Without that context,
`let key = Some(_.product)` fails with
`cannot infer record type of _.product; annotate`.

`f(_ + 1)`, `(_ + 1 + 2)`, `[_]`, `(_, 1)` and `(_)` are invalid; write
`f((_ + 1))`. Bare projections in list/tuple elements or operator operands remain
invalid, as are `f(_.f + 1)` and `(_.f + 1)`; use an explicit lambda for computations
beyond a selector chain. Parenthesising a projection permits its syntax in an
infer-only position, but does not supply a missing expected type.
Invalid placeholder placement retains the diagnostic
`_ is only valid as a placeholder in a call or operator section`.

```go4t
package main
import "std/string"
func main() IO[Unit] {
  [1, 2, 3] |> map((_ + 1)) |> map(show) |> string.join(", ", _) |> println
}
```

```go4t
package main
type Row = Row{qty: Int}
type Box = Box{row: Row}
func main() IO[Unit] {
  let rows = [Row{qty: 3}, Row{qty: 7}]
  let qty: func(Row) Int = (_.qty)
  do {
    println(show(rows |> map(_.qty)))
    println(show(map([(3, "a"), (7, "b")], (_.0))))
    println(show(map([Box{row: Row{qty: 11}}], _.row.qty)))
    println(show(qty(Row{qty: 9})))
  }
}
```

```go4t error
package main
type Row = Row{qty: Int}
func bad() Int { let qty = _.qty; qty(Row{qty: 3}) }
func main() IO[Unit] { ioPure(unit()) }
```

### 4.6 Conditionals, tuples, lists and records

`if c { a } else { b }` requires Bool c and equal branch types; `else` is mandatory.
`else if` chains are expressions. Tuples have at least two fields; `(a)` groups.
`t.0` is zero-based projection, requiring a known tuple type and an in-range index.
Lists `[a, b]` lower to `Cons(a, Cons(b, Nil))`, all elements one type; `[]` is Nil.

`C{f: e, g: e2}` supplies all constructor fields exactly once, in any order.
`x.f` reads a field; `x{f: e}` produces an immutable update. For multiple
constructors, **every** constructor must declare each selected field with the
same alias-expanded type, compared before instantiating the receiver parameters.
Positions may differ. Updates preserve the actual constructor and untouched
fields; receiver and replacements evaluate once, left to right in written order.
Missing fields are reported before incompatible field types, naming the first
constructor lacking one. Unknown receiver type requires annotation:
`cannot infer record type of x.f; annotate`.

### 4.7 Pattern matching

```ebnf
Arms    := {Separator} ['|'] {Separator} Arm {ArmSep Arm} [ArmSep]
ArmSep  := (';' | '|') {';' | '|'}
Arm     := Pattern ['if' Expr] '=>' Expr
Pattern := '_' | Int | Float | String | 'true' | 'false'
         | LowerIdent | LowerIdent '@' Pattern
         | ConstructorName ['(' [Pattern {',' Pattern} [',']] ')']
         | ConstructorName RecordPatternFields
         | '(' Pattern ',' Pattern {',' Pattern} ')' | '(' Pattern ')'
         | '[' [Pattern {',' Pattern} [',' ListRest] [',']] ']'
ConstructorName := [Ident '.'] UpperIdent
RecordPatternFields := '{' ('..' [','] | Ident ':' Pattern {',' Ident ':' Pattern} [',' '..'] [',']) '}'
ListRest := '..' [LowerIdent | '_']
```

Constructors may be qualified/private within their owning module. Arms run in
order: the first matching pattern whose guard is true yields the value. All arms
have one type. Variables bind once per pattern; guards see them. Record patterns
must name all fields or use `..`; positional patterns also work for records.

Record patterns require at least one field or a rest marker: `C{..}` is valid,
but `C{}` and `C{, ..}` are not. A nullary positional pattern may be `C` or
`C()`. Arm separators may repeat or trail the last arm; tuple patterns do not
allow a trailing comma.

List patterns desugar in the parser: `[]` is `Nil`, `[p1, p2, p3]` is
`Cons(p1, Cons(p2, Cons(p3, Nil)))`, and `[p1, p2, ..rest]` is
`Cons(p1, Cons(p2, rest))`. Fixed-length patterns match exactly that many
elements; a rest binds the remaining list, including an empty tail, without
copying it. `..` or `.._` ignores the tail. Rest requires at least one preceding
element and must be last; a trailing comma is allowed. Elements may themselves
be any patterns. Formatting retains list syntax (canonicalizing `.._` to `..`)
and leaves explicit `Nil`/`Cons` patterns in constructor syntax.

List patterns follow the existing binding rules: `let [a, b] = xs`,
`func([a, b]) => a + b`, and `[a, ..rest] <- action` parse but fail the
irrefutability requirement, even if the supplied list literal has the right
length. Use `match` to handle both matching and nonmatching lengths.

Matches must be exhaustive, including nested constructors/tuples. Guarded arms
contribute no coverage. Bool's `true` and `false` cover the finite domain;
Int/Float/String literals require a catch-all. Missing-pattern diagnostics list
at most five witnesses. Irrefutable binding patterns cannot select just one
constructor of a sum or a literal.

### 4.8 Interpolation and do

Interpolation requires `Show`, not `Debug`; write `${debug(x)}` for structural
output. For money, prefer `float.showFloatFixed(value, decimals)` from `std/float`
to the shortest Float `show` form; whole floats display with `.0` (for example `1.0`).
A `do` block uses one Monad constructor throughout. `<-` extracts a value,
a bare action sequences and discards it, and the final expression is an action.
Irrefutable binds, lets and local functions are supported. Desugaring is nested
`bind`; it is not restricted to IO. Option stops on None; Result stops on Err;
List enumerates combinations in order; State threads its state. Use `ioPure`,
`Some`, `Ok`, or qualified `prelude.pure` for the final result as appropriate.
When the final expression has a resolved expected type `IO[T]`, a pure value of
type `T` is implicitly lifted with the prelude's `ioPure` (§5.1), including
`unit()` for `IO[Unit]`. This is expected-type checking, not a change to `do`:
other Monads still require their own action constructors, and `<-` remains
restricted to `do`.

## 5. Type system and diagnostics

### 5.1 Inference

Inference is Hindley–Milner with rank-1 top-level/eligible-let polymorphism,
higher kinds, flat partial constructor unification, rigid declared parameters,
and post-inference type-class dictionaries. Parameters are never generalised.
For example `F[a] ~ Result[E, A]` can solve `F := Result[E]`, `a := A`.

Expected types flow into function body tails from declared return types and
into `let x: T = e` from `T`; through `if` branches and `match` arms (a match
without an expectation uses its first arm's inferred type); into constructor/call
arguments from instantiated parameter types after unifying the callee result;
into record field values from field types; into lambda bodies and unannotated
parameters from expected function results/parameters; and into a `do` block's
final expression from the block's expectation. This bounded bidirectional checking
resolves projections and unannotated lambda field access without searching for
record types or looking ahead to later statements. Other positions, including
list/tuple elements and operator operands, remain infer-only. Numeric literals
are unaffected: an integer literal is always Int, never contextually Float.

**Implicit IO lift.** In function body tails (including lambda bodies), `if`
branches, `match` arms, `do` final expressions, and annotated let right-hand
sides, a resolved expected type `IO[T]` permits a pure expression `e` of inferred
type `U` to be checked as `ioPure(e)`: `U` must not unify with `IO[_]`, and must
unify with `T`. The `IO` constructor must already be resolved; an unknown Monad
head does not select IO. Calls still provide contextual types inside the pure
value, but argument positions, record fields, operator operands and list/tuple
elements do **not** themselves lift.

An unresolved type variable is not lifted; write `ioPure` explicitly when the
pure result type is not known. An existing IO value, including `IO[IO[T]]`, is
never lifted or flattened; use an explicit join to flatten nested IO. A mismatch
reports the original types, for example `expected IO[String], got Int`, without
mentioning an attempted lift. The synthetic call is bound directly to the
prelude's `ioPure`, even when that name is shadowed. Pure and IO arms may be mixed;
only pure arms lift, and a wholly pure `if`/`match` can be lifted as a whole.
There is no new syntax: formatting preserves the source, hover shows the lifted
`IO[T]` type, and evaluation is as strict as an explicit `ioPure(e)`.

Unresolved kind-`*` variables with no class constraint default to Unit with a
warning, e.g. `type of None defaulted to Unit; add an annotation or use the value`.
Unresolved higher-kinded or class-constrained variables fail. Arithmetic's Int
default is separate; no numeric-literal polymorphism is introduced.

```go4t error
package main
func main() IO[Unit] { println(show(1 + 1.5)) }
```

### 5.2 Diagnostics

Errors: `file:line:col: message`. Warnings:
`file:line:col: warning: message`. Positions are one-based rune columns and span
ends are exclusive. CLI diagnostics print the source line and a `^~~~` underline;
multiline spans clamp to the first line, missing spans use one caret, and unreadable
sources retain the positioned message without an excerpt. Tabs, CRLF and EOF are
handled. Expected types are contextual: `type mismatch: expected T, got U`.

Type printing preserves source-written aliases (§3.3). If the complete printed
type exceeds 160 Unicode characters, subtrees at depth 3 or greater are replaced
by `…` (the root is depth 0; application arguments and function parameters/results
increase depth). Exactly 160 characters are not elided. Shallow names and lists
are not cut mid-token, so 160 is an elision trigger, not a hard output cap.
`check`, `build` and `run` accept `--full-types` to disable this elision without
expanding aliases. LSP hover uses the same default printer.

Diagnostics and hover use source-level names, never compiler-generated identifiers.
Synthesized placeholder and section parameters display as `_`, with projection
paths preserved (`_.product`, `_.0`), and never produce unused-variable warnings.
Data types, aliases, classes and constructors use the import alias of the file
being diagnosed or hovered (`http.Response`, or `web.Response` after
`import web "std/http"`); names from that file's own module and implicit prelude
re-exports remain bare. With multiple aliases for one module, the first import
is used; types from a transitive module not directly imported use its package
name. Qualification is identical with `--full-types`.

Missing-pattern witnesses for `List[_]` always use list syntax, regardless of
whether the source wrote list or `Nil`/`Cons` patterns, including lists nested
inside other types. `[]` denotes an empty list, `[_]` exactly one element, and
`[_, _, ..]` at least two elements. A trailing `..` denotes an unconstrained
tail; constrained elements retain their own witness, e.g. `[true, ..]`.
An entirely unconstrained column remains `_`. This representation does not
change coverage or the five-witness limit.

Examples of stable message forms:

```text
non-exhaustive match; missing: None, Some(_)
no instance Ord[A] (type parameter A is constrained only by Eq + Show)
no instance Show[Currency]; hint: add 'deriving Show' or write 'instance Show[Currency]'
no instance Show[Hidden]; hint: write 'instance Show[Hidden]' (or 'deriving Debug' for a structural form)
deriving Show is only supported for enums and single-field newtypes; derive Debug and write instance Show[T] for a display form
x is unused; prefix with _ to silence
shadows prelude name map
```

User-data missing Debug/Eq/Ord hints suggest that deriving class or a handwritten
instance. Show hints suggest `deriving Show` for enums (all constructors
nullary) and single-field newtypes; other data types suggest a handwritten Show
instance or `deriving Debug` for structural output. Builtins, functions, rigid parameters and other classes do
not receive deriving hints. Recursive instance errors preserve the underlying
hint and requiring context.

Unused lets, local/lambda parameters, pattern variables and named do binds warn;
`_`-prefixed bindings and top-level/instance-method parameters are exempt. Guards
and closure captures count as uses; shadowed bindings have independent use records.
Warnings do not make a successful check fail. The checker reports its first type
error; the editor discards its old successful index on failure (§8.5).

## 6. Evaluation, IO and concurrency

### 6.1 Strictness and purity

Evaluation is strict, with arguments left to right. Data is immutable; constructors
allocate, updates copy. A discarded ordinary expression still evaluates. Functions,
closures and dictionaries are first-class. There is no source mutable reference,
null, exception syntax or implicit side-effect statement.

`IO[A]` is an action description. Constructing standard IO does not execute the
action; running main's action does. **Arguments are still strict**: `ioPure(1 / 0)`
panics before returning an action. A trusted extern can violate purity if its Go
implementation does effects under a non-IO signature; the checker cannot inspect
foreign semantics.

### 6.2 Failure and recovery

`ioFail(msg)` is a deferred `Failure(msg)`. Bind stops at the first failed action.
`std/io` defines `IoError` with constructors `NotFound(String)`,
`PermissionDenied(String)`, `AlreadyExists(String)`, `Timeout`, `Cancelled`,
`Panic(String)`, `Network(String)`, and `Failure(String)`. It derives Eq and Debug;
its hand-written Show instance returns `io.ioErrorMessage(e)`, not the constructor.
Timeout's message is `context deadline exceeded`; Cancelled's is `context canceled`.
Other messages are their String payload, including empty text.

`io.tryIO(action)` executes lazily and returns `IO[Result[io.IoError, A]]`:
success becomes Ok; IO failures become Err with their typed error; recovered
panics become `Err(io.Panic(fmt.Sprint(value)))` **without** an added `panic:` prefix.
Capture includes bind continuations while running the action, not earlier
argument construction. `io.ioRaise(e)` raises an existing IoError without losing
its constructor; `io.ioFailWith(e)` and `result.liftResult(Err(e))` instead raise
`Failure(show(e))`.

`io.tryIOCause(action)` has the same execution and recovery boundaries, returning
`IO[Result[io.IoFailure, A]]`, where
`IoFailure = IoFailure{error: IoError, cause: Option[GoError]}`. Foreign failures
retain the original Go error as `Some`; failures constructed by `ioFail` or
`ioRaise` and recovered panics have `None`. `io.cause(f)` returns this optional
handle, and `io.goErrorMessage(e)` returns its original Go message, which may
differ from the classified display message. `io.ioRaiseCause(f)` preserves the
supplied IoError and cause; nullary Timeout/Cancelled use `ioErrorMessage`'s
canonical messages. `Debug[IoFailure]` renders cause presence as `None` or
`Some(<GoError>)` without inspecting the handle. Existing `tryIO` results and
`Err(io.NotFound(_))` matches are unchanged; use `tryIOCause` to retain a cause.

Every runtime IO failure is `*rt.Failure{Kind, Message, Cause}`, implementing Go `error`.
Go errors from filesystem, stream, time/random and natural-ABI extern boundaries
are classified in this order (including wrapped errors): not-exist → NotFound,
permission → PermissionDenied, already-exists → AlreadyExists,
`context.DeadlineExceeded` → Timeout, `context.Canceled` → Cancelled,
`net.Error` with `Timeout() == true` → Timeout, other `net.Error` (including URL
errors) → Network, otherwise Failure. Existing typed failures retain their
identity, kind, message and cause, including when wrapped. Every newly classified
failure retains the original Go error in `Cause`. `Failure.Unwrap()` returns it;
`Failure.Is(target)` delegates to `errors.Is(Cause, target)` when non-nil and
otherwise uses the Kind-to-OS/context-sentinel table. Timeout/Cancelled have
canonical messages because their ADT constructors carry no payload. Generated
code registers the `rt.Shapes.IoError` constructor bridge and
`rt.Shapes.IoErrorOf` destructor bridge alongside Result. IoFailure uses the
existing foreign-record constructor/field bridge, not a second record ABI.

`rt.RunMain` reports ordinary IO failures as `error: <message>` and exits 1;
a panic while running its action prints `error: panic: <message>` and exits 2.
Success exits normally (0). A panic while constructing main's action is outside
that recovery and follows Go's panic/stack-trace path. `io.exit(code)` terminates
the process and is not recoverable. Installed `go4t run` preserves the child exit
status; an outer `go run ./cmd/go4t ...` adds Go's `exit status N` and itself
collapses nonzero exits to 1.

```go4t
package main
import "std/io"
func main() IO[Unit] {
  do {
    answer <- io.tryIO(ioFail("unavailable"))
    let typed: Result[io.IoError, Int] = answer
    println(show(typed))
  }
}
```

### 6.3 Concurrency

An effect executes with an implicit cancellation/deadline environment: in Go,
`rt.IO` is `func(ctx context.Context) (any, error)`. This is not a source-language
value and does not change purity. `RunMain` supplies a background context cancelled
by SIGINT/SIGTERM; cancellation reports `error: cancelled` and exits 1. Embedders
run the action returned by `Main() rt.IO` with their own context. Bind passes the
same context through both actions and checks it before starting the continuation.
A `do` block whose Monad is the standard `Monad[IO]` instance (`std/io`) is
lowered to one deferred action whose steps run in sequence through `rt.Run`,
which performs the same checks as Bind (cancellation before each step, error
classification, cancellation after it); a self-call in tail position of such a
function becomes a cancellation-checked loop. Observable behaviour is that of
the nested bind: pure lets are evaluated after the preceding effect, a failed
step skips the rest, and non-tail recursion keeps ordinary calls. Other Monads
and unresolved dictionaries keep the nested bind.
Sleep and HTTP stop when cancelled; stdin actions stop waiting immediately.
An already-blocked stdin read may retain one worker until input/EOF arrives;
stdin buffer access is serialized and that cancelled read can consume the input.
File operations check cancellation before starting but do not interrupt a syscall.

Import `std/conc`. `fork` starts an action on a goroutine **when run**, returning
an opaque Fiber with a child context. `await` returns its cached outcome; repeated
or concurrent awaits do not rerun it. `cancel(fiber)` requests cancellation and
returns immediately, even for an uncooperative extern. Awaiting an unfinished,
cancelled fiber fails with cancellation; a completed fiber retains its outcome.
Parent cancellation reaches child fibers, while cancelling one child does not
cancel its parent or siblings. An await also stops when its own caller is cancelled.

`parMapIO` starts mapper/actions concurrently, cancels siblings on the first
observed failure, joins them and returns input-ordered results on success.
`parBoth` joins both and returns argument-ordered values; failure alone does not
cancel its sibling. Both retain the first **observed** failure; concurrent ties
are unspecified. A foreign action that ignores cancellation can delay a normal
join. Parent cancellation ends either wait after at most a 100 ms cleanup grace.

`race` returns the first completion, failure included, not the first success.
It cancels the loser and joins it before returning, with a 100 ms cleanup grace.
`withTimeout(millis, action)` fails with typed `Timeout` when its deadline expires
(`errors.Is(err, context.DeadlineExceeded)` remains true in Go). `timeout` is the
Option form of the same operation: Some on timely success, None on its own expiry.
Both propagate timely action failures and enclosing-context cancellation/deadlines.
Nonpositive budgets do not start the action; `withTimeout` fails immediately and
`timeout` returns None. Positive duration overflow fails rather than wrapping.
Timers start before launch; completion/expiry ties are unspecified.
Expired actions are cancelled and joined, bounded by the deadline plus 100 ms.
Cooperative actions therefore cannot continue a bind after their cancelled sleep.
An extern that ignores its context can outlive that bound and still perform effects.

Worker panics, including mapper construction, become typed `Panic(fmt.Sprint(value))`
IO failures without a `panic:` prefix; awaiting one uses exit 1 unless handled. There is no
global join; unawaited fibers do not keep a completed main alive. §10 gives all
edge cases.

### 6.4 HTTP

Import `std/http` for deferred client requests and a cancellable server. `Request`
has `method`, `url`, `path`, `query`, `headers`, `body`; `Response` has `status`,
`headers`, `body`. Query and header collections are `List[(String, String)]`;
request/response bodies are complete Strings, not streams. Non-2xx responses
are normal values; transport failures use Network, Timeout or Cancelled.

`request(r)`, `get(url)` and `post(url, contentType, body)` have a 30 s timeout
covering connection, redirects and reading the full body. `requestWithTimeout(r,
millis)` replaces that budget; an enclosing deadline/cancellation still wins.
Clients use the absolute `r.url`; a nonempty `r.path` replaces its decoded path,
and `r.query` pairs append to the URL's existing query. Repeated pairs are retained.
An empty method follows Go's GET default. HTTP redirects and response compression
follow `net/http` defaults. No retry policy is added by the library.

`response(status, body)` constructs a response with no headers.
`jsonResponse[A json.Encode](status, a)` uses `std/json` to encode the body and
sets `Content-Type: application/json`. Header lookup is case-insensitive and
returns the first matching value or None: `requestHeader(r, name)` and
`responseHeader(r, name)`. Client Host headers set the request's host override;
server requests expose Host in their header list.

`serve(port, handler)` listens on all local interfaces and blocks until its IO
context is cancelled. Each request runs in its own `net/http` goroutine with
the request context, descended from the server context. Incoming `url` is the
request URL (normally relative), `path` is decoded, and `query` contains decoded
key/value pairs. Header/query keys from Go maps are sorted, repeated values keep
their order. Handler construction/action panics use the existing worker
panic-to-Panic failure boundary. Handler failure returns 500 with the exact
message body and writes `http handler: <message>` plus a newline to stderr.
Cancelled handler failures are silent when the request context is already done (client disconnect or server shutdown), and normal server shutdown logs nothing.
Cancellation closes the listener and gives active handlers a 5 s graceful
shutdown window, then force-closes connections; serve ends with Cancelled
(or Timeout for a deadline). Noncooperative foreign handlers can outlive it.

The ABI does not depend on generated record layouts: rt returns
`(status, headers, body)` through `rt.Shapes.Tuple3/Untuple3`, and `std/http`
constructs Response. The server passes six individual Request fields to a
go4t adapter which constructs Request and returns the handler's Response as a
triple. Codegen emits `rt_tuple3`/`rt_untuple3` with source directives like Tuple2.

### 6.5 Laws and proofs

A `law` declares an equation universally quantified over its typed, monomorphic
parameters. A `proof` of the same name is required, with the same number of
parameters; its untyped names rebind those parameters positionally. Laws are
not callable functions and have no generated Go representation.

```ebnf
LawDecl   := 'law' Ident '(' [Param {',' Param} [',']] ')' '{' Expr '==' Expr {Separator} '}'
ProofDecl := 'proof' Ident '(' [Ident {',' Ident} [',']] ')' TacticBlock
TacticBlock := '{' {Separator} Tactic {Separator} '}'
Tactic    := 'refl'
           | 'match' Ident '{' {Separator} ['|'] {Separator} [TacticArm {ArmSep TacticArm} [ArmSep]] '}'
           | 'if' Expr TacticBlock 'else' TacticBlock
           | 'rewrite' Name '(' [Expr {',' Expr} [',']] ')' TacticBlock
           | ? sample identifier ? '(' Int ')'
           | ? induction identifier ? Ident ['{' {Separator} ['|'] {Separator} [TacticArm {ArmSep TacticArm} [ArmSep]] '}']
TacticArm := TacticPattern '=>' Tactic
TacticPattern := '_' | LowerIdent
              | ConstructorName ['(' [TacticPattern {',' TacticPattern} [',']] ')']
              | ConstructorName TacticRecordFields
              | '(' TacticPattern ',' TacticPattern {',' TacticPattern} ')'
              | '(' TacticPattern ')'
TacticRecordFields := '{' ('..' [','] | Ident ':' TacticPattern {',' Ident ':' TacticPattern} [',' '..'] [',']) '}'
```

The lexical classes `? sample identifier ?` and `? induction identifier ?`
match exactly the contextual identifiers `sample` and `induction`, respectively.
Unlike `refl`, these spellings are not reserved lexer keywords.

The outermost expression in a law body must be `==`; parenthesize a side
containing lower-precedence Boolean operators. This `==` is an equation
delimiter, not a call to `Eq`: both sides must have the same type but need no
`Eq` instance. Equality means **definitional equality** of normalized terms,
up to renaming bound variables (alpha equivalence), not equality according to a
user-defined `Eq` instance. There is no eta-equivalence rule.

| Tactic | Meaning |
| --- | --- |
| `refl` | Normalize both sides under the current assumptions and require identical normal forms. |
| `match x { C(y) => refl; D => refl }` | Split a proof variable into constructor cases, binding fields. Cases must be exhaustive; nested constructor, record and tuple patterns, variables and `_` are supported, but not literals, guards, as-patterns or list-pattern sugar. Use `Nil`/`Cons` for lists. |
| `if condition { refl } else { refl }` | Split on a Bool expression, assuming it true or false in the corresponding branch. |
| `rewrite name(args) { refl }` | Instantiate a law and replace all occurrences of its left side with its right side in both goal sides, then continue with the nested tactic. A left side that instantiates to a lone free variable is rejected. |
| `sample(N)` | Test the equation with `go4t test`; do not prove it. This tactic occupies the whole proof body. `N` is an Int literal in 1..100000. All quantified types require usable bounded generators, and the result requires `Eq`; direct Float results compare bits. Equations whose dependencies are all law-eligible are rejected with an instruction to prove them instead. |
| `induction x { C(y) => tactic; D => tactic }` | Split a quantified datatype variable into exhaustive constructor cases. Before each arm, rewrite with the enclosing law instantiated at every field of the same datatype, leaving other arguments unchanged. Existing structural-descent and argument-order checks apply. |
| `induction x` | Perform constructor induction with `refl` in every arm after the automatic induction-hypothesis rewrites. |

A sampled law is tested, not proved, and cannot be used by `rewrite`. `check`
validates sampled laws without normalization and emits one positioned
tested-not-proved notice per accepted sampled law.

`go4t test` executes ordinary test functions and sampled laws, including imported
sampled laws, and fails on the first counterexample within each sampled law; the
diagnostic includes the random seed, sample index, and binding displays.

Normalization unfolds pure functions, beta-reduces calls, substitutes lets,
selects decidable match branches, and reduces records and resolved type-class
methods. An unknown earlier match arm blocks selection of later arms. A
recursive function (or instance method) certified by the termination check is
unfolded only when the argument at its decreasing position is a constructor or
tuple; otherwise the call stays neutral, so both sides of a goal stop at the
same depth. Projections of record updates reduce even when the base is
symbolic: `(r{f: v}).f` is `v` and `(r{f: v}).g` is `r.g`, for single-
constructor records. For single-constructor records, nested updates over a
symbolic base merge into one update with later writes winning and fields ordered
by declaration index, so `r{f: a}{f: b}` and `r{f: b}`, and `r{g: c, f: b}` and
`r{f: b, g: c}`, share normal forms; multi-constructor record updates are not
merged. Inside an arm of a stuck `match s`, `s` is assumed equal
to the arm's pattern, and inside a stuck `if c`, `c` is assumed `true` or
`false`, so repeated eliminations of the same scrutinee collapse. Int
`+`, `-`, `*` and unary negation use polynomial normal forms modulo 2^64,
matching wrapping runtime arithmetic. Comparisons and `/`/`%` are not an
arithmetic decision procedure, but prelude Int and String comparisons are
oriented canonically (`a > b` is `b < a`, `a >= b` is `b <= a`) and `==`
operands are ordered, and `x == x` / `x < x` decide for alpha-equal
operands. `if` assumptions compare normalized operands, so `now >= at + t`,
`at + t <= now` and `now >= t + at` describe the same guard.

`Map` values are modelled symbolically. A map whose keys are all constants
(Int, String, Bool or tuples of those) normalizes to a sorted literal; an
insertion with a symbolic key stays an `insert` node, with later insertions of
an identical key shadowing earlier ones. `mapLookup`, `mapMember`, `mapDelete`,
`mapSize` and `mapFromList` reduce through such nodes, producing key-equality
conditions of the form `k2 == k` that an `if` tactic can assume:
`mapLookup(mapInsert(m, k, v), k2)` becomes `if k2 == k { Some(v) } else
{ mapLookup(m, k2) }`, and `mapLookup(mapDelete(m, k), k2)` becomes
`if k2 == k { None } else { mapLookup(m, k2) }` even when `m` is symbolic.
Distinct symbolic insertions are never reordered, so equality of two symbolic
maps is definitional only. `mapToList`, `mapKeys`, `mapValues`, `mapFold`,
`mapMap` and `mapUnion` reduce only between literal maps. These rules apply
only when the comparator is the `compare` method of a prelude Int/String/Bool
instance, a derived `Ord`, or tuples and compositions of those, because they
identify `compare(a, b) == EQ` with `a == b`; custom comparators and
hand-written `Ord` instances leave every map operation neutral (never tainted).

When `refl` fails, the diagnostic prints both normal forms, `stuck on:` the
outermost neutral scrutinee or neutral recursive argument of each side (one
line when both agree), and `differs at:` the first pair of subterms that are
not alpha-equal.

Only the law-reachable fragment must terminate; ordinary programs retain general
recursion. Recursive calls must leave all preceding arguments unchanged and
strictly decrease the first changed argument to a pattern-bound structural
subterm. Self-`rewrite` uses the same rule for the induction hypothesis: the
first changed argument must come from an enclosing `match`. Cycles among
different laws are rejected, so mutual rewrites cannot certify one another.
A finite normalization fuel budget also bounds unfolding of open terms;
exhaustion is an incomplete-normalization error, never a successful proof.

Induction follows the recursion shape of the unfolded function, not the data
type. Accumulator-based definitions such as `std/list` `length` (a `fold` whose
accumulator advances at each `Cons`) do not reduce `length(Cons(x, r))` to
`1 + length(r)`, so an induction hypothesis stated with the accumulator at
its initial value does not apply. State the law over the fold's accumulator
(`fold(xs, n, f) == n + ...`) and rewrite with that, or prove over a directly
structurally recursive function.

Law dependencies are checked transitively for unsafe operations: IO, untrusted
externs and potentially panicking arithmetic are tainted. Integer `/` and `%`
are eligible only with a nonzero literal divisor (including a unary negative
literal); a variable divisor is tainted. Host resource exhaustion, such as
allocation failure or stack exhaustion, is outside this analysis.

The trusted extern whitelist is the ten prelude primitives (`EqInt`, `LtInt`,
`EqString`, `LtString`, `EqBool`, `ShowInt`, `ShowBool`, `ShowString`,
`DebugString`, `UnitValue`) plus the 32 `std/string` primitives. Trust is tied to
the owning standard-library declaration, not merely a matching foreign symbol.
String primitives fold only when their arguments are constant; they do not
provide symbolic string reasoning. The 13 `std/map` externs are modelled by the
normalizer as described above and are therefore eligible. `std/set` and
`std/array` externs remain tainted. Callback and dictionary implementations must themselves be
eligible. `Float` is forbidden in law parameters, side types and subterms,
including nested data fields: NaN prevents reflexivity of runtime equality.

Law-related type diagnostics include the following message templates (ordinary
type mismatch and name-resolution diagnostics still apply):

```text
law %s has no proof
proof %s has no law %s
duplicate law %s
duplicate proof %s
proof %s: want %d parameters, got %d
duplicate parameter %s
laws do not support Float
match variable %s is not a proof variable
cannot split %s
non-exhaustive match; missing: %s
unknown law %s
arity mismatch: want %d arguments, got %d
tactic match does not support list patterns
tactic match does not support literal or as-patterns
```

Proof verification additionally rejects non-decreasing recursion, cyclic law
dependencies, tainted dependencies, exhausted fuel and unequal residual normal
forms. A type-correct `refl` is not by itself evidence that a law is true:
run `go4t check` to verify it. See [tutorial §13](docs/tutorial.md#13-laws)
for complete programs.

## 7. Prelude, deriving and standard modules

### 7.1 Classes and module distribution

These are class signatures, not standalone programs:

```text
class Functor[F[_]] { fmap[A, B](fa F[A], f func(A) B) F[B] }
class Monad[M[_]] : Functor[M] { pure[A](a A) M[A]; bind[A, B](ma M[A], f func(A) M[B]) M[B] }
class Foldable[F[_]] { foldr[A, B](fa F[A], init B, f func(A, B) B) B }
class Traversable[F[_]] : Functor[F] { traverse[M[_] Monad, A, B](fa F[A], f func(A) M[B]) M[F[B]] }
class Show[A] { show(a A) String }
class Debug[A] { debug(a A) String }
class Eq[A] { eq(a A, b A) Bool }
class Ord[A] : Eq[A] { compare(a A, b A) Ordering }
```

| Module | Contents (only §2.3's fixed names are implicit) |
| --- | --- |
| `std/array` | Persistent arrays and their loaded instances |
| `std/chan` | Opaque directional channels: make/send/recv/close, explicit asRecv/asSend upcasts |
| `std/conc` | fork/await, parallel map/pair, race/timeout |
| `std/float` | Float instances, conversions/math/formatting; minInt/maxInt/abs |
| `std/http` | Request/Response, request/get/post/requestWithTimeout, response/jsonResponse, requestHeader/responseHeader, serve/serveOn/serveOnReady |
| `std/io` | IO instances, IoError (Eq/Debug/Show), IoFailure (Debug), ioRaise/ioErrorMessage, tryIOCause/cause/goErrorMessage/ioRaiseCause, console, arguments/environment, files, streaming foldLines/forEachLine, clocks, randomInt, randomFloat, sleep, tryIO |
| `std/json` | Json, Encode/Decode and codecs; not implicitly exported |
| `std/lazy` | Lazy, delay/force; explicitly **nonmemoising** |
| `std/list` | List/instances; folds, traversal, searching, slicing, sorting, `catOptions`, `mapMaybe`, accumulator helpers |
| `std/map` | Persistent maps and their loaded instances |
| `std/mutmap` | IO-only opaque ordered MutMap handles: new/insert/lookup/delete/size, ascending toList snapshots, freeze/thaw; no standard class instances |
| `std/prelude` | Classes above; Ordering/Option and primitive/pair/Option instances; opaque GoError handle; core combinators including qualified `prelude.when` / `prelude.unless` for conditional Monad actions |
| `std/ref` | IO-only opaque Ref cells: new/get/set, atomic modify/update |
| `std/regex` | Go-regexp matching, captures, replacement and splitting |
| `std/result` | Result/instances; Result and Option helpers; liftResult |
| `std/set` | Persistent sets and their loaded instances |
| `std/state` | State and Functor/Monad instances, state operations |
| `std/string` | Rune-oriented strings, Unicode classification, parsing/padding |
| `std/dyn` | Value, an Eq/Debug sum for foreign `any` and variadic arguments; not implicitly exported |

Functor/Monad are provided for Option, List, Result[E], IO and State[S];
Array and Map[K] have Functor. Option/List have Foldable/Traversable. Show/Eq/Ord
cover Int, Float, String, Bool, Ordering, Option, List, Result and pairs with
component constraints. Unit has Eq and Debug, **not Show or Ord**. Debug covers
Int/Float/String/Bool/Unit, Ordering/Option/List/Result and pairs. Map/Set/Array
have Show/Eq/Debug, not automatic Ord/Foldable; Ref, Fiber and channel handles have
no standard class instances. The complete explicit and derived inventory is in `docs/stdlib.md`.

Float Show and Debug share the shortest `g` formatter: `show(1.0)` and
`debug(1.0)` are both `"1.0"`. Finite forms without a decimal point or exponent
gain `.0`, preserving the distinction from Int. Exponents and NaN/infinities
are retained unchanged (`1e+21`, `NaN`, `+Inf`, `-Inf`).

### 7.2 Deriving rules

Supported classes: **Eq, Ord, Debug, Show, Functor, Foldable, Traversable, Encode,
Decode** (Show only for enums and single-field newtypes; the last two qualified through a `std/json` import). Derivation
expands to ordinary checked instances; duplicate deriving names and explicit
instance conflicts are errors.

**Show is derivable for enums and single-field newtypes.** An enum (all
constructors nullary) renders each constructor as its unqualified source name:
`show(C)` is `"C"`. A newtype (exactly one constructor with exactly one field,
positional or named) delegates to the field's Show: `show(C(x)) = show(x)`.
Other data types require an explicit user-facing display instance; Debug remains
structural output. The exact rejection for other shapes is:
`deriving Show is only supported for enums and single-field newtypes; derive Debug and write instance Show[T] for a display form`.

| Class | Generated contract |
| --- | --- |
| Eq | Equal constructor and corresponding equal fields; function fields unsupported |
| Ord | Constructor declaration order, then lexicographic fields; requires Eq instance; function fields unsupported |
| Debug | `C`, `C(f1, f2)`, `C{a: f1, b: f2}` via each field's Debug; strings quoted with Go escapes |
| Show | No Debug requirement. Enums render source constructor names without module qualification; single-field newtypes delegate to the field's Show |
| Functor | Map the last type parameter; preserve unmapped fields and shape |
| Foldable | Right-associated fold over last-parameter occurrences in declaration/tuple order; ignore unmapped fields |
| Traversable | Map with Monad effects in that order, preserving shape; requires Functor, does not derive it automatically |
| Encode / Decode | JSON representation and path-aware errors below |

Eq/Ord/Debug/Show/Encode/Decode constrain **every** data parameter with that class,
including phantom parameters. Higher-kinded derivations require at least one
parameter and fix all but the last without constraining them. The mapped parameter
may occur directly, inside tuples of any arity, recursively, or as the last
argument of a type with the corresponding instance. Occurrence inside a function
or a non-last argument is rejected, naming constructor and field (one-based for
positional fields). An unmapped function field is legal for these higher-kinded
classes. Nested instances are resolved normally; traversal's failure policy is
that of its Monad. List derives Eq/Ord and has an explicit Debug instance:
its structural form is the list literal, `[a, b]` or `[]`, with each element
rendered via Debug and separated by `, `. Thus `debug([Some(1.5), None])`
is `"[Some(1.5), None]"`; nested lists and list fields use the same form.
Lists built with Cons/Nil render identically to equivalent list literals.
Show also uses brackets but renders each element via Show instead of Debug.
Result derives Eq/Ord/Debug and has an explicit Show (`Err(e)` / `Ok(a)`);
Option also retains its handwritten display instance. Ordering derives Show,
preserving the display strings `"LT"`, `"EQ"` and `"GT"`.

```go4t
package main
import p "std/prelude"
type Tree[A] = Leaf(A) | Branch(Tree[A], Tree[A]) deriving Eq, Ord, Debug, Functor, Foldable, Traversable
func main() IO[Unit] {
  let tree = Branch(Leaf(1), Leaf(2))
  println(debug(p.fmap(tree, (_ + 1))))
}
```

```go4t
package main
type Currency = JPY | USD | EUR deriving Show
func main() IO[Unit] { println("${JPY}|${USD}|${EUR}") }
```

```go4t
package main
type AccountId = AccountId(String) deriving Show
func main() IO[Unit] { println(show(AccountId("A-1"))) }
```

```go4t error
package main
type Hidden = Hidden(Int, String) deriving Show
func main() IO[Unit] { ioPure(unit()) }
```

### 7.3 JSON

`std/json` defines `Json = JNull | JBool(Bool) | JInt(Int) | JNum(Float) |
JStr(String) | JArr(List[Json]) | JObj(Map[String, Json])`, `Encode[A].encode(a) Json` and
`Decode[A].decode(j) Result[String, A]`. Import e.g. `codec "std/json"` and use
`deriving codec.Encode, codec.Decode`.

Primitive codecs cover Int/Float/String/Bool/Unit, Json itself, Option, List,
Array, Map[String, V], and tuples of every arity ≥ 2. Pair instances are in source;
the compiler materializes other tuple arities when used. Json encoding is identity;
its decode returns Ok. Unit/None encode as null; Some encodes its payload directly,
so Some(unit()), Some(None), Some(JNull) cannot round-trip distinctly from None.

| Constructor shape | Encoding |
| --- | --- |
| Sole record `Item{sku: String, qty: Int}` | `{"sku":"x","qty":2}` |
| Record in a sum `Named{name: String}` | `{"$tag":"Named","name":"x"}` |
| Positional `Shipped(String)` | `{"$tag":"Shipped","$values":["track"]}` |
| Nullary `Pending` | `{"$tag":"Pending"}` |

All derived forms require objects. Tags precede declaration-ordered fields or
`$values`; sole records omit tags. Record decoding requires all declared fields,
even Option fields (supply null), ignores extra fields, and returns the first
field error in declaration order. Positional/tuple arrays need exact arity.
Function fields, also syntactically nested ones, reject Encode/Decode deriving;
alias-hidden unsupported fields fail normal instance resolution.

`toJson` is compact; `toJsonPretty` uses two spaces and no trailing newline.
Go JSON escaping includes HTML and U+2028/U+2029 escaping. Parsed/derived objects
retain member/declaration order when printed as Json; duplicate parsed keys keep
first position and last value. Ordinary Map operations discard order metadata;
Encode[Map] always uses lexicographic String-key order, even for custom comparators.

Numbers in Json are exact `JInt(Int)` or `JNum(Float)`. Int encoding uses JInt
and round-trips exactly across the full int64 range. Int decoding accepts JInt
directly or integral JNum values within inclusive [-2^53, 2^53]; Float decoding
accepts JNum directly and JInt through `toFloat`, which may round large integers.
Printing JInt uses exact decimal digits without `.0`; Float retains Go's
`encoding/json` formatting, including integral output and exponent thresholds.
Integer `-0` normalizes to `0`; Float negative zero prints `-0`. Printing NaN or
infinity panics `cannot encode non-finite JSON number`. Parsing consumes exactly
one value, accepts surrounding whitespace, and uses JInt for integer literals
without decimal/exponent syntax that fit int64, otherwise JNum. Malformed/trailing
input or Float overflow returns `Err("invalid JSON: " + reason)`.

Decode errors are `expected <what> at <path>`, rooted at `$`, with `.name`, quoted
`["a.b"]`, and zero-based `[2]` segments. Examples:
`expected String at $.items[2].sku`, `expected field at $.sku`,
`expected array of length 2 at $`, `expected Status constructor at $["$tag"]`.
Containers prefix nested paths and preserve the first error; lists/tuples use
index order, Map values key order.

## 8. Compilation and tooling

### 8.1 Go generation and performance (informative)

The compiler emits **one Go file per go4t module in one Go package**, not separate
Go packages. `main.go` is the entry; dependency basenames use sanitized path tails
and deterministic collision suffixes. Names/dictionaries are module-prefixed.
`codegen.Generate(*types.Program, string) (map[string][]byte, error)` is the public
generator. The module loader feeds `types.Check([]*ast.Module)`, with entry last.

Result-position matches, conditionals and blocks emit their control flow directly
into the enclosing Go function, including nonrecursive functions. This keeps
large bodies visible to Go's inlining cost model rather than hiding them inside
returned immediately invoked closures, and avoids repeatedly expanding nested
closure names while inlining conditional branches. Expression-position forms
still use value-producing closures. Both forms preserve lexical scope, selected
branch evaluation and result representations; matches evaluate their scrutinee
once and retain arm order, guards and bindings. Only eligible recursive calls
require a tail-recursion plan.

The deterministic `bench.CompileSource(N)` workload contains N data types deriving
Eq/Ord/Debug, N polymorphic functions constrained by Eq + Debug and N match
expressions (5,203 source lines at N=400). `BenchmarkCompile` measures in-process
parse + module loading + checking with `ReportAllocs`, total allocation and a
`runtime.ReadMemStats` live-heap sample every millisecond and at completion.
The reported peak is the largest **observed** heap increase above a post-GC
baseline, not an exact allocator high-water mark. Generated Go bytes include all
dependency files. The separate `go build` wall time includes compilation/linking,
uses the normal Go build cache, and removes the output binary before each
iteration; it does not include go4t checking or code generation.

Linux/amd64 Ryzen 9 7950X3D calibration (single iteration, Go 1.26.1,
before Pass 13 type-print elision; decimal MB/KB, not a CI portability guarantee):

| Program | N | Source lines | Check ms | Alloc MB | Sampled peak MB | Go KB | Build ms | Longest error chars |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Generated scale | 400 | 5,203 | 51.42 | 36.265 | 11.313 | 1,631.319 | 2,239.28 | 559 |
| All std modules | — | 1,679 | 12.15 | 7.412 | 2.749 | 160.795 | 291.49 | 557 |

The diagnostic probe injects Bool where Int is expected beneath 32 nested
`Option` constructors. The checker returns one error; its complete rendered
length, including position, is counted in Unicode characters. `compile-*`
entries in `bench/budget.json` bound check/build nanoseconds, allocated bytes,
sampled heap bytes, generated Go bytes and diagnostic characters at **2×** these
calibration measurements. Run
`go test ./bench -run . -bench Compile -benchtime=1x` to enforce them.

The std fixture imports every embedded std module into one compiled program
(source lines include the entry and dependencies). Literal single-module
concatenation is skipped: std modules have distinct `package` declarations and
qualified references such as `prelude.pure` and `r.Err`; flattening them is not a
semantics-preserving operation. The fixture preserves those namespaces, then
checks, generates and builds the entire module graph together.

Classes become interfaces and instances dictionary structs; constrained functions
receive dictionaries. Known Int/Bool/Float locals, parameters/results, pattern
bindings and tail-loop state use int64/bool/float64. Monomorphic single-constructor
records whose fields are all Int, Bool or Float store those fields as native Go
scalars; their erased representation remains a constructor pointer. Other
constructor/tuple fields, polymorphic values, dictionary methods, extern wrappers
and runtime calls remain `any` boundaries. Higher-order values adapt native
signatures to erased callbacks; strings and other aggregates remain erased.
Native operators preserve overflow, zero division, signed Float zero and NaN-low
ordering, including literal operands.

List and array folds with known scalar callbacks retain unboxed accumulators.
Numeric-record folds may reuse a private accumulator; input records remain
immutable and replacement expressions evaluate before writes.

Prelude Ord[Int] and Ord[String] comparisons may use native collection ordering.
Custom comparisons retain their declared ordering. Sorting preserves the order
of equal keys.

Direct self calls in result position (through if/match/block tails) become loops.
Arguments evaluate left-to-right before simultaneous reassignment; each iteration
has fresh source bindings, preserving closure snapshots. Non-tail, aliased,
mutual-recursive, shadowed, nested-lambda/do-continuation and dictionary-changing
polymorphically recursive calls remain ordinary calls.

Concrete constrained in-program direct calls may specialize a function with
at most 40 AST nodes (including visited declaration/type nodes), at most eight
copies per distinct dictionary tuple. All dictionaries, including context/super,
must be concrete. Unknown/indirect/oversized/ninth-tuple calls use the original;
residual type parameters remain erased. Copies live in the callee module.

Generated Go is go/format-formatted, with `//line` directives on functions,
closures, match arms and relevant results. Source filenames containing newlines
are rejected. Runtime shapes bridge the single generated List/Option/Result,
Ordering, tuple and loaded Json representations; missing shape registration is a
runtime programming error, not a source-language fallback.

`codegen.Generate(program, package)` resolves externs in the current working
directory. `codegen.GenerateInDir(program, package, dir)` uses `dir` as the Go
module context for extern inspection; an empty `dir` preserves the current-directory
behavior. Both emit the same per-module Go source map.

Stable `list.sortBy` uses array-backed stable sorting, O(n log n) comparisons
and linear conversion space; the old quadratic `insertBy` helper is removed.
`sortOn` computes each key once; `nub` remains worst-case quadratic.

### 8.2 Commands and flags

```text
go4t init <module-path>
go4t get <path>@<version>
go4t check [--full-types] FILE.gt|DIR...
go4t build [--full-types] [-o FILE | --emit-go DIR [--package NAME]] FILE.gt|DIR...
go4t run [--full-types] FILE.gt|DIR... [-- args...]
go4t test FILE.gt|DIR...
go4t eval 'EXPR' [FILE.gt|DIR...]
go4t bind IMPORT [FILE.gt|DIR...] [-o FILE.gt|DIR] [--prefix STR] [--shim DIR]
go4t fmt [-w|-d|-l] FILE...|DIR
go4t lsp
```

- `init` creates `go.mod` and `go4t.mod` in the current directory; it refuses
  to overwrite either existing file. `go.mod` contains `module <module-path>`,
  `go 1.22`, `require github.com/reoring/go4t v0.0.0` and a local replacement for `go4t`.
  Before writing, it runs `go list -m -json go4t` from the current directory:
  a resolved `Dir` (normally a checkout inherited from a parent Go module) is
  used directly. Otherwise it extracts the binary's embedded runtime to
  `<user-cache>/go4t/runtime/<sha256>` and replaces `go4t` with that absolute
  directory. On Linux `<user-cache>` is `$XDG_CACHE_HOME`, or `$HOME/.cache`
  when unset. The digest covers embedded file paths and contents; extraction
  is published atomically and reused. No `.go4t` tree is created in the project.
  `go4t.mod` initially contains only `module <module-path>`. Keep the referenced
  checkout/cache directory available; local replacements are machine-specific.
- `get` accepts exactly one `<path>@<version>` (not `@none`). From the nearest
  ancestor `go4t.mod` root, which must also contain `go.mod`, it runs
  `go get <path>@<version>` with the caller's Go environment and streams.
  On success it queries Go's selected module version and adds/updates one
  `require <path> <version>` in `go4t.mod`, preserving comments and other
  requirements. Version queries such as `@latest` are recorded as the selected
  version, not the query. It does not generate bindings.
  On Go failure `go4t.mod` is unchanged; any `go.mod` or `go.sum` changes made by
  Go are not rolled back. Ordinary Go `replace` directives support offline
  local libraries; no separate download manager or go4t lockfile is used.
- `check` loads, type-checks and verifies laws and proofs (§6.5); it performs no
  foreign-signature validation or program execution. `build`, `run`, `test` and
  `eval` do not verify proofs and omit laws/proofs from generated code; their
  source still must parse and type-check.
- `--full-types` on `check`, `build` or `run` prints diagnostic types without
  depth elision (§5.2), retaining alias names. It may precede or follow inputs;
  after `run`'s `--` it is a program argument, not a compiler option.
- `build` produces an executable in the current directory. Its default name is
  the first input directory's basename or the first input filename without `.gt`.
  `-o FILE` selects the executable path; an existing directory is rejected.
  Generated Go sources remain in `<first-file-stem>_gen/` or `<DIR>/_gen/`.
  `--emit-go DIR` emits sources only and is incompatible with `-o`.
  `--package NAME` requires `--emit-go` and defaults to `main`. Other package
  names expose `func Main() rt.IO` and the typed entry-module API in §8.7.
  NAME is a valid non-keyword Go identifier other than `_`.
  Flags can precede or follow inputs. Rewrites remove obsolete compiler-generated
  Go files, including `go4t_export.go`, and preserve unrelated output-directory files.
  Go build diagnostics reach stderr; build failures exit nonzero.
- `run` emits, builds and executes the whole package, inheriting stdin/stdout/
  stderr and forwarding arguments only after `--`. In a Go module resolving
  `go4t`, its generated sources remain and the executable is temporary. Outside
  such a module, all generated sources and the executable are temporary: it
  does not create `<file>_gen` or `<DIR>/_gen` beside the input.
- `test` replaces user main **before checking**. It discovers non-extern,
  monomorphic top-level functions whose names start `test` with a nonempty
  suffix, no value parameters, and explicit `Bool` or `IO[Bool]` return syntax.
  Other functions are still checked. Tests run once in file/declaration order,
  print `PASS name` / `FAIL name` / `FAIL name: error`, then `N passed, M failed`.
  Sequential helpers contain at most 64 tests each; bounded aggregation levels
  preserve that order and count every result without a suite-sized expression
  or scope. Large suites use the same parser limits and validation as small ones.
  False/IO errors do not stop later tests; failures exit 1; an empty suite is
  `0 passed, 0 failed`, exit 0. Panics use ordinary runtime policy, not a new
  per-test recovery; tests may catch them with `io.tryIO`. Alias return syntax
  is not test discovery syntax. Generated helper and result names avoid names
  declared by the input.
- `eval` removes user main, checks `println(show(EXPR))`, then tries an IO bind
  wrapper if needed. If both fail it reports the **first** diagnostic. EXPR must
  be exactly one expression. Supporting imports are copied from the first entry
  file; absent files it has only implicit names. Results require Show, even IO
  results: Unit has no Show. Test/eval use temporary generated directories and
  do not replace ordinary build output. Source labels are `<test>` / `<eval>`.

For `run`, `test`, and `eval`, the nearest ancestor `go.mod` of the generated/input
directory (the working directory for standalone eval) is used when
`go list -m go4t` succeeds there. Otherwise the CLI creates a temporary `go4trun`
module with `require github.com/reoring/go4t v0.0.0` and `replace github.com/reoring/go4t => ./go4t`, extracts its embedded
`rt/` Go source tree (including runtime subpackages) into `go4t/`, and writes a
minimal `module github.com/reoring/go4t` file with the distribution's Go version. It inspects externs
in this temporary module, emits sources into `prog/`, builds the whole package,
and cleans the module and executable on success or failure. The installed binary
needs only a Go toolchain on PATH, not a checkout, module cache entry, source-path
hint, or environment override.

`build` retains the executable and generated-source directory. Without a usable
Go module, compilation uses the embedded runtime in a temporary module and removes
that module on success or failure. It does not modify the user's `go.mod`.
`--emit-go` selects source-only output and prints no module-configuration note.
For extern inspection, `build` uses the output directory's usable Go module,
then the input's usable module if the output is outside one, then the embedded
runtime as above; writing sources outside a module does not discard an input
module's foreign dependencies.

The standalone fallback supports externs only from `github.com/reoring/go4t/rt` and its subpackages.
Other extern packages require an application Go module resolving both the package
and `go4t`; otherwise the diagnostic is
`extern "<pkg>" requires a Go module: run inside a directory with go.mod that provides it`.
The temporary module never copies unrelated application dependencies or downloads
third-party packages.

### 8.3 Externs and bind

Foreign packages are inspected once per compilation using `go list` export data
and Go's parser/type importer in the module/build-constraint/toolchain context
selected by `GenerateInDir` (§8.1); standalone CLI execution uses its extracted
runtime module (§8.2).
Each exported package-level function gets an adapter; methods are reached through
generated package-level shims (`bind --shim`), not directly. Generic package
functions bind as templates instantiated from concrete call-site types; variadic
functions are not direct extern targets.

Boundary invariant (ADR 0011): preserve the ordinary Go value or its opaque
handle identity; do not silently replace it with a serialization view. Named
JSON views may normalize or lose information, but adding those views never
changes the representation chooser. Explicitly documented lossy scalar rules
below, including `time.Time` → UnixNano, remain unchanged.

| go4t type | Natural Go ABI | Boundary |
| --- | --- | --- |
| Int | int64, also int | int64 is canonical; int narrowing follows Go on 32-bit targets |
| Float | float64 | Binary64 |
| String | string | No encoding conversion |
| Bool | bool | Direct |
| Unit | rt.Unit | No-result Go functions are not Unit |
| IO[T] | rt.IO | Action forwarded; `(T, error)` return may instead generate deferred IO |
| List[String] | []string | Copy conversion, preserving order; nil/empty → Nil |
| List[Int] | []int64 | Copy conversion, preserving order |
| Option[T], primitive T | adjacent `(T, bool)` values | None → zero payload/false; Some(zero) remains present |
| Other types | any | Erased generated representation, not an invented native layout |

Primitive Option payloads: Int, Float, String, Bool, Unit. Options of other types
use any. True Go aliases are accepted; distinct named primitive types are not
coerced. Fully all-any signatures (including all-any parameters returning rt.IO
for declared IO) keep forwarding; otherwise every natural parameter/result must
match. Mismatches during generation report
`extern F: Go signature X does not match declared type Y` at the extern position.

For Go `(T, error)` returns declared as IO[T], the call, conversions and boxing
are **inside** the generated context-taking action. Re-running repeats the call;
non-nil errors become classified `*rt.Failure` errors (§6.2). The adapter checks
cancellation before the call, but cannot interrupt a foreign function without context support.
For cancellable foreign effects, return `rt.IO` and honor its execution context,
as the HTTP runtime behind `std/http` does (§6.4). For a pure natural-ABI proof,
`examples/typed-extern` binds `strings.ToUpper(string) string` and slice-returning
functions without a runtime-specific HTTP shim.

`bind` emits a complete declaration file in lexical exported-function order,
stdout by default. `-o` names a file or an existing directory (`bindings.gt`).
Receiving source files/directory select their package spelling and Go load
context; absent them it emits `package main`. Output path alone does not select
context. Parameters are `p0`, `p1`, etc.; names lower-case the first Go letter,
or prefix it (`--prefix str` produces `strToUpper`). Collision comments do not
rename names. Primitive `(T, bool)` results infer Option and `(T, error)` infer
IO; adjacent inputs are emitted independently, never guessed to be Option.
Without `--shim`, only the natural-ABI table above is inferred. With `--shim`,
the foreign mirrors below are inferred too. Unsupported functions receive
`// skipped: Name (reason)`; unsupported methods use `Type.Method`. Generic
receiver methods, instantiated named structs/interfaces, unnamed interfaces,
unsupported channel elements, and unsupported result tuples remain skipped.
Named unsupported values become handles, not invented record layouts. Reserved
names suggest a prefix.

| Go boundary | go4t boundary |
| --- | --- |
| Generic package function | A template instantiated from concrete call-site types; every type parameter is determined and its Go constraint is satisfied. |
| Generic function or callback | IO, including returned callbacks; conversion and invocation occur inside the action after cancellation checks. |
| Unresolved or unsupported generic instantiation | A compile error at the use site; ordinary Unit defaulting does not determine Go type arguments. |
| Generic receiver method or instantiated named struct/interface | Unsupported. |
| Iterator parameter | Unsupported; iterator results remain materialized Lists. |

Int, Float, Bool, and String select int64, float64, bool, and string as generic Go
type arguments. List and Map compose these mappings. Known foreign records and
handles retain their Go identity. Context, error, and optional-result conventions
are determined by the original Go declaration, not by substituted type arguments.

| Foreign Go type | go4t mirror with `--shim` | Conversion |
| --- | --- | --- |
| bool, string | Bool, String | Direct |
| int, int8/16/32/64, uint, uint8/16/32/64, uintptr | Int | Go integer conversion; narrowing/wraparound follows Go |
| float32/64 | Float | Binary64 on the go4t side |
| []byte | String | Copy raw bytes, not a text decoding |
| []T | List[T] | Recursive copy, nil/empty becomes Nil |
| iter.Seq[T], iter.Seq2[K, V] results | List[T], List[(K, V)] | Eager materialisation in iteration order; iterator parameters are skipped |
| map[K]T with scalar K | Map[K, T] | Recursive copy; scalar key ordering |
| *T parameter, where T mirrors to a scalar, record, collection or bytes | Ref[T] | Non-null in-out cell; copy its value into Go, call with its address, then copy the updated value back; forces IO |
| *T result or record field with a value mirror | Option[T] | nil becomes None; non-nil becomes Some(copy) |
| chan T, <-chan T, chan<- T with mirrorable T | Chan[T], RecvChan[T], SendChan[T] | Opaque directional handles; preserve the native Go channel and reuse the ordinary element converters; unsupported elements retain the signature's skip reason |
| chan struct{}, <-chan struct{}, chan<- struct{} | Chan[Unit], RecvChan[Unit], SendChan[Unit] | Anonymous completion signals carry Unit |
| Channel element T encountered in a bound package | new<T>Chan(buffer Int) IO[Chan[T]] | Allocate a fresh native channel when the action executes; generated names follow the existing binding-name precedence |
| time.Duration | Int | Nanoseconds; the named duration type is erased (documented lossy projection) |
| time.Time | Int | Lossy UnixNano projection: location and monotonic metadata are discarded; zero time becomes 0, and 0 reconstructs zero time |
| error parameter (including callback parameters and variadic elements) | GoError | Non-null opaque builtin handle; preserve the original Go error identity in both directions; does not itself force IO |
| error in a non-trailing result, record field, package variable, or container element | Option[GoError] | nil becomes None; otherwise Some(original error); reverse conversion preserves the same error, including its wrapping chain |
| Trailing error result | IO failure | Unchanged: nil is success; non-nil is classified as an IoError and retained as the cause available through io.tryIOCause / io.cause |
| Exported nongeneric named T implementing json.Marshaler, with *T implementing json.Unmarshaler | `<t>ToJson(recv T) dyn.Value` | Pure explicit view of the ordinary mirror (including named non-struct types); encoding/json plus runtime any↔dyn conversion. Marshal errors panic with `go4t json view: <type>: <err>`; existing binding names win and skipped views receive a collision comment |
| Same JSON-capable T | `<t>FromJson(v dyn.Value) IO[T]` | Deferred, cancellation-aware explicit view; encode the dynamic input and unmarshal into T. Conversion errors are IO failures retaining the original Go error as cause; successful results use the ordinary mirror. Unsupported or lock-copying views are skipped |
| Exported struct whose fields are all exported and mirrorable | Record named after the struct | Declared in its owning Go package's binding module, first field letter lower-cased, `deriving Eq, Debug` unless a field contains an opaque handle or GoError (directly or through records, containers or Option); neither leaf has those instances, and Debug derivation cannot render a handle placeholder. Opaque struct leaves keep their native state. A field whose lowered name is a go4t keyword gains a trailing `_` (`SELinuxOptions.Type` → `type_`); promoted embedded fields follow Go lookup, ambiguous fields omitted. A struct with any unexported field, or a pointer embedding, is private state and becomes a handle (`kubernetes.Clientset`): copying its exported fields would silently discard the rest |
| Named non-struct with mirrorable underlying type | Underlying mirror | No distinct go4t type; shim performs the named Go conversion |
| Other exported named types | Opaque handle | Includes interfaces, cyclic structs and structs with nonmirrorable fields |
| Opaque named struct value or pointer | Same opaque handle | A lock-free value result is copied into one fresh mutable pointer box; a pointer preserves identity; passing a boxed handle back as a Go value dereferences it. Package-variable getters, struct-handle field getters and struct-handle receivers use addresses instead. By-value parameters, results and container elements containing locks are skipped with `copies lock value T` |
| Mirrorable exported field of an opaque named struct | `<type><Field>(recv T) IO[F]`, `<type>Set<Field>(recv T, value F) IO[Unit]` | Deferred, cancellation-aware accessors observe or mutate the handle; struct-handle fields are returned by address. Setters that copy a lock value, unmirrorable fields and binding-name collisions receive a skip comment |
| Named T implementing exported interface I | `<t>As<I>(recv T) I` | Pure upcast for handles, records and named scalar/bytes/slice/map sources (receiver uses the underlying go4t type; the shim restores named T and boxes it only when *T is required) in binding module P when T and I belong to P or its direct imports and at least one is declared in P, including interface-to-interface widening; generic/constraint interfaces, `error` and identity casts are excluded; foreign source homonyms gain an owner suffix while P's own types keep the unsuffixed name; other binding-name collisions are skipped |
| Two or three results, except `(A, bool)` | (A, B), (A, B, C) | Runtime tuple; trailing error is removed first and forces IO[tuple]; more than three payload results are skipped |
| Function parameter or result with mirrorable signature | func(A, ...) R or func(A, ...) IO[R] | Erased callback adapter in both directions; leading context or trailing error forces IO, and returned Go functions run when their IO is executed |
| any | dyn.Value | Runtime tagged conversion; see §10.7 |

Otherwise, a struct without exported fields is a handle, including an empty struct:
turning a private-state object such as `sql.DB` into an empty record would lose
its identity. `*Handle` stays the same handle, not Option. With `--shim`, handles
and value records are declared only by the binding module of their owning Go
package. Other bindings reference them through generated `go:` imports, such as
`import go_io "go:io"` and `go_io.Reader`; import aliases avoid generated names.
All callers therefore share the owning module's type identity. This applies to
`BindWithShim`; natural-ABI-only `Bind` output is unchanged.

JSON interfaces only add named views. For example, `metav1.Time` and
`resource.Quantity` use opaque handles, while the all-exported
`intstr.IntOrString` uses a record; all also expose JSON views. Native by-value
record crossings copy opaque field values, preserving their state (including a
timestamp's location), not the address of a temporary box. go4t record storage
and selection retain the original handle box. No ordinary crossing marshals JSON.
`ToJson` treats failed serialization of a valid domain value as a programming
error; `FromJson` treats invalid external input as an ordinary fallible IO action.
Likewise, `net/url.URL` is a record with a `Userinfo` handle field; its fields
are all exported even though `Userinfo` itself has private state. It has no
derived Eq or Debug, `urlHostname` / `urlString` operate on copies and are pure,
and `parse` returns `IO[Option[URL]]` rather than an opaque URL handle.

Error mirrors preserve identity rather than projecting messages (ADR 0011 §3C).
`GoError` is a builtin like `Ref`, so generated bindings need no import for it.
An ordinary predicate such as `IsNotFound(error) bool` binds as
`isNotFound(p0 GoError) Bool`; receiving an error handle alone does not change
the existing purity rule. Use `io.goErrorMessage` for an explicit message
projection, or pass a retained `io.cause` to `go:errors` predicates such as `is`.
Direct error parameters, callback parameters and `...error` elements are
non-null: a Go caller passing nil through such a go4t-visible parameter is not
representable and the callback adapter panics with
`go4t error: nil error parameter`. Ordinary `[]error` and map values instead
contain `Option[GoError]`, preserving nil elements. Go typed-nil errors are
non-nil interfaces and remain `Some` handles, just as in Go.

Pointer meaning depends on position (ADR 0011 §2C). Direct pointer parameters
with scalar, record, slice, map or bytes payloads use the opaque builtin `Ref`;
import `std/ref` for `new`, `get` and `set`. This includes read-only Go APIs:
the binder does not infer whether a callee writes. Variadic `...*T` parameters
become `List[Ref[T]]`, and callback adapters perform writeback in both directions.
Pointers inside records or ordinary containers, pointer results, pointer-to-handle
identity and unresolved type-parameter pointers retain their existing
representations. Method receivers retain their existing receiver rule.
Each cell read and write takes the runtime Ref lock; the foreign call itself is
not a cell-atomic transaction. Writeback occurs before returning payloads,
optional absence or a Go error, including when the callee cancels its context.
An already-cancelled IO does not call Go or write a cell. Ref parameters cannot
represent nil; a native callback given a nil in-out pointer panics with
`go4t ref: nil pointer parameter`.

Lock detection follows Go vet's copylocks predicate:
`Implements(*T, sync.Locker) && !Implements(T, sync.Locker)`, recursively through
struct fields, arrays and embedding. Embedding `sync.Locker` itself, or providing
value-receiver `Lock`/`Unlock` methods, does not make a struct a lock. A pointer to
a lock-containing struct remains an opaque handle. Struct-handle receivers use
the pointer method set whenever it contains the method, irrespective of locks.

Channels preserve native identity as opaque directional handles. Every channel
operation and typed channel constructor is IO; allocation of shared mutable state
is an effect. Element conversion reuses the ordinary shim converters. Capability
upcasts are explicit and pure. Channel-bearing and effectful callbacks force their
enclosing foreign call into IO.

`std/chan` exports `make`, `send`, `recv`, `close`, `asRecv` and `asSend`.
`make[A](buffer)` returns `IO[Chan[A]]`; `send(SendChan[A], A)` and
`close(SendChan[A])` return `IO[Unit]`; `recv(RecvChan[A])` returns
`IO[Option[A]]`. Closed receive returns `None`. Send and receive honor cancellation.
Closing an already closed channel fails with
`Failure("chan.close: close of closed channel")`.

`asRecv` and `asSend` are explicit, identity-preserving capability upcasts from
`Chan`. Receive-only channels cannot be sent to or closed; send-only channels
cannot be received from. Channels have no standard class instances, and channel
operations are law-ineligible externs.

`std/chan.make` allocates go4t-only channels. Native Go parameters require a
generated typed channel constructor or an imported channel. The type system does
not distinguish native element representations that share one go4t mirror;
incompatible conversions fail with `IoError.Failure`, without a forwarding channel.

Concrete handles, records and named scalar/container sources expose `<t>As<I>` upcasts whenever T
or *T implements an exported nongeneric method-set interface I. Binding package
P considers its own source types against interfaces in P and its direct imports,
and direct-import source types against interfaces declared in P. Bindings stay
in P's module; unrelated imports and transitive sources are not searched.
Foreign sources are resolved only after the implementation check succeeds.
Interface handles also expose widening upcasts; for example,
`strings.readerAsReader` yields `io.Reader`, and `io.readCloserAsReader` widens
`io.ReadCloser` to `io.Reader`. Handle upcasts preserve identity; record upcasts
use the existing copy boundary. The builtin `error` interface is not an upcast
target. Binding-name precedence is method/function > field accessor > constructor/value getter > upcast > JSON view; ties keep first-seen order. Each loser emits `// skipped: <display> (name <name> taken by <winner display>)`. When one source has upcasts to same-named interfaces, both names include the owning package suffix before the interface name: the last path element CamelCased, with its parent prepended for version segments (`meta/v1` → `AsMetaV1Object`, `runtime` → `AsRuntimeObject`). If those still collide, use the full generated import alias without `go_`, CamelCased.
When multiple candidate sources share a simple name, foreign sources gain the
same owner qualifier before the source name (`qImplAsI`); P's own type keeps
`implAsI` and wins ties. Equal short qualifiers fall back to the full generated
import alias. For example, `client.unstructuredAsObject` converts the directly
imported `unstructured.Unstructured` into the interface declared by `client`.

The companion shims preserve the erased extern ABI (`any` parameters/result).
Record constructors cannot live in the shim package, because it cannot import
the generated program. Instead, codegen emits companion `init` registrations
beside the program's ordinary record ADTs, keyed by
`<owning Go package import path>.<Go type name>`, not the calling shim's path.
The Go spelling is retained even when a prelude collision adds `Go` to the
go4t name (`OptionGo` still registers as `<owner>.Option`). The binder carries
these keys as semantic module metadata, preserved by the binding cache and
checker; they are not inferred from comments. Without explicit metadata, a
record in a `go:` module uses that module's import path without `go:`.
`rt.RegisterForeignRecord` supplies constructor/field operations,
just as `rt.Shapes` supplies List/Option/Json operations. Nested records in extern
signatures are registered recursively. No reflection or second record ABI is
used. Structs, slices and maps cross by copy; in-out pointer parameters copy back
through Ref cells after the call. Private state is not copied.

Legacy bindings written with `bind -o` have no semantic owner metadata and
retain the fallback key `<extern target import path>.<go4t source type name>`.
These inspection bindings compile but are not a supported runtime path;
use `go:` imports to execute record conversions.

A leading `context.Context` parameter is omitted and forces an `IO[R]` result.
The shim receives the executing IO's context, so cancellation and deadlines
reach Go. An error-only result becomes `IO[Unit]`; no result becomes `Unit`.
Errors are classified `*rt.Failure`, not nested `Result` values.

The **only semantic inference from Go function names** is error classification:
when a bound package exports nongeneric `func IsXxx(error) bool` predicates with a
nonempty `Xxx`, its shim's `init()` registers one `rt.ErrorClassifier`. The first
matching predicate wins in this order: `IsNotFound` → `NotFound`,
`IsAlreadyExists` → `AlreadyExists`, `IsForbidden`, `IsUnauthorized` →
`PermissionDenied`, `IsTimeout`, `IsServerTimeout` → `Timeout`, then all remaining
`IsXxx` predicates alphabetically → `Failure`. Messages remain `err.Error()`,
except those remaining predicates prefix `Xxx: `. Packages without matching
predicates emit no classifier. `rt.ClassifyError` preserves nil and existing
`*rt.Failure` values (including wrapped failures), then tries classifiers in
registration order before its OS/context/network defaults. Classifiers of every
importable package in a bound `go:` package's transitive Go dependency closure are
registered automatically during package initialization, before IO executes,
without a separate user import. No other binding semantics are inferred from names.
Every newly classified `rt.Failure` stores the original error in `Cause`,
including classifier-selected failures. A wrapped existing failure remains the
same inner `*rt.Failure`; its cause is not replaced by the wrapper.
`Failure.Unwrap` exposes that cause to `errors.As`; `Failure.Is` delegates to
`errors.Is(Cause, target)` whenever a cause exists, falling back to its five
OS/context sentinels only when `Cause == nil`.

Any call that receives a handle other than `GoError` also forces `IO[R]`,
whatever it returns: as a method receiver (`strings.Builder.Len` is `IO[Int]`, `ServeMux.Handle` is
`IO[Unit]`), as a parameter, directly or inside a `List`/`Map`/`Option`
(`func(b *strings.Builder) int` binds as `IO[Int]`), or as the receiver of a
field getter (`requestHeader` is `IO[Map[String, List[String]]]`). A handle is
shared Go state; reading it is an observation. Records, maps and slices cross
as copies and stay pure. Constructors, package-value getters and upcasts
return a handle without reading through it and stay pure. A package variable
whose value is a struct handle returns `&pkg.Var`, not a boxed copy of its value.
Error predicates retain their existing pure binding convention; other handles
keep shared-state observations outside the pure fragment that laws reason about.

Variadic `...T` becomes a final `List[T]`; `...any` becomes `List[dyn.Value]`.
`std/dyn.Value` has `Null`, `Bool`, `Int`, `Float`, `Str`, `Bytes`, `Time`,
`List` and `Map` constructors and derives Eq/Debug. List/Map contents convert
recursively; Bytes preserves bytes and Time uses nanoseconds.

Function-typed parameters, including named function types, become go4t
`func(A, ...) R` or `func(A, ...) IO[R]`. The shim converts arguments into go4t
values, calls the erased function, and converts its result back. A callback's
leading context is omitted; its IO runs with that context, or with the enclosing
call's context when it has none. Callback failures return through Go's error
result unchanged; a context-taking callback without an error result panics on
IO failure because Go provides no error channel.

The one shape-based special case is a method whose sole ordinary parameter is
`dest ...any` and whose sole result is `error`: it becomes
`(recv Handle, count Int) IO[List[dyn.Value]]`. The shim allocates `count`
`*any` slots, calls the method, then decodes the values. Negative counts fail
with `go4t scan: negative count N`. This supports `sql.Row.Scan` and `sql.Rows.Scan`.

`bind IMPORT --shim DIR` additionally writes `DIR/go4t_shim_<pkgbase>.go` and
appends method bindings after the ordinary functions, sorted by receiver type
and method name. `DIR` must be inside a Go module; otherwise the command reports
`--shim requires DIR inside a Go module (with go.mod)`. Missing directories are
created. The Go package name comes from existing non-test `.go` files, or from
the last directory component sanitized into a Go identifier. The shim import
path is resolved with `go list` in that module. Without receiving source inputs,
`--shim DIR` also selects that Go module for loading `IMPORT`; without `--shim`,
binding output and load-context selection are unchanged.

Exported methods of exported named types include value and pointer receivers
(and promoted methods), once each; aliases do not introduce additional receiver
types. Without `--prefix`, the receiver type name is lower-cased as a word
(`URL.Hostname` → `urlHostname`, `DB.ExecContext` → `dbExecContext`,
`HTTPClient.Do` → `httpClientDo`); with `--prefix` the Go spelling is kept after
the prefix. A Go type whose name is owned by the implicit prelude (`Result`,
`Option`, `List`, …) binds with a `Go` suffix (`sql.Result` → `ResultGo`) so
generated bindings never shadow prelude names; writing the unsuffixed spelling
reports `module … has no exported name Result; the Go type Result is bound as
sql.ResultGo because Result is a prelude name`. Each receiver uses its record/value
mirror or handle, and each method adds
`extern func <type><Method>(recv <Type>, ...) ... = "<shim import path>".<Type>_<Method>`.
Pointer receivers on records operate on a fresh copy, without mutating the
original go4t record. Unsupported methods get `// skipped: Type.Method (reason)`.

Handles are **opaque**, not ordinary data constructors: matching, `deriving`,
and `show` on a handle are undefined. Obtain them from bound Go constructors,
not by constructing the nullary handle constructor. The shim accepts `recv any`,
then checks and asserts the original Go receiver type before calling the method.
A wrong dynamic type panics with
`go4t shim: Type_Method: receiver is <dynamic Go type>`. A typed nil pointer
retains the underlying method's nil behaviour. `examples/typed-extern` also
demonstrates explicit erased Go helpers and generated methods.

`go:` imports use this declaration-and-shim pipeline on demand. Shims use the
receiving build module's Go import paths beneath the build output; generated
bindings and shims may be reused from the import-time cache (ADR 0003).


### 8.4 Formatting

`fmt` prints canonical source; `-w` rewrites changed files preserving permissions;
`-d` prints deterministic full-file unified diffs and exits 0;
`-l` lists unformatted paths and exits 1 if any. Flags are mutually exclusive;
`--` ends flag parsing. Directories recurse over regular `.gt` files without
following directory symlinks; explicit files need not have a `.gt` extension.
Duplicate absolute paths are removed; output is path-sorted.

Formatting parses only: it needs neither main, type checking nor Go package
loading. Invalid source is never overwritten, but multiple-file writes are not
transactional. Style is two-space indentation, blank lines between declarations,
multiline blocks, no trailing commas, spaced operators; records exceeding 80 rune
columns and long pipeline chains break across lines. Single-line match arms omit
`|`, multiline arms retain it. Comments preserve text/order and leading/trailing
attachment (trailing whitespace removed), including interpolation comments.
Parentheses preserve semantics; integral Floats keep a Float spelling.

### 8.5 Editor protocol

`go4t lsp` is a synchronous stdio JSON-RPC 2.0 / LSP 3.17 server, with
Content-Length framing, full-document sync, zero-based UTF-16 positions and
exclusive spans. Supported: initialize/initialized/shutdown/exit;
didOpen/didChange/didClose; diagnostics, hover, definition and documentSymbol.
It loads the directory module and transitive graph using the CLI loader.
Open buffers overlay disk; increasing full-change versions replace them; stale,
incremental or unopened changes are ignored. Close clears diagnostics/restores disk.
A failed check invalidates the old index: hover/definition are null, symbols empty.

Hover uses the checker's type printer and adjacent declaration comments;
definitions include qualified cross-module names, aliases, fields, parameters
and methods (methods navigate to class signatures; common fields to the first
constructor). Document symbols are hierarchical in declaration order; generated
derivations are excluded. Builtins have no source target. Embedded std uses
`go4t-std:///std/list/list.gt`; `go4t/stdlib` returns that source for a `uri`.

The sales-report acceptance session exercises both open source files, the full
`summarize` function signature and `rates` type on hover, `parseCsv` navigation
from `main.gt` into `domain.gt`, document symbols, and separate full-buffer
changes producing an unused-let warning and a type error with exclusive ranges.
The exact observed stdio exchange and editor-host availability are recorded in
[`docs/lsp-verification.md`](docs/lsp-verification.md); the framed replay is
`lsp/sales_report_test.go`.

No completion, rename, incremental edits, file watching, workspace symbols or
LSP formatting is advertised. The dependency-free VS Code client spawns this
server; setup and executable-path configuration are in [editors/README.md](editors/README.md).

### 8.6 Performance evidence (historical, not a portability guarantee)

The runtime benchmark harness times built-program startup, input construction,
workload and output; compilation is excluded from `BenchmarkPrograms`. It
validates output even during calibration. The separate `BenchmarkCompile`
measurements and budgets are described in §8.1. `bench/budget.json` contains
nanoseconds/iteration for runtime workloads and unit-suffixed `compile-*` limits;
excess fails the test. `GO4T_BENCH_RECORD=1` disables budget enforcement only;
`GO4T_BENCH_ROOT` selects the checkout for runtime benchmarks and the generated
Go module's runtime dependency (in-process compile benchmarks use the checkout
being tested). CI invokes
`go test ./bench -run . -bench . -benchtime=1x -timeout=12m`.

Runtime budgets execute the five `bench/perf` kernels with `GOMAXPROCS=1` and
`GOGC=100`, validate exact stdout and bound GNU-time wall duration. Missing GNU
time or a kernel input skips the affected check.

Historical Linux/amd64 Ryzen 9 7950X3D, Go 1.26.1 host measurements from the perf
slice (single samples; **not measured on a CI runner**):

| Workload | Before ns/op | First after ns/op | Integrated ns/op | Budget ns/op |
| --- | ---: | ---: | ---: | ---: |
| Sort 1,000,000 shuffled Ints | >180,000,000,000 (deadline) | 1,592,269,718 | 1,577,251,407 | 3,184,539,436 |
| Map insert/lookup 100,000 | 139,528,301 | 127,266,911 | 117,439,934 | 254,533,822 |
| Construct/fold 10,000,000 | 790,179,553 | 659,464,204 | 688,849,817 | 1,318,928,408 |
| wc 10,000,000 bytes | 748,133,630 | 714,679,810 | 711,582,734 | 1,429,359,620 |

Budgets are twice the first successful after sample; sort's before is a censored
lower bound, not a completed measurement. Its algorithm change explains much of
the gain. Cross-host/CI headroom is not established by these numbers. Tests and
examples are regression evidence, not universal stack/time guarantees; only
eligible direct-self tail calls have bounded call-stack lowering.

### 8.7 Exported Go API

`go4t build --emit-go DIR --package NAME FILE.gt` produces an importable Go
package. Its `go4t_export.go` contains natural Go mirrors, conversion functions,
and wrappers for eligible functions of the **entry module**. Every exported
mirror type carries a Go doc comment naming its go4t declaration and how to
construct and switch on it (enum constants for all-nullary sums, one struct per
constructor for payload sums, field mapping for records), so the Go shape can
be read from the generated file without trial compilation. `Main() rt.IO`
remains available; ordinary executable builds do not generate this API.
See [the Go shell example](examples/go-shell) for a Go controller shell calling
pure go4t decision logic.

| go4t type | Exported Go mirror |
| --- | --- |
| `Int`, `Float`, `String`, `Bool` | `int64`, `float64`, `string`, `bool` |
| `Unit` | `rt.Unit` |
| `List[T]` | `[]T` |
| `Option[T]` | `rt.Option[T]` with `Value T; Present bool`; `rt.Some(value)` / `rt.None[T]()` |
| `Result[E, A]` | `rt.Result[E, A]` with `Err E; Ok A; IsOk bool` |
| `Map[K, V]` | `map[K]V`, only for scalar `K` (`Int`, `Float`, `String`, `Bool`) |
| `Set[T]` | `map[T]struct{}`, only for scalar `T` |
| Tuples of arity 2–8 | `rt.Tuple2[A, B]` … `rt.Tuple8[...]`, fields `F0`, `F1`, … |
| `go:` opaque handle types | Native `*pkg.T` for struct-underlying handles, otherwise `pkg.T` (including interfaces); identity, not a data mirror |
| Non-parameterised user data | Struct, enum or sealed interface as below |
| Fully applied user data `T[A1, …, An]` | Monomorphic struct, enum or sealed interface; arguments and substituted constructor fields must be mirrorable |
| `IO[T]` as the entire result | `func(context.Context) (T, error)` |

Every component must itself be mirrorable; aliases are expanded before deciding.
Functions, classes, type parameters, bare or partially applied parameterised data,
non-scalar Map/Set keys, IO in a parameter or nested position, and foreign handles
without Go identity metadata (legacy `go4t bind` files) are not mirrorable.
Only non-extern top-level functions without type parameters or constraints are
eligible. Their first name rune is upper-cased (`decide` → `Decide`).
`main` and `_`-prefixed functions are silently omitted. Other ineligible entry
functions appear once as `// not exported: <name> (<reason>)`, including externs.
There is no opaque `any` fallback in this API; §8.3's erased extern ABI is unchanged.
Legacy handles are skipped with `foreign handle <Type> without Go identity`,
never exported as nullary enums.

Handles imported through `go:` retain native Go identity in both directions,
including inside lists, options, map values, tuples, records and IO results.
Struct handles cross as pointers; a value-origin struct is boxed if necessary.
No new type declaration, type doc comment or `String()` method is generated for
a native handle. Go-owned records still use the go4t-shaped record mirrors;
their handle fields follow the same identity rule.

An entry-module data type keeps its source name, such as `Observed`. Dependency
types use the module file prefix with its first rune upper-cased, followed by the
type name (`model.Remote` → `ModelRemote`); prefix collisions receive deterministic
numeric suffixes. Only types transitively reachable from eligible signatures are
emitted, sorted by Go name.
If an exported function name collides with a type or constructor mirror, the
function receives an `Fn` suffix (`nodeState` → `NodeStateFn`) while the
type/constructor name stays unchanged; an already-taken suffixed name still
produces `Go export name collision: <name>`.

A fully applied data type is named `<T><MirrorName(A1)>…<MirrorName(An)>`,
using each argument's mirror spelling without non-identifier characters:
`Fetched[Plan]` → `FetchedPlan`, `Fetched[List[String]]` → `FetchedListString`,
and `Pair[Int, Option[Bool]]` → `PairIntOptionBool`. Constructor names use the
instance prefix, such as `FetchedPlanFound`. Each distinct substituted type
identity has one mirror and converter pair; aliases do not create additional
instances. A mirror spelling that clashes with a user-declared type is a
generation error at that user declaration, even if that declaration is otherwise
unreachable.

- One record constructor becomes a struct with each field's first rune upper-cased.
  Duplicate exported field spellings are a generation error at the type declaration.
- One positional constructor with payloads becomes a struct with `F0`, `F1`, ….
- All-nullary constructors become an `int` enum. Constants are `<Type><Ctor>` in
  declaration order, starting at zero; `String()` returns the source constructor name.
  This is independent of `Debug`, including any custom enum `Debug` instance.
- Other sums become `interface { is<Type>() }` with one `<Type><Ctor>` struct per
  constructor. Marker methods use **value receivers**: type-switch on
  `StepCleanup`, not `*StepCleanup`. Nullary branches are empty structs.
- A recursively embedded value struct with no slice, map or sum-interface
  indirection is a generation error (`recursive Go value type <name>`), because
  Go cannot represent its infinite size. Recursion through those indirections works.

Record structs, single positional structs, and every constructor value type of a
payload sum have `func (v <Mirror>) String() string` **only if** the go4t type has a
resolvable `Debug` instance in the program. For a parameterised type this resolves
the instance with its concrete arguments. The method converts through
`fromGo_<Mirror>` (the sum's converter for a constructor value) and calls that
instance's `Debug` dictionary method, so the result is exactly go4t `debug(v)`,
including custom instances. The sum interface itself has no `String` method. Without `Debug`, no
`String` method is generated for these structs. A record field exported as
`String` conflicts with this method and is a generation error.

Names colliding with another generated declaration are generation errors rather
than silently renamed API members. Programmatic `codegen.Generate` calls without
module metadata also reject a mirror/internal name collision.
Private conversion helpers are named `toGo_<Mirror>` and `fromGo_<Mirror>` (composed
types use deterministic composite names). Lists and maps are copied recursively
in both directions. Map/Set input keys are sorted before building the persistent
collection; §10.7 specifies Float NaN-key boundaries.

Conversions copy on every call, with O(size of arguments and result) copy cost.
Exported functions SHOULD therefore accept a **projection** containing only what
the decision needs, rather than whole observed objects.

For example, `func fetch(id Int) IO[Observed]` is exported as
`func Fetch(id int64) func(context.Context) (Observed, error)`.
Call `Fetch(id)(ctx)` to run the effect. The context reaches the underlying
`rt.IO`; success is converted to the mirror, and failure returns the zero result
and a `*rt.Failure` error. The wrapper does not run the IO while constructing it.

#### Integrating with go build

Place this directive in a Go file of the consumer package, beside `core.gt`,
with the intended `go4t` version on `PATH`:

```go
//go:generate go4t build --emit-go core --package core core.gt
```

Run `go generate ./...` and commit `core/go4t_export.go` **and all sibling
generated files**. Ordinary `go build` then compiles the checked-in Go package;
it does not invoke go4t or run generation automatically. In CI, use the same
go4t version and run:

```sh
go generate ./... && git diff --exit-code
```

This catches generated-code drift. See the Go shell example's README for its
temporary-module recipe; its `ignore`-tagged example file is selected explicitly
with `go generate main.go`.

## 9. Known limitations

- No character type (use one-rune Strings), arbitrary-precision integers,
  general exception syntax or automatic global fiber joining.
  Mutable references are available as opaque `Ref[A]` cells through `std/ref`;
  allocation, reads and writes require IO.
- No expression type ascription (use typed lets/parameters); field access needs a
  known receiver type. `if` always needs `else`; use qualified `prelude.when` /
  `prelude.unless` for conditional effects. Bindings require irrefutable patterns.
- Rank-1 polymorphism only; no inferred polymorphic recursion, local type parameter
  declarations, multi-parameter classes, arbitrary specialized/overlapping instances,
  partial aliases, automatic currying or implicit numeric coercion.
- Deriving Show supports enums and single-field newtypes; other payload-bearing
  types need handwritten display instances. Higher-kinded deriving follows only supported
  last-parameter positions; deriving Traversable/Ord does not supply Functor/Eq for you.
- Pair Show/Debug/Eq/Ord instances do not imply arbitrary tuple-arity instances;
  JSON explicitly supports every tuple arity. Unit lacks Show/Ord. Fiber lacks
  standard class instances.
- JSON nullable Option encoding loses Some(null-like value) identity;
  unsupported/non-finite serialization is not silently repaired.
- Trusted externs can break purity. Foreign ABI errors are generation-time, not
  `check` errors. Building and running require a Go toolchain. Standalone programs
  use the embedded runtime. Generic Go functions bind by call-site monomorphisation;
  generic receiver methods and instantiated generic named types are still skipped.
- Native scalars/specialization are selective, not universal unboxing. Mutual or
  non-tail recursion can still consume stack; large allocations remain
  host-resource bounded. Lazy does not memoise.
  Runtime costs are workload-dependent. `bench/PERF.md` records go4t-versus-Go
  measurements for million-element collections, a million-step Float simulation,
  and a depth-20 tree.
  Persistent `Map` inserts allocate copied search-path nodes to preserve earlier
  versions; use `std/mutmap` inside IO for update-heavy throughput, then `freeze`
  when a pure persistent snapshot is needed.
- Laws do not infer comparison transitivity or ordered arithmetic under Int
  wraparound; variable divisors are tainted. There is no `requires` clause,
  higher-order quantification or Float law support. Encode guards as Bool
  equations and split them with `if`. `std/map` is modelled symbolically and eligible;
  `std/mutmap`, `std/ref`, channels and other handles require IO for observations
  and are never law-eligible. Extern-dependent laws may use `sample(N)`, run by
  `go4t test`, reported as tested rather than proved, and unusable as rewrite lemmas.
  `induction x` supplies constructor arms and induction hypotheses. Accumulator-style
  functions (including the library's length via a fold) generally need a
  generalized lemma over the accumulator, with the structurally decreasing
  argument before any changing accumulator arguments; a simple length induction
  hypothesis is not sufficient.
- Editor support is the deliberately small protocol surface in §8.5. Historical
  example READMEs can show pre-module spellings; use this specification and the
  checked tutorial for current imports and signatures. `selfhost/` is separately
  owned and is not covered by this language-documentation acceptance.

## 10. Boundary behaviour

The tables below consolidate the Pass 12 boundary contracts. Function names in
library tables are relative to the owning module in §7; an unqualified spelling
in a table does **not** add an implicit export. [stdlib.md](docs/stdlib.md) gives
signatures, one-line semantics and boundaries for every actual source declaration,
including exported low-level helpers and private JSON helpers.

### 10.1 Syntax, types and numbers

For money, use `float.showFloatFixed(value, decimals)` from `std/float` rather
than the shortest Float `show` form.

| Boundary | Behaviour |
| --- | --- |
| Float exponent without digits / source overflow | `bad float literal: exponent requires digits` / `bad float literal <text>`; `.5` and `1.` are not Float literals |
| Mixed Int/Float, Float `%` | Rejected; no implicit conversion |
| Int `+ - *`, negation, abs | Signed int64 wraparound; abs(minInt()) = minInt() |
| Int `/` and `%` | Truncate toward zero / dividend-signed remainder; zero divisor panics `runtime error: integer divide by zero`; minInt() / -1 = minInt(), minInt() % -1 = 0 |
| Float `/ 0.0` | Signed infinity or NaN, not an integer-division panic |
| Float equality/order | NaN never equals itself but compares EQ to another NaN, below every non-NaN; signed zeros equal and compare EQ |
| Float to Int | truncate/floor/ceil/round use Go conversion after rounding; NaN/infinite/out-of-range results are implementation-dependent, not saturated |
| Float parsing | Go ParseFloat string grammar (broader than source literals, including hex and `.5`); whitespace not trimmed; syntax/range error → None; valid NaN/infinity → Some |
| Float display | Same shortest `g` form as Debug, with `.0` for finite forms lacking a decimal point or exponent; NaN/+Inf/-Inf unchanged; fixed decimal formatting is ties-to-even, preserves zeros; negative precision means shortest fixed form |
| Float Debug | `strconv.FormatFloat(f, 'g', -1, 64)` plus `.0` when the result contains none of `.`, `e`, `NaN`, `Inf`; `0.0`, `-2.0`, `NaN`, `+Inf`, `-Inf`, `1e+21`, `1e-07` retain their structural forms |
| powInt | 0^0 = 1; multiplication wraps; negative exponent gives 1 for base 1, parity-dependent ±1 for -1, otherwise 0 (including base 0) |
| sqrt / absFloat / min / max | Negative finite sqrt → NaN; absFloat(-0) → +0; min/max ties retain first argument |
| Alias arity/cycle | Exact application required; `arity mismatch for alias Pair: want 1 arguments, got 0`; `type alias cycle involving Loop` |
| Alias instance specialization | Repeated/erased/nested variables or function head → `instance head must be a type constructor`; permutations of distinct direct variables allowed |
| Common fields | Missing constructor field wins over type mismatch; rigid declaration types must agree even if a particular instantiation could make them equal; update preserves constructor |
| Local recursion | Only bare-name lambda lets recurse; consecutive local funcs share one scope group; local own type parameters → `local functions cannot have type parameters`; duplicate name → `duplicate local function <name>` |
| Placeholder placement | Bare holes are whole arguments/operator-section operands; projection sections `(_.f)` / `(_.0)` allow selector chains and omit parentheses in the expected-type contexts of §4.5; each projection has one independent parameter; fixed expressions run per application |
| Constraints | Duplicate class on one parameter fails; Eq + Ord is legal and reuses Ord; absent rigid class diagnostics list constraints in order |
| Match coverage | Guards add no coverage; true/false are exhaustive; other literal domains need default; maximum five missing witnesses; List witnesses always use `[]`, `[_]`, `[_, _, ..]`-style syntax (§5.2) |
| List patterns | `[]` and `[p1, p2, ...]` match exact lengths; `[p1, p2, ..rest]` binds the remaining list without copying, including an empty tail; final `..` / `.._` ignores it; trailing comma allowed |
| List rest placement/name | Rest needs a preceding element (`list rest pattern needs a preceding element`); it must be last (`.. must be last in list pattern`); only a lower-case name or `_` may follow `..` (`list rest must be a lower-case name or _`) |
| List binding patterns | Refutable: let / lambda parameter / do bind report `let pattern must be irrefutable` / `lambda parameter pattern must be irrefutable` / `bind pattern must be irrefutable`; use `match` |
| Deriving | Show supports enums (source constructor names) and single-field newtypes (field Show), without requiring Debug; Ord needs Eq, Traversable needs Functor; unmapped functions allowed only where the generated class never examines them |
| Pure Lazy | Construction stores thunk; each force reruns it, no memoization |
| Huge counts/widths | Empty/negative ordinary inputs follow tables below; huge positive allocations can panic/exhaust resources, never silently truncate |

### 10.2 Lists and folds

`head` returns `Option[A]` (`None` on `Nil`), whereas `tail` returns `List[A]` directly (`tail(Nil) = Nil`, and a nonempty list returns its remaining elements).

List function names below belong to `std/list`, except implicit core combinators
and the explicitly imported class methods in `std/prelude`.

| Function(s) | Empty/singleton and other boundary behaviour |
|---|---|
| `Show[List[A]].show` | Empty renders `"[]"`; singleton renders `"[" + show(x) + "]"`; strings use their unquoted `Show` representation. |
| `Debug[List[A]].debug` | Empty renders `"[]"`; nonempty renders elements via `debug`, separated by `, ` inside brackets. Strings stay quoted; nested lists and record fields retain literal form, e.g. `[[1], []]` and `Row{items: [1, 2]}`. Cons/Nil construction does not affect rendering. |
| `Eq[List[A]].eq` | Two empty lists equal; empty/nonempty unequal; corresponding elements compared via `Eq`, with length mismatch unequal. |
| `Ord[List[A]].compare` | Empty precedes nonempty; lexicographic element comparison; equal shared prefixes compare by length. |
| `map` / `fmap` on List | `Nil → Nil`, callback not called; `[x] → [f(x)]`. |
| `flatMap` / `bind` / `concatMap` on List | `Nil → Nil`, callback not called; singleton returns `f(x)`; empty produced lists contribute no elements. `pure(x)` for List is `[x]`. |
| `catOptions` | Keep Some payloads in input order, discard None; empty/all-None → Nil; singleton Some(x) → [x]. |
| `mapMaybe` | Apply the callback exactly once per element, left-to-right; keep Some payloads in order, discard None; Nil → Nil without calling the callback. |
| `forEach` | `Nil → pure(unit())`, callback not called; singleton sequences one action; short-circuits according to the chosen Monad. |
| `mapM` | `Nil → pure(Nil)`, callback not called; singleton maps one action and wraps its value in a singleton list; preserves order. |
| `fold` | `fold(Nil, init, f) = init`; singleton is `f(init, x)`; left-to-right fold. |
| `length` | `length(Nil) = 0`; singleton length is 1. |
| `append` | `append(Nil, ys) = ys`; `append(xs, Nil) = xs` by value. |
| `filter` | `filter(Nil, p) = Nil`; singleton kept iff `p(x)`; order preserved. |
| `reverse` | Empty and singleton unchanged by value. |
| `zip` | Either input empty gives `Nil`; truncates to the shorter input, including singleton inputs. |
| `head` | `head(Nil) = None`; `head([x]) = Some(x)`; later elements ignored. |
| `tail` | `tail(Nil) = Nil`; `tail([x]) = Nil`, never an empty-list panic. |
| `take` | `take(n <= 0, xs) = Nil`, including `take(-1, xs)` and `minInt()`; empty input stays empty; `n > length(xs)` returns all elements. |
| `drop` | `drop(n <= 0, xs) = xs`; empty input stays empty; `drop(n >= length(xs), xs) = Nil`. |
| `sum` | `sum(Nil) = 0`; singleton is its element; signed Int overflow wraps. |
| `elem` | Empty gives false; singleton compares with `Eq`; stops at the first match. |
| `sortBy` | Empty/singleton unchanged; comparator not called for these inputs; equal elements retain input order. |
| `sort` | Same boundaries as `sortBy`; uses the element's `Ord.compare`. |
| `maximum` / `minimum` | Empty gives `None`; singleton gives `Some(x)`; ties retain the earliest element. |
| `zipWith` | Either list empty gives `Nil` with no callback; singleton pair gives `[f(x, y)]`; truncates to the shorter length. |
| `unzip` | Empty gives `(Nil, Nil)`; singleton `(a,b)` gives `([a], [b])`; order preserved. |
| `any` | Empty is false; singleton is `p(x)`; first true result short-circuits. |
| `all` | Empty is true; singleton is `p(x)`; first false result short-circuits. |
| `find` | Empty/no match gives `None`; singleton match gives `Some(x)`; first match wins. |
| `findIndex` | Empty/no match gives `None`; singleton match gives `Some(0)`; indices are zero-based. |
| `partition` | Empty gives `(Nil, Nil)`; singleton goes into exactly one side; each side retains input order. |
| `nub` | Empty/singleton unchanged; first representative of each `Eq` equivalence class retained. |
| `range` | `range(5, 1)` and `range(n, n)` give `Nil`; `range(n, n+1) = [n]`; `range(-2, 2) = [-2,-1,0,1]`. Half-open endpoints do not increment past `maxInt()` or decrement below `minInt()`. |
| `replicate` | `n <= 0` gives `Nil`, including `replicate(-1, x)` and `minInt()`; `n = 1` gives `[x]`. |
| `intercalate` | Empty outer list gives `Nil`; singleton outer list gives its inner list; empty separator is ordinary flattening. Empty inner lists still have separators between them: `intercalate([0], [[], []]) = [0]`. |
| `flatten` | Empty outer list or all-empty inner lists give `Nil`; singleton outer list gives its inner list. |
| `last` | Empty gives `None`; singleton gives `Some(x)`. |
| `init` | Empty and singleton give `Nil`; longer lists lose exactly the last element. |
| `splitAt` | `n <= 0` gives `(Nil, xs)`; `n >= length(xs)` gives `(xs, Nil)`; empty input gives `(Nil, Nil)` for every n. |
| `span` | Empty gives `(Nil, Nil)`; immediate predicate failure gives `(Nil, xs)`; all true gives `(xs, Nil)`; stops at first false. |
| `takeWhile` | Empty or immediate predicate failure gives `Nil`; all true returns all elements; stops at first false. |
| `dropWhile` | Empty or all true gives `Nil`; immediate predicate failure returns original list; stops at first false. |
| `lookup` | Empty or absent key gives `None`; duplicate keys return the first value; key equality uses `Eq`. |
| `indexed` | Empty gives `Nil`; singleton gives `[(0, x)]`. |
| `iterate` | `n <= 0` gives `Nil` without calling `f`; `n = 1` gives `[x]` without calling `f`; positive n invokes `f` exactly n-1 times. |
| `product` | Empty gives 1; singleton is its element; zero produces zero; Int multiplication overflow wraps. |
| `count` | Empty gives 0; singleton gives 1 or 0 according to its predicate. |
| `unfold` | Immediate `None` gives `Nil`; one `Some((x, seed))` followed by `None` gives `[x]`; a step that never returns `None` does not terminate. |
| `chunksOf` | Empty gives `Nil`; `n <= 0` gives `Nil` (including `chunksOf(0, xs)` and negative/minimum counts); singleton positive input gives `[[x]]`; positive n beyond the length gives `[xs]` for nonempty xs; incomplete final chunks are retained. |
| `sortOn` | Empty gives `Nil` without calling key; singleton returns its value and computes its key once; keys compare through `Ord` and equal keys preserve input order. |

Foldable `foldr` on an empty List/Option returns init; `foldl` is left-associated
through continuations. `toList`/`foldMapList` preserve fold order. Traversal of an
empty structure calls pure without invoking the callback. List Debug and Show
both use brackets, with Debug or Show respectively for each element.

Accumulator helpers remain exported from `std/list` (their names do not begin
`_`). They are not separate conventions; normal entrypoints supply the initial
accumulator. Here `rev(acc)` means `reverse(acc)`.

| Helper | Boundary/result rule |
|---|---|
| `zipWithAcc(xs, ys, f, acc)` | Either input empty returns `rev(acc)`; otherwise emits pairs as `zipWith`, following that prefix. |
| `findIndexFrom(xs, p, i)` | Empty/no match gives `None`; first match at offset j gives `Some(i+j)`. |
| `rangeAcc(lo, hi, acc)` | `lo >= hi` returns acc; otherwise ascending `[lo, hi)` prepended to acc. |
| `replicateAcc(n, x, acc)` | `n <= 0` returns acc; positive n prepends n copies. |
| `initAcc(xs, acc)` | Empty/singleton returns `rev(acc)`; otherwise that prefix followed by `init(xs)`. |
| `splitAtAcc(n, xs, acc)` | `n <= 0` gives `(rev(acc), xs)`; exhaustion gives `(rev(acc) ++ xs, Nil)` by value; otherwise accumulates the taken prefix before reversing. |
| `spanAcc(xs, p, acc)` | Empty gives `(rev(acc), Nil)`; immediate failure gives `(rev(acc), xs)`; otherwise extends prefix up to first failure. |
| `indexedFrom(xs, i, acc)` | Empty returns `rev(acc)`; otherwise that prefix followed by elements indexed starting at i. |
| `iterateAcc(n, x, f, acc)` | `n <= 0` returns `rev(acc)`; otherwise that prefix followed by `iterate(n,x,f)`, still no extra final callback. |
| `unfoldAcc(seed, step, acc)` | Immediate `None` returns `rev(acc)`; otherwise that prefix followed by the unfolding. |
| `chunksOfAcc(n, xs, acc)` | Empty or `n <= 0` returns `rev(acc)`; otherwise that prefix followed by the chunks. |

### 10.3 Strings and regex

Indexes/widths count Unicode runes, not bytes, grapheme clusters or terminal
columns. Literal substring matching uses Go byte-string matching without Unicode
normalization. Primitive display/equality helpers belong to `std/prelude`,
regex functions to `std/regex`, other string operations to `std/string`.

| Function(s) | Boundary behaviour |
|---|---|
| `showInt` / `showBool` | Int64 signed decimal (`0 → "0"`, minimum Int includes its minus sign); Bool renders `"true"` or `"false"`. |
| `showString` / `Show[String].show` | Identity, including empty strings; does not add quotes. |
| `eqString` / `Eq[String].eq` | Exact byte equality; empty equals empty. |
| `ltString` / `Ord[String].compare` | Go lexicographic byte order; empty precedes every nonempty string; equal strings compare `EQ`. |
| String `+` / interpolation | Concatenate; empty text is identity; interpolation uses `Show`, not quoted display. |
| `strLength` | Empty is 0; one astral rune is 1; `e` plus combining accent is 2. |
| `strConcat` | Empty list is `""`; singleton is its element; empty elements contribute nothing. |
| `join` | Empty list is `""`; singleton is its element without separator; empty elements still delimit fields. |
| `split` | Absent separator gives `[s]`; nonempty separator on empty input gives `[""]`; adjacent/trailing separators retain empty fields. `split("", s)` yields one-rune strings; `split("", "") = Nil`. |
| `lines` | Splits only on `\n` and drops exactly one trailing empty field. Empty input gives `Nil`; `"\n" → [""]`; `"a\n\n" → ["a", ""]`; CR is retained in CRLF input. |
| `words` | Empty/Unicode-whitespace-only input gives `Nil`; runs of Unicode whitespace delimit nonempty fields. |
| `trim` | Removes Unicode whitespace only at the ends; empty or all-whitespace becomes `""`. |
| `toUpper` / `toLower` | Empty unchanged; Unicode simple case mapping, not locale-specific or full case folding. |
| `startsWith` / `endsWith` / `contains` | Empty prefix/suffix/substring matches every string, including empty; a nonempty needle does not match empty input. |
| `replace` | All non-overlapping literal occurrences; absent old value leaves s unchanged. Empty old inserts new at the start, between runes and at the end; `replace("", "", "-") = "-"`. |
| `substring` | Half-open rune interval with bounds clamped to `[0, length]`; reversed/equal ranges empty. `substring("a日本", -1, 2) = "a日"`; end past length clamps; start past length or negative end yields `""`; even Int extrema are safe. |
| `chars` | Same as `split("", s)`; empty gives `Nil`. |
| `repeat` | `n <= 0` gives `""` (negative counts fixed to stop panicking); empty s gives `""`; positive n repeats exactly n times. |
| `parseInt` | Signed base-10 Int64. `"+5" → Some(5)`; leading zeros allowed; both signed bounds accepted. Overflow, empty, surrounding whitespace, underscores, decimal fractions, non-ASCII digits and `0x` notation give `None`. |
| `showPadLeft` | Decimal number including sign, right-aligned with spaces; width <= decimal length (including any negative width) leaves the decimal unchanged, never truncates. |
| `codePoint` | Empty gives `None`; otherwise first rune only, e.g. `"日a" → Some(26085)`; malformed UTF-8 decodes as U+FFFD. |
| `fromCodePoint` | Valid Unicode scalar gives its one-rune string, including NUL at 0. Negative, > U+10FFFF, or surrogate U+D800..U+DFFF gives U+FFFD; large Ints are checked before conversion, never wrap into another character. |
| `isDigit` | Empty false; every rune must be Unicode decimal digit, e.g. `"12٣９"` true, `"1a"` false. |
| `isAlpha` | Empty false; every rune must be Unicode letter, e.g. `"é日本Z"` true, `"a1"` false. |
| `isSpace` | Empty false; every rune must be Unicode whitespace, e.g. ASCII whitespace and U+2003 true, `" a"` false. |
| `isUpper` / `isLower` | Empty false; every rune must have the respective Unicode case property; digits, uncased letters, punctuation or the opposite case make the result false. |
| `indexOf` | First match in rune indices; no match gives `None`; empty sub gives `Some(0)` even on empty input; `indexOf("日🙂本", "🙂") = Some(1)`. |
| `padRight` / `padLeft` | Pad with spaces only when width exceeds rune count; nonpositive/minimum Int widths leave s unchanged; never truncate; empty string pads to positive width. |
| `reverseString` | Empty/singleton unchanged; reverses code points, so combining accents move independently of their base characters. |
| `splitOnce` | First occurrence only; no match gives `None`; separators at an end produce an empty side. Empty separator gives `Some(("", s))`, even for empty s. |
| `charAt` | Empty, negative/minimum Int index, or index >= rune count gives `None`; valid index gives exactly one rune; malformed UTF-8 decodes as U+FFFD. |
| `showQuoted` | Empty gives `"\"\""`; escapes quotes, backslashes, control characters using Go string syntax; printable Unicode remains readable. |
| `regexMatch` | Searches anywhere; empty pattern matches even empty input; no match or invalid pattern gives false. |
| `regexFind` | No match or invalid pattern gives `None`; an actual empty match gives `Some("")`, not `None`. |
| `regexFindAll` | No match or invalid pattern gives `Nil`; returns non-overlapping matches in input order. Empty pattern on `"日🙂"` gives `["", "", ""]`; on empty input gives `[""]`. Adjacent empty-match handling follows Go regexp. |
| `regexReplace` | Invalid pattern/no match leaves input unchanged; replacement uses `$1`, `${name}`, `$$` expansion, with absent groups expanded to empty. Empty pattern inserts at rune boundaries, including both ends. |
| `regexSplit` | Invalid pattern gives `Nil`; no delimiter match gives `[s]`; matched leading/trailing nonempty delimiters retain empty fields. Empty pattern splits nonempty input into runes. `regexSplit("", "") = Nil`, while a nonempty valid pattern on empty input gives `[""]` (Go regexp's explicit empty-input rule). |
| `regexCaptures` | Invalid pattern/no match gives `None`; first match's capture groups only, no whole-match element; groups ordered by opening parentheses. Unmatched optional group gives `""`; a matching pattern with no groups gives `Some(Nil)`, including empty pattern/input. |

### 10.4 Persistent collections

Maps/sets are immutable AVL trees: single-key operations use O(log n) comparisons
and path-copying space, sizes are cached O(1), traversals O(n). The Ord comparator
must define one consistent total ordering: EQ identifies a single key/element.
`By` helpers have an explicit comparator and must use the same ordering throughout
a tree's lifetime. Building ordered collections requires Ord, not Go hashing;
Show/Eq/Debug of existing collections require only their component dictionaries,
not Ord.

| Operation | Semantics |
| --- | --- |
| `emptyMap` | Empty map, with no key constraint. |
| `mapInsert` | Insert/replace the key and value; preserve the old map. |
| `mapLookup` | `Some(value)` if present, otherwise `None`. |
| `mapDelete` | Remove a key; return the original tree when absent. |
| `mapMember` | Whether the key is present. |
| `mapSize` | Number of distinct keys; constant time. |
| `mapFromList` | Build from pairs; the last pair wins for duplicate keys. |
| `mapToList` | Pairs in ascending key order. |
| `mapKeys` | Keys in ascending order. |
| `mapValues` | Values in ascending key order, not sorted by value. |
| `mapAdjust` | Apply `f` exactly once to an existing value; do not call `f` if absent. |
| `mapAlter` | Call `f` exactly once on the current `Option`; `Some` inserts/replaces, `None` deletes. |
| `mapUnion` | Left-biased union; a duplicate keeps the left key/value. |
| `mapMap` | Map values in ascending-key order without needing `Ord`; keep keys and tree shape. |
| `mapFold` | Left fold in ascending-key order; an empty map returns `init`. |
| `groupBy` | Group under ascending keys, preserving input order within every group. |
| `Debug[Map[K, V]]` | `{k: v, k2: v2}` using Debug[K]/Debug[V] in ascending key order; empty `{}`; no Show/Ord required. |

| Operation | Semantics |
| --- | --- |
| `emptySet` | Empty set without an element constraint. |
| `setInsert` | Add an element; an equivalent existing element is retained. |
| `setDelete` | Remove an element; absent elements leave the set unchanged. |
| `setMember` | Whether an equivalent element exists. |
| `setSize` | Distinct element count; constant time. |
| `setFromList` | Deduplicate; retain the first representative of equivalent elements. |
| `setToList` | Elements in ascending order. |
| `setUnion` | Elements in either set, retaining the left representative on overlap. |
| `setIntersection` | Left elements also present in the right set. |
| `setDifference` | Left elements absent from the right set. |
| `Debug[Set[A]]` | `#{a, b}` using Debug[A] in ascending element order; empty `#{}`; no Show/Ord required. |

| Operation | Semantics |
| --- | --- |
| `arrayFromList` | Preserve list order. |
| `arrayToList` | Preserve array order. |
| `arrayLength` | Element count; constant time. |
| `arrayGet` | Zero-based O(1) access; `None` for negative indexes or `i >= length`. |
| `arraySet` | Copy and replace at a valid index; return the original unchanged outside `[0, length)`. |
| `arrayMap` | Map in ascending index order into a new array. |
| `arrayFold` | Left fold in ascending index order; empty arrays return `init`. |
| `arrayPush` | Copy and append one element. |
| `arraySlice` | Half-open `[start, end)`; clamp both bounds to `[0, length]`; reversed/empty intervals yield an empty array. |
| `arraySortBy` | Stable sort of a copy using `Ordering`; never modify the input. |
| `Debug[Array[A]]` | `#[a, b]` using Debug[A] in index order; empty `#[]`; no Show required. |

Map Show is `{k: v}`, Set Show `#{a, b}`, Array Show `#[a, b]` (empty `{}`, `#{}`,
`#[]`). Eq compares ordered lists/pairs or index-order elements; Functor maps Array
elements / Map values only. Arrays copy on update/push/stable sort; slices may
share immutable storage, but empty slices do not retain the backing allocation.

### 10.5 IO, Result and process boundaries

| Function | Boundary behaviour when the IO is run |
|---|---|
| `println` | Empty text still prints a newline; output errors fail the IO. |
| `readLine` | Empty EOF gives `None`; a blank line gives `Some("")`; LF/CRLF terminators are stripped; a final unterminated nonempty line is `Some(line)`; other read errors fail the IO. |
| `readAll` | Empty remaining input gives `""`; includes bytes already buffered by prior `readLine` calls; read errors fail the IO. |
| `args` | No program arguments gives `Nil`; executable name excluded; empty argument strings retained. |
| `getEnv` | Absent name gives `None`; present empty value gives `Some("")`; values are not trimmed. |
| `ioFail` | Every message, including empty text, constructs a deferred `Failure(msg)`; no output or failure occurs merely by constructing it. |

| Operation | Boundary |
| --- | --- |
| ioPure / ioBind | Values already evaluated before action construction; bind skips continuation after IO failure |
| Result fmap/bind / pure | Map only Ok; first Err skips continuation; pure is Ok; Eq compares constructor/payload, Ord places Err before Ok |
| fromOption/fromResult | Default used for None/Err but evaluated strictly before call; maybe/either invoke only selected callback |
| tryIO | Deferred/repeatable `IO[Result[io.IoError, A]]`; typed failures preserved; action panic becomes Panic(payload text without prefix); cannot capture construction panic or process exit |
| ioRaise / ioFailWith | Raise the supplied IoError unchanged / raise Failure(show(e)); arguments and Show rendering are strict, failure is deferred |
| ioErrorMessage / Show[IoError] | Payload text; Timeout → `context deadline exceeded`, Cancelled → `context canceled`; Debug retains constructors and quotes String payloads |
| readFile | Read entire contents; missing path fails with NotFound; other filesystem errors classified as in §6.2 |
| foldLines | Deferred file-line fold in source order; empty file returns init without calling step; LF/CRLF stripped, bare CR retained, final unterminated line delivered; no fixed line-length limit; streaming storage scales with the longest line plus the accumulator |
| foldLines / forEachLine | Missing path → NotFound; permission errors → PermissionDenied; other filesystem and step errors classified as in §6.2; first failed step or observed cancellation stops processing; cancellation checked between lines and after the final step; opened file closes on completion, failure, cancellation or panic |
| forEachLine | Defined through foldLines with a Unit accumulator; invokes f once per line in order; empty file returns unit() without invoking f |
| writeFile / appendFile | Create parent directories (0755) and files (0644), subject to umask; overwrite / append; write and close errors fail |
| fileExists | Missing → false; existing file/directory → true; other stat errors fail |
| listDir | Sorted entry names, including directories, not full paths; empty directory → Nil |
| removeFile | File or empty directory removed; absent path or nonempty directory fails |
| print / eprintln | No-newline stdout / newline stderr; stream errors fail |
| nowMillis / monotonicMillis | Unix milliseconds / elapsed monotonic milliseconds since runtime initialization, sampled when run |
| formatTime | Pure UTC; accepts Go layout, including negative Unix milliseconds |
| randomInt | Inclusive signed range, even full Int domain; equal endpoints fixed; lo > hi → `randomInt: lo must be <= hi`; not cryptographic |
| randomFloat | Uniform Float in [0,1), sampled when run; never fails; same automatically seeded RNG as randomInt; not cryptographic |
| sleep | Nonpositive immediate; positive duration overflow → `sleep: duration out of range` |
| exit / run | Explicit process exit not recoverable; installed run preserves status; outer Go launcher collapses nonzero to 1 |
| RunMain failure/panic | Ordinary error → `error: <msg>`, exit 1; action panic → `error: panic: <msg>`, exit 2; construction panic remains outside recovery |

### 10.6 Concurrency

| Operation | Boundary |
| --- | --- |
| fork | Deferred; each run creates a child context and distinct fiber; worker failure/panic cached |
| await | Repeat/concurrent awaits safe; action never reruns; caller cancellation or unfinished-fiber cancellation ends the wait; completed outcome stays cached |
| cancel | Deferred, idempotent cancellation request; returns Unit without waiting; unfinished-fiber awaits fail with typed Cancelled (`errors.Is(err, context.Canceled)`); completed outcomes are unchanged |
| parMapIO | Empty → Nil, no mapper calls; mapper/action deferred and panic-protected; results in input order; first failure cancels siblings, then joins |
| parBoth | Joins both; pair in argument order; first observed failure retained, not fixed left bias; failure alone does not cancel sibling |
| race | First completion including failure; cancels and joins loser, abandoning uncooperative code after 100 ms; completion ties unspecified |
| withTimeout | <= 0 → typed Timeout without starting; overflow → `withTimeout: duration out of range`; expiry is Timeout (`errors.Is(err, context.DeadlineExceeded)`); timely failure propagates |
| timeout | Option form of withTimeout; <= 0 → None without starting; overflow → `timeout: duration out of range`; success including erased nil → Some; own expiry → None; timely errors/enclosing deadlines propagate |
| Timeout cleanup | Expired action cancelled and joined until its deadline + 100 ms; uncooperative externs may outlive this bound |
| Duration bound | Positive milliseconds must be <= MaxInt64 / 1,000,000 (Go time.Duration nanoseconds) |
| Worker panic | Typed `Panic(fmt.Sprint(value))` IO failure without a `panic:` message prefix, including losing/timed-out tasks; awaiting it is an error path, not RunMain direct panic |
| Lifetime | Cancellation propagates to children; no global join; uncooperative tasks can delay normal parMapIO/parBoth joins; enclosing cancellation bounds cleanup to 100 ms |

### 10.7 JSON and foreign ABI

| Boundary | Behaviour |
| --- | --- |
| JSON parse | Exactly one value; surrounding whitespace accepted; integer literals without decimal/exponent syntax that fit int64 become JInt, other numbers become JNum(Float); trailing/malformed/Float-overflow input → Err with `invalid JSON: ` prefix |
| JSON printing | Compact or two-space pretty without trailing newline; Int prints exact decimal digits without `.0`; Float retains encoding/json formatting, including integral output and exponent thresholds; HTML/Unicode separators escaped; non-finite Float panics |
| Object order | Parsed source order / derived declaration order; duplicate keys first position/last value; ordinary Map encode lexicographic |
| Numeric round trip | Int round-trips exactly across the full int64 range. Float decoding accepts JInt via conversion. Int decoding accepts JInt directly or integral JNum values within inclusive [-2^53, 2^53]; other values return `expected Int at <path>`. Integer `-0` normalizes to `0`; Float negative zero prints `-0` |
| Option codec | null always None; Some(null-like value) indistinguishable from None |
| Records | Every field required, Option included; unknown fields ignored; first declaration-order error |
| Positional/tuple decode | Exact array length; constructor tags required except sole record; unknown tag reports `<Type> constructor` |
| Decode paths | `$` root, identifier `.field`, quoted key brackets, zero-based indexes; nested first failure preserved |
| Natural extern Option | None → zero/false; present zero not mistaken for absence; only primitive Option payloads expand |
| Natural extern lists | Nil/empty Go slices → Nil; conversions copy and preserve order, no mutable slice aliasing |
| Natural Go types | True aliases accepted, distinct named types require a shim; Go int narrowing follows target width; no-result functions need a shim for Unit |
| Extern errors | `(T, error)` conversion/call occurs inside IO; creation is lazy; each run repeats Go call; check alone does not validate foreign ABI |
| bind inference | Does not guess Option inputs; `--shim` adds the foreign mirror table in §8.3; unsupported signatures retain reason comments; declarations are toolchain-dependent |
| bind method handles | `--shim DIR` requires a Go module; handles cross as any and must not be constructed, matched, derived or shown; a wrong receiver type panics with `go4t shim: Type_Method: receiver is <dynamic Go type>` |
| Foreign struct copy | Exported fields and nested collections/pointers copy both ways; unexported state is zeroed on input; pointer-receiver changes do not mutate the go4t record; private-state and nonmirrorable structs remain handles |
| Foreign time | Duration is nanoseconds; Time uses UnixNano, zero time → 0, 0 → zero time; Unix epoch and zero time are therefore indistinguishable at this boundary; locations/monotonic clock metadata are lost |
| Foreign any decode | nil → Null; bool → Bool; signed/unsigned integer widths → Int (uint64 wraps by Go conversion); float32/64 → Float; string → Str; []byte → Bytes; time.Time → Time; []any → List; map[string]any → Map; all other values → Str(fmt.Sprint(value)) |
| Foreign any encode | Null → nil; Bool/Int/Float/Str → bool/int64/float64/string; Bytes → copied []byte; Time → time.Time; List/Map → recursively copied []any/map[string]any |
| Foreign callback error | A callback returning error executes go4t IO with its own Go context or the enclosing context, returning its error to Go; the outer fallible shim preserves an existing `*rt.Failure` (including its message/cause) through wrappers; newly classified errors retain the original Go error as `Cause`, accessible through `io.tryIOCause` / `io.cause` |
| Foreign Scan shape | Only methods with sole `...any` parameter and sole error result; count allocates slots; errors stop decoding; negative count is an IO failure |
| Runtime shapes | Generated constructors registered once; unregistered Tuple2/Tuple3/Ordering/Ok/Err/List bridge use panics (programming/configuration error), not a source value |
| Exported List/Map/Set | Copy recursively in both directions, with no mutable slice/map aliasing; nil/empty inputs become empty go4t collections; empty List returns a nil slice and empty Map/Set returns an allocated empty Go map |
| Exported Float collection keys | Use go4t's NaN-low Ord; a single NaN key is retained. Multiple NaN keys in a Go map/set cannot remain distinct under that Ord and panic with `go4t: duplicate NaN map key` / `go4t: duplicate NaN set key`, avoiding an iteration-order-dependent survivor |
| Exported enum | Zero value is the first declared constructor; `String()` is the source constructor spelling |
| Exported parametric mirror name | A clash with a user-declared type produces `Go export name collision: <name>` at that user declaration |
| Exported function name | Type/constructor mirrors keep their names; a colliding function receives `Fn`. An already-taken suffixed name, or a collision with another function, still produces `Go export name collision: <name>` |
| Exported invalid constructor | Unknown enum value, nil sum, or unsupported constructor representation panics with `invalid constructor`; sum constructors cross as values, not pointers |
| Exported IO | Result-only context-taking function; returns converted success or zero result and `*rt.Failure`; no nested/parameter IO or opaque `any` fallback |

### HTTP boundaries

| Operation | Boundary |
| --- | --- |
| `request`, `get`, `post` | Deferred/replayable; 30 s overall budget; HTTP 4xx/5xx are Responses, transport errors are Network, expiry Timeout, cancellation Cancelled |
| `requestWithTimeout(r, millis)` | Nonpositive budget fails Timeout without sending; duration overflow fails `Failure("requestWithTimeout: duration out of range")`; parent cancellation is checked first; body reads share the deadline |
| Request URL/path/query | Absolute URL required; nonempty path overrides URL path, explicit query pairs append; invalid URL/method/scheme fails Network |
| Headers | Repeated pairs preserved; lookup is case-insensitive and first-match; absent is None, present empty is Some(""); Host is supported on both sides |
| `response`, `jsonResponse` | Pure constructors; first has no headers, second encodes via Encode and sets application/json; strict JSON encoding can panic before an IO is constructed |
| `serve`, `serveOn`, `serveOnReady` | Binding is deferred; bind failure is Network; cancellation stops accepting and allows up to 5 s for shutdown before force-close. `serve` preserves all-interface binding; `serveOn`/`serveOnReady` select the host explicitly (`"127.0.0.1"` for loopback, `""` for all interfaces). |
| Readiness | `serveOnReady` invokes its callback once after successful binding with the actual port, including when requested port is 0; callback completes before accepting. Callback failure/panic closes the listener and propagates. Fork alone is not a readiness signal. |
| Handler contexts | One goroutine per request; client disconnect or server cancellation reaches its IO context; handlers are concurrent, not sequential |
| Handler failure/panic | HTTP 500, exact failure message body (no added newline), stderr `http handler: <message>` plus newline; only Cancelled with an already-done request context suppresses logging, and normal shutdown is silent; construction and action panics use Panic failure |

### 10.8 Modules, diagnostics and tools

| Boundary | Behaviour |
| --- | --- |
| Directory inputs | check/build/run/test/eval load immediate .gt only; fmt recursively loads regular .gt; explicit entry files must share directory |
| Import search | Nearest source root before matching go4t.mod requires (longest prefix, project Go graph) before GO4T_PATH before embedded std; first existing directory wins even invalid/empty |
| Privacy / identity | `_` private outside owning module; aliases file-scoped; cycles deduplicated; global duplicate instances name both modules |
| Prelude shadow | Local wins, top-level shadow warns; no implicit JSON or non-listed methods/functions |
| Diagnostic spans | One-based rune starts/exclusive ends; CLI caret clamps first line; missing end gives one caret; warnings exit 0 |
| test | Empty suite succeeds; only explicit Bool/IO[Bool], monomorphic zero-arg `test` plus suffix; user main removed before checking; false/IO failure continues, panic uses ordinary runtime policy |
| eval | One expression; imports from first entry file; Show needed; failed pure and IO wrappers report original error; no special Unit display |
| build | Executable by default; -o names a file and rejects existing directories; --emit-go emits sources only; --package requires --emit-go and defaults to main; generated sources and unrelated Go files are preserved |
| init | Refuses either existing manifest; resolved go4t Dir wins over content-addressed user-cache runtime; no project-local runtime extraction |
| get | Exactly path@version; requires go.mod beside nearest go4t.mod; Go failure leaves go4t.mod unchanged; success updates without duplicate requires |
| fmt | No typecheck/main required; -l differences exit 1, -d differences exit 0; invalid file not written; earlier writes survive later error |
| LSP overlays | Full sync and increasing versions only; stale/incremental/unopened changes ignored; close restores disk; failed check drops index |
| LSP framing | 16 MiB body / 8192-byte header limit; malformed/truncated framing errors; EOF ends cleanly |
| LSP lifecycle | shutdown → null, exit after shutdown succeeds; premature exit errors `LSP exit before shutdown`; unknown notifications ignored; cancellation/trace notifications accepted synchronously |
| Native scalars | Polymorphic/data/dictionary/runtime boundaries still boxed; aliases share representation |
| Specialization | <= 40 AST nodes, <= 8 concrete dictionary tuples; 41 nodes/ninth distinct tuple fall back; copies reuse exact tuple and preserve residual polymorphism |
| TCO | Direct same-binding self tail calls only; simultaneous parameter update; unchanged dictionaries and fresh closure-visible bindings |

LSP request error codes:

| Code | Message / condition |
| --- | --- |
| -32700 | `Parse error` — malformed JSON body |
| -32600 | `Invalid Request` — malformed JSON-RPC envelope or ID |
| -32002 | `Server not initialized` — request before initialize |
| -32600 | `Server already initialized` — duplicate initialize |
| -32600 | `Server has shut down` — request after shutdown |
| -32602 | `Invalid params`, `Invalid position`, `expected an absolute file URI`, or `expected a .gt document` |

Module errors include `invalid import path "PATH"`, `module "PATH" not found`,
`entry files must belong to one directory`, `no .gt files in DIR`,
`invalid go4t.mod in DIR: expected module <name>`,
`invalid go4t.mod in DIR: line N: expected require <path> <version>`,
`invalid go4t.mod in DIR: duplicate require PATH`,
`resolve dependency PATH: go list -m -json PATH: ...`,
`package NAME differs from NAME`,
`duplicate or invalid import alias ALIAS`, `unknown import alias ALIAS`,
`module MODULE has no exported name NAME`, `NAME is private to module MODULE`,
and `duplicate instance CLASS[TYPE] in modules FIRST and SECOND`.
