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

## Ordinary programs

On 2026-11-04 we asked whether, after ADR 0003–0005, go4t was usable for
ordinary programs, not only decision cores. Two programs were written from
docs and examples only:

| Program | Scope | First run | Proved laws |
| --- | --- | --- | --- |
| `examples/todo` (primary agent) | CLI `add\|list [--all]\|done\|rm\|serve`; JSON file persistence (`TODO_FILE`), derived `json.Encode/Decode`; `std/http` routes `GET/POST /todos`, `POST /todos/<id>/done`, `DELETE /todos/<id>`. `todo.gt` + `main.gt`: about 260 lines. | All CLI paths and HTTP routes behaved as intended; the only `check` failures were in laws. | 3 |
| `logstat` (fresh agent, using only `README.md`, `docs/`, `examples/`; scratch probe, not committed) | Regex line parser, `std/map` aggregation, sorted table and `--json` via derived codecs, `import "go:path/filepath"` for `filepath.base`. | Text and JSON output were correct; 30-line fixture, 25 valid, 5 skipped. | 3 |

The todo law `markDone(markDone(xs, id), id) == markDone(xs, id)` exposed a
prover gap: `differs at: t{2: true}{2: true} vs t{2: true}`. Nested updates
over a symbolic record were not merged. The same-day fix added the rule in
SPEC §6.5's normalization paragraph: nested single-constructor updates merge,
later write wins, fields ordered by index. The law then proved by list induction.

Documented limits were hit as predicted. At the time, `list.length`'s accumulator
fold meant `count(markDone(xs, id)) == count(xs)` was stated over a user-defined
structural `count` (SPEC). ADR 0014 subsequently lifted this limit with
owner-checked structural models for selected `std/list` functions; the todo
example now proves the same preservation law over `list.length`, without the
structural copy. Runtime accumulator implementations are unchanged.
Regex is not law-eligible: a concrete parsing example,
`parseLine("…") == Some(…)`, was rejected with
`law depends on extern go4t/rt.RegexCaptures`. Concrete examples belong in
`go4t test` functions. A deliberately false law,
`lookup(open(add(s, text, now)), s.next) == Some(…)`, was rejected with stuck
normal forms, as it should be: it is false when `s.next` already occurs in
`s.items`.

Two library gaps were observed, not fixed: `std` has no mutable reference
(a stateful HTTP server keeps state in a file or a `go:` handle), and
`std/io` has no named-file line streaming (`readFile` reads whole files).
`go4t build DIR` emits Go into `DIR/_gen`; a binary needs
`go build ./DIR/_gen`. The CI example loop ran `main.gt` alone, which does
not load sibling files; it now runs multi-file example directories
(`.github/workflows/ci.yml`).

The limit found today was in the prover's normal forms, not in the language
or bindings; two ordinary programs ran first try, and the remaining gaps are
library surface (`Ref`, streaming) and proof coverage of externs.

Same-day follow-ups added IO-only `Ref` cells, named-file streaming through
`foldLines`/`forEachLine`, executable output from `go4t build` (with `--emit-go`
for sources only), and directional channels with native Go bindings. Five
full-size runtime comparisons produce matching go4t and Go outputs. The ratios
recorded that morning ranged from 1.75x to 31.78x; the same-day follow-up below
brought unboxed kernels to 1.6–4x and `mutmap` to 5x, leaving persistent-Map
updates at 19–24x. `bench/PERF.md` records the
workloads, shared-VM caveats, commands, and CPU profile.

## Scale: the self-hosted compiler

On 2026-11-04 we asked whether anything larger than about 300 lines had
been shown to work in the current language. `selfhost/`, a go4t→Go compiler
written in go4t, last reached its fixed point at baseline `6b189c0`. At HEAD
it failed to parse (`parser.gt:757: expected expression, found "alias"`),
and its prelude path no longer existed.

The same-day catch-up took one deep-worker agent about 48 minutes.
Selfhost grew from 4,426 to 5,447 lines, adding `modules.gt`,
`derive_higher.gt`, and `derive_json.gt`. It now loads `std/` modules and
local `go4t.mod` roots and accepts deriving
(`Debug`/`Show`/`Functor`/`Foldable`/`Traversable`/`JSON`), multiple
constraints, pipelines and sections, `Float`, aliases and common fields,
list patterns, local recursion groups, and implicit IO tail lift (ADR 0005).
It parses `law` and `proof`; `import "go:…"` is rejected with
`selfhost does not bind Go packages`.

`bash selfhost/bootstrap.sh` took 29.5 s on the primary's run: 34 examples
produced byte-identical stdout to `cmd/go4t`; two nondeterministic examples
(`concurrency`, `sales-report`) were compared on exit status and line count.
There were nine explicit skips for `go:` imports, network listeners,
cluster, and Go-module dependencies. The result was
`FIXED POINT: stage2 == stage3`.

A 5.4k-line program with a lexer, parser, type checker with classes and
deriving, and code generator compiles and reproduces itself under the
current language; the remaining scale limits are the ones measured in
`bench/PERF.md` (1.6–4x of Go where values stay unboxed, 19–24x for persistent-Map updates) and the
type-system expressiveness items in SPEC §9, not the ability to write large
programs.

## Removing the measured limits

On 2026-11-04, the same day as the measurements above, follow-up work addressed
the observed limits rather than changing the evaluation workloads.

**Native lowering.** Native Ord comparison, native sorting and scalar folds,
and native numeric-record fields with a private fold accumulator reduced
sortints to 0.24 s (4.00x Go) and nbody to 0.30 s (1.58x). Treefold remained
1.75x. Input records remain immutable; replacement expressions are evaluated
before private accumulator writes.

**Maps.** Shared immutable map entries reduced copied payload structure, but
wordfreq still took 2.45 s (~27x) and mapheavy about 2.3 s (~19x).
IO-only `std/mutmap` brought mapheavy-mut to 1.13 s (9.42x), with `freeze` for
persistent snapshots. An order-8 persistent B-tree was rejected: mapheavy
improved 18.7%, but wordfreq regressed 5.5% and allocated more bytes.
Replacement metadata reuse was also tried and rejected. Remaining map costs
were approximately 50% GC, 19% node allocation, and 4% comparisons; mutmap still
pays for boxing, IO closures, and Option allocation. See the dated follow-up in
[`bench/PERF.md`](../bench/PERF.md) and its retained
[`map`](../bench/perf/results/2026-11-04-map.json),
[`map2`](../bench/perf/results/2026-11-04-map2.json), and
[`specialize`](../bench/perf/results/2026-11-04-specialize.json) evidence.

**Type friction.** The measurement recorded 0 entries for expression ascription,
bidirectional typing, or local polymorphism. It found 69 `if`-else sites;
qualified `prelude.when` and `prelude.unless` now express conditional Monad
actions without a type-system change. Arguments remain strict, but skipped IO
effects do not run.

**Generic Go functions.** Concrete call-site instantiation expanded binding
reach: skipped declarations fell from 40 to 0 in slices, 10 to 0 in maps,
3 to 1 in sync, 25 to 16 in sets, and 90 to 76 in workqueue. Go constraints
remain checked at the use site, and generic calls and callbacks use IO.
Generic receiver methods are still skipped; this is not support for arbitrary
generic Go types. [ADR 0006](adr/0006-generic-go-functions.md) records the boundary.

**Sampled laws and induction.** `sample(N)` keeps equations outside the trusted
proof fragment executable through `go4t test`, including imported sampled laws.
They are reported as `tested, not proved` and cannot supply rewrite lemmas.
The `induction` tactic performs exhaustive constructor induction and applies
structurally decreasing induction hypotheses automatically, with either
explicit arm tactics or bare `induction x` using `refl` in each arm.
[ADR 0007](adr/0007-sampled-laws.md) records the distinction between testing
and proof.

Runtime for data-heavy code moved from 11–32x to 1.6–4x where values stay
unboxed, while persistent maps remain 19–24x. Int/String boxing accounts for
under 4% of CPU samples in every kernel; the persistent Map's dominant cost is
search-path copying (`rt.collEntryBranch`, 44.3% of mapheavy samples), the
price of persistence, in the same range as other persistent collections.
Before [ADR 0008](adr/0008-direct-style-io.md), `mutmap`'s 9.4x was tree
allocation about 14%, IO closure allocation (`rt.Bind` plus continuations)
about 17%, `Some` 6%, and GC 35% of CPU samples. The ADR therefore rejected
element unboxing for List/Map/Array and chose direct-style IO lowering: `do`
blocks over the builtin IO instance become one Go function of sequential
statements, with top-level self tail calls as cancellation-checked loops.
Result: mapheavy-mut 1.04 s → 0.55 s (5.00x Go), peak RSS 385 MB → 113 MB,
and `rt.Bind` absent from the profile; the 4x target was not reached, and
what remains (AVL traversal, GC, comparison, node and `Some` allocation, Int
boxing) is entirely on the data-structure side. The lowering preserves
evaluation order and `tryIO` capture ranges, falls back to ordinary calls for
non-tail recursion under `fork`/`foldLines`/`tryIO`, and was accepted on
byte-identical stdout for the 41 deterministic CI examples, the law harness
and the self-hosting fixed point. The next step on this path is deferred
until an IO-heavy workload demands it.
The 19–24x applies only to code that updates a Map while folding. A decision
core's `Observed` maps are built once by `observe` and only read by `decide`,
so controllers pay none of it; update-heavy aggregation belongs to `std/mutmap`
in IO.

## Third-party friction

On 2026-11-04, four fresh agents wrote docs-only programs: logstat (log
aggregation), ghstars (JSON repository reports and a local HTTP fetch), csvpivot
(whole-file and streaming CSV pivots), and turnstile (order-state replay).
They learned from documentation and examples, without inspecting compiler or
runtime implementation. All four programs ran on their first runtime attempts;
that does not mean every initial proof check or test invocation succeeded.
The source observations are retained in
`.tmp/probe-{logstat,ghstars,csvpivot,turnstile}/FRICTION.md` in the evaluation
checkout; these ignored artifacts are not part of a packaged release.

The counts below include every line containing `Verdict:` in those logs,
counted once by its first verdict category. They count observations, not
programs, failed commands, or independent defects.

| Program | Worked first try | Language gap | Library gap | Doc gap | My error | Other | Total |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| logstat | 3 | 1 | 1 | 1 | 0 | 1 | 7 |
| ghstars | 0 | 3 | 2 | 0 | 3 | 0 | 8 |
| csvpivot | 0 | 2 | 2 | 0 | 0 | 2 | 6 |
| turnstile | 4 | 0 | 1 | 0 | 3 | 0 | 8 |

“Other” includes logstat's concluding usability verdict and csvpivot's
“no obstacle yet” and “documented boundary” observations. Turnstile also labels
two “my error” observations as secondary doc gaps and qualifies one successful
false-law diagnostic as a language gap only if generated bindings were expected.
Those secondary labels are not double-counted. Ghstars and csvpivot describe
first-try runtime successes outside their `Verdict:` lines, hence their zero
counts in that column.

Top gaps and disposition:

- **JSON Int exactness:** turnstile exported `9007199254740993` as
  `9007199254740992`. Fixed the same day by preserving Int values exactly in
  JSON rather than passing their encoding through Float.
- **HTTP loopback and ephemeral ports:** ghstars could only bind all interfaces
  on a preselected port. Fixed the same day with host-selecting server APIs and
  bound-port notification for port zero.
- **Documentation corrections:** this change replaces unsupported list-append
  notation with `list.append`, locates Int-bound helpers in `std/float`, explains
  the minimum-Int literal restriction, documents parentheses on conditional
  equation operands, distinguishes failed-proof diagnostics from sampled
  counterexample bindings, cross-references generated Go-name inspection, and
  makes whole-file/streaming line-ending parity explicit.
- **Documented limits remain:** Float laws are outside the proof fragment
  (Float addition is also genuinely order-sensitive); sorting is outside
  universal proofs; there is no standard CSV codec or URL parser. The probes
  used Int-cents proofs, explicit sampling, and application-level CSV/URL parsing
  rather than treating those limits as proved behavior.

## Operational soak: node-drain-typed (2026-11-04)

Question: nothing had run longer than one reconcile. `examples/node-drain-typed/soak`
drives a dedicated kind cluster (one worker, a 2-replica Deployment, a
PodDisruptionBudget `minAvailable: 1`) through cordon → evict until `MarkDrained`
→ `--release` → wait for replacement pods, for three hours.

| measure | value |
| --- | --- |
| cycles | 839, all drained, all recovered |
| reconciles per cycle | mean 4.25, max 16 |
| wall per cycle | mean 0.41 s, max 20.8 s (PDB back-off) |
| max RSS of one reconcile | 35.8–38.4 MB, flat over three hours |
| evictions blocked by the PDB | 49, all classified by the `TooManyRequests:` prefix and retried |
| unclassified stderr | 8 × `pods "…" not found` (exit 1, next reconcile succeeded) |

The eight failures are the only finding: a pod observed by `observe` was gone by
the time `apply` evicted it (ReplicaSet rollover between the two). The API
error was already classified as `io.NotFound`; the example raised it instead of
treating a vanished pod as evicted. `evict` now maps `NotFound` to "nothing to
do", the same way `observe` treats a missing node.

Decisions from the observation: no `Throttled` constructor is added to `IoError`
(the prefix classification carried 49 back-offs without ambiguity); `Conflict`
was never observed because cordon is a merge patch and eviction is a create,
neither of which carries a resourceVersion, so there is nothing to add until a
controller uses `Update`. Memory and reconcile time did not drift.

The prefix-based 429 classification above describes the recorded soak; `evict` now uses the retained `io.cause` with `apierrors.isTooManyRequests` instead (ADR 0011 §3C).

## Migrating a real controller one layer at a time (2026-11-04)

Question: can an existing Go program adopt go4t gradually, or only be rewritten
whole? The answer came from `examples/approval-controller`, an AppThrust
platform controller extracted with its six Go tests kept as the oracle. In
three steps the Go side shrank from 271 to 88 lines (the reconcile loop,
manager wiring, and the controller-runtime client) while `observe`, `decide`
and `apply` moved into go4t, and the oracle never changed. That required the
export side to pass native handles (`*kubernetes.Clientset`,
`client.Client`, `*unstructured.Unstructured`) by identity (SPEC §8.7), and
`examples/node-drain-shell` shows the same shape on a kind cluster.

Each step surfaced a binder rule that was silently lossy rather than
missing, which is the worse failure: a function that binds, runs, and
returns the wrong thing. The ones found and their fixes:

| Found in | Lossy rule | Fix |
| --- | --- | --- |
| observe | `unstructured.Unstructured` could not be passed as `client.Object`: upcasts only looked inside one package | ADR 0009: symmetric direct-import upcasts |
| vet of generated shims | package variables, fields and receivers copied lock-bearing structs | ADR 0009: address-of shims, by-value lock crossings skipped |
| list options | `client.InNamespace` (a named string) had no upcast | ADR 0010: scalar/collection implementers upcast |
| apply | `meta.SetStatusCondition(*[]Condition, …)` bound but mutated a copy | ADR 0011: argument `*T` binds as `Ref[T]` |
| apply | `metav1.Time` crossed as its JSON wire form; `UnmarshalJSON` moved the location | ADR 0011: JSON is a view (`timeToJson`/`timeFromJson`), the value is a handle |
| soak | `apierrors.IsTooManyRequests(error)` received a fresh error built from a message | ADR 0011: errors cross as the `GoError` handle; `Failure` keeps its cause |

The rule that came out of it (ADR 0011): the boundary never converts lossily.
It copies all-exported structs, keeps everything else by identity, and the
only lossy operations are named functions (`…FromJson`, `…Parse`) or table
rows marked lossy (`time.Time` → `Int`). Every rule above was pinned by a
fixture that runs the generated shim, and node-drain-typed's 15 laws and
2.4 s warm check were unchanged through all of it.
