# node-drain-typed: observe → decide → apply, all in go4t

The same node-drain controller as [node-drain-controller](../node-drain-controller),
without the Go shell. Kubernetes is reached through `go:` imports of the typed
clientset; nothing is committed under `go4tbind/`.

```
decide.gt   types, projection corev1.Pod → Pod, decision core, 15 laws
main.gt     observe (List/Get), apply (merge-patch cordon, EvictV1), entry
deps.go     keeps the Go modules the imports need in go.sum
```

Run against any cluster (needs `go.mod`'s `replace go4t => ../..`):

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
| `go4t check` | <1 s | 44 s on an empty binding cache, 1.8 s on hits (240 bound packages) |
| `go4t run` cold / warm | — | 150 s / 85 s before the cache; Go compilation of the bound packages dominates |

The projection laws are the new territory: a DaemonSet owner anywhere in
`ownerReferences` yields `DaemonSet` (induction over the owner list), the
mirror annotation wins over ownership (symbolic `Map` from ADR 0002), and
`creationTimestamp` (a `meta.Time` handle) never influences the projection
(ADR 0004/0011). The timestamp law still quantifies over every handle and uses
`refl`: it neither inspects nor compares the handle, so it needs no Eq instance
and no sampled fallback. All 15 laws remain proved.

Before the per-package binding cache every `check` regenerated all bindings
(47 s); with it, editing your own `.gt` files is a 1.8 s `check`. Cache
location and switches are in [docs/cli.md](../../docs/cli.md).

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
