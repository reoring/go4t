# pipe-partial: specification delta

## Lexical structure and layout

Add `|>` to punctuation, lexed as one token by longest-match before `|` and `>`.
Add `|>` to leading continuation tokens: a line beginning with `|>` continues
the preceding expression even across blank lines and comments. A newline after
`|>` also continues the expression. Explicit semicolons remain separators.
The existing `|` match-arm/type-alternative and `||` rules are unchanged.

## Expressions

Add `|>` below `||` in the precedence table. All binary operators, including
`|>`, remain left-associative.

- `x |> f(a, b)` desugars to `f(x, a, b)`.
- `x |> f` desugars to `f(x)`.
- `x |> (func(y) => body)` applies that lambda to `x`.
- A call with direct `_` arguments becomes a lambda with one inferred parameter
  per placeholder, in left-to-right order: `add(1, _)` becomes
  `func(a) => add(1, a)`, and `f(_, x, _)` becomes `func(a, b) => f(a, x, b)`.
- A placeholder call is already a lambda when used on the right of a pipe.
  Thus `xs |> map(_, f)` applies that lambda to `xs`, and
  `xs |> join("\n", _)` becomes `join("\n", xs)`, not a three-argument call.
- A parenthesised binary operator may have `_` as either or both whole operands:
  `(_ + 1)`, `(2 * _)`, `(_ == x)`, and `(_ < _)`. Two holes produce a two-argument
  lambda, not a curried function. The rule applies to every binary operator,
  including `|>`; for example `(_ |> f)` becomes `func(a) => f(a)`.

Grammar additions (other expression productions remain unchanged):

```text
CallArg := Expr | '_'
Section := '(' '_' BinOp Expr ')'
         | '(' Expr BinOp '_' ')'
         | '(' '_' BinOp '_' ')'
```

A placeholder must occupy a whole argument or a whole operand of the
parenthesised operator. It does not implicitly abstract arbitrary expressions:
`f(_ + 1)`, `(_ + 1 + 2)`, `[_]`, `(_, 1)`, and `(_)` are errors. Use
`f((_ + 1))` to pass a section as an argument. Nested calls and sections have
independent placeholder scopes: `f(_, g(_))` has one outer parameter and a
separate one-parameter lambda as its second argument.

The diagnostic for other expression uses is exactly:

```text
_ is only valid as a placeholder in a call or operator section
```

It points at the offending underscore. Parsing rejects misuse before checking;
the CLI preserves the existing `file:line:col: message` wrapper. Wildcards in
match, lambda, `let`, and `do` patterns, and `_` in kind parameter declarations,
are unchanged.

## Types, lowering, and runtime

No new types, AST node kinds, prelude signatures, runtime functions, or ABI rules.
The parser emits ordinary `ast.Call`, `ast.Lambda`, and `ast.Ident` nodes.
Synthesized names cannot capture source identifiers, including identifiers inside
string interpolation. The existing checker infers placeholder parameter types,
preserves call arity, generalises eligible lambda lets, and resolves ordinary
Eq/Ord dictionaries for comparison sections. The existing code generator uses
the unchanged uniform `any` ABI. Fixed arguments are evaluated in the synthesized
lambda body, just as in the explicit-lambda expansion.

Removed limitation: call arguments can now be omitted explicitly with `_`, and
parenthesised binary operator sections and multiline pipelines can replace
manual lambda/nested-call spelling.

## Verification

All commands ran in the `pipe-partial` worktree. Go tests and CLI runs were
wrapped with `SERIALIZE_VM_TESTS_MODE=shared
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --`.

- `go test ./lexer ./parser ./types ./codegen -run 'TestPipe|TestPlaceholder'`:
  failed before implementation for missing `|>` and placeholder syntax; passed
  after implementation.
- `gofmt -w lexer/lexer.go lexer/pipe_partial_test.go parser/parser.go parser/pipe_partial_test.go types/pipe_partial_test.go codegen/pipe_partial_test.go`:
  completed successfully.
- `go test ./lexer ./parser ./types ./codegen`: all four packages passed, including
  the existing generated-Go parsing/typechecking harness.
- `go run ./cmd/gofx run examples/pipe-partial/main.gfx`: exit 0; observed output
  is recorded in the example README.
- `go run ./cmd/gofx check examples/wc/main.gfx`: exit 0, no diagnostics.

## Deviations

None.

## Duplicated names

None. The example uses only the existing prelude.
