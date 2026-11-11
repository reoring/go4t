# ADR 0017: Law declarations: optional and inline proofs, several `requires`, predicate bodies

Status: Accepted (design); implementation notes are appended below.

## Context

Measured over the repository's own go4t code (47 laws in `examples/`, 326 in
`laws/testdata`), with the repository's parser:

- 35 of 47 example proofs (74%) and 228 of 326 fixture proofs (70%) are exactly
  `{ refl }`. Every example proof repeats its law's parameter names unchanged
  (47 of 47; 318 of 326 fixtures) and directly follows its law (47 of 47).
- 7 of 47 example laws and 43 of 326 fixture laws state a predicate as
  `(…) == true` or `(…) == false`; among the 23 fixture laws with `requires`
  it is 13.
- `go4t fmt` prints a precondition on the law's header line whatever its
  length; the README's range law becomes one 131-column line.

The checker already treats a precondition as an ordered list of conjuncts
(ADR 0012, ADR 0016). The surface syntax should say what the checker does.

## Decision

```ebnf
LawDecl := 'law' Ident [TParams] '(' [Param {',' Param} [',']] ')'
           { ? requires identifier ? Expr }
           '{' LawBody {Separator} '}'
           [ 'proof' TacticBlock ]
LawBody := Expr '==' Expr | Expr
```

(`[TParams]` is ADR 0021.) The separate `proof Name(x, y) { … }` declaration
keeps its grammar and meaning.

### 1. A law without a proof is checked by `refl`

A law that has neither an inline proof nor a `proof` declaration in its module
is proved by `refl`. The diagnostic `law %s has no proof` no longer exists.

- The checker supplies the proof; the parser adds nothing to `File.Decls`. The
  supplied `ast.ProofDecl` has `Implicit: true`, the law's own parameter names
  and a `TRefl` positioned at the law body. It is not a declaration symbol
  (no document symbol, no hover, never printed by `fmt`).
- When that `refl` fails, the diagnostic is the ordinary `refl` failure,
  reported at the law's body, followed by a last line
  `no proof was written for this law; it was checked by refl`.
- `go4t fmt` never removes a written proof: a separate `proof f(x) { refl }`
  stays as written.

### 2. Inline proof

`} proof { tactic }` after the law's closing brace is that law's proof. The
tactic block may start on the same line or after newlines; it is recognized
by `proof` followed by `{` (a declaration is `proof` followed by a name).

- The parser emits an `ast.ProofDecl` with `Inline: true`, the law's name and
  one parameter per law parameter with the same name, immediately after the
  `LawDecl` in `File.Decls`. Everything downstream sees an ordinary proof.
- An inline proof plus a `proof` declaration for the same law is
  `duplicate proof %s`.
- `sample(N)` may be the inline tactic.
- Canonical layout:

  ```go4t
  law name(a Int) {
    lhs == rhs
  } proof {
    tactic
  }
  ```

### 3. Several `requires` clauses

A law may carry any number of `requires` clauses. Their meaning is their
conjunction, in written order — exactly the order in which ADR 0016 checks and
assumes conjuncts.

- AST: `LawDecl.Requires` remains the one semantic precondition, the
  left-associated `&&` of the clauses, so no checker code changes.
  `LawDecl.RequiresClauses` lists the written clauses (the operands of that
  chain) for the formatter only, the way `Call.Pipe` keeps a pipeline's
  surface; it has more than one element only when several clauses were
  written.
- Canonical layout: a single clause stays on the header line when the line
  fits 80 columns. Otherwise, and whenever there are several clauses, each
  clause is on its own line, indented once, and the body brace starts a line:

  ```go4t
  law name(t Int, at Int, now Int)
    requires 0 <= t && t <= 1000
    requires 0 <= at && at <= now
  {
    lhs == rhs
  }
  ```

- Diagnostics that point at the precondition keep pointing at the whole
  precondition.

### 4. Predicate bodies

A body whose outermost operator is not `==` is a predicate: it states that the
Bool expression is `true`. `law succ(a Int) requires a < 100 { a + 1 > a }`
means `(a + 1 > a) == true`.

- AST: `LHS` is the expression, `RHS` a synthesized `true`, and
  `LawDecl.Predicate` is set so the formatter prints only the expression.
- The body must have type Bool:
  `law body must be an equation (lhs == rhs) or a Bool expression, got %s`.
- **The ambiguous shape stays an error.** `==` binds tighter than `&&`, `||`
  and `|>`, so `a && b == b && a` reads as `a && (b == b) && a`. A predicate
  whose outermost operator is `&&`, `||` or `|>` and that has an
  unparenthesized `==` operand at that level is rejected at the body:
  `ambiguous law body: == binds tighter than && and ||; write (lhs) == (rhs) for an equation, or parenthesize the comparison`.
- `fmt` does not rewrite `(p) == true` into `p`, or the reverse.

## Not changed

Laws still generate no runtime code; `requires` is never checked at run time;
`sample(N)` still rejects laws with `requires`; a written `proof` declaration
may still rename the law's parameters positionally.

## Consequences

- `SPEC.md` §6.5: grammar, the removed and added diagnostics, layout.
- Laws in `examples/` and the documentation lose their `refl`-only proofs
  where that reads better; both spellings remain valid.
- The self-hosted compiler must skip the new forms (ADR 0023).

## Implementation notes

- Implemented without changing the shared AST. The parser records operator
  grouping depths locally while reading a law body; parentheses do not become
  AST nodes. The formatter explicitly groups comparisons in predicate chains.
- Implicit proofs are paired after all written declarations are registered,
  retained only in the checked program's proof map, and use the law's parameter
  names. Inline proofs are ordinary adjacent declarations with source-free
  inherited parameter bindings; tooling resolves their uses to the law header.
- The open-term property oracle slices predicate bodies from source and uses
  literal `true` for their synthesized right side. It excludes statement
  separators and handles parentheses around whole equations.
- No semantic deviations. Precondition failures retain the whole-precondition
  location and do not acquire the implicit-`refl` failure note.
