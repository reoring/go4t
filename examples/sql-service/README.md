# SQL service: Go packages from go4t

A small `std/http` service backed by SQLite. The go4t program imports
`go:database/sql` and `go:example.com/sql-service/glue` directly, opens a temporary
SQLite file, creates a table, inserts three names, and exposes:

- `GET /count`: `{"count":3}` as JSON, using `QueryRowContext` and the generated
  scan-to-`List[dyn.Value]` adapter.
- `GET /metrics`: Prometheus's default Go/process metrics through `promhttp`.

## Run

Use Go 1.25 or newer (required by the pinned dependencies). Build the CLI from
the repository root before entering this example's separate Go module:

```sh
go build -o /tmp/go4t-cutover ./cmd/go4t
cd examples/sql-service
/tmp/go4t-cutover run . -- 18099
```

The first run compiles modernc SQLite and client_golang from source and can take
several minutes; later runs use Go's build cache and the import-time binding cache.
No binding-generation command or committed generated sources are needed.
`go4t get <module>@<version>` only updates dependencies and records the selected
version in `go4t.mod`.

In another terminal:

```sh
curl --fail http://127.0.0.1:18099/count
# {"count":3}
curl --fail http://127.0.0.1:18099/metrics
# Prometheus text including go_goroutines and process_cpu_seconds_total
```

The server prints the database filename. Stop it with Ctrl-C and remove that
file afterward. An optional second argument chooses the SQLite filename:
`/tmp/go4t-cutover run . -- 18099 /tmp/my-sql-service.db`. Use a new filename
each run: table creation deliberately fails instead of silently reusing an
existing DB. Without that argument the filename uses Go's temporary directory,
the current time, and a random suffix.

The checked-in `go.mod` requires `go4t` and replaces it with this repository
(`../..`); `deps.go` retains that runtime dependency when tidying the module.
Commit module files and checksums, not generated bindings. Shims are generated
beneath the ignored build output; checking the source does not write them.

## Hand-written Go integration

`glue/glue.go` registers `modernc.org/sqlite` through its blank import and exposes:

```go
func RenderMetrics() (string, error)
```

`RenderMetrics` runs the real `promhttp.Handler()` against an
`httptest.ResponseRecorder`, returning its body or a non-200 error. Import-time
binding exposes `glue.renderMetrics() IO[String]`; the go4t `/metrics` branch
awaits that action and sends the body through `std/http`. The native
`http.Handler` never crosses the erased go4t ABI. Channel-based metrics
collection stays in Go. There is no hand-written SQL adapter. `tempDir` is an
ordinary natural-ABI extern for Go's `os.TempDir`, not another Go helper.

## Offline regression

From the repository root:

```sh
go test ./cmd/go4t -run 'TestGet|TestSqlService' -v
```

`TestSqlServiceExample` copies the source and module files into a fresh module,
uses `GOFLAGS=-mod=mod`, `GOPROXY=off`, and `GOWORK=off`, and checks the pinned
SQLite and Prometheus versions are cached. If either is absent, it skips with
the exact missing version. Otherwise it exercises the metrics bridge through a
`go:` import, starts the service, curls both routes, asserts count 3 and real
Go/process metric names, and checks that SQLite created a nonempty database.
Cleanup kills the process group and removes the temporary module, database,
generated code, and executable scratch space.
