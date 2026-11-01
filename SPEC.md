# gofx Language Specification

gofx is a statically typed, strict, functional language with Go-flavoured syntax,
higher-kinded types, type classes, immutable algebraic data, and explicit `IO`.
It compiles to Go. This document describes the merged Pass 12 implementation;
[the tutorial](docs/tutorial.md) teaches it and [the standard library](docs/stdlib.md)
lists every source declaration. `DESIGN.md` records historical design contracts,
not an alternative definition of current behaviour.

*Error* means compile-time rejection; *warning* is nonfatal. Grammar below is
schematic EBNF: separators include explicit and layout-inserted semicolons,
comma-separated syntax permits the parser's optional trailing comma, and `Name`
is an identifier optionally qualified by a file-local import alias. Complete
`gofx` fences are checked by `docs/check_test.go`; `gofx error` fences must fail.

## 1. Lexical structure

### 1.1 Source and comments

Source is UTF-8. A module consists of `.gfx` files sharing a `package` spelling
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
else true false given deriving`.

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

### 1.4 Layout

A newline inserts `;` when the previous token can end an expression (identifier,
Int, Float, String/interpolated string, Bool, `)`, `]`, `}`, `_`), the innermost
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
File       := 'package' Ident {Separator} {Import} {Decl}
Import     := 'import' [Ident] String {Separator}
Decl       := Class | Instance | TypeDecl | AliasDecl | Func | Extern
Name       := Ident ['.' Ident]
```

Imports precede declarations. Top-level declarations in one module are mutually
visible, including recursion across files. `check`, `build` and `run` require an
entry `func main() IO[Unit]`; `test` and `eval` replace the entry (§8).
Types/classes, constructors and values have separate namespaces, so
`type Sku = Sku(String)` is legal. Duplicate declarations within a namespace are
errors. Record field names belong to their constructors. Builtin types cannot
be redeclared.

### 2.2 Modules and resolution

A directory input loads its immediate `.gfx` files in lexical filename order,
not recursively. Explicit filenames select only those entry files, all in one
directory and with one package spelling. Imported modules load every immediate
`.gfx` file. Module identities are absolute directories (embedded std identities
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

1. The importing module's nearest ancestor `gofx.mod`, containing one line
   `module <name>`; absent a file, the entry program's directory is the root.
   An exact module name refers to the root; a `<name>/` prefix is stripped.
2. Nonempty `GOFX_PATH` entries in platform path-list order.
3. The compiler's embedded `std/` sources for `std/...` imports.

The first existing directory wins, even if it is empty or invalid; errors do
not fall through. A filesystem std override retains its logical std identity
for intrinsic types and reexports, alongside its absolute-directory identity.

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
`Int Float String Bool IO Map Set Array Fiber` and tuple types are independent of
imports. All instances in loaded modules participate in resolution.

Module declarations and lexical bindings win over implicit names. Top-level
shadowing warns `shadows prelude name X`; it is legal. Explicitly import
`std/prelude` for `prelude.fmap`, `prelude.pure`, `prelude.bind`,
`prelude.foldr`, and `prelude.traverse`: those methods are **not** implicit.
Other library operations require their owning module, e.g. `string.trim` or
`list.sort`. §7 lists the distribution.

```gofx
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
| `IO[A]` | `(*) -> *`; deferred action producing A or failing |
| `Fiber[A]` | `(*) -> *`; opaque concurrent result handle |
| `Map[K, V]` | `(*, *) -> *`; persistent comparator-ordered map |
| `Set[A]` | `(*) -> *`; persistent comparator-ordered set |
| `Array[A]` | `(*) -> *`; immutable random-access sequence |
| `TupleN` / `(A, B, ...)` | N parameters, N ≥ 2 |

Opaque builtins have no source constructors to pattern-match. Data-type and
alias parameters have kind `*`; function/class parameters can have higher kinds.

### 3.2 Type expressions and parameters

```ebnf
Type    := Name ['[' Type {',' Type} ']']
         | 'func' '(' [Type {',' Type}] ')' Type
         | '(' Type ')' | '(' Type ',' Type {',' Type} ')'
TParams := '[' TParam {',' TParam} ']'
TParam  := Ident ['[' '_' {',' '_'} ']'] [Name {'+' Name}]
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

### 3.3 Algebraic data and aliases

```ebnf
TypeDecl  := 'type' Ident ['[' Ident {',' Ident} ']'] '=' Ctor {'|' Ctor}
             ['deriving' Name {',' Name}]
Ctor      := Ident | Ident '(' Type {',' Type} ')'
           | Ident '{' Ident ':' Type {',' Ident ':' Type} '}'
AliasDecl := 'alias' Ident ['[' Ident {',' Ident} ']'] '=' Type
```

Constructors are nullary, positional, or record-form. Records also support
positional construction and patterns. Recursive and parameterised data are
allowed; deriving rules are in §7.2. There are no zero-field record constructors.

Aliases share the type namespace, introduce no constructors, and expand before
inference, field checking and instance resolution. They are transparent, not
Go-style distinct named types. Forward references/chains work; all aliases,
including unused ones, are validated, and cycles are errors. Every alias use must
supply exactly its parameters: aliases cannot themselves be partially applied,
although a fully applied alias may expand to a higher-kinded constructor. Expansion
uses declaration-scope parameter identity; diagnostics display expanded types.

### 3.4 Functions and extern declarations

```ebnf
Func   := 'func' Ident [TParams] '(' [Param {',' Param}] ')' Type Block
Param  := Ident Type
Extern := 'extern' 'func' Ident [TParams] '(' [Param {',' Param}] ')' Type
          '=' String '.' Ident
```

Top-level functions have explicitly declared rank-1 type parameters and return
types. Their final expression must match the return type. Extern declarations
bind exported package-level Go functions; `check` validates only the gofx type.
`build`/`run` inspect foreign signatures and generate adapters (§8.3).

### 3.5 Classes and instances

```ebnf
Class      := 'class' Ident '[' TParam ']' [':' Name '[' Ident ']']
              '{' {MethodSig} '}'
MethodSig  := Ident [TParams] '(' [Param {',' Param}] ')' Type
Instance   := 'instance' Name '[' Head ']' ['given' Constraint {',' Constraint}]
              '{' {Ident '(' [Ident {',' Ident}] ')' Block} '}'
Head       := Name ['[' HeadArg {',' HeadArg} ']']
            | '(' Ident ',' Ident {',' Ident} ')'
HeadArg    := Ident
Constraint := Name '[' Ident ']'
```

Classes have one parameter, possibly higher-kinded, and at most one superclass
on that parameter. Methods may add their own parameters/constraints. Each method
must be implemented exactly once with matching value arity. A subclass instance
requires its superclass instance with compatible head and context.

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
Expr       := 'func' '(' [LParam {',' LParam}] ')' '=>' Expr
            | 'do' '{' {DoStmt Separator} Expr [Separator] '}'
            | 'match' Expr '{' Arms '}'
            | 'if' Expr Block 'else' (Block | IfExpr)
            | Binary
LParam     := Ident [Type] | IrrefutablePattern
Binary     := Unary {BinOp Unary}                    // precedence below
Unary      := ['-' | '!'] Postfix
Postfix    := Primary {'(' [CallArg {',' CallArg}] ')' | '.' Ident | '.' Int | RecordFields}
CallArg    := Expr | '_' | Projection
Primary    := Name | Int | Float | String | Interp | 'true' | 'false'
            | Name RecordFields | '(' Expr ')' | '(' Expr ',' Expr {',' Expr} ')'
            | '[' [Expr {',' Expr}] ']' | Block | Section
Block      := '{' {Stmt Separator} Expr [Separator] '}'
Stmt       := LetStmt | LocalFunc | Expr
LetStmt    := 'let' (Ident | IrrefutablePattern) [':' Type] '=' Expr
LocalFunc  := 'func' Ident '(' [Param {',' Param}] ')' Type Block
DoStmt     := (Ident | IrrefutablePattern) '<-' Expr | LetStmt | LocalFunc | Expr
Section    := '(' '_' BinOp Expr ')' | '(' Expr BinOp '_' ')' | '(' '_' BinOp '_' ')'
            | '(' Projection ')'
Projection := '_' '.' (Ident | Int) {'.' (Ident | Int)}
RecordFields := '{' Ident ':' Expr {',' Ident ':' Expr} '}'
```

Precedence, low to high: `|>`; `||`; `&&`; `== != < <= > >=`; `+ -`;
`* / %`; unary `- !`; postfix. Binary operators associate left. The grammar's
`Name` in expression position is resolved against imports versus record fields;
not every `x.f` is a qualified module name.

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

```gofx
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
not nested lambdas. A projection may also occupy a whole call argument without
parentheses: `map(xs, _.qty)` is `map(xs, func(x) => x.qty)`, so
`xs |> map(_.qty)` works too. Each projection has its own parameter scope:
`map(_, _.qty)` has one outer hole and an independent projection lambda.
Record types and tuple arity must be known from the call argument context or an
explicit function type annotation; projection sections use ordinary field-access
rules, including common fields across constructors (§4.6).

`f(_ + 1)`, `(_ + 1 + 2)`, `[_]`, `(_, 1)` and `(_)` are invalid; write
`f((_ + 1))`. Unparenthesised `_.f` outside a whole call argument is also invalid,
as are `f(_.f + 1)` and `(_.f + 1)`; use an explicit lambda for computations
beyond a selector chain. Invalid placeholder placement retains the diagnostic
`_ is only valid as a placeholder in a call or operator section`.

```gofx
package main
import "std/string"
func main() IO[Unit] {
  [1, 2, 3] |> map((_ + 1)) |> map(show) |> string.join(", ", _) |> println
}
```

```gofx
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

```gofx error
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
Arms    := ['|'] Arm {(';' | '|') Arm}
Arm     := Pattern ['if' Expr] '=>' Expr
Pattern := '_' | Int | Float | String | 'true' | 'false'
         | LowerIdent | LowerIdent '@' Pattern
         | ConstructorName ['(' Pattern {',' Pattern} ')']
         | ConstructorName '{' [Ident ':' Pattern {',' Ident ':' Pattern}] [',' '..'] '}'
         | '(' Pattern ',' Pattern {',' Pattern} ')' | '(' Pattern ')'
         | '[' [Pattern {',' Pattern} [',' ListRest] [',']] ']'
ListRest := '..' [LowerIdent | '_']
```

Constructors may be qualified/private within their owning module. Arms run in
order: the first matching pattern whose guard is true yields the value. All arms
have one type. Variables bind once per pattern; guards see them. Record patterns
must name all fields or use `..`; positional patterns also work for records.

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
output. A `do` block uses one Monad constructor throughout. `<-` extracts a value,
a bare action sequences and discards it, and the final expression is an action.
Irrefutable binds, lets and local functions are supported. Desugaring is nested
`bind`; it is not restricted to IO. Option stops on None; Result stops on Err;
List enumerates combinations in order; State threads its state. Use `ioPure`,
`Some`, `Ok`, or qualified `prelude.pure` for the final result as appropriate.

## 5. Type system and diagnostics

### 5.1 Inference

Inference is Hindley–Milner with rank-1 top-level/eligible-let polymorphism,
higher kinds, flat partial constructor unification, rigid declared parameters,
and post-inference type-class dictionaries. Parameters are never generalised.
For example `F[a] ~ Result[E, A]` can solve `F := Result[E]`, `a := A`.

Unresolved kind-`*` variables with no class constraint default to Unit with a
warning, e.g. `type of None defaulted to Unit; add an annotation or use the value`.
Unresolved higher-kinded or class-constrained variables fail. Arithmetic's Int
default is separate; no numeric-literal polymorphism is introduced.

```gofx error
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
deriving Show is only supported for enums (all constructors nullary); derive Debug and write instance Show[T] for a display form
x is unused; prefix with _ to silence
shadows prelude name map
```

User-data missing Debug/Eq/Ord hints suggest that deriving class or a handwritten
instance. Show hints suggest `deriving Show` only for enums (all constructors
nullary); other data types suggest a handwritten Show instance or `deriving Debug`
for structural output. Builtins, functions, rigid parameters and other classes do
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

`ioFail(msg)` is a deferred failure. Bind stops at the first failed action.
`io.tryIO(action)` executes lazily and returns `IO[Result[String, A]]`: success
becomes Ok; IO errors become Err(error text); recovered panics become
Err(`fmt.Sprint(value)`) **without** an added `panic:` prefix. Capture includes
bind continuations while running the action, not earlier argument construction.
`result.liftResult` turns Err(e) into `ioFail(show(e))`.

`rt.RunMain` reports ordinary IO failures as `error: <message>` and exits 1;
a panic while running its action prints `error: panic: <message>` and exits 2.
Success exits normally (0). A panic while constructing main's action is outside
that recovery and follows Go's panic/stack-trace path. `io.exit(code)` terminates
the process and is not recoverable. Installed `gofx run` preserves the child exit
status; an outer `go run ./cmd/gofx ...` adds Go's `exit status N` and itself
collapses nonzero exits to 1.

```gofx
package main
import "std/io"
func main() IO[Unit] {
  do {
    answer <- io.tryIO(ioFail("unavailable"))
    let typed: Result[String, Int] = answer
    println(show(typed))
  }
}
```

### 6.3 Concurrency

Import `std/conc`. `fork` starts an action on a goroutine **when run**, returning
an opaque Fiber. `await` returns its cached outcome; multiple awaits do not rerun
it. `parMapIO` starts all mapper/actions concurrently, joins all and returns
input-ordered results. `parBoth` joins both and returns argument-ordered values.
Both retain the first **observed** failure; concurrent ties are unspecified and
a blocked sibling still blocks the join.

`race` returns the first completion, failure included, not the first success.
`timeout` returns Some on timely success, propagates timely failure, or None on
expiry. Nonpositive timeout returns None without starting the action. Timers
start before launch; completion/expiry ties are unspecified. Positive duration
overflow fails rather than wrapping.

Worker panics, including mapper construction, become ordinary IO errors
`panic: <value>`; awaiting one therefore uses exit 1 unless handled. Race losers
and timed-out tasks can continue to perform effects: abandonment is **not
cancellation**. There is no cancellation API or global join; unawaited fibers do
not keep a completed main alive. §10 gives all edge cases.

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
| `std/prelude` | Classes above; Ordering/Option and primitive/pair/Option instances; core combinators |
| `std/list` | List/instances; folds, traversal, searching, slicing, sorting, `catOptions`, `mapMaybe`, accumulator helpers |
| `std/result` | Result/instances; Result and Option helpers; liftResult |
| `std/io` | IO instances, console, arguments/environment, files, clocks, randomInt, randomFloat, sleep, tryIO |
| `std/float` | Float instances, conversions/math/formatting; minInt/maxInt/abs |
| `std/string` | Rune-oriented strings, Unicode classification, parsing/padding |
| `std/regex` | Go-regexp matching, captures, replacement and splitting |
| `std/map`, `std/set`, `std/array` | Persistent collections and their loaded instances |
| `std/conc` | fork/await, parallel map/pair, race/timeout |
| `std/state` | State and Functor/Monad instances, state operations |
| `std/lazy` | Lazy, delay/force; explicitly **nonmemoising** |
| `std/json` | Json, Encode/Decode and codecs; not implicitly exported |

Functor/Monad are provided for Option, List, Result[E], IO and State[S];
Array and Map[K] have Functor. Option/List have Foldable/Traversable. Show/Eq/Ord
cover Int, Float, String, Bool, Ordering, Option, List, Result and pairs with
component constraints. Unit has Eq and Debug, **not Show or Ord**. Debug covers
Int/Float/String/Bool/Unit, Ordering/Option/List/Result and pairs. Map/Set/Array
have Show/Eq/Debug, not automatic Ord/Foldable; Fiber has no standard class
instances. The complete explicit and derived inventory is in `docs/stdlib.md`.

Float Show follows Go's `fmt.Println`: shortest `g` formatting, so `show(1.0)`
is `"1"`. Debug is structural: it appends `.0` to finite integral forms without
a decimal point or exponent, so `debug(1.0)` is `"1.0"` and remains distinct
from Int. Exponents and NaN/infinities are retained unchanged.

### 7.2 Deriving rules

Supported classes: **Eq, Ord, Debug, Show, Functor, Foldable, Traversable, Encode,
Decode** (Show only for enums; the last two qualified through a `std/json` import). Derivation
expands to ordinary checked instances; duplicate deriving names and explicit
instance conflicts are errors.

**Show is derivable only for enums (all constructors nullary).** It renders each
constructor as its unqualified source name: `show(C)` is `"C"`. Payload-bearing
data types require an explicit user-facing display instance; Debug remains
structural output. The exact rejection for any non-nullary constructor is:
`deriving Show is only supported for enums (all constructors nullary); derive Debug and write instance Show[T] for a display form`.

| Class | Generated contract |
| --- | --- |
| Eq | Equal constructor and corresponding equal fields; function fields unsupported |
| Ord | Constructor declaration order, then lexicographic fields; requires Eq instance; function fields unsupported |
| Debug | `C`, `C(f1, f2)`, `C{a: f1, b: f2}` via each field's Debug; strings quoted with Go escapes |
| Show | Enums only: every constructor must be nullary; `C` renders as `"C"` without quotes or module qualification in the output |
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

```gofx
package main
import p "std/prelude"
type Tree[A] = Leaf(A) | Branch(Tree[A], Tree[A]) deriving Eq, Ord, Debug, Functor, Foldable, Traversable
func main() IO[Unit] {
  let tree = Branch(Leaf(1), Leaf(2))
  println(debug(p.fmap(tree, (_ + 1))))
}
```

```gofx
package main
type Currency = JPY | USD | EUR deriving Show
func main() IO[Unit] { println("${JPY}|${USD}|${EUR}") }
```

```gofx error
package main
type Hidden = Hidden(Int) deriving Show
func main() IO[Unit] { ioPure(unit()) }
```

### 7.3 JSON

`std/json` defines `Json = JNull | JBool(Bool) | JNum(Float) | JStr(String) |
JArr(List[Json]) | JObj(Map[String, Json])`, `Encode[A].encode(a) Json` and
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

Numbers in Json are Float. Int encoding may round above binary64's exact range;
Int decoding requires a finite integral number in [-2^63, 2^63). Printing NaN or
infinity panics `cannot encode non-finite JSON number`. Parsing consumes exactly
one value, accepts surrounding whitespace, and rejects malformed/trailing input
or Float overflow with `Err("invalid JSON: " + reason)`.

Decode errors are `expected <what> at <path>`, rooted at `$`, with `.name`, quoted
`["a.b"]`, and zero-based `[2]` segments. Examples:
`expected String at $.items[2].sku`, `expected field at $.sku`,
`expected array of length 2 at $`, `expected Status constructor at $["$tag"]`.
Containers prefix nested paths and preserve the first error; lists/tuples use
index order, Map values key order.

## 8. Compilation and tooling

### 8.1 Go generation and performance (informative)

The compiler emits **one Go file per gofx module in one Go package**, not separate
Go packages. `main.go` is the entry; dependency basenames use sanitized path tails
and deterministic collision suffixes. Names/dictionaries are module-prefixed.
`codegen.Generate(*types.Program, string) (map[string][]byte, error)` is the public
generator. The module loader feeds `types.Check([]*ast.Module)`, with entry last.

Classes become interfaces and instances dictionary structs; constrained functions
receive dictionaries. Known Int/Bool/Float locals, parameters/results, pattern
bindings and tail-loop state use int64/bool/float64. Polymorphic values,
constructor/tuple fields, dictionary methods, extern wrappers and runtime calls
remain `any` boundaries. Higher-order values adapt native signatures to erased
callbacks; strings/aggregates remain erased. Native operators preserve overflow,
zero division, signed Float zero and NaN-low ordering, including literal operands.

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

Stable `list.sortBy` uses array-backed stable sorting, O(n log n) comparisons
and linear conversion space; the old quadratic `insertBy` helper is removed.
`sortOn` computes each key once; `nub` remains worst-case quadratic.

### 8.2 Commands and flags

```text
gofx check FILE.gfx|DIR...
gofx build [-o DIR | --emit-go DIR --package NAME] FILE.gfx|DIR...
gofx run FILE.gfx|DIR... [-- args...]
gofx test FILE.gfx|DIR...
gofx eval 'EXPR' [FILE.gfx|DIR...]
gofx bind IMPORT [FILE.gfx|DIR...] [-o FILE.gfx|DIR] [--prefix STR]
gofx fmt [-w|-d|-l] FILE...|DIR
gofx lsp
```

- `check` loads/checks only; no foreign-signature validation or execution.
- `build` emits Go **source, not a binary**. Default: `<first-file-stem>_gen/`,
  or `<DIR>/_gen/` for a directory. `-o` changes it. `--emit-go` and `--package`
  must occur together, are incompatible with `-o`, and expose `func Main() rt.IO`
  instead of Go main. NAME must be a valid non-keyword Go identifier other than
  `_` or `main`. Flags can precede/follow inputs. Rewrites remove obsolete
  compiler-generated Go files but preserve unrelated output-directory files.
- `run` emits, builds and executes the whole package, inheriting stdin/stdout/
  stderr and forwarding arguments only after `--`. Its generated sources remain;
  the executable is temporary. A Go module above generated/input paths must
  resolve `gofx/rt` and extern imports; installation alone does not configure
  arbitrary application directories. Use the checkout or a Go module with a
  `require`/`replace` for the checkout.
- `test` replaces user main **before checking**. It discovers non-extern,
  monomorphic top-level functions whose names start `test` with a nonempty
  suffix, no value parameters, and explicit `Bool` or `IO[Bool]` return syntax.
  Other functions are still checked. Tests run once in file/declaration order,
  print `PASS name` / `FAIL name` / `FAIL name: error`, then `N passed, M failed`.
  False/IO errors do not stop later tests; failures exit 1; an empty suite is
  `0 passed, 0 failed`, exit 0. Panics use ordinary runtime policy, not a new
  per-test recovery. Alias return syntax is not test discovery syntax.
- `eval` removes user main, checks `println(show(EXPR))`, then tries an IO bind
  wrapper if needed. If both fail it reports the **first** diagnostic. EXPR must
  be exactly one expression. Supporting imports are copied from the first entry
  file; absent files it has only implicit names. Results require Show, even IO
  results: Unit has no Show. Test/eval use temporary generated directories and
  do not replace ordinary build output. Source labels are `<test>` / `<eval>`.

### 8.3 Externs and bind

Foreign packages are inspected once per compilation using `go list` export data
and Go's parser/type importer in the module/build-constraint/toolchain context.
Each exported package-level function gets an adapter; methods, generics and
variadic functions are not extern targets.

| gofx type | Natural Go ABI | Boundary |
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
are **inside** the generated action. Re-running repeats the call; non-nil errors
propagate unchanged. For example `gofx/rt/stdhttp.GetStatus(string) (int64, error)`
returns any HTTP status, including non-2xx, or a network error.

`bind` emits a complete declaration file in lexical exported-function order,
stdout by default. `-o` names a file or an existing directory (`bindings.gfx`).
Receiving source files/directory select their package spelling and Go load
context; absent them it emits `package main`. Output path alone does not select
context. Parameters are `p0`, `p1`, etc.; names lower-case the first Go letter,
or prefix it (`--prefix str` produces `strToUpper`). Collision comments do not
rename names. Primitive `(T, bool)` results infer Option and `(T, error)` infer
IO; adjacent inputs are emitted independently, never guessed to be Option.
Unsupported functions receive `// skipped: Name (reason)`: pointers, callbacks,
unsupported integer widths/named types/multiple or absent results, generics,
variadics, and uninferable any/rt.IO. Explicit externs can still describe erased
or rt.IO functions that bind cannot infer. Reserved names suggest a prefix.

### 8.4 Formatting

`fmt` prints canonical source; `-w` rewrites changed files preserving permissions;
`-d` prints deterministic full-file unified diffs and exits 0;
`-l` lists unformatted paths and exits 1 if any. Flags are mutually exclusive;
`--` ends flag parsing. Directories recurse over regular `.gfx` files without
following directory symlinks; explicit files need not have a `.gfx` extension.
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

`gofx lsp` is a synchronous stdio JSON-RPC 2.0 / LSP 3.17 server, with
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
`gofx-std:///std/list/list.gfx`; `gofx/stdlib` returns that source for a `uri`.

No completion, rename, incremental edits, file watching, workspace symbols or
LSP formatting is advertised. The dependency-free VS Code client spawns this
server; setup and executable-path configuration are in [editors/README.md](editors/README.md).

### 8.6 Performance evidence (historical, not a portability guarantee)

The benchmark harness times built-program startup, input construction, workload
and output; compilation is excluded. It validates output even during calibration.
`bench/budget.json` contains nanoseconds/iteration; excess fails the test.
`GOFX_BENCH_RECORD=1` disables budget enforcement only; `GOFX_BENCH_ROOT` selects
the checkout. CI invokes `go test ./bench -run '^TestBudgets$' -bench . -benchtime=1x -timeout=12m`.

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

## 9. Known limitations

- No mutable references, character type (use one-rune Strings), arbitrary-precision
  integers, general exception syntax, cancellation or structured fiber lifetime.
- No expression type ascription (use typed lets/parameters); field access needs a
  known receiver type. `if` always needs `else`; bindings require irrefutable patterns.
- Rank-1 polymorphism only; no inferred polymorphic recursion, local type parameter
  declarations, multi-parameter classes, arbitrary specialized/overlapping instances,
  partial aliases, automatic currying or implicit numeric coercion.
- Deriving Show requires all constructors to be nullary; payload-bearing types
  need handwritten display instances. Higher-kinded deriving follows only supported
  last-parameter positions; deriving Traversable/Ord does not supply Functor/Eq for you.
- Pair Show/Debug/Eq/Ord instances do not imply arbitrary tuple-arity instances;
  JSON explicitly supports every tuple arity. Unit lacks Show/Ord. Fiber lacks
  standard class instances.
- JSON Float representation loses large-Int precision and nullable Option identity;
  unsupported/non-finite serialization is not silently repaired.
- Trusted externs can break purity. Foreign ABI errors are generation-time, not
  `check` errors. Building/running requires a Go toolchain and resolvable Go module.
- Native scalars/specialization are selective, not universal unboxing. Mutual or
  non-tail recursion and recursive library folds/maps can still consume stack;
  large allocations remain host-resource bounded. Lazy does not memoise.
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
| Float display | Shortest `g` form, including NaN/+Inf/-Inf; fixed decimal formatting is ties-to-even, preserves zeros; negative precision means shortest fixed form |
| Float Debug | `strconv.FormatFloat(f, 'g', -1, 64)` plus `.0` when the result contains none of `.`, `e`, `NaN`, `Inf`; `0.0`, `-2.0`, `NaN`, `+Inf`, `-Inf`, `1e+21`, `1e-07` retain their structural forms |
| powInt | 0^0 = 1; multiplication wraps; negative exponent gives 1 for base 1, parity-dependent ±1 for -1, otherwise 0 (including base 0) |
| sqrt / absFloat / min / max | Negative finite sqrt → NaN; absFloat(-0) → +0; min/max ties retain first argument |
| Alias arity/cycle | Exact application required; `arity mismatch for alias Pair: want 1 arguments, got 0`; `type alias cycle involving Loop` |
| Alias instance specialization | Repeated/erased/nested variables or function head → `instance head must be a type constructor`; permutations of distinct direct variables allowed |
| Common fields | Missing constructor field wins over type mismatch; rigid declaration types must agree even if a particular instantiation could make them equal; update preserves constructor |
| Local recursion | Only bare-name lambda lets recurse; consecutive local funcs share one scope group; local own type parameters → `local functions cannot have type parameters`; duplicate name → `duplicate local function <name>` |
| Placeholder placement | Bare holes are whole arguments/operator-section operands; projection sections `(_.f)` / `(_.0)` allow selector chains and omit parentheses only as whole call arguments; each projection has one independent parameter; fixed expressions run per application |
| Constraints | Duplicate class on one parameter fails; Eq + Ord is legal and reuses Ord; absent rigid class diagnostics list constraints in order |
| Match coverage | Guards add no coverage; true/false are exhaustive; other literal domains need default; maximum five missing witnesses; List witnesses always use `[]`, `[_]`, `[_, _, ..]`-style syntax (§5.2) |
| List patterns | `[]` and `[p1, p2, ...]` match exact lengths; `[p1, p2, ..rest]` binds the remaining list without copying, including an empty tail; final `..` / `.._` ignores it; trailing comma allowed |
| List rest placement/name | Rest needs a preceding element (`list rest pattern needs a preceding element`); it must be last (`.. must be last in list pattern`); only a lower-case name or `_` may follow `..` (`list rest must be a lower-case name or _`) |
| List binding patterns | Refutable: let / lambda parameter / do bind report `let pattern must be irrefutable` / `lambda parameter pattern must be irrefutable` / `bind pattern must be irrefutable`; use `match` |
| Deriving | Show only for enums (all constructors nullary), rendering source constructor names; Ord needs Eq, Traversable needs Functor; unmapped functions allowed only where the generated class never examines them |
| Pure Lazy | Construction stores thunk; each force reruns it, no memoization |
| Huge counts/widths | Empty/negative ordinary inputs follow tables below; huge positive allocations can panic/exhaust resources, never silently truncate |

### 10.2 Lists and folds

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
| `ioFail` | Every message, including empty text, constructs a failing deferred IO; no output or failure occurs merely by constructing it. |

| Operation | Boundary |
| --- | --- |
| ioPure / ioBind | Values already evaluated before action construction; bind skips continuation after IO failure |
| Result fmap/bind / pure | Map only Ok; first Err skips continuation; pure is Ok; Eq compares constructor/payload, Ord places Err before Ok |
| fromOption/fromResult | Default used for None/Err but evaluated strictly before call; maybe/either invoke only selected callback |
| tryIO | Deferred/repeatable; error text preserved; action panic payload captured without prefix; cannot capture construction panic or process exit |
| readFile | Read entire contents; filesystem errors fail IO |
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
| fork | Deferred; running same fork action twice creates distinct fibers; worker failure/panic cached |
| await | Repeat/concurrent awaits safe; action never reruns; original error returned unchanged |
| parMapIO | Empty → Nil, no mapper calls; mapper/action both deferred and panic-protected; results in input order; joins all even after failure |
| parBoth | Joins both; pair in argument order; first observed failure retained, not fixed left bias |
| race | First completion including failure; loser can continue effects and publish into a buffered channel |
| timeout | <= 0 → None without starting; positive overflow → `timeout: duration out of range`; success including erased nil → Some; timely error propagates; completion/expiry tie unspecified |
| Duration bound | Positive milliseconds must be <= MaxInt64 / 1,000,000 (Go time.Duration nanoseconds) |
| Worker panic | Ordinary IO error `panic: <value>`, including losing/timed-out tasks; awaiting it is an error path, not RunMain direct panic |
| Lifetime | No cancellation or global join; blocked task blocks parMapIO/parBoth; abandoned fibers end with process |

### 10.7 JSON and foreign ABI

| Boundary | Behaviour |
| --- | --- |
| JSON parse | Exactly one value; surrounding whitespace accepted; trailing/malformed/non-finite-range number → Err with `invalid JSON: ` prefix |
| JSON printing | Compact or two-space pretty without trailing newline; HTML/Unicode separators escaped; non-finite number panics |
| Object order | Parsed source order / derived declaration order; duplicate keys first position/last value; ordinary Map encode lexicographic |
| Numeric round trip | Int may round through Float; maxInt encodes as 2^63 and cannot decode as Int; decode refuses fractional/nonfinite/out-of-range Int |
| Option codec | null always None; Some(null-like value) indistinguishable from None |
| Records | Every field required, Option included; unknown fields ignored; first declaration-order error |
| Positional/tuple decode | Exact array length; constructor tags required except sole record; unknown tag reports `<Type> constructor` |
| Decode paths | `$` root, identifier `.field`, quoted key brackets, zero-based indexes; nested first failure preserved |
| Natural extern Option | None → zero/false; present zero not mistaken for absence; only primitive Option payloads expand |
| Natural extern lists | Nil/empty Go slices → Nil; conversions copy and preserve order, no mutable slice aliasing |
| Go types | True aliases accepted, distinct named types not coerced; Go int narrowing follows target width; no-result function is not Unit |
| Extern errors | `(T, error)` conversion/call occurs inside IO; creation is lazy; each run repeats Go call; check alone does not validate foreign ABI |
| bind inference | Does not guess Option inputs; skips uninferable any/rt.IO and unsupported functions with reason comments; declarations are toolchain-dependent |
| Runtime shapes | Generated constructors registered once; unregistered Tuple2/Ordering/Ok/Err/List bridge use panics (programming/configuration error), not a source value |

### 10.8 Modules, diagnostics and tools

| Boundary | Behaviour |
| --- | --- |
| Directory inputs | check/build/run/test/eval load immediate .gfx only; fmt recursively loads regular .gfx; explicit entry files must share directory |
| Import search | Source root before GOFX_PATH before embedded std; first matching directory is authoritative even invalid/empty |
| Privacy / identity | `_` private outside owning module; aliases file-scoped; cycles deduplicated; global duplicate instances name both modules |
| Prelude shadow | Local wins, top-level shadow warns; no implicit JSON or non-listed methods/functions |
| Diagnostic spans | One-based rune starts/exclusive ends; CLI caret clamps first line; missing end gives one caret; warnings exit 0 |
| test | Empty suite succeeds; only explicit Bool/IO[Bool], monomorphic zero-arg `test` plus suffix; user main removed before checking; false/IO failure continues, panic uses ordinary runtime policy |
| eval | One expression; imports from first entry file; Show needed; failed pure and IO wrappers report original error; no special Unit display |
| build | Go sources not executable; -o vs --emit-go exclusive; --emit-go/--package paired; unrelated Go files preserved |
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
| -32602 | `Invalid params`, `Invalid position`, `expected an absolute file URI`, or `expected a .gfx document` |

Module errors include `invalid import path "PATH"`, `module "PATH" not found`,
`entry files must belong to one directory`, `no .gfx files in DIR`,
`invalid gofx.mod in DIR: expected module <name>`, `package NAME differs from NAME`,
`duplicate or invalid import alias ALIAS`, `unknown import alias ALIAS`,
`module MODULE has no exported name NAME`, `NAME is private to module MODULE`,
and `duplicate instance CLASS[TYPE] in modules FIRST and SECOND`.
