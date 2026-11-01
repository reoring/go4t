# HTTP server and client

From the repository root:

```sh
go run ./cmd/gofx run examples/http
# Or choose a free port instead of the default 8080:
go run ./cmd/gofx run examples/http -- 18080
```

Observed default-port run (exit 0):

```text
sum: 3
```

The program starts `http.serve` on a fiber, checks readiness via `/health`, calls
its own `/sum?a=1&b=2` endpoint, decodes the response with `std/json`, prints the
sum, and calls `conc.cancel` on the server fiber. It cancels the server even if
the client/decoder fails. Only loopback networking is needed; no Internet access
or external service is required. Readiness probing is bounded by 1 s and retries
Network failures while the asynchronous server binds; the HTTP library itself
does not retry. Select a free port, since an already-running service can answer
the probe. Invalid integer arguments fall back to 8080.

| Route | Result |
| --- | --- |
| `GET /health` | 200, `{"ok":true}` |
| `GET /sum?a=1&b=2` | 200, `{"sum":3}` |
| Missing or noninteger sum arguments | 400 with a diagnostic |
| Any other method/path | 404, `not found` |

`http.Request` carries method, URL, decoded path/query, headers and a String body.
`http.Response` carries status, headers and a String body. Headers are lists of
String pairs; `requestHeader` and `responseHeader` find the first value without
case sensitivity. `jsonResponse` uses `std/json.Encode` and adds
`Content-Type: application/json`.

Clients default to a 30 s deadline (`requestWithTimeout` overrides it) and use
the IO context throughout the request/body read. Non-2xx statuses are Responses,
not failures. Transport failures are typed Network/Timeout/Cancelled errors.
The server uses each request's context in its own goroutine. Handler failures
and panics become 500 with the exact message body and an `http handler:` stderr
line. Cancelling a running server closes its listener and allows up to 5 s for
graceful shutdown. `conc.cancel` itself returns immediately, and unawaited fibers
do not keep main alive.

No custom extern or HTTP shim is needed. Across the internal ABI, runtime client
results and server handler results are `(status, headers, body)` tuples using
`rt.Shapes.Tuple3/Untuple3`. The gofx module constructs Response records; for
incoming requests the runtime passes all six fields to a gofx Request adapter.
