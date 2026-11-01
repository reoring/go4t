# Go shell, go4t core

`core.gt` models a controller's pure decision: observed state becomes a `Step`.
Go owns the mutable world and can switch on typed values rather than inspecting
an erased runtime representation. See [SPEC §8.7](../../SPEC.md#87-exported-go-api).
Each conversion copies on every call, so exported functions should take a projection containing only what the decision needs, not whole observed objects.

Build and run from a temporary Go module (starting in the repository root):

```sh
repo="$PWD"
work=$(mktemp -d)
cd "$work"
go mod init goshell
go mod edit -require=go4t@v0.0.0 -replace="go4t=$repo"
mkdir bin
go build -o "$work/bin/go4t" go4t/cmd/go4t
export PATH="$work/bin:$PATH"
cp "$repo/examples/go-shell/core.gt" ./core.gt
cp "$repo/examples/go-shell/main.go" ./main.go
go generate main.go
go run main.go
go vet ./core
```

The checked-in Go consumer has an `ignore` build tag so ordinary repository
package discovery does not depend on generated code. `go generate main.go` and
`go run main.go` explicitly select it. No generated package or extra `go.mod` is
checked in for this example. `TestGoShellExample` creates its own temporary
module, emits the package, and compiles/runs this same consumer (removing the
example-only build tag and adapting its module import path). It also runs
`go generate ./...` and checks that every generated file is byte-identical.

In a real consumer package, keep the directive in a Go file without the example's
`ignore` tag:

```go
//go:generate go4t build --emit-go core --package core core.gt
```

Commit `core/go4t_export.go` and its sibling generated files so ordinary
`go build` needs no generation step. Pin the go4t version and run
`go generate ./... && git diff --exit-code` in CI to catch drift.

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
        fmt.Printf("cleanup: %v\n", step)
    default:
        panic("expected cleanup")
    }
}
```

[`main.go`](main.go) additionally verifies record, enum, list and option round
trips, including independent copies of nested slices. Its output is:

```text
pending: [cache secrets]
cleanup: Cleanup(7)
round trips: Ready record list option
```

`%v` on a record or constructor value uses the go4t type's `Debug` instance,
including custom instances; these structs have no `String()` without `Debug`.
Enums always print their constructor names, independently of `Debug`.

`_helper` stays private. The generic `identity[A]` is not exported and is
explained by `// not exported: identity (type parameters)` in `go4t_export.go`.
The empty go4t `main` retains the existing `Main() rt.IO` embedding entry point;
the decision call does not need it.
