# Diagnostics

Run commands from the repository root. `main.gt` is a valid executable; the
`bad_*.gt` files deliberately exercise checker diagnostics. Missing-pattern and
missing-instance examples exit 1. `bad_unused.gt` contains only warnings and
exits 0. Warnings do not prevent `build` or `run`.

The source excerpt is clamped to the first line of a multiline expression. A
caret is followed by tildes for the remaining span. Missing patterns are concrete
uncovered shapes, with `_` for unconstrained fields; at most five are listed.

`go run` adds its own `exit status 1` line for failing checks; it is included in the
captured output below. The installed `go4t check` command prints the same
compiler diagnostics without that Go-launcher line.

## Valid program

```console
$ go run ./cmd/go4t run examples/diagnostics/main.gt
count: 3
```

Exit 0, with no warnings. Both Bool alternatives are exhaustive; `_ignored`
deliberately ignores the second field of the disabled alternative.

## Missing constructors

```console
$ go run ./cmd/go4t check examples/diagnostics/bad_missing_ctor.gt
examples/diagnostics/bad_missing_ctor.gt:4:3: non-exhaustive match; missing: None, Some(_)
  match item {
  ^~~~~~~~~~~~
exit status 1
```

The guarded wildcard does not cover either constructor.

## Nested constructors

```console
$ go run ./cmd/go4t check examples/diagnostics/bad_missing_nested.gt
examples/diagnostics/bad_missing_nested.gt:4:3: non-exhaustive match; missing: Some(Some(_))
  match item {
  ^~~~~~~~~~~~
exit status 1
```

## Tuple and Bool

```console
$ go run ./cmd/go4t check examples/diagnostics/bad_missing_tuple.gt
examples/diagnostics/bad_missing_tuple.gt:4:3: non-exhaustive match; missing: (true, _)
  match pair {
  ^~~~~~~~~~~~
exit status 1
```

## Literal hole

```console
$ go run ./cmd/go4t check examples/diagnostics/bad_missing_literal.gt
examples/diagnostics/bad_missing_literal.gt:4:3: non-exhaustive match; missing: Some(1)
  match item {
  ^~~~~~~~~~~~
exit status 1
```

`Some(1)` is one uncovered value; adding `Some(_)` covers all remaining Int values.

## Missing instance hint

```console
$ go run ./cmd/go4t check examples/diagnostics/bad_hint.gt
examples/diagnostics/bad_hint.gt:6:11: no instance Show[Hidden]; hint: write 'instance Show[Hidden]' (or 'deriving Debug' for a structural form)
  println(show(Hidden))
          ^~~~
exit status 1
```

## Unused locals

```console
$ go run ./cmd/go4t check examples/diagnostics/bad_unused.gt
examples/diagnostics/bad_unused.gt:4:7: warning: forgotten is unused; prefix with _ to silence
  let forgotten = 1
      ^~~~~~~~~
examples/diagnostics/bad_unused.gt:5:23: warning: argument is unused; prefix with _ to silence
  let constant = func(argument Int) => "ok"
                      ^~~~~~~~
examples/diagnostics/bad_unused.gt:7:10: warning: value is unused; prefix with _ to silence
    Some(value) => println(constant(2))
         ^~~~~
```

Exit 0. These are a let binding, a lambda parameter, and a constructor-pattern
variable. Prefixing each name with `_` silences its warning.

## Alias mismatch

```console
$ go run ./cmd/go4t check examples/diagnostics/bad_alias_mismatch.gt
examples/diagnostics/bad_alias_mismatch.gt:8:3: type mismatch: expected Rates, got Int
  1
  ^
exit status 1
```

The annotation uses `alias Rates = Map[Currency, Float]`, so the diagnostic
keeps `Rates` instead of expanding the map. Aliases remain transparent to type
checking and code generation; unrelated inferred types are not renamed.
Long types replace subtrees at depth 3 with `…` only when their complete spelling
exceeds 160 characters. Add `--full-types` to `check`, `build` or `run` to retain
all depths; alias names remain intact.

## Existing-program compatibility

```sh
go run ./cmd/go4t check examples/wc/main.gt
```

Observed exit 0, with no stdout, stderr, or warnings; no wc source change needed.
