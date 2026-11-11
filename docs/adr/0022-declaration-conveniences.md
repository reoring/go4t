# ADR 0022: Record shorthand, top-level `let`, bare operator sections, trailing commas, spelling hints

Status: Accepted (design); implementation notes are appended below.

## Context

Small frictions, each counted over the repository's go4t code or confirmed
with the CLI:

- 122 of 222 type declarations name their only constructor after the type;
  112 of those are records (`type Row = Row{qty: Int}`).
- A named constant is a nullary function called with `()`; `let limit = 10`
  at top level is `expected declaration, found "let"`.
- `map(xs, _ + 1)` is rejected; the section needs its own parentheses,
  `map(xs, (_ + 1))`.
- A trailing comma is accepted in parameter lists, arguments, lists and
  records, and rejected in tuples and type arguments.
- Parameters are `x Int` (1,696 uses), record fields `n: Int` (486), `let`
  annotations `x: Int` (21). Writing one in the style of another gives
  `expected identifier, found ":"` and the like.

## Decision

### 1. Record shorthand

```ebnf
TypeDecl := 'type' Ident [DataParams] '=' (RecordFields' | Ctor {'|' Ctor}) ['deriving' Name {',' Name}]
```

`type Row = {qty: Int, sku: String}` declares the type `Row` with the single
record constructor `Row`. Construction, patterns, field access, deriving and
the exported Go struct are those of `type Row = Row{…}`.

- The shorthand is the type's only constructor:
  `the record shorthand declares the only constructor of %s; name the constructors to add alternatives`.
- `fmt` keeps whichever spelling was written (`ast.TypeDecl.Shorthand`).

### 2. Top-level `let`

```ebnf
ConstDecl := 'let' LowerIdent [':' Type] '=' Expr
```

A top-level `let` is a named constant: **a nullary pure function that is
referenced without parentheses.** It has no other semantics; code generation
and the law normalizer treat it as that function.

- The type may be omitted only when the value is an Int, Float, String or
  Bool literal (a numeric literal may be negated):
  `top-level let needs a type unless its value is a literal: let %s: T = …`.
- No type parameters. Not an action:
  `a top-level let cannot be an IO action; write func %s() IO[…]`.
- Constants may refer to functions and to each other; a cycle among
  constants is `constant initialization cycle: a, b`.
- `name()` on a constant is `%s is a constant, not a function; write %s without ()`.
- Visibility (`_name`), module qualification (`mod.name`) and the
  prelude-shadowing warning are those of a function.
- A constant is law-eligible exactly when its value is.
- `go4t test` does not treat a constant named `test…` as a test.
- `--emit-go` exports a constant as the Go function `Name() T`.

### 3. Operator sections as call arguments

A call argument may be an operator section without its own parentheses:
`map(xs, _ + 1)`, `filter(xs, 0 < _)`, `fold(xs, 0, _ + _)`. The accepted
shapes are exactly the parenthesized ones (one operator, whole unary
operands). `f(_)` is still the call's own placeholder and `f(_.x)` a
projection; `f(_ + 1 + 2)` is still invalid.

`fmt` prints a section that is a direct call argument without parentheses.

### 4. Trailing commas

A trailing comma is accepted in tuple expressions, tuple patterns, tuple
types, type applications and type-parameter lists. One parenthesized element
with a comma is not a tuple: `a tuple needs at least two elements`. The
formatter's output does not change.

### 5. Spelling hints

Diagnostics only; no syntax changes.

| Written | Diagnostic |
| --- | --- |
| `func f(x: Int)`, `func(x: Int) => …` | `parameters are written "x Int", without ":"` |
| `let x Int = 1` | `let annotations are written "let x: Int = …"` |
| `type R = R{n Int}` | `record fields are written "n: Int"` |
| `type T = A(n Int)` | `positional constructor fields have no names; write A{n: Int} to name them` |

## Not changed

A short lambda spelling (`x => e`) was considered and left out: `func(x) => e`
is the Go-flavoured form and sections cover the common one-operator case.

## Consequences

- `SPEC.md` §2.1, §3.2, §3.3, §4.1, §4.5, §5.2.
- `ast.ConstDecl` and `ast.TypeDecl.Shorthand` are new.
- These forms are outside the self-hosted compiler's subset (ADR 0023).

## Implementation notes

Constants are registered as `FuncInfo` entries with an ordinary synthesized
nullary `FuncDecl`; `FuncInfo.Constant` retains the surface declaration.
The checker infers a constant identifier as that function's return type.
Code generation calls the same function emitter and direct-call path, and law
evaluation applies the same global unfolding machinery with no arguments.
Termination and extern-taint checking visit that function body. There is no
initializer cache or second evaluation model.

Registration happens in the checker after module qualification, not solely in
the resolver, so the single unnamed compilation-unit path is covered too.
The surface `ConstDecl` is not replaced or mutated into a function; the synthetic
body shares its value solely for ordinary checking annotations. Re-checking and
formatting the same unnamed input preserves its syntax. Named modules retain
the resolver's existing syntax-copy boundary.

Cycle detection follows checked global references, including paths through
functions, and reports source-spelled constant names in sorted order.
Function-only recursion remains governed by the existing recursion checks.

The existing parser also accepts compound fixed operands such as `(_ + 2 * 3)`
and `(1 + 2 < _)`. These pre-existing forms are preserved, and their bare call
argument forms are accepted identically; §3's "one operator, whole unary operands"
describes the documented grammar rather than narrowing existing syntax.
