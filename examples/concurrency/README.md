# Concurrent IO

Run from the repository root:

```sh
go run ./cmd/gofx run examples/concurrency/main.gfx
```

Observed output:

```text
await: first, second, third; again: first
parMapIO: [300, 200, 100]
wall time: 301 ms; sum of sleeps: 600 ms
parallel faster than sequential: true
parBoth: (7, ready)
race: fast
timeout slow: None
timeout fast: Some(ready)
```

The first three tasks sleep concurrently and can be awaited more than once.
`parMapIO` preserves input order even though the shortest task completes first.
Its measured wall time was 301 ms, below the 600 ms sum of the requested sleeps;
wall time varies with scheduling and system load.

`parBoth` preserves argument order and supports different result types. `race`
returns the first completion, including failure. `timeout` returns `None` for a
late result and `Some(value)` for an early success; early failures still fail IO.

**Abandonment is not cancellation.** The losing race task and expired timeout
task keep running and may perform effects while the process lives. Their results
are discarded, and their eventual failures/panics are captured. The runtime does
not wait for them, or other unawaited fibers, after `main` finishes. This example
therefore exits without waiting for either 1,000 ms slow task.
