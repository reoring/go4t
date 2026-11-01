# SQL service: Go packages from gofx

A small `std/http` service backed by SQLite. The gofx program opens a temporary
SQLite file through generated `database/sql` bindings, creates a table, inserts
three names from a `List[String]`, and exposes:

- `GET /count`: `{"count":3}` as JSON, using `QueryRowContext` and the generated
  scan-to-`List[dyn.Value]` adapter.
- `GET /metrics`: Prometheus's default Go/process metrics through `promhttp`.

## Run

Use a Pass 18 compiler with foreign-mirror/`std/dyn` support and Go 1.25 or newer
(the pinned Go dependencies require Go 1.25). From this directory, with `gofx`
on `PATH`:

```sh
gofx get modernc.org/sqlite@v1.59.0
gofx get github.com/prometheus/client_golang@v1.24.1
gofx bind --module database/sql
gofx bind example.com/sql-service/glue --shim glue -o glue
gofx run main.gfx -- 8080
```

In another terminal:

```sh
curl --fail http://127.0.0.1:8080/count
curl --fail http://127.0.0.1:8080/metrics
```

The server prints the database filename. Stop it with Ctrl-C and remove that
file afterward. An optional second argument chooses the SQLite filename:
`gofx run main.gfx -- 8080 /tmp/my-sql-service.db`. Use a new filename each run:
table creation deliberately fails instead of silently reusing an existing DB.
Without that argument the filename uses Go's temporary directory, the current
time, and a random suffix. The checked-in `go.mod` replaces `gofx` with this
repository (`../..`); no published runtime module is required.

## What is generated

`get` records the selected versions in `gofx.mod`, updates Go's dependency files,
and binds each loadable non-internal, non-test, non-main package in the requested
module. Each package produces `bindings.gfx` and `gofx_shim_<base>.go` beneath
`gofxbind/<Go import path>/`. For example:

```text
gofxbind/modernc.org/sqlite/{bindings.gfx,gofx_shim_sqlite.go}
gofxbind/github.com/prometheus/client_golang/prometheus/{bindings.gfx,gofx_shim_prometheus.go}
gofxbind/github.com/prometheus/client_golang/prometheus/promhttp/{bindings.gfx,gofx_shim_promhttp.go}
gofxbind/database/sql/{bindings.gfx,gofx_shim_sql.go}
glue/{bindings.gfx,gofx_shim_glue.go}
```

These are ordinary project-relative gofx imports, not a second package resolver.
The Go shim import paths belong to `example.com/sql-service`. Regenerating with
the same inputs produces identical bytes; `gofx get --no-bind <module>@<version>`
only updates dependencies. `bind --module` regenerates bindings without `go get`.
Applications should commit their generated binding artifacts alongside their Go
module files. This recipe and its test generate them with the compiler in use,
so a pre-Pass-18 binding snapshot cannot silently hide the new SQL adapters.

## The only hand-written Go

`glue/glue.go` is **19 physical lines**, including blank lines, imports, and braces
(`wc -l glue/glue.go`). It contains the required blank import of
`modernc.org/sqlite` and one function:

```go
func RenderMetrics() (string, error)
```

`RenderMetrics` runs the real `promhttp.Handler()` against an
`httptest.ResponseRecorder`, returning its body or a non-200 error. Ordinary
`bind` generates `glue.renderMetrics() IO[String]`; the gofx `/metrics` branch
awaits that action and sends the body through `std/http`. The native
`http.Handler` never crosses the erased gofx ABI. There is no driver-registration
directive and no hand-written SQL adapter. `tempDir` is an ordinary natural-ABI
extern for Go's `os.TempDir`, not an additional Go helper.

This is an intentional integration adjustment from the original callback-shim
example: separately generated modules give native handles distinct nominal
types, and `std/http` exposes value requests/responses rather than native HTTP
handles. The single ordinary-ABI wrapper avoids a callback round trip and three
interface-adaptation wrappers. The foreign-bind fixture, not this example,
proves the callback conversion rule.

## Offline regression and skip counts

From the repository root:

```sh
go test ./cmd/gofx -run 'TestGet|TestBindModule|TestSqlService' -v
```

`TestSqlServiceExample` uses a fresh module, `GOFLAGS=-mod=mod`, `GOPROXY=off`,
and `GOWORK=off`. It verifies both pinned versions with `go list -m -json`, runs
both `get` commands twice, checks byte-for-byte idempotence, generates the SQL
and glue bindings, and executes the metrics bridge. If a pinned module is not
in the local cache, the test skips with the exact missing version. With the
foreign-mirror slice present it then starts `gofx run`, curls both routes,
asserts count 3 and real Go/process metric names, kills the process group, and
removes the temporary module, database, generated code, and executable scratch
space.

Observed `// skipped:` counts (functions **and** methods, using the shim path):

| Package | Before Pass 18 | This get-bind slice before foreign-mirror integration |
| --- | ---: | ---: |
| `database/sql` | 78 | 78 |
| `github.com/prometheus/client_golang/prometheus` | 191 | 191 |

Enumeration changes do not change binding rules. The focused test logs the
post-integration counts on the same real packages; no reduced skip count is
claimed here without running the foreign-mirror implementation. Before that
slice is merged, the test explicitly skips only its final SQL/server phase
because embedded `std/dyn` is absent. Pending assertions are `sql.open`, the
context/variadic `dBExecContext` and `dBQueryRowContext` calls, dynamic `rowScan`,
and the live HTTP `/count` and `/metrics` requests. Generation, idempotence,
and execution of the real metrics bridge already run with the old rules.
