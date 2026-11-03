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
| `go4t check` | <1 s | 47 s every run (bindings regenerated in memory) |
| `go4t run` cold / warm | — | 150 s / 85 s (27 Go packages bound, 236 shim files, 66 MB) |

The projection laws are the new territory: a DaemonSet owner anywhere in
`ownerReferences` yields `DaemonSet` (induction over the owner list), the
mirror annotation wins over ownership (symbolic `Map` from ADR 0002), and
`creationTimestamp` (a `dyn.Value`) never influences the projection (ADR 0004).

The timings put the generation cache first on the roadmap: a 47 s `check`
is outside the inner loop.

## Binding gaps found by running

- `kubernetes.Clientset` was a record until a live `Nodes().Get` panicked on
  nil internals: a struct with unexported fields is private state and is now
  a handle (SPEC §8).
- `Eviction` needs the pod namespace; the API server reported
  `an empty namespace may not be set during creation` and the `Pod`
  projection gained `namespace`.
- `StatusError` 404/429 cross as `IoError.Failure` with a message; the port
  matches message text. A `rt.ClassifyError` hook for `apierrors.APIStatus`
  would restore `NotFound`.
