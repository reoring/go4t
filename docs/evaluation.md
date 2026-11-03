# Evaluation: where go4t has been used, and what we learned

[日本語](evaluation.ja.md)

## Go versus laws on the same decision core

The last row of the comparison table in the README needs a caveat. We wrote the node-drain `decide` in plain Go side
by side (the Go version and its `rapid` tests are recorded in ADR 0002, not
shipped in this repository): the decision body is the same length, the
type advantages are real but small, and property-based testing with
`pgregory.net/rapid` caught all three mutations we tried (evicting a DaemonSet,
moving the deadline check behind the PDB check, `>=` → `>` on the `now == at`
boundary) in its default 100 runs, because its generator deliberately favours
zero, empty and equal values. What remains unique to laws is that they are
proofs rather than probabilities: no seed, no run count, no generator bias
to reason about. Whether that is worth a second language is a judgement about
how deep the branches are, how often their order changes, and how much you
trust a probabilistic green in CI. For a `decide` with two `if`s, stay in Go.

## Ports and law rounds

The language was validated by porting real code, not by writing samples.

**Laws were validated on five decision cores in a row** before we called them
usable: the attachment-repair controller below (4 laws), a node-drain
controller written from scratch (`examples/node-drain-controller`, 10 laws:
withdrawal → missing node → cordon → deadline → PDB → first evictable pod),
a zone-placement core over `Map` capacity and usage (6 laws, including
induction over a fallback list), an admission core chaining four gates through
`Result` (7 laws), and a structural rewrite of the drain core proved
equivalent to the original (`decide(o) == decideV2(o)`). The first three rounds
each exposed a completeness gap in the normalizer (a correct law that would
not close); each was fixed and pinned as a fixture. Rounds four and five needed
no evaluator change. No round found a soundness defect: the normalizer is
checked against the real runtime by an open-term property test (every law
side and partial proof goal, ~75 000 random substitutions compared with
`go4t run` output). The admission round also produced the first case of the
checker catching a *false law*: the author's `fullPassAdmits` forgot that a
negative quota is rejected, and the diagnostic named the condition
(`stuck on: if (q < 0) …`) rather than a sample. The record, including the
Go/`rapid` comparison that cut our own claims down, is in
[`docs/adr/0001-equational-laws.md`](adr/0001-equational-laws.md) and
[`docs/adr/0002-laws-map-and-diagnostics.md`](adr/0002-laws-map-and-diagnostics.md).

**Two production Kubernetes controllers** (`examples/approval-controller`,
`examples/attachment-repair-controller`) had their decision logic moved to
go4t while the original Go test files stayed byte-identical as the oracle.
All 6 + 22 original tests pass unchanged. The approval port surfaced a decision
branch the original tests never exercised; the exhaustiveness checker refused
to compile until it was handled. The attachment port moved a multi-step
workflow (observe → authorize → compare-and-swap delete → publish) into a
pure `Decide(Observed) Action` that Go calls up to three times per reconcile.

Each readability fix in those ports was the same correction — a Go
`(value, ok)`, flag or sentinel became a constructor:

| Go habit in the input | Became | Effect |
| --- | --- | --- |
| `gatesValid: Bool` + `gates: List[Gate]` | `gates: List[GateEvidence]`, `GateEvidence = Gate \| InvalidGate` | first-failure order identical to the Go loop |
| `Option[Int]` with "≤ 0 means invalid" | `Ttl = NoTtl \| InvalidTtl \| TtlNanos(Int)` | the invalid case cannot be forgotten |
| `present: Bool` + `Option[Int]` | `Timestamp = NoTimestamp \| InvalidTimestamp \| AtNanos(Int)` | one match instead of nested if/match, 20 → 11 lines |
| `fromOption(gen, -1)` | `gen != Some(plan.generation)` | no sentinel |

**A 110 k-line port of `client-go`** was done as a scale experiment, not as a
product. It compiled (after three compiler fixes — a bounded test harness, and
direct lowering of result-position `match` and `if`/blocks that otherwise became
closure-heavy codegen — that are now part of the language), but the generated
binding layer (44 k lines of go4t plus 44 k of Go, committed) grew larger than
the code it served. We concluded at the time that go4t should stay in the
decision core. The port lives on the `experiment/client-go-gofx` branch
(`ports/client-go`) and as the standalone repository
[reoring/client-go-go4t](https://github.com/reoring/client-go-go4t), whose
`RESULTS.md` and `CODE_STATS.md` record what was verified and the before/after
code statistics.

## The whole controller in go4t

That conclusion was revised once the binding layer stopped being something
you commit. With `import "go:…"` (ADR 0003) bindings are generated at build
time; measured on Go's standard library the shim mode already mirrored
net/http 145/153 functions, and the remaining skips were three mechanical
shapes (multiple results, `iter.Seq`, returned funcs), all closed. Two further
rules came from reaching Kubernetes: a struct that defines its own JSON wire
form (`metav1.Time`, `resource.Quantity`) crosses as `dyn.Value`, so
`corev1.Pod` is a plain record; and a struct with private state
(`kubernetes.Clientset`) is a handle, found when a copied clientset panicked on
nil internals against a live API server.

[`examples/node-drain-typed`](../examples/node-drain-typed) is the node-drain
controller with observe and apply in go4t against the typed clientset, run
against a kind cluster: `Cordon`, seven `Evict(ns, name)` in order (DaemonSet
and static pods untouched), `MarkDrained`; `--release` uncordons. The ten
decision laws are unchanged. The projection from `corev1.Pod` to the decision's
`Pod` is pure and has five laws of its own — a DaemonSet owner anywhere in
`ownerReferences` (induction over the list), the mirror annotation winning over
ownership (symbolic `Map`), timestamp independence (ADR 0004 lets laws quantify
over records that contain Float inside `dyn.Value`). These are the first laws
over Kubernetes objects, and they are the point a `rapid` generator cannot
reach: writing a generator for `corev1.Pod` means deciding the shape of
`ownerReferences` by hand, whereas the law quantifies over all of them.

What remains is cost, not reach: `go4t check` regenerated 240 packages of
bindings on every run (47 s) until the per-package binding cache brought a
hit to 1.8 s; `go4t run` cold is dominated by compiling the bound packages
(150 s cold, 85 s warm).
