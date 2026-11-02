# Node drain controller

A controller written go4t-first: the decision core (`decide/decide.gt`) was
written together with its laws, then wrapped by a Go shell (`controller.go`)
that observes an in-memory cluster and applies one action per reconcile.

```
go4t check decide/decide.gt        # 10 laws, exit 0
go generate ./... && go test ./... # Go shell against the generated package
```

## Decision

`decide(Observed) Action` with `Action = Noop | Cordon | Uncordon | Evict(name)
| Wait(msg) | MarkDrained | MarkFailed(NodeMissing | TimedOut)`. Order of
concerns: withdrawn request → node missing → cordon → deadline → PDB block →
first evictable pod → drained. DaemonSet and Mirror pods are never evicted.

## Laws

| Law | Statement | Proof |
| --- | --- | --- |
| `withdrawnIsNoop` | withdrawn request on a node we did not cordon is `Noop` | `refl` |
| `withdrawnUncordons` | withdrawn request on our cordoned node is `Uncordon` | `refl` |
| `cordonFirst` | schedulable node is cordoned before anything else | `refl` |
| `missingFails` | missing node fails with `NodeMissing` | `refl` |
| `skipsDaemonSet`, `skipsMirror` | `decide` with a DaemonSet/Mirror pod in front equals `decide` without it | case split (see comment in source) |
| `evictsFirstManaged` | first Managed pod is the one evicted | `refl` |
| `drainedWhenEmpty` | no pods left → `MarkDrained` | `refl` |
| `evictNextNotTimedOut` | eviction progress never reports a timeout | induction on the pod list |
| `timesOutExactly` | with `DeadlineAt(at)`, `MarkFailed(TimedOut)` iff `nowNanos >= at`, for any pods and PDB state | `if` on the clock + lemma |

Each law quantifies over the whole `Observed` record: `o{requested: true, node:
Cordoned}` means "any observation with these fields". Breaking `_firstEvictable`
so that DaemonSet pods are evicted fails `skipsDaemonSet` with
`LHS: Evict(name) RHS: MarkDrained`.

## Go shell

`Reconcile(Cluster, *Drain)` builds `decide.Observed`, calls `decide.Decide`,
and applies the action. `Cluster.Evict` returns `*EvictionBlocked` when a
PodDisruptionBudget refuses; the controller records the message and the next
reconcile decides `Wait(msg)`, then retries. Nullary-only sums (`NodeState`,
`PodKind`, `FailReason`) are mirrored as Go `int` enums; payload-bearing sums
(`Action`, `Deadline`) as interfaces with one struct per constructor.
