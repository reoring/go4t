# HTTP report service: std-only acceptance program

Two production files, one module (`package main`), and a third file containing
language tests. No external packages, `extern` declarations, or module manifest
are needed: all imports are `std/*`.

- `service.gfx`: order/report schemas, pure per-customer `Map` aggregation,
  `GET /report`, and `GET /health`.
- `main.gfx`: upstream mock, two server fibers, bounded readiness/closure probes,
  client demonstration, argument defaults, and cleanup.
- `service_test.gfx`: `testAggregate() Bool` and `testMock() IO[Bool]`.

## Run

From the repository root, with ports **18081** (upstream) and **18082** (service)
free:

```sh
go run ./cmd/gofx run examples/http-service
```

Observed stdout on Linux, exit **0**, no stderr:

```text
report 200: {Ada: 16.0, Ben: 7.25}
upstream down 502: Get "http://127.0.0.1:18081/orders": dial tcp 127.0.0.1:18081: connect: connection refused
```

Both listeners are cancelled and probed closed before the program exits; this
is a self-contained acceptance demonstration, not a process that stays running.
`GET /orders` serves three `{id, customer, amount, currency}` records, all USD.
`GET /report` uses an explicit 1000 ms GET request, derives order decoding,
aggregates amounts by customer, and derives JSON report encoding. Its body is
`{"totals":{"Ada":16,"Ben":7.25}}`. `GET /health` returns `{"ok":true}`.
Transport failures become HTTP 502 with the exact `IoError` message as the body;
JSON parse/decode failures become HTTP 500 with their exact message. Other
method/path combinations return 404 `not found`.

Choose other ports with positional program arguments (also exercised, exit 0):

```sh
go run ./cmd/gofx run examples/http-service -- 18181 18182
```

The first argument is the upstream port; the second is the service port. Missing
or non-integer arguments use their respective defaults. Supply distinct, free,
valid TCP ports. The failure message contains the chosen upstream port and the
OS's connection-error wording. Clients use loopback, but `http.serve` binds all
local interfaces: run this example on a trusted local machine.

All fixture amounts are in one currency. This example does not perform foreign
exchange conversion, currency validation, or production-grade monetary rounding;
`Float` is used deliberately for the language acceptance exercise.

## Actual line count

After `gofx fmt -w`:

| Boundary | Physical lines, including blanks |
| --- | ---: |
| Pure aggregation and HTTP handler (`service.gfx:15–42`) | 28 |
| Schemas, aggregation, handler (`service.gfx:9–42`) | 34 |
| The above plus the actual `http.serve` fork (`main.gfx:93`) | **35** |
| Entire `service.gfx`, including package/imports, plus that server-start line | 43 |
| Both production files, including mock/client/lifecycle support | 146 |
| Third file: language tests | 61 |
| Complete checked module | 207 |

The GET → decode → aggregate → serve portion meets the approximately 40-line
goal at **35 lines**, not a claimed 30-line whole executable. The formatter puts
the fold expression on one long line; physical line count is not a measure of
expression simplicity. Readiness, shutdown, the failure demonstration, imports,
and test support are counted separately rather than hidden.

## Test and tooling

With port **18083** free, run serially:

```sh
go run ./cmd/gofx test examples/http-service
```

Observed stdout, exit **0**:

```text
PASS testAggregate
PASS testMock
2 passed, 0 failed
```

The IO test uses real HTTP to the mock, checks the report, JSON content type,
health and method handling, cancels the mock to check 502, and replaces it with
malformed order JSON to check the exact 500 decode message. It always cancels
its server fibers after capturing action failures.

The [Go-developer walkthrough](../../docs/service-walkthrough.md) includes the
friction log, formatting and injected-diagnostic transcripts, a real stdio LSP
hover/definition exchange and its regression test, and measured check timings.
