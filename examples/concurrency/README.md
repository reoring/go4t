# Concurrent IO

Run from the repository root:

```sh
go run ./cmd/go4t run examples/concurrency/main.gt
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

**Cancellation stops cooperative effects.** The losing race task and expired
timeout task are cancelled and joined before returning. Their sleeping actions
stop immediately, so a print sequenced after the sleep will not run. An extern
that ignores its context is abandoned after a 100 ms cleanup grace (`timeout`:
deadline + 100 ms), and may still perform effects after the operation returns.

`conc.cancel(fiber)` requests cancellation without waiting; awaiting an unfinished,
cancelled fiber fails. A completed fiber retains its cached outcome.
`conc.withTimeout(millis, action)` fails on expiry instead of returning `None`;
timely failures still propagate in either form. `parMapIO` cancels siblings on its
first observed failure, then joins them. Uncooperative foreign code can delay that
join; cancelling the enclosing action bounds cleanup to 100 ms.

SIGINT or SIGTERM cancels the running main action and its child fibers, reporting
`error: cancelled` with exit status 1. There is no global join when main succeeds:
unawaited fibers do not keep the process alive.
