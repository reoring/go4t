# Native testing helpers

Source authority: client-go v0.37.2 `util/testing/{fake_handler,
fake_openapi_handler,tmpdir,remove_file}.go` and `fake_handler_test.go`, commit
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. Sources retain Apache-2.0 attribution.

These are implementation sources, **not yet executed or accepted**. Prepared
focused command, from `ports/client-go`: `go4t test util/testing`.

## Public capability mapping

| Upstream | Native |
| --- | --- |
| `TestInterface.Errorf/Logf` | `TestInterface{error,log}` IO callbacks taking already-formatted strings |
| `LogInterface.Logf` | `LogInterface{log}` |
| `FakeHandler` | `newFakeHandler(status,body,logger,skip)`; explicit lock and typed state |
| `FakeHandler.RequestReceived/RequestBody` | `snapshot(handler).requestReceived`; each captured `Request` contains its body |
| `FakeHandler.SetResponseBody` | `setResponseBody(handler,body)` |
| `FakeHandler.ServeHTTP` | `serveHTTP(handler,Request) IO[Response]`; `newServer` supplies real loopback HTTP |
| `ValidateRequestCount` | `validateRequestCount(handler,test,count) IO[Bool]` |
| `ValidateRequest` | `validateRequest(handler,test,path,method,Option[body]) IO[Unit]` |
| `SkipRequestFn` | optional native predicate over captured request, bypassing count/checked state |
| `FakeOpenAPIServer` | `FakeOpenAPIServer{httpServer,servedDocuments,requestCounters,lock}` |
| `NewFakeOpenAPIV3Server` | `newFakeOpenAPIV3Server(path)`; actual external kube-openapi V3 service |
| private `testMux` registration/dispatch | host OpenAPI mux delegates external registered handlers; native wrapper counts every request, including 404 |
| `MkTmpdir` | `mkTmpdir(prefix) IO[String]`, ordinary classified IO failure |
| `MkTmpdirOrDie` | `mkTmpdirOrDie(prefix) IO[String]`, converts failure to `io.Panic` |
| `CloseAndRemove` | `closeAndRemove(List[Option[deps/os.File]])`; close before remove; nil entries ignored; first error fails IO |

`servedDocuments` is a native map of canonical `deps/.../spec3.OpenAPI` identities,
not JSON substitutes or copied services. `requestCounters(server)` reads the
counter snapshot under its lock. Request path/query comparisons use URL parsing
and canonical query encoding, so ordering of query pairs is not significant.
Optional expected bodies preserve `None` (do not check) versus `Some("")` (check
empty). Validation marks the handler checked even when it reports a mismatch.
Later non-skipped requests raise exactly
`got request after having been validated`.

Native diagnostic messages retain count/body/parse/nil/checked distinctions.
They render captured native request fields rather than Go's `%#v` pointer dump.
The loopback adapter buffers request bodies into immutable `Request` values.
This is an explicit native IO boundary; it is not a general streaming HTTP server
API. Streaming acceptance uses the separate socket-backed Streaming fixtures.

## Retained behavior tests

`testing_test.gt` covers real HTTP path/method/body/query validation, independent
mismatch reporting, skipped requests, response replacement, checked-state panic,
temporary-directory cleanup, and actual OpenAPI V3 discovery/404 counters.
`fixtures/apps/v1.json` is the controlled OpenAPI schema input. No expected oracle
trace or successful output is replayed by these fixtures.

No build, test, formatter, or linter was run by this helper's implementation
owner. `closeServer` closes active connections then waits for the listener;
callers should use `sync.bracket` or `sync.ensuring` for cleanup.
