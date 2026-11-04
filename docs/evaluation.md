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

Documented limits were hit as predicted. `list.length` is an accumulator
fold, so `count(markDone(xs, id)) == count(xs)` is stated over a structural
`count` (SPEC). Regex is not law-eligible: a concrete parsing example,
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
full-size runtime comparisons produce matching go4t and Go outputs. The recorded
go4t wall-time ratios range from 1.75x to 31.78x; persistent collections, boxing,
and allocation dominate the measured overhead. `bench/PERF.md` records the
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
`bench/PERF.md` (runtime 1.75x–31.78x of Go on data-heavy kernels) and the
type-system expressiveness items in SPEC §9, not the ability to write large
programs.
