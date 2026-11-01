# Persistent collections

From the repository root:

```sh
go run ./cmd/gofx run examples/collections/main.gfx
```

Observed output:

```text
word frequency: {blue: 2, green: 1, red: 3}
orders by customer: {Ada: [coffee x1], Bob: [tea x2, cake x3]}
deduplicated: #{blue, green, red}
intersection: #{blue, red}
array[1]: Some(20)
updated: #[10, 99, 30]
original: #[10, 20, 30]
out of range: None
map functor: {blue: 20, green: 10, red: 30}
array functor: #[11, 100, 31]
```

`mapAlter` counts words without a separate insertion/update branch. `groupBy`
sorts customer keys while keeping each customer's orders in input order. Sets
remove duplicates and support intersection. Arrays support zero-based random
access and copy-on-write updates: `arraySet` leaves the original array unchanged.
The final lines exercise `Functor[Map[String]]` and `Functor[Array]`.
