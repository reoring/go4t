# node-drain-shell: Go owns the client, go4t owns reconcile

The Go shell constructs a `*kubernetes.Clientset` and passes that same native
handle to exported go4t IO functions. `core/main.gt` performs observation,
cordon/uncordon patches and eviction; `main.go` owns arguments, client setup,
the context and the reconcile loop. No Go adapter interface or copied client
record sits between them.

`kubernetes.Clientset` has private fields (including `coreV1`) and an embedded
pointer, so it binds as an opaque handle, not a record. SPEC §8.7 exports that
handle as `*kubernetes.Clientset`, and `IO[T]` as
`func(context.Context) (T, error)`.

## Three forms of the same controller

| | [Go shell + go4t decide](../node-drain-controller) | This example | [All go4t](../node-drain-typed) |
| --- | --- | --- | --- |
| Clientset ownership | Go shell boundary (`Cluster` interface; example uses an in-memory fake) | Go creates and owns the real `*kubernetes.Clientset` | go4t creates the clientset through `go:` |
| Code in go4t | Decision only | Projection, observe, decide, apply and reconcile | Projection, observe, decide, apply, reconcile and entry |
| Laws | 10 decision laws | Same 10 decision + 5 projection laws | Same 10 decision + 5 projection laws |
| Generate step | `go generate ./...` | **Required:** `go generate ./...` | None separately; `go4t run` generates and builds |

**Export naming:** the copied `decide.gt` renames helpers `nodeState` → `projectNode`
and `podKind` → `projectPodKind` because they collided with `type NodeState` and
`type PodKind` under SPEC §8.7's first-rune upper-casing.

The 15 laws and their proofs are otherwise unchanged. They constrain pure
projection and decisions, not API availability or the success of effects.

## Generate and run

From the repository root, with Go, kind, Docker and kubectl installed:

```sh
go run ./cmd/go4t check examples/node-drain-shell/core
cd examples/node-drain-shell
go generate ./...
go build -o core_gen/node-drain-shell .

kind create cluster --name go4t-drain-shell \
  --kubeconfig /tmp/go4t-drain-shell.kubeconfig --config kind.yaml
kubectl --kubeconfig /tmp/go4t-drain-shell.kubeconfig \
  wait --for=condition=Ready nodes --all --timeout=180s
kubectl --kubeconfig /tmp/go4t-drain-shell.kubeconfig apply -f workload.yaml
kubectl --kubeconfig /tmp/go4t-drain-shell.kubeconfig \
  rollout status deployment/node-drain-shell-nginx --timeout=180s
kubectl --kubeconfig /tmp/go4t-drain-shell.kubeconfig \
  get pods -l app=node-drain-shell-nginx -o wide

./core_gen/node-drain-shell go4t-drain-shell-worker /tmp/go4t-drain-shell.kubeconfig
kubectl --kubeconfig /tmp/go4t-drain-shell.kubeconfig \
  get node go4t-drain-shell-worker -o jsonpath='{.spec.unschedulable}{"\n"}'
./core_gen/node-drain-shell go4t-drain-shell-worker /tmp/go4t-drain-shell.kubeconfig --release
./core_gen/node-drain-shell go4t-drain-shell-worker /tmp/go4t-drain-shell.kubeconfig --release
kubectl --kubeconfig /tmp/go4t-drain-shell.kubeconfig \
  get node go4t-drain-shell-worker -o jsonpath='{.spec.unschedulable}{"\n"}'
```

The fixture prefers the worker for two nginx replicas, but tolerates the
control-plane taint so replacements can move there after cordon. Check the
initial pod listing shows both on the worker. No PodDisruptionBudget is used
in this fixture.

The shell reconciles every two seconds, for at most 60 attempts. It prints
each action, retains `Drain.LastBlock` between calls, and prints `blocked:`
messages to stderr. Drain completes at `MarkDrained`; release continues through
`Uncordon` to `Noop`. IO errors, `MarkFailed` and exhausting the attempt limit
exit nonzero. A repeated release prints only `Noop`.

`go generate` runs `go run ../../cmd/go4t build --emit-go core_gen --package core core`
inside this example's Go module. All emitted sources, `go4t_export.go`, and
`go4tbind/` shims live under `core_gen/`, already excluded by the repository's
`*_gen/` rule. The shell imports `nodedrainshell/core_gen` as package `core`;
no generated Kubernetes bindings need to be committed. `deps.go`, `go.mod`
and `go.sum` retain the dependencies needed to regenerate them.

This mutates the named node and evicts its managed pods: use a disposable
cluster. The commands deliberately leave the cluster running for inspection;
when finished, remove only this example's cluster:

```sh
kind delete cluster --name go4t-drain-shell
```

## Observed run

On kind `go4t-drain-shell` (one control-plane, one worker; Kubernetes v1.35.0),
both initial nginx pods were ready on the worker and owned by a ReplicaSet.
The drain's exact stdout was:

```text
Cordon
Evict("default", "node-drain-shell-nginx-7bfbd9b678-d2bdb")
Evict("default", "node-drain-shell-nginx-7bfbd9b678-s2f27")
MarkDrained
```

The worker's `.spec.unschedulable` then read `true`. Both replacements moved
to the control-plane. The `kindnet`/`kube-proxy` DaemonSet pods and all four
static control-plane pods retained their original UIDs; the control-plane's
scheduling state and drain annotation were unchanged.

The first `--release` run printed:

```text
Uncordon
Noop
```

The following `--release` run printed:

```text
Noop
```

The worker's `.spec.unschedulable` was absent again (empty JSONPath output),
and the drain annotation was removed. All three runs exited zero with no
`blocked:` messages. The drain run took 6.18 seconds and release 2.04 seconds.

The final `go4t check` passed all 15 laws in 8.87 seconds; this CLI is silent
on successful checks rather than printing a law count. `go generate ./...`
succeeded in 43.97 seconds and `go build -o core_gen/node-drain-shell .` in
42.03 seconds, excluding serializer lock waits. The exported functions were:

```go
func Apply(p0 *go0.Clientset, p1 Drain, p2 Action) func(context.Context) (Drain, error)
func Observe(p0 *go0.Clientset, p1 Drain) func(context.Context) (Observed, error)
func Reconcile(p0 *go0.Clientset, p1 Drain) func(context.Context) (Outcome, error)
```

Here `go0` imports `k8s.io/client-go/kubernetes`; none of the three functions
was marked `// not exported`. Apart from the helper-name collisions noted
above, no export limitation or compiler change was needed.
