# Go shell, gofx core

`core.gfx` models a controller's pure decision: observed state becomes a `Step`.
Go owns the mutable world and can switch on typed values rather than inspecting
an erased runtime representation. See [SPEC §8.7](../../SPEC.md#87-exported-go-api).

Build and run from a temporary Go module (starting in the repository root):

```sh
repo="$PWD"
work=$(mktemp -d)
cd "$work"
go mod init goshell
go mod edit -require=gofx@v0.0.0 -replace="gofx=$repo"
go run gofx/cmd/gofx build --emit-go ./core --package core "$repo/examples/go-shell/core.gfx"
cp "$repo/examples/go-shell/main.go" ./main.go
go run main.go
go vet ./core
```

The checked-in Go consumer has an `ignore` build tag so ordinary repository
package discovery does not depend on generated code. `go run main.go` explicitly
selects it. No generated package or extra `go.mod` is checked in.
`TestGoShellExample` creates its own temporary module, emits the package, and
compiles/runs this same consumer (adapting only its module import path).

The core Go consumer is:

```go
package main

import (
    "fmt"
    "goshell/core"
)

func main() {
    observed := core.Observed{
        ClassMatches: true,
        PendingDeps:  []string{"cache", "secrets"},
        ValuesValid:  true,
    }
    switch step := core.Decide(observed).(type) {
    case core.StepDependenciesPending:
        fmt.Println("pending:", step.F0)
    default:
        panic("expected pending dependencies")
    }
    observed.Deleting = true
    switch step := core.Decide(observed).(type) {
    case core.StepCleanup:
        fmt.Println("cleanup:", step.F0)
    default:
        panic("expected cleanup")
    }
}
```

[`main.go`](main.go) additionally verifies record, enum, list and option round
trips, including independent copies of nested slices. Its output is:

```text
pending: [cache secrets]
cleanup: 7
round trips: Ready record list option
```

`_helper` stays private. The generic `identity[A]` is not exported and is
explained by `// not exported: identity (type parameters)` in `gofx_export.go`.
The empty gofx `main` retains the existing `Main() rt.IO` embedding entry point;
the decision call does not need it.
