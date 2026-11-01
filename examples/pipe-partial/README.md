# Pipelines and partial application

Run from the repository root:

```sh
go run ./cmd/gofx run examples/pipe-partial/main.gfx
```

Observed output:

```text
apple
banana
pear
scores: 4, 6, 8
steps: 16
sections: true, true
```

The example drops empty or whitespace-only lines, trims and sorts the remaining
lines, then joins them. Each leading `|>` continues the previous expression.
`filter(_, nonEmpty)`, `map(_, trim)`, and `sortBy(_, compare)` make unary
functions; `join("\n", _)` places the input in the second argument instead.

The score pipeline uses `(_ + 1)` and `(2 * _)`. The final lines exercise a
named partial application, first-argument insertion (`add(4)`), an explicit
lambda, the two-argument section `(_ < _)`, and `(_ == "apple")`.
