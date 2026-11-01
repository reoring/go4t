# perf — Wave B specification delta

## Grammar, CLI, and representation

No new language grammar, types, diagnostics, or CLI flags. After module integration,
`codegen.Generate(*types.Program, string)` returns `map[string][]byte`, one file per
module in a single Go package, for executable and embedding entry points.

- Checked `Int`, `Bool`, and `Float` locals, ordinary function parameters/results,
  pattern bindings, and tail-loop state use Go `int64`, `bool`, and `float64`.
  Transparent aliases have the same representation, including local function
  parameter annotations through parameterized aliases.
- Polymorphic values, data/tuple fields, dictionary methods, extern wrappers, and
  `rt.*` calls retain their existing `any` ABI. Scalar results are unboxed when
  crossing back into native code. Strings and aggregate values remain erased.
- Direct calls use native signatures. Taking a function as a value adapts it to
  the erased higher-order ABI. Monomorphic lambdas/local functions have native
  inner parameters/results and an erased callback adapter at that boundary.
  There is no second exported entry point or compatibility alias for ordinary
  native functions.
- Arithmetic and scalar comparison operators use native Go operations. Go's
  short-circuiting is preserved. Float ordering preserves the language's NaN-low
  order with native scalar tests; Float equality remains IEEE equality.
- Literal operations that Go would otherwise constant-fold execute through small
  typed functions. This preserves signed Int overflow, runtime division-by-zero
  panics, Float negative zero, and IEEE division by zero (including a literal zero
  divisor and a nonconstant numerator).
- The existing direct-self tail-call lowering remains a loop. Loop parameters and
  argument temporaries have the corresponding native types, arguments are
  evaluated before simultaneous reassignment, and fresh lexical bindings preserve
  closure snapshots. Source directives still immediately precede generated
  functions, closures, returns, and relevant control-flow locations.

Module integration preserves native signatures and assigns specialized copies to
the callee's module, with `<module>_<function>__special<N>` names. The benchmark
harness builds the entire emitted directory, not only `main.go`; its workloads
explicitly import the split standard modules. The sorting cutover below now lives
in `std/list`, calling qualified `std/array` operations. Its large/stability tests
live in `std/stdlib_extras_test.go`; the obsolete monolithic prelude is not restored.

The example demonstrates:

```go
func addInts_(a_ int64, b_ int64) int64 { return (a_ + b_) }
func sumPair__special1(a_ int64, b_ int64) int64 {
    return any((Combine_Int{}).Combine(a_, b_)).(int64)
}
```

The dictionary call in the second function is an intentional `any` boundary.

## Specialization contract

A direct checked function reference can select a generated copy when:

1. The callee has an in-program body and type-class constraints (not an extern).
2. Its declaration has at most **40 AST nodes**, counted by `ast.Walk`, including
   the declaration, parameter/type syntax, and body nodes visited by that walk.
3. Every supplied dictionary is concrete, recursively including context and
   superclass dictionaries.
4. The function has fewer than **8 distinct dictionary-tuple copies**, or this
   tuple already has a copy.

Copies are keyed by function and concrete dictionary tuple, discovered in a
stable traversal order, and named `<mangled-function>_special<N>`. They bind
concrete dictionaries statically and omit the hidden dictionary parameters.
Calls exposed inside copies are considered with the same size/copy limits.
Recursive copies retain tail-loop lowering. The original generic entry remains
for unknown dictionaries, indirect calls, oversized functions, and excess tuples.
Type parameters not determined by the dictionary tuple remain erased: e.g.
`keep[A Show, B](x A, y B) B` shares one `Show[Int]` copy across different `B` types.
It does not incorrectly freeze `B` from the first call site.

Tests cover the inclusive 40-node boundary, 41-node fallback, eight-copy cap,
ninth-tuple fallback, tuple deduplication, superclass/context behavior inherited
from existing tests, transitive concrete calls, recursive copies, first-class
functions, and residual unconstrained polymorphism.

## Sorting cutover

The original prelude list insertion sort was quadratic on shuffled input and
could not finish the required million-element benchmark within three minutes.
With Main's approval, this slice also replaces its implementation with:

```gofx
func sortBy[A](xs List[A], cmp func(A, A) Ordering) List[A] {
  arrayToList(arraySortBy(arrayFromList(xs), cmp))
}
```

`sort(xs)` remains `sortBy(xs, compare)`. Ordering, stability, empty/singleton
behavior, and persistent input semantics are unchanged. The backing Go stable
sort uses O(n log n) comparisons (and Go's stable sort swap bound), instead of
quadratic list insertion. List/array conversions use linear space. The unused
`insertBy` helper, its old API inventory entry, and its dedicated boundary test
are removed; no deprecated alias remains. Prelude regression coverage adds a
10,000-element reversed list and stability across tied keys. The older
`stdlib-extras.md` complexity description is updated accordingly.

## Benchmark workloads and budget enforcement

`bench/bench_test.go` builds `cmd/gofx`, compiles each `.gfx` workload, and builds
its generated Go outside the timed region. Each timed iteration starts the
compiled executable and waits for completion. Program-side data construction,
computation, output, and process startup are timed. Compiler/build time and
harness-side generation of the wc input are excluded. Every run validates its
stdout and exit status; execution and compilation have three-minute deadlines.

| Program | Workload | Checked output |
|---|---|---|
| `sort.gfx` | `sort` of 1,000,000 deterministic pseudo-random Ints, LCG seed 1 | `1000000` |
| `map.gfx` | Persistent Map insert of 100,000 distinct keys, then lookup of every key | `5000050000` |
| `fold.gfx` | Construct `[0, 10000000)` and fold its 10,000,000 elements | `49999995000000` |
| `wc.gfx` | The wc example on 1,000,000 repetitions of `redxx blue\n`: exactly 10,000,000 bytes | 1,000,000 lines; 2,000,000 words; both word counts 1,000,000 |

`budget.json` maps workload names to **nanoseconds per iteration**. The harness
fails with `<name> exceeded budget: <actual> ns/op > <budget> ns/op` when a measured
average exceeds its budget. `GOFX_BENCH_RECORD=1` disables only budget enforcement
for calibration; it never disables workload/output validation.
`GOFX_BENCH_ROOT` selects the checkout whose compiler and runtime are built,
allowing the copied harness to measure master without editing master.

The CI `bench` job runs:

```sh
go test ./bench -run '^TestBudgets$' -bench . -benchtime=1x -timeout=12m
```

The existing example loop now includes a sibling `gostd.gfx` when present: the
natural-extern example requires both files, as its README already specifies.

## Before/after measurements

Host: Linux/amd64, AMD Ryzen 9 7950X3D, Go **1.26.1**; benchmark suffix `-30`.
These are single-iteration **development-host measurements, not CI-runner
measurements**. Main approved host calibration. Budgets are exactly twice the
first successful after measurement, in integer nanoseconds.

| Workload | Master before (ns/op) | After (ns/op) | Budget (ns/op) |
|---|---:|---:|---:|
| sort 1,000,000 | **>180,000,000,000** (deadline; incomplete) | 1,592,269,718 | 3,184,539,436 |
| Map 100,000 | 139,528,301 | 127,266,911 | 254,533,822 |
| fold 10,000,000 | 790,179,553 | 659,464,204 | 1,318,928,408 |
| wc 10 MB | 748,133,630 | 714,679,810 | 1,429,359,620 |

The sort baseline is a censored lower bound, not a fabricated completed timing.
The algorithmic sorting fix accounts for most of its improvement; these numbers
do not isolate codegen changes from that fix. Single samples are noisy, and
cross-host performance is not established by this measurement.

Baseline was run **before changing codegen or the prelude**, using an independent
copy of the harness and workloads:

```sh
baseline=$(mktemp -d /tmp/gofx-perf-baseline.XXXXXX)
cp bench/bench_test.go bench/*.gfx "$baseline/"
GOFX_BENCH_ROOT=/workspace/wip/gofx GOFX_BENCH_RECORD=1 \
  go test "$baseline/bench_test.go" -run '^$' -bench . -benchtime=1x -timeout=12m
```

Observed baseline: sort reported `sort exceeded 180 s: context deadline exceeded`;
the remaining three sub-benchmarks completed with the table's timings. Overall
exit status was 1 because the sort timed out. Master was not edited.

After measurement:

```sh
GOFX_BENCH_RECORD=1 go test ./bench -run '^$' -bench . -benchtime=1x -timeout=12m
```

Observed: PASS, exit 0. A separate budget-enforced run passed at
1,593,388,123 / 120,568,266 / 702,302,394 / 754,755,254 ns/op respectively.

## Verification and observed example

Commands ran in `.wt/perf` through
`/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --`:

- `go test ./codegen ./bench` — passed (6.039 s / 0.002 s).
- Final `go test ./codegen ./bench ./prelude` — passed (6.379 s / cached / 1.282 s).
  This includes a `go vet` subprocess over the generated performance example,
  exact native-signature checks, alias-aware local functions, arithmetic/NaN/
  short-circuit boundaries, million-step TCO, source-map checks, and specialization
  limits. Obsolete tests pinning `rt.AddInt`/`rt.AddFloat` and concrete dictionary
  operator dispatch were removed, with their behavior covered by native-operator
  execution tests instead.
- `go test ./bench -run '^TestBudgets$' -bench . -benchtime=1x -timeout=12m` — passed,
  all four workloads below their budgets.
- `go run ./cmd/gofx check examples/wc/main.gfx` — ran once, exit 0, no diagnostics.
- Every `examples/*/main.gfx` — final complete fixture-aware loop passed, exit 0.
  It used `input.txt`/`args.txt` when present, included `gostd.gfx` for typed externs,
  and routed the io-effects report/log to a temporary directory to preserve the
  existing root fixtures. The records example retained its expected defaulting
  warning. An initial single-file loop exposed the typed-extern dependency error;
  that invocation and CI loop were corrected, not the example's language code.
- `go run ./cmd/gofx build --emit-go /tmp/gofx-perf-emitted.V4DU0k --package perf examples/perf/main.gfx`
  — exit 0, no stdout; the emitted native and specialized functions were read back.

Performance example stdout:

```text
native: 42
specialized: 42
tail loop: 500000500000
```

No repo-wide suite, selfhost build, commit, push, or live infrastructure access was
performed. Temporary benchmark/build/example directories are removed; the
harness uses `testing.TempDir` for compiler binaries and generated programs.

## Deviations and unresolved risks

- Approved scope extension: replace prelude insertion sorting to make the specified
  shuffled million-element list workload practical; remove obsolete `insertBy`.
- Approved calibration deviation: host-measured budgets, not an unobserved CI run.
  GitHub Actions execution and cross-machine budget headroom remain unverified.
- The master sort baseline did not complete within its bounded deadline; the table
  explicitly records the lower bound. There are no unfinished code paths or
  compatibility shims in this slice.
