# Local Go-module dependency

`lib/` is a library module named `example.com/lib`, with `go.mod`, `gofx.mod`
and the `shapes/` source directory. `app/` imports `example.com/lib/shapes`.
Its `gofx.mod` requires `example.com/lib v1.0.0`; its **go.mod** replaces the
library with `../lib` and the runtime with the repository at `../../..`.
These relative replacements keep this example portable within the checkout.
No network or published module version is needed.

From the repository root:

```sh
gofx run examples/deps/app
# Without installing:
go run ./cmd/gofx run examples/deps/app
```

Observed output:

```text
area: 42
```

The loader asks `go list -m -json example.com/lib` in `app/`, uses its resolved
`Dir`, and loads `shapes/*.gfx`. Resolution order is the importing source root,
matching gofx.mod requirements, GOFX_PATH, then embedded std. Existing empty or
invalid source directories are errors rather than fallback opportunities.

To update the local dependency with an installed CLI:

```sh
cd examples/deps/app
GOPROXY=off gofx get example.com/lib@v1.1.0
```

`get` runs `go get` in the project root, then updates the existing gofx.mod
requirement to the version selected by Go. It does not append duplicates.
The local replacement allows this version change without a remote tag. Go may
also edit go.mod/go.sum; it owns replacements, selected versions and checksums.

For a new application elsewhere, run `gofx init example.com/myapp` in an empty
directory. This writes both manifests, with Go 1.22 and `require gofx v0.0.0`.
If `go list -m -json gofx` resolves from that directory, its `Dir` becomes the
runtime replacement. Otherwise the embedded runtime is cached at
`$XDG_CACHE_HOME/gofx/runtime/<sha256>` on Linux, falling back to
`$HOME/.cache/gofx/runtime/<sha256>`, and the absolute cache path is used.
The digest covers embedded file paths and contents. No `.gofx/runtime` directory
is created inside the application. Existing manifests are not overwritten;
the absolute runtime replacement must be adjusted if moving to another machine.
