# ADR 0020: Or-patterns and negative literal patterns

Status: Accepted (design); implementation notes are appended below.

## Context

`Red | Green => true` is `expected "=>", found "|"`. Counted with the
repository's parser over 720 `match` expressions in `std/`, `examples/`,
`selfhost/` and `bench/`:

- 42 matches have arms with identical bodies; 115 arms would merge (57 if
  only alternatives that bind no variables are counted).
- 130 matches end their constructor patterns with `_ =>`. Estimated from
  constructor names in the same package, 87 of them leave two or more
  constructors unnamed. Adding a constructor does not make those matches fail
  to compile, which is the property the language advertises. Many are meant
  ("anything else is an error"); the rest are the cheap way to avoid repeating
  a body.

`match n { -1 => … }` is `expected pattern, found "-"`.

A `|` directly after a pattern is always an error today, so alternation takes
no syntax from existing programs. `|` remains an arm separator after an arm's
body, and `go4t fmt` never emits it between expression arms.

## Decision

```ebnf
Pattern    := PatternAlt {'|' PatternAlt}
PatternAlt := … existing alternatives …
            | '-' (Int | Float)
```

### Or-patterns

- An or-pattern may appear wherever a pattern may, including nested
  (`Some(1 | 2)`) and in tactic arms (ADR 0018).
- **Meaning by expansion.** An arm `p1 | p2 if g => e` behaves exactly like the
  consecutive arms `p1 if g => e` and `p2 if g => e`. Nested alternatives
  expand left to right (`ast.Alternatives`). Exhaustiveness, redundancy,
  evaluation order and generated code follow the expansion.
- Every alternative binds the same variable names, each at the same type:

  ```text
  or-pattern alternatives must bind the same variables; %s is not bound in every alternative
  ```

  A type difference is the ordinary `type mismatch` at the later binding.
- Or-patterns are refutable, so `let`, lambda parameters and `do` binds reject
  them: `or-patterns are only allowed in match arms`.
- **Laws.** The normalizer evaluates an or-arm as its expansion. A stuck
  `match` assumes the scrutinee equal to *one* alternative per expanded arm,
  never to the or-pattern as a whole.
- `fmt` prints `p1 | p2 => e` with single spaces and keeps alternatives on one
  line.

Implementation strategy (not a contract): check an or-arm once, with its
variables bound from the alternatives, then replace it in the checker's copy
of the syntax with its expanded arms sharing one body. Coverage, tail-call and
structural-recursion analyses, deriving, code generation and the law
normalizer then see or-free patterns and need no change. The source tree the
formatter and the language server print from keeps the or-pattern.

### Negative literal patterns

`-` followed by an Int or Float literal is a literal pattern for the negative
value, including the smallest Int (ADR 0019). Anything else after `-` is
`expected a number after "-" in a pattern`.

## Soundness evidence required of the implementation

- Fixtures with or-patterns in law-reachable functions, proved and refuted;
  the open-term property test compares them with the runtime like any other.
- A stuck-match fixture in which assuming the whole or-pattern would prove a
  false law.

## Consequences

- `SPEC.md` §4.7 and the tactic-pattern grammar in §6.5.
- `ast.POr` and `ast.Alternatives` are new.
- Or-patterns are outside the self-hosted compiler's subset; `std/` and the
  examples it compiles do not use them (ADR 0023).

## Implementation notes
