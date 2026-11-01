# Pipelines and partial application

Run from the repository root:

```sh
go run ./cmd/go4t run examples/pipe-partial/main.gt
```

Observed output:

```text
apple
banana
pear
scores: 4, 6, 8
steps: 16
keys: tea, tea, tea; qty: 2; sorted: [coffee, tea]
sections: true, true
```

The example drops empty or whitespace-only lines, trims and sorts the remaining
lines, then joins them. Each leading `|>` continues the previous expression.
`filter(_, nonEmpty)`, `map(_, trim)`, and `sortBy(_, compare)` make unary
functions; `join("\n", _)` places the input in the second argument instead.

The score pipeline uses `(_ + 1)` and `(2 * _)`. The final lines exercise a
named partial application, first-argument insertion (`add(4)`), an explicit
lambda, the two-argument section `(_ < _)`, and `(_ == "apple")`.

The key examples use a declared `Option[func(Sale) String]` return to check
`Some(_.product)`, an annotated `let f: func(Sale) String = _.product`, a
`match` returning `_.qty`, and `let key: func(Sale) String = func(s) => s.product`.
`list.sortOn(..., func(s) => s.jpy)` still infers `s` from its input list.
Without an expected function type, `let key = Some(_.product)` is rejected:
`cannot infer record type of _.product; annotate`.
