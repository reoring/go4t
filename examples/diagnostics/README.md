# Diagnostics

Run commands from the repository root. `main.gfx` is a valid executable; the
`bad_*.gfx` files deliberately exercise checker diagnostics. Missing-pattern and
missing-instance examples exit 1. `bad_unused.gfx` contains only warnings and
exits 0. Warnings do not prevent `build` or `run`.

The source excerpt is clamped to the first line of a multiline expression. A
caret is followed by tildes for the remaining span. Missing patterns are concrete
uncovered shapes, with `_` for unconstrained fields; at most five are listed.

`go run` adds its own `exit status 1` line for failing checks; it is included in the
captured output below. The installed `gofx check` command prints the same
compiler diagnostics without that Go-launcher line.

## Valid program

```console
$ go run ./cmd/gofx run examples/diagnostics/main.gfx
count: 3
```

Exit 0, with no warnings. Both Bool alternatives are exhaustive; `_ignored`
deliberately ignores the second field of the disabled alternative.

## Missing constructors

```console
$ go run ./cmd/gofx check examples/diagnostics/bad_missing_ctor.gfx
examples/diagnostics/bad_missing_ctor.gfx:4:3: non-exhaustive match; missing: None, Some(_)
  match item {
  ^~~~~~~~~~~~
exit status 1
```

The guarded wildcard does not cover either constructor.

## Nested constructors

```console
$ go run ./cmd/gofx check examples/diagnostics/bad_missing_nested.gfx
examples/diagnostics/bad_missing_nested.gfx:4:3: non-exhaustive match; missing: Some(Some(_))
  match item {
  ^~~~~~~~~~~~
exit status 1
```

## Tuple and Bool

```console
$ go run ./cmd/gofx check examples/diagnostics/bad_missing_tuple.gfx
examples/diagnostics/bad_missing_tuple.gfx:4:3: non-exhaustive match; missing: (true, _)
  match pair {
  ^~~~~~~~~~~~
exit status 1
```

## Literal hole

```console
$ go run ./cmd/gofx check examples/diagnostics/bad_missing_literal.gfx
examples/diagnostics/bad_missing_literal.gfx:4:3: non-exhaustive match; missing: Some(1)
  match item {
  ^~~~~~~~~~~~
exit status 1
```

`Some(1)` is one uncovered value; adding `Some(_)` covers all remaining Int values.

## Missing instance hint

```console
$ go run ./cmd/gofx check examples/diagnostics/bad_hint.gfx
examples/diagnostics/bad_hint.gfx:6:11: no instance Show[Hidden]; hint: write 'instance Show[Hidden]' (or 'deriving Debug' for a structural form)
  println(show(Hidden))
          ^~~~
exit status 1
```

## Unused locals

```console
$ go run ./cmd/gofx check examples/diagnostics/bad_unused.gfx
examples/diagnostics/bad_unused.gfx:4:7: warning: forgotten is unused; prefix with _ to silence
  let forgotten = 1
      ^~~~~~~~~
examples/diagnostics/bad_unused.gfx:5:23: warning: argument is unused; prefix with _ to silence
  let constant = func(argument Int) => "ok"
                      ^~~~~~~~
examples/diagnostics/bad_unused.gfx:7:10: warning: value is unused; prefix with _ to silence
    Some(value) => println(constant(2))
         ^~~~~
```

Exit 0. These are a let binding, a lambda parameter, and a constructor-pattern
variable. Prefixing each name with `_` silences its warning.

## Existing-program compatibility

```sh
go run ./cmd/gofx check examples/wc/main.gfx
```

Observed exit 0, with no stdout, stderr, or warnings; no wc source change needed.
