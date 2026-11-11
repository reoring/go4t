# ADR 0023: The self-hosted compiler skips laws token by token and follows the lexical rules

Status: Accepted (design); implementation notes are appended below.

## Context

The self-hosted compiler (`selfhost/`) implements a subset of the language and
erases laws. It erases them by *parsing* them: `prErasedDecl` reads the law's
parameters and body and `prTacticBlock` reads the proof's tactic grammar.
Every change to law syntax therefore breaks the bootstrap until the
self-hosted parser is taught the new form — it happened twice in one day
(`requires`, then the `induction` tactic in `examples/todo`). ADRs 0017, 0018
and 0021 change law syntax again.

`selfhost/bootstrap.sh` compiles every `examples/*/main.gt` not listed in its
`SKIP` table through the self-hosted compiler and compares the output with the
Go compiler's.

## Decision

### 1. Laws and proofs are skipped by token structure

The self-hosted parser no longer parses the inside of a `law` or `proof`
declaration. From the keyword it skips tokens, tracking the nesting of `(`,
`[`, `{`, until the declaration's end:

- a declaration ends at a `}` at nesting depth 0 whose next token, ignoring
  `;`, is the end of input or a keyword that starts a declaration;
- `proof` followed by `{` continues the law it follows (inline proof,
  ADR 0017); `proof` followed by an identifier starts a declaration.

Braces inside a `requires` expression (record updates, blocks, matches) are
nested, so they never end the declaration. String and interpolation tokens
are single tokens to the lexer, so their contents are not scanned.

After this, no change to law, precondition, proof or tactic syntax touches
`selfhost/`, as long as a law ends with a top-level `}`.

### 2. The self-hosted lexer follows ADR 0019

- A line-leading `-` where a statement could end is the same error.
- Int literals accept `_` separators and the `0x`, `0b`, `0o` prefixes, and
  the smallest Int after a unary `-`.

The two compilers must agree on how source is split into tokens and
statements; a difference here is a silent difference in meaning.
`examples/literals` exercises these forms and is compiled by both.

### 3. What stays outside the subset

Or-patterns (ADR 0020) and the record shorthand, top-level `let`, bare
operator sections and trailing-comma positions of ADR 0022 are not
implemented in `selfhost/`. `std/` and the examples the bootstrap compiles do
not use them. An example that does is listed in `bootstrap.sh`'s `SKIP` table
with that reason — an explicit exclusion, like the existing ones.

Non-associative comparisons need no change: the self-hosted parser accepts a
superset, and the Go compiler rejects chains before any program reaches both.

## Consequences

- `SPEC.md` §9 states the subset.
- `selfhost/parser.gt` loses its tactic parser.

## Implementation notes
