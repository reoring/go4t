# A std-only HTTP service, for a Go developer

The runnable [HTTP-service example](../examples/http-service/README.md) exercises
P1: JSON GET → derived decode → per-customer aggregation → JSON response, using
only the standard library. It is deliberately both a service and its acceptance
client: it runs an upstream and a service, reads a report, stops the upstream,
observes a 502, then stops the service and exits successfully.

This walkthrough records the actual implementation and tooling session from
2026-11-01. The exact run output and honest line-count boundaries are in the
example README. The core is **35 physical lines** including data declarations
and the server-start statement; the two complete production files are **146
lines**. The third, test-only-by-convention file adds **61 lines**. This is not a
claim that an entire robust demonstration fits in 30 lines.

## 1. Files, imports, and records

`main.gt` and `service.gt` both say `package main`. Like Go files in one
package, their top-level declarations are mutually visible. Unlike Go module
resolution, a directory argument loads its immediate `.gt` files in lexical
filename order; it does not recursively traverse subdirectories. A `go4t.mod`
is unnecessary for this std-only directory. Imports are file-scoped, so each
file imports the standard modules whose qualified names it uses. The third file,
`service_test.gt`, is loaded too: `_test` is a naming convention here, not a
Go-style build exclusion.

The service's data declarations are:

```text
type Order = Order{id: Int, customer: String, amount: Float, currency: String} deriving json.Decode, json.Encode

type Report = Report{totals: Map[String, Float]} deriving json.Encode, json.Decode

type Health = Health{ok: Bool} deriving json.Encode
```

These are single-constructor records, not Go struct tags or generated files you
maintain. `json.Decode` checks each required field and its type; `json.Encode`
produces JSON objects using those field names. The mock needs `Order` encoding,
the service needs its decoding, and the acceptance client needs `Report`
decoding. Derivation supplies the class instances for those uses.

The mock supplies Ada's USD orders of 12.5 and 3.5, and Ben's USD order of 7.25.
All amounts are in the same currency. Aggregation deliberately keys only by
customer, not by currency: this is neither an FX converter nor a general
multi-currency ledger. `Float` is binary64, not decimal money. JSON can render
16.0 as `16`, while `show` for the decoded `Float` prints `16.0`.

## 2. Pure aggregation with `Map`

The entire pure function is:

```text
func aggregate(orders List[Order]) Map[String, Float] {
  fold(orders, map.emptyMap(), func(totals, order) => map.mapAlter(totals, order.customer, func(previous) => Some(result.fromOption(previous, 0.0) + order.amount)))
}
```

Think of this as a Go loop with an accumulator, except that `Map` is persistent:
`mapAlter` returns the updated map without mutating the previous one. The fold
starts with an empty map and threads each updated map into the next step.
`previous` is `Option[Float]`, not a Go zero value plus an `ok` flag.
`result.fromOption(previous, 0.0)` supplies zero for an absent customer;
`Some(newTotal)` tells `mapAlter` to insert/replace the entry. Returning `None`
would delete it, but this aggregator never does that.

String keys have `Ord`, and map traversal/JSON object encoding is in ascending
key order. Output order therefore does not depend on Go map iteration. An empty
order list yields an empty map, and repeated customer names accumulate rather
than overwrite. `testAggregate` asserts both the fixture totals and the empty
case.

## 3. HTTP routing and the upstream GET

The handler signature is:

```text
func handle(upstream String, r http.Request) IO[http.Response]
```

Unlike a Go `http.HandlerFunc`, this returns a deferred action whose result is a
response value. It does not write to a `ResponseWriter`. Its routes are exact
method/path predicates:

| Predicate | Result |
| --- | --- |
| `r.method == "GET" && r.path == "/health"` | 200 JSON `{"ok":true}` |
| `r.method == "GET" && r.path == "/report"` | Fetch, decode, aggregate, respond |
| Anything else, including `POST /report` | 404 `not found` |

The report branch sends this GET:

```text
fetched <- io.tryIO(http.requestWithTimeout(http.Request("GET", upstream + "/orders", "", [], [], ""), 1000))
```

`http.Request` fields are method, URL, path override, query pairs, header pairs,
and body. Empty path means use the URL's path. The explicit 1000 ms request
budget covers connection, redirects, and reading the full body. It also obeys
an enclosing cancellation/deadline. There is no service retry policy.
`http.get(url)` is more concise but has the library's default 30-second budget;
it is used by the demo client and by probes wrapped in a shorter deadline.

Responses are values even for non-2xx HTTP statuses. `tryIO` catches transport
failures, not HTTP status codes. This controlled upstream only serves successful
orders; the example adds no policy for other upstream status codes. Any returned
body is decoded by the report branch.

## 4. `tryIO`, `IoError`, and decode errors

`IO[A]` is an effectful action returning `A`; constructing it is not running it.
A `do` block sequences actions, and `<-` binds their successful results. A
failure skips the remaining normal continuation, much as an early error return
would in Go.

`io.tryIO(action)` changes the action's outcome into
`IO[Result[io.IoError, A]]`. `IoError` is a tagged sum including `Network(String)`,
`Timeout`, `Cancelled`, and other failure kinds. The HTTP handler maps any caught
IO error as follows:

```text
Err(e) => http.response(502, io.ioErrorMessage(e))
```

`ioErrorMessage` preserves the error's message without the constructor wrapper.
For this Linux run, stopping the upstream produced exactly:

```text
Get "http://127.0.0.1:18081/orders": dial tcp 127.0.0.1:18081: connect: connection refused
```

That is the 502 body, not an added `error:` prefix. `show(e)` would return the
same message; `debug(e)` is for structural constructor rendering. When cleanup
is complete and a captured error should still fail the demo, `io.ioRaise(e)`
re-raises the typed error rather than changing it into a generic Failure.

JSON parsing and schema decoding are separate pure operations, both using
`Result[String, _]`:

```text
let decoded: Result[String, List[Order]] = do {
  value <- json.parseJson(response.body)
  json.decode(value)
}
```

The annotation selects the desired `Decode` instance. This `do` sequences
Results rather than IO: malformed text stops at `parseJson`, and a syntactically
valid but wrongly shaped value stops at `decode`. Both feed
`Err(message) => http.response(500, message)`. The IO test serves `[{}]`, which
is valid JSON but lacks `id`, and asserts the exact status **500** and body
**`expected field at $[0].id`**. A decoding failure is returned explicitly as a
response; it does not deliberately fail the handler's IO or emit a handler
failure log.

## 5. `jsonResponse` and the returned report

On successful decode:

```text
Ok(orders) => http.jsonResponse(200, Report(aggregate(orders)))
```

`jsonResponse` is pure: it uses the derived `Encode` instance, renders compact
JSON, and adds `Content-Type: application/json`. The enclosing `ioPure` turns
that already-constructed response into a successful handler action. The report
shape is `{"totals":{"Ada":16,"Ben":7.25}}`. The client parses and derives
`Report` decoding again before displaying `{Ada: 16.0, Ben: 7.25}`; it does not
just print the server's JSON as though that proved decoding.

## 6. `fork`, `cancel`, `withTimeout`, and lifecycle

`main` reads program arguments and defaults to upstream port 18081 and service
port 18082. It starts two fibers:

```text
upstream <- conc.fork(http.serve(upstreamPort, mock))
service <- conc.fork(http.serve(servicePort, handle(upstreamURL, _)))
```

The underscore section binds `upstreamURL`, leaving a one-argument request
handler. A fiber is a goroutine-like running action with a child context, not a
plain goroutine handle divorced from cancellation. `serve` blocks until its
context is cancelled. Clients use loopback, but the library server binds all
interfaces; the example is for a trusted local machine and free, distinct ports.

A successful `fork` is **not** a listener-readiness acknowledgement. `waitUp`
probes a real endpoint, retries only `Network` failures with a 10 ms cooperative
sleep, and rejects a non-200 response. Each probe sequence is enclosed by
`conc.withTimeout(2000, ...)`, so the whole sequence, including requests and
sleeps, has a two-second budget. `withTimeout` raises a typed Timeout; it does not
return a Boolean or an optional result.

After displaying the report, `demonstrate` calls `conc.cancel(upstream)`. Cancel
requests cancellation and returns immediately; it does not join. Even
`conc.await` can return Cancelled before the server has finished closing its
listener, so awaiting a cancelled fiber would not be a closure acknowledgement.
`waitDown` instead observes that HTTP transport has become unavailable, again
under a two-second budget, before making the second `/report` request. This
avoids assuming an arbitrary shutdown delay. A nonempty 502 body is not treated
as an IO failure by the demo client; it checks the response status explicitly.

`main` captures the demonstration outcome with `tryIO`, cancels **both** server
fibers, probes both endpoints closed, and then returns the original outcome.
Thus an ordinary action failure does not skip the cancellation statements.
The successful default run exits **0** after printing the report and the
expected failure. These probes establish local endpoint state, not ownership
of an already occupied port: reserve free ports for the demonstration.

## 7. The two language tests

`go4t test` discovers zero-argument, nongeneric `test*` functions returning `Bool`
or `IO[Bool]` and replaces the normal main entry point. It does not run this
example's demonstration as test setup.

- `testAggregate() Bool`: two Ada orders become 16.0, Ben remains 7.25, and an
  empty list produces an empty map.
- `testMock() IO[Bool]`: starts the actual mock on port **18083**, fetches
  `/orders` over HTTP, calls the real handler (which itself fetches over HTTP),
  decodes and checks the report, checks its content type, health response and
  wrong-method 404, stops the upstream and checks 502, then starts a deliberately
  malformed upstream to check the exact 500 decode message. Both server lifetimes
  capture failures and cancel before propagating the outcome.

Run this IO test serially with 18083 free. There is no standard server handle
that exposes an ephemeral port; the fixed test port is an explicit constraint,
not a claim of parallel-safe network-test isolation. The separate `run`
acceptance exercises the service listener and client path end to end; the IO
test uses direct service-handler calls to isolate assertions while keeping the
upstream boundary real.

## Friction log

No unexpected parser, checker, code generator, runtime, formatter, or LSP defect
was encountered. The initial complete program checked and ran successfully.
The following is the complete lookup/helper/formatting friction observed in
this exercise, not a proposed language redesign. There was no unexpected
diagnostic to suppress or compiler/library bug to patch.

| Point and exact code | What had to be looked up or was missing | Diagnostic |
| --- | --- | --- |
| `package main` in all three files; `import "std/json"` repeated where used | SPEC §2: directory loading, file-scoped imports, shared declarations, no required manifest for std-only use. `_test.gt` is not excluded from ordinary loads. | None; lookup only. |
| `http.requestWithTimeout(http.Request("GET", upstream + "/orders", "", [], [], ""), 1000)` | `docs/stdlib.md`, `std/http/http.gt`, and `examples/http/main.gt`: constructor field order, millisecond budget, URL/path semantics. Wanted a GET-with-timeout convenience helper; the API requires a six-field Request. | None; existing API works. |
| `fetched <- io.tryIO(...)` and `io.ioErrorMessage(e)` | SPEC §6.2/§6.4: typed failures versus ordinary non-2xx responses, message-preserving 502 body, `ioRaise` versus raising a generic failure. | None; lookup only. |
| `let decoded: Result[String, List[Order]] = do { value <- json.parseJson(response.body); json.decode(value) }` | `std/json` docs and HTTP example: parsing text is separate from derived decoding, and the result annotation selects the schema. Wanted a text-to-derived-value helper; used the existing two-step idiom instead. | None; existing API works. |
| `map.mapAlter(totals, order.customer, func(previous) => Some(result.fromOption(previous, 0.0) + order.amount))` | `std/map/map.gt` and sales-report example: persistent-map update signature, absent-value handling, fold argument order, deterministic key ordering. The package-qualified names are more verbose than Go's map indexing. | None; lookup only. |
| `show(report.totals)` and `Report(aggregate(orders))` | Float/Map/JSON docs: integral Floats display with `.0`, but JSON numeric text need not retain it. | None; lookup only. |
| `portArg(tail(argv), 18082)` and `head(args) \|> flatMap(string.parseInt) \|> result.fromOption(_, fallback)` | Existing HTTP/sales examples and IO docs: program arguments exclude the executable; missing or noninteger values can fall back through Option. | None; lookup only. |
| `conc.fork(http.serve(...))`, `conc.cancel(upstream)`, `conc.withTimeout(2000, waitDown(...))` | SPEC §6.3/§6.4, stdlib docs, `rt/conc.go`: fork is not ready, cancel is not joined, cancelled await is not listener closure. Wanted a ready/closed server handle; bounded endpoint probes are necessary with this API. | None; lifecycle friction, not a bug. |
| `let port = 18083` in `testMock` | `serve` returns `IO[Unit]`, not a listener/address handle. Binding port 0 cannot give the test a URL. A free fixed port and serial execution remain necessary here. | None; missing test convenience. |
| `func testAggregate() Bool` / `func testMock() IO[Bool]` | SPEC §8, `examples/cli-tools/`, CLI test code: discovery and return-type conventions, replacement main, IO failure reporting. | None; lookup only. |
| `aggregate(sampleOrders())` | `docs/lsp-verification.md`, `lsp/sales_report_test.go`, test-client helpers: Content-Length framing, initialization/shutdown, UTF-16 zero-based positions, publication order, cross-file definition. | None; real stdio session had empty diagnostics. |
| `"expected field at $$[0].id"` after formatting | `fmt -w` canonicalized the original `"expected field at $[0].id"` with the literal-dollar escape; runtime text still contains one `$`. It also inserted branch `\|` markers and collapsed the fold and multi-line Boolean assertion to long lines. | None; formatting remained semantically correct and `fmt -l` was empty. |
| `Report(aggregate(200))` | Deliberately injected wrong argument to compare default and full type printing; restored immediately afterward. This was not an accidentally encountered bug. | `type mismatch: expected List[Order], got Int` (full transcript below). |

## Tooling pass: observed commands and output

All commands below ran from the worktree root. Only `go test` used the prescribed
VM lock wrapper. No repository-wide test/build/lint/format pass was run, no
compiler/runtime changes were made, and `selfhost/` was untouched.

### Formatting and normal execution

```sh
go run ./cmd/go4t check examples/http-service
go run ./cmd/go4t fmt -w examples/http-service
go run ./cmd/go4t fmt -l examples/http-service
go run ./cmd/go4t run examples/http-service
go run ./cmd/go4t run examples/http-service -- 18181 18182
go run ./cmd/go4t test examples/http-service
```

Every command exited **0**. The initial check and both format commands produced
no output; in particular the `-l` result was clean after `-w`. Both runs printed
the two lines recorded in the README (the alternate-port failure line contained
18181 instead of 18081). The test output was exactly:

```text
PASS testAggregate
PASS testMock
2 passed, 0 failed
```

### Injected type error: default versus `--full-types`

The temporary edit at `service.gt:34` replaced `aggregate(orders)` with
`aggregate(200)`. No other part of the program changed for this probe.

```sh
go run ./cmd/go4t check examples/http-service
```

Observed stderr, exit **1**:

```text
examples/http-service/service.gt:34:67: type mismatch: expected List[Order], got Int
            Ok(orders) => http.jsonResponse(200, Report(aggregate(200)))
                                                                  ^~~
exit status 1
```

```sh
go run ./cmd/go4t check --full-types examples/http-service
```

Observed stderr, exit **1**:

```text
examples/http-service/service.gt:34:67: type mismatch: expected List[Order], got Int
            Ok(orders) => http.jsonResponse(200, Report(aggregate(200)))
                                                                  ^~~
exit status 1
```

These messages are intentionally reported as **identical**: neither type is
long enough to be elided. SPEC §5.2 only elides subtrees at depth 3 when a complete
type spelling exceeds 160 characters; `--full-types` disables that elision but
does not expand aliases. This service does not naturally exercise the long-type
case, and this comparison is not evidence for that separate behavior. The final
`exit status 1` line is from the outer `go run`. The edit was restored to
`aggregate(orders)` before the LSP session and timing checks.

### Real stdio LSP hover and definition

Built a temporary CLI executable to avoid repeatedly including Go compilation:

```sh
go build -o examples/http-service/.tooling-go4t ./cmd/go4t
examples/http-service/.tooling-go4t lsp
```

A Python subprocess client sent UTF-8 JSON bodies with
`Content-Length: <byte count>\r\n\r\n` framing, exactly as in the
[sales-report session](lsp-verification.md). The CLI exited **0** after
shutdown/exit, and stderr was empty. `$SERVICE` below replaces the absolute
`file:///workspace/wip/go4t/.wt/http-service/examples/http-service` URI prefix.
`<service_test.gt>` and `<service.gt>` mean the exact complete on-disk source
texts, not literal placeholder text sent to the server. Positions and ranges
are zero-based UTF-16 positions.

```text
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
← {"jsonrpc":"2.0","id":1,"result":{"capabilities":{"definitionProvider":true,"documentSymbolProvider":true,"hoverProvider":true,"positionEncoding":"utf-16","textDocumentSync":1},"serverInfo":{"name":"go4t"}}}
→ {"jsonrpc":"2.0","method":"initialized","params":{}}
→ {"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"$SERVICE/service_test.gt","languageId":"go4t","version":1,"text":"<service_test.gt>"}}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SERVICE/service_test.gt","version":1,"diagnostics":[]}}
→ {"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"$SERVICE/service.gt","languageId":"go4t","version":1,"text":"<service.gt>"}}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SERVICE/service.gt","version":1,"diagnostics":[]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SERVICE/service_test.gt","version":1,"diagnostics":[]}}
→ {"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":"$SERVICE/service_test.gt"},"position":{"line":9,"character":2}}}
← {"jsonrpc":"2.0","id":2,"result":{"contents":{"kind":"plaintext","value":"aggregate : func(List[Order]) Map[String, Float]"},"range":{"start":{"line":9,"character":2},"end":{"line":9,"character":11}}}}
→ {"jsonrpc":"2.0","id":3,"method":"textDocument/definition","params":{"textDocument":{"uri":"$SERVICE/service_test.gt"},"position":{"line":9,"character":2}}}
← {"jsonrpc":"2.0","id":3,"result":{"uri":"$SERVICE/service.gt","range":{"start":{"line":14,"character":5},"end":{"line":14,"character":14}}}}
→ {"jsonrpc":"2.0","id":4,"method":"shutdown","params":{}}
← {"jsonrpc":"2.0","id":4,"result":null}
→ {"jsonrpc":"2.0","method":"exit"}
```

`lsp/http_service_test.go` replays this through the existing Content-Length-framed
`Serve` test client. It reads the real example files, checks empty diagnostics,
URI/version publication order, the exact aggregate hover signature/range, its
cross-file definition into `service.gt`, and shutdown. It does not simulate an
editor or claim VS Code attachment.

### Compile-scale timing for this actual program

Used Bash's `time` keyword on the restored program and the prebuilt CLI, not on
`go run` (which would include building the compiler):

```sh
TIMEFORMAT='wall_seconds=%3R user_seconds=%3U sys_seconds=%3S'
for run in 1 2 3 4 5; do
  printf 'sample %s: ' "$run"
  time examples/http-service/.tooling-go4t check examples/http-service
done
```

All five checks exited **0**, with no check diagnostics. Observed timings:

| Sample | Wall ms | User CPU ms | System CPU ms |
| --- | ---: | ---: | ---: |
| 1 | 22 | 15 | 9 |
| 2 | 21 | 18 | 8 |
| 3 | 27 | 20 | 7 |
| 4 | 18 | 18 | 9 |
| 5 | 19 | 15 | 9 |

Median wall time: **21 ms**; range **18–27 ms**. Each sample is a new CLI process
checking the **207-line, three-file module**, including imported embedded
standard-library sources. These are local warm-filesystem observations, not a
cold-cache benchmark or a CI performance budget. `check` loads and typechecks;
it does not validate foreign signatures, generate/build Go, or execute the
servers. The original two-file production program is 146 lines, but these
measurements intentionally include its third test file because directory
loading includes that file.

### Focused Go regression checks and cleanup

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests -- go test ./lsp ./docs
```

Observed test output, exit **0**:

```text
ok  	go4t/lsp	0.317s
ok  	go4t/docs	0.250s
```

The wrapper acquired shared slot 1 and released it after the command. This
includes the new HTTP-service LSP session replay and the existing documentation
checks. The documentation package's existing executable-snippet inventory does
not automatically include this new walkthrough; its excerpts are explanatory,
and the real example was exercised by the separate CLI checks above.

The temporary `.tooling-go4t` binary was removed with
`rm -- examples/http-service/.tooling-go4t` (exit 0). The injected type error was
restored, the stdio subprocess exited normally, and both demonstration runs and
the IO test cancelled their server fibers. No helper scripts or generated
binaries were retained. No commit, push, live-cluster access, compiler/runtime
change, or `selfhost/` change was made.

Residual constraints are explicit: free local ports are required (18083 for
the serial IO test), servers bind all local interfaces, connection-refused text
is OS-dependent, all fixture orders share USD, and the short injected type error
does not demonstrate long-type elision. No unresolved implementation defect was
observed in this acceptance exercise.
