# node-drain-typed: observe → decide → apply, all in go4t

The same node-drain controller as [node-drain-controller](../node-drain-controller),
without the Go shell. Kubernetes is reached through `go:` imports of the typed
clientset; nothing is committed under `go4tbind/`.

```
decide.gt   types, projection corev1.Pod → Pod, decision core, 15 laws
main.gt     observe (List/Get), apply (merge-patch cordon, EvictV1), entry
deps.go     keeps the Go modules the imports need in go.sum
```

Run against any cluster (needs `go.mod`'s `replace github.com/reoring/go4t => ../..`):

```sh
kind create cluster --name go4t-drain
go4t check examples/node-drain-typed                 # 15 laws
go4t run examples/node-drain-typed -- go4t-drain-control-plane ~/.kube/config
go4t run examples/node-drain-typed -- go4t-drain-control-plane ~/.kube/config --release
```

Each run is one reconcile and prints the action it applied. Against a kind
cluster with a 2-replica deployment, the sequence observed was `Cordon`, seven
`Evict(ns, name)` (ReplicaSet pods only; `kindnet`/`kube-proxy` DaemonSet pods
and the static control-plane pods stay), then `MarkDrained`; `--release`
uncordons and the next run is `Noop`.

## What the comparison measured

| | Go shell + go4t core | go4t only |
| --- | --- | --- |
| Effectful code | `controller.go` 122 lines + fake `Cluster` interface | `main.gt` 130 lines against the real clientset |
| Projection to `Observed` | Go, untestable by laws | `project`, `podKind`, `nodeState`: pure, 5 laws |
| Decision laws | 10 | the same 10, unchanged, plus 5 projection laws |
| `go4t check` | <1 s | 28.88 s on an empty binding cache, 2.57 s on hits (247 bound packages; measurements below) |
| `go4t build` cold / warm bindings | — | 53.93 s / 30.14 s before export-discovery batching → 31.81 s / 8.92 s after; warm Go build cache |

The projection laws are the new territory: a DaemonSet owner anywhere in
`ownerReferences` yields `DaemonSet` (induction over the owner list), the
mirror annotation wins over ownership (symbolic `Map` from ADR 0002), and
`creationTimestamp` (a `meta.Time` handle) never influences the projection
(ADR 0004/0011). The timestamp law still quantifies over every handle and uses
`refl`: it neither inspects nor compares the handle, so it needs no Eq instance
and no sampled fallback. All 15 laws remain proved.

Before the per-package binding cache every `check` regenerated all bindings
(47 s). The older `run` figures (150 s cold / 85 s warm) also predate that cache;
they are not current startup measurements. Cache switches are in
[docs/cli.md](../../docs/cli.md).

### Build-cost remeasurement (2026-11-11)

Machine: Linux 6.8.0-64-generic, AMD Ryzen 9 7950X3D KVM VM (30 vCPUs),
`go version go1.26.1 linux/amd64`, `GOMAXPROCS=7`, Go package parallelism 8.
These single measurements were **taken under concurrent load**, using the shared
VM test lock; wall times exclude lock admission. No controller binary was run,
and no cluster or kubeconfig was accessed.

Seconds, **before → after** the changes below. Both arms kept Go's build cache
warm and used separate private binding caches for `check` and `build`.
(a) starts with empty binding caches; (b) repeats unchanged; (c) appends only a
trailing comment to `main.gt`, restored afterward. `check` proved all 15 laws in
every case. Binding counts were 247 for checking and 251 for building, including
classifier-only companions.

| Case | Bind/load | Type-check / laws | go4t codegen | Emit files | `go build` | Wall |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| (a) check | 22.14 → 27.92 | 0.80 → 0.90 | — | — | — | 22.98 → 28.88 |
| (a) build | 22.53 → 24.64 | 0.74 → 0.80 | 30.32 → 5.98 | 0.05 → 0.06 | 0.19 → 0.23 | 53.93 → 31.81 |
| (b) check | 1.42 → 1.64 | 0.82 → 0.88 | — | — | — | 2.27 → 2.57 |
| (b) build | 1.58 → 2.08 | 0.77 → 0.82 | 27.48 → 5.65 | 0.06 → 0.08 | 0.18 → 0.22 | 30.14 → 8.92 |
| (c) check | 1.55 → 1.45 | 0.82 → 0.80 | — | — | — | 2.43 → 2.28 |
| (c) build | 2.44 → 1.62 | 1.04 → 1.12 | 32.10 → 5.67 | 0.11 → 0.06 | 0.25 → 0.17 | 36.08 → 8.72 |
| Temporary binary backend after an enclosing-repository commit (separate diagnostic) | — | — | — | — | 39.58 → 3.35 | 39.58 → 3.35 |

The avoidable work was **export discovery**, not recompilation of unchanged
shims: codegen invoked `go list -json -export -deps` 248 times, taking 20.66 s
inside the warm 27.48 s codegen phase. Resolving all extern imports in one call
reduced that to one invocation (0.22 s), while retaining Go signature validation.
All 509 generated Go files had identical paths and SHA-256 hashes across all six
builds. Shims were already separate Go packages in the stable `_gen/go4tbind`
tree; rewriting identical bytes did not defeat Go's cache. Warm final `go build`
invocations performed no compilation or linking. No emission layout or generated
Go code changed.

VCS stamping was a separate source of main-package recompilation: an unrelated
commit in the enclosing repository changed Go's embedded revision despite
identical generated files. The diagnostic row compares that observed compile
plus link with a fresh-output, compile-only probe using `-buildvcs=false`
(zero compiler invocations, one linker invocation, no program execution).
`run`, `test` and `eval` now disable stamping for their temporary binaries;
`build` retains Go's default stamping for distributable executables.
The six controlled rows explicitly disabled stamping in **both** arms so
concurrent repository commits could not invalidate the comparison.

Commands from the repository root, wrapped by `serialize-vm-tests` in shared mode:

```sh
go build -o /tmp/go4t-run ./cmd/go4t
export GOMAXPROCS=7 GOWORK=off GOFLAGS='-p=8 -buildvcs=false -x' GO4T_TRACE=1
GO4T_CACHE="$PWD/.measure/controlled-before-check" /tmp/go4t-run check examples/node-drain-typed
GO4T_CACHE="$PWD/.measure/controlled-before-build" /tmp/go4t-run build -o "$PWD/.measure/node-drain" examples/node-drain-typed
```

Go's cache was warmed first with a build of the same generated sources and flags;
it was never cleared. The commands were repeated for (b) and (c), then with the
changed CLI and fresh `controlled-after-check` / `controlled-after-build` caches.
Python's monotonic clock measured wall time; temporary timers bracketed
parse/binding load, type-check/laws, `GenerateInDir`, file emission and `go build`.
`-x` identified compiler/linker actions. Those timers and measurement files were
removed afterward. The temporary-binary probe was
`go build -buildvcs=false -x -o ../../.measure/ephemeral-binary ./_gen` from this
example's directory, with `GOMAXPROCS=7 GOFLAGS=-p=8 GOWORK=off`.

## Binding gaps found by running

- `kubernetes.Clientset` was a record until a live `Nodes().Get` panicked on
  nil internals: a struct with unexported fields is private state and is now
  a handle (SPEC §8).
- `Eviction` needs the pod namespace; the API server reported
  `an empty namespace may not be set during creation` and the `Pod`
  projection gained `namespace`.
- Closed: `StatusError` 404/429 are classified by the generated `api/errors`
  shim's `init()`, which registers its exported `IsXxx(error) bool` predicates.
  `IsNotFound` becomes `io.NotFound`, so a pod gone between observe and apply
  counts as evicted. For 429, `evict` uses `io.tryIOCause` and passes the retained
  `GoError` to `apierrors.isTooManyRequests`; it no longer classifies by the
  `TooManyRequests: ` message prefix. The explicit `apierrors` import exposes
  that pure predicate; automatic classifier discovery still supplies IoError
  kinds throughout each bound package's Go dependency closure.
