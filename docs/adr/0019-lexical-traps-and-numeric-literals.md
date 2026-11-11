# ADR 0019: A leading `-` is rejected, comparisons do not chain, Int literals get separators and bases

Status: Accepted (design); implementation notes are appended below.

## Context

Three places where the surface syntax misleads, each confirmed with the CLI.

**A line-leading `-` joins the previous line** (SPEC §1.4).

```go4t
func f(x Int) Int {
  let y = x
  -y
}
```

is read as `let y = x - y` and reported as `block must end with an expression`
at the closing brace. The formatter already defends against this: a statement
that starts with a negation is printed in parentheses. No line of the 15,114
in `std/`, `examples/`, `selfhost/` and `bench/` starts with a binary `-`
(nor with `+` or `*`); the leading continuations in use are `|>`, `==`, `&&`.

**Comparisons are one left-associative level.** `a >= b == c` is legal, so
`go4t fmt` rewrites the law body `(score(k) >= 0) == true` to
`score(k) >= 0 == true`. Bool has `Ord`, so `a <= b <= c` type-checks for
Bools as `(a <= b) <= c`. No expression in the corpus compares a comparison.

**Int literals are decimal digit runs only.** `1_000_000` is
`unknown identifier _000_000`, `0xFF` is `unknown identifier xFF`, and the
smallest Int cannot be written (`-9223372036854775808` is
`bad integer literal 9223372036854775808`). Preconditions made large bounds
common: 34 literals of seven or more digits, 13 of them
`9223372036854775807`.

## Decision

### 1. A `-` cannot start a line where a statement could end

Where the layout rule would otherwise decide between "continue the previous
expression" and "start a new statement" and the next line starts with `-`,
the lexer reports an error instead of choosing:

```text
ambiguous "-" at the start of a line: end the previous line with "-" to subtract, or write (-x) for a negative value
```

Considered and rejected: treating the leading `-` as unary (a new statement).
It fixes the example above and silently breaks its mirror image —
`let total = price` / `  - discount` would discard the subtraction while a
leading `+`, `*`, `&&` or `|>` still continues. Either guess is wrong for
someone; the two readings have the same tokens, so the language declines to
guess. Canonically formatted code is unaffected (`fmt` already writes `(-x)`),
and a `-` after an operator, `=`, `=>`, `(`, `[` or `,` was never ambiguous.

Every other leading continuation token keeps its meaning.

### 2. Comparison operators do not associate

`==`, `!=`, `<`, `<=`, `>`, `>=` take operands of higher precedence only.
A second comparison at the same level is an error at its operator:

```text
comparison operators do not chain; parenthesize one comparison, as in (a < b) == c
```

- A law equation's sides follow the same rule: `(a >= b) == true`.
- `fmt` parenthesizes a comparison that is an operand of a comparison, on
  either side, and both sides of a law equation.
- Chained-comparison sugar (`a <= x <= b`) is not introduced. It is now
  free syntax.

### 3. Int literals

```ebnf
Int := Digits | '0' ('x'|'X') HexDigits | '0' ('b'|'B') BinDigits | '0' ('o'|'O') OctDigits
```

- `_` may separate digits and may follow a base prefix, as in Go:
  `1_000_000`, `0xFF_FF`, `0b1010_0001`, `0o755`. It cannot lead, trail or
  repeat.
- A literal's value must fit Int. The one magnitude that does not, 2^63, is
  accepted directly after a unary `-` and denotes the smallest Int
  (`-9223372036854775808`, `-0x8000000000000000`); the result is an `IntLit`,
  not a negation.
- A decimal literal with leading zeros stays decimal (`007` is 7), as today.
- Float literals are unchanged: no separators, no base prefixes.
- Tuple indices (`t.0`) stay plain decimal digits.
- The formatter prints an Int literal as written (`ast.IntLit.Text`).
- Diagnostics:

  ```text
  bad number literal %s: "_" must separate digits
  bad number literal %s: expected %s digits after %s
  bad number literal %s: a number cannot be followed by a letter
  integer literal %s is out of range for Int
  digit separators and base prefixes are not supported in Float literals
  ```

  The third replaces today's split into a number and an identifier.

## Consequences

- `SPEC.md` §1.3, §1.4, §4.1, §4.2; the expression grammar is written by
  precedence level so that the generated-sentence test can state
  non-associativity.
- The self-hosted lexer follows rules 1 and 3 (ADR 0023).
- Breaking: a line-leading binary `-` and a chained comparison are now errors.
  Neither occurs in the repository.

## Implementation notes
