# Log-line parser and table

Run from the repository root:

```sh
go run ./cmd/gofx run examples/stdlib-extras/main.gfx
```

Observed output:

```text
accepted 4, rejected 1
#   | SERVICE  | LEVEL | MESSAGE
-- page 1 --
1   | api      | WARN  | slow request
2   | api      | ERROR | timeout
-- page 2 --
3   | cache    | INFO  | warm
4   | worker   | INFO  | started
deferred checksum: 50005000
```

`regexCaptures` extracts the level and payload, then `splitOnce` divides the payload
at its first `=`. `partition` separates parsed and rejected lines. `sortOn` orders
accepted records by service, preserving the two API records' original order;
`indexed`, `chunksOf`, and rune-width `padRight` produce a numbered, paged table.

The expensive checksum is wrapped in `delay(func(_) => ...)` before the IO sequence
and evaluated only at `force`. `Lazy` is deliberately **non-memoising**: calling
`force` again would repeat that pure computation, not retrieve a cached value.
