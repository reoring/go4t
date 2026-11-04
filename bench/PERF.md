# Runtime performance: go4t versus Go

## Result

All five full-size workloads finish and produce byte-identical output in all
three runs of each implementation. No kernel overflows the stack or reaches the
175-second execution limit. Runtime is nevertheless a material cost: these
idiomatic go4t programs take 1.75–31.78 times the Go wall time. The largest costs
are in collection-heavy programs, not the depth-20 recursive tree.

This is a runtime comparison, separate from `bench/bench_test.go` and
`bench/compile.go`. It adds no CI budget and changes no compiler or runtime code.
The programs, input generator, and harness live in [`perf/`](perf/).

### Measurements, 2026-11-04

Each wall value is the minimum of three fresh process executions. Each RSS value
is independently the minimum of the three **peak** resident-set measurements;
wall and RSS minima need not come from the same execution. Ratio is minimum
go4t wall time divided by minimum Go wall time. Binary sizes are unstripped file
sizes in bytes, excluding the separate profiling build.

| Kernel | go4t wall s | Go wall s | Ratio | go4t RSS KB | Go RSS KB | Checksum equal | go4t binary bytes | Go binary bytes |
| --- | ---: | ---: | ---: | ---: | ---: | :---: | ---: | ---: |
| wordfreq | 2.86 | 0.09 | 31.78x | 305048 | 53760 | yes | 5415941 | 2469404 |
| sortints | 1.75 | 0.06 | 29.17x | 116400 | 9840 | yes | 5401660 | 2556747 |
| nbody | 2.28 | 0.21 | 10.86x | 10176 | 2160 | yes | 5392311 | 2556115 |
| mapheavy | 2.95 | 0.13 | 22.69x | 222480 | 39600 | yes | 5394877 | 2542930 |
| treefold | 0.07 | 0.04 | 1.75x | 40560 | 19440 | yes | 5376011 | 2542920 |

The complete samples, output checksums, input/source hashes, compiler hash,
commands, and profile text are retained in
[`perf/results/2026-11-04.json`](perf/results/2026-11-04.json).

### Environment and uncertainty

- Linux 6.8.0-64-generic, amd64, KVM guest exposing 30 CPUs; host CPU model
  reported as AMD Ryzen 9 7950X3D.
- `go version go1.26.1 linux/amd64`; `GOMAXPROCS=1`, `GOGC=100` for both
  implementations. No CPU pinning, compiler optimization flags, or SIMD code.
- Checkout base: `c2a89b1`, with concurrent working-tree changes. The compiler
  is an explicitly built snapshot, SHA-256
  `36e2f7c9cb941c2bf844928d385768360906f18be6f641971e9e0ed7102bdc17`.
  The table is not a measurement of the unmodified base commit.
- All builds, runs, and profiling commands pass through
  `serialize-vm-tests --class short --`. This is a shared admission class, **not
  exclusive access to the VM**. Other agents were active; one-minute load was
  5.46 at the start and 6.56 at the end. Minima reduce, but do not eliminate,
  scheduler/cache/GC variation.
- GNU time reports hundredths of a second. The 0.04–0.13-second Go kernels make
  ratios coarse; the displayed decimal places are arithmetic, not statistical
  confidence. Wordfreq's three go4t peak RSS values were 524404, 305048, and
  309456 KB. Reporting only its minimum must not conceal that variation.
- Input generation and compilation are outside the timer. Process startup,
  input reading, tokenization/input parsing, allocation, computation, and output
  are inside it. Input files are not cold-cache controlled. Runs alternate go4t
  then Go three times for each kernel; there is no separate warm-up execution.

## 2026-11-04 follow-up

The same-day follow-up changed lowering and collection internals, not the
workloads' output contracts. These are retained follow-up measurements, not a
new run of the original table; the shared-VM and timer-resolution caveats above
still apply.

| Kernel | After go4t wall s | After ratio to Go |
| --- | ---: | ---: |
| sortints | 0.24 | 4.00x |
| nbody | 0.30 | 1.58x |
| wordfreq | 2.45 | ~27x |
| mapheavy | ~2.3 | ~19x |
| mapheavy-mut | 1.13 | 9.42x |
| mapheavy-mut, after direct-style IO ([ADR 0008](../docs/adr/0008-direct-style-io.md)) | 0.55 | 5.00x |
| treefold | 0.07 | 1.75x |

Native prelude Ord comparison avoids dictionary calls for Int and String keys.
Native sorting and scalar folds keep known values unboxed. Numeric records store
native scalar fields, and numeric-record folds can reuse a private accumulator
without mutating input records. Persistent maps share immutable key/value entries
across copied search-path nodes. `std/mutmap` adds an IO-only ordered mutable
alternative, with persistent snapshots through `freeze`.

### Negative results

- **Order-8 persistent B-tree:** reduced mapheavy time by 18.7% but increased
  wordfreq time by 5.5% and allocated more bytes; rejected.
- **Replacement metadata reuse:** copying cached metadata instead of rebalancing
  replacements gave no measurable wall-time benefit (mapheavy 2.18 to 2.16 s;
  wordfreq 2.45 to 2.46 s); rejected. Neither experiment is the landed collection
  design.
- **Element unboxing for List/Map/Array:** not attempted because Int/String
  boxing is under 4% of CPU samples in every kernel: mapheavy `convT64` 1.75%;
  wordfreq `convTstring` 1.2% plus `convT64` 0.8%; mapheavy-mut `convT64` 2.5%.
  [ADR 0008](../docs/adr/0008-direct-style-io.md) records the decision to pursue
  direct-style IO lowering instead.
- **Profile interpretation:** these per-site percentages are CPU-sample shares
  recomputed from the raw profiles in
  [`2026-11-04-map2.json`](perf/results/2026-11-04-map2.json), not byte shares.

The remaining map profile attributes approximately 50% to GC, 19% to node
allocation, and 4% to comparisons. These are approximate profile categories,
not additive cumulative call-tree percentages. Comparison dispatch is no longer
the dominant measured cost.

### Direct-style IO lowering

Before [ADR 0008](../docs/adr/0008-direct-style-io.md), mapheavy-mut spent
about 17% of CPU samples allocating IO closures (`rt.Bind` plus continuation
lambdas). Lowering `do` blocks over the builtin IO instance to one Go function
of sequential statements, with top-level self tail calls as
cancellation-checked loops, removed that cost: mapheavy-mut went from 1.04 s to
0.55 s (5.00x Go) and peak RSS from 385 MB to 113 MB; `rt.Bind` no longer
appears in the profile. The 4x target was not reached. The remaining 520 ms is
AVL traversal 29%, GC 21%, comparison 13%, node allocation 12%, Int boxing
12% and `Some` allocation 6%; all of it is on the data-structure side. The
persistent kernels did not move (mapheavy 2.4 s, wordfreq 2.5 s; the change
does not touch pure code). Acceptance was byte-identical stdout on the 41
deterministic CI examples, the law property harness, and the self-hosting
fixed point. Evidence:
[`2026-11-04-directio.json`](perf/results/2026-11-04-directio.json).

Evidence and experiment details:
[`2026-11-04-map.json`](perf/results/2026-11-04-map.json),
[`2026-11-04-map2.json`](perf/results/2026-11-04-map2.json), and
[`2026-11-04-specialize.json`](perf/results/2026-11-04-specialize.json).

## Workloads and output contracts

Both implementations of each kernel read the same generated stdin file. The
small files select sizes/seeds; the large wordfreq file contains the actual
text. The generator is `perf/gen/main.go`.

| Kernel | Exact workload | go4t representation | Go representation |
| --- | --- | --- | --- |
| wordfreq | 20,000,000 bytes: 2,000,000 nine-character words plus separators; vocabulary `word00000`–`word04095`; Park–Miller LCG seed 17, multiplier 48271, modulus 2147483647; newline every 16 tokens | `string.words`, tail-recursive `list.fold`, persistent `Map[String, Int]`, sorted frequency list | `strings.Fields`, mutable `map[string]int`, sorted slice |
| sortints | Input `1000000 1`; generate 1,000,000 values in-program with the same LCG, seed 1; sort ascending; sum indices 0, 1000, …, 999000 | Tail-recursive `List[Int]` generation, `arrayFromList`, `arraySortBy`, tail-recursive checksum | `[]int64`, `slices.Sort`, loop |
| nbody | Input `1000000`; 1,000,000 steps of a five-body, three-dimensional softened-gravity simulation; G=1, dt=0.001, squared-distance softening=0.01; identical initial states in both sources | `Array[Body]`, record updates, array fold/map, tail-recursive timestep loop | Fixed `[5]body` arrays and loops |
| mapheavy | Input `1000000`; insert keys 0–999999 with value `3*key+1`, then look up all 1,000,000 keys and count only hits with the correct value | Persistent `Map[Int, Int]`, tail-recursive insert/lookup loops | Pre-sized mutable `map[int64]int64`, loops |
| treefold | Input `20`; build both children independently down to depth zero, then count all nodes including leaves | `Tree = Leaf \| Node(Tree, Tree)`, structural recursion | Pointer nodes, nil leaves, structural recursion |

These compare ordinary implementations, not identical underlying algorithms.
In particular, the go4t map is an ordered persistent AVL tree, whereas Go's map
is a mutable hash table. `arraySortBy` uses stable sorting of boxed values,
whereas `slices.Sort` sorts native integers in place. The measured ratios include
those library/representation choices; they are not isolated compiler overhead.

The nbody loop computes all five contributions per body from the old positions
(the self-contribution has zero displacement), updates velocity, then position.
It is a softened-gravity workload, not the benchmark-game solar-system dataset.
Both energy calculations use the same operation order and include kinetic and
softened potential energy. go4t prints with `float.showFloatFixed(..., 9)`; Go
uses `fmt.Printf("%.9f\n", ...)`. Equality is at that printed precision, not a
claim of bitwise equality of the complete simulated states.

Tree depth zero is a leaf. Depth 20 contains 1,048,575 allocated branch nodes
and 1,048,576 leaf occurrences, yielding 2,097,151 counted nodes. The Go nil leaf
and go4t nullary constructor do not require a separate heap object per leaf.
The benchmark does not substitute a closed-form count or share child subtrees.

Exact single-line outputs, including a trailing newline:

| Kernel | Output |
| --- | --- |
| sortints | `1072161167218` |
| nbody | `-0.001046538` |
| mapheavy | `1000000` |
| treefold | `2097151` |

Wordfreq's checksum is its complete ordered top-ten output. Counts sort
descending; ties sort by word ascending:

```text
word01597 571
word02008 566
word00951 565
word03004 565
word02452 558
word02900 558
word03013 558
word02967 557
word01772 556
word02809 556
```

The wordfreq input SHA-256 is
`6e2f2e2d9202c2a2e5acd59b59794234b69de379c71e14b2a7559c433b2f870a`;
the output SHA-256, including newlines, is
`5e3e7d2acea4cb964e6d01a049b717ccb9f09f612237955a34a3aac509fa4819`.
The harness requires all six outputs for a kernel to be nonempty, identical,
and from successful processes. It also compares the profiling output.

## Where time goes

Wordfreq has the worst ratio in the recorded run. A separate build adds
`defer perfProfile()()` at generated Go `main` entry and links
`perf/profile.go.txt`. This starts/stops `runtime/pprof` around the original
program without changing its algorithm. None of the timed binaries contains
that instrumentation. The full 20 MB workload is run once for the CPU profile;
its output matches the timed binaries.

Exact `go tool pprof -top -nodecount=10` output:

```text
File: profile-bin
Build ID: 1b7b43d6fe636d0e6f2ffefc2aa794050a6ec3b6
Type: cpu
Time: 2026-11-04 09:34:07 JST
Duration: 2.89s, Total samples = 2850ms (98.58%)
Showing nodes accounting for 1360ms, 47.72% of 2850ms total
Dropped 36 nodes (cum <= 14.25ms)
Showing top 10 nodes out of 104
      flat  flat%   sum%        cum   cum%
     330ms 11.58% 11.58%      490ms 17.19%  runtime.tryDeferToSpanScan
     150ms  5.26% 16.84%      150ms  5.26%  cmpbody
     150ms  5.26% 22.11%      320ms 11.23%  go4t/rt.LtString (inline)
     140ms  4.91% 27.02%      640ms 22.46%  runtime.scanObjectSmall
     120ms  4.21% 31.23%      120ms  4.21%  runtime.memclrNoHeapPointers
     110ms  3.86% 35.09%      510ms 17.89%  main.dict_prelude_Ord_String.Compare (inline)
     100ms  3.51% 38.60%     1130ms 39.65%  go4t/rt.collBalance
     100ms  3.51% 42.11%      430ms 15.09%  go4t/rt.collLookup
      80ms  2.81% 44.91%       80ms  2.81%  runtime.(*mspan).moveInlineMarks
      80ms  2.81% 47.72%      630ms 22.11%  runtime.mallocgcSmallScanNoHeader
```

- **Persistent structures:** `rt/coll.go` copies AVL search paths on insertion,
  including replacements. Wordfreq performs a lookup and a replacement for
  each of two million words. `collBalance` has 39.65% cumulative samples and
  `collLookup` 15.09%; string comparisons also appear directly in the top ten.
  These cumulative figures include callees and must not be added to their
  allocation/comparison percentages.
- **Boxing and allocation:** generated list cells and option/tuple fields carry
  `any`, as do `rt.collNode` keys and values. The generated increment function
  crosses that boundary for each word. The profile directly shows scanning,
  clearing, and allocation costs; `scanObjectSmall` is 22.46% cumulative and
  `mallocgcSmallScanNoHeader` 22.11%. [INFERENCE] The boxed list plus copied AVL
  paths explain much of the memory/time gap, but this CPU profile does not
  isolate how much belongs to boxing versus persistence. No allocation-profile
  percentage is claimed.
- **Recursion:** wordfreq's input fold, integer generation, map loops, and
  timestep loop are tail-recursive. Tree construction/folding is deliberately
  non-tail-recursive, but its maximum depth is only 20. All requested sizes
  complete. [INFERENCE] Stack growth is not the leading limit at these sizes;
  this does not establish safety for a million-deep non-tail-recursive list.
- **Other kernels:** sortints pays for the list-to-boxed-array path, immutable
  sorting copy, comparator calls, and a different sorting algorithm. Nbody
  repeatedly creates immutable arrays/records rather than only updating native
  fields. These source-level differences identify plausible costs, not measured
  CPU breakdowns: only wordfreq is profiled here.

The result supports million-element workloads, not performance parity. For a
throughput-sensitive core, the 10–32x differences in four kernels are substantial
even though each finishes in roughly three seconds on this machine. No compiler
optimization is attempted as part of this measurement.

## Reproduction and exact commands

From the repository root, a fresh complete run is:

```sh
bench/perf/run.sh
```

Requirements: Go, Python 3.9+, Bash, GNU `/usr/bin/time`, GNU `timeout`, and the
serializer. `SERIALIZE_VM_TESTS_RUNNER` overrides the serializer executable path.
Optional positional kernel names restrict the run; `--compiler FILE` selects a
built compiler snapshot; `--work-dir DIR` selects a **new** directory beneath
`bench/perf/.work`. Without it, the harness creates a unique artifact directory.
`--emit-go DIR --package main` is the source-emission interface; old compiler
snapshots that reject an explicit main package are not supported.

The successful recorded command was:

```sh
bench/perf/run.sh --compiler bench/perf/.work/measured-20261104-current/go4t --work-dir bench/perf/.work/results-20261104
```

It exited 0. Its compiler snapshot had been built by the harness with
`GOMAXPROCS=1 GOGC=100` and this command (also exited 0):

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go build -o /workspace/go4t/bench/perf/.work/measured-20261104-current/go4t ./cmd/go4t
```

The harness sets `GOMAXPROCS=1`, `GOGC=100`,
`SERIALIZE_VM_TESTS_QUEUE_TIMEOUT_SECONDS=180`, and
`SERIALIZE_VM_TESTS_TIMEOUT_SECONDS=180`. It generates the shared inputs with:

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go run ./bench/perf/gen /workspace/go4t/bench/perf/.work/results-20261104/input
```

For each kernel it emits Go, builds the emitted package, and builds the plain-Go
package. For example, the exact wordfreq build and first timing commands were:

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- /workspace/go4t/bench/perf/.work/measured-20261104-current/go4t build --emit-go /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/generated --package main /workspace/go4t/bench/perf/wordfreq/main.gt
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go build -o /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/go4t-bin /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/generated
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go build -o /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/go-bin /workspace/go4t/bench/perf/wordfreq/go
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- bash -c 'input=$1; shift; exec "$@" < "$input"' go4t-perf-input /workspace/go4t/bench/perf/.work/results-20261104/input/wordfreq.txt /usr/bin/time -f '%e %M' -o /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/go4t-1.time timeout --kill-after=5s 175s /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/go4t-bin
```

The serializer backgrounds its child. Opening stdin **inside** that reservation
is necessary; redirecting the serializer's stdin itself yielded EOF in an early
harness attempt. That attempt is excluded. The final harness also rejects empty
matching output, so an empty wordfreq result cannot masquerade as a valid run.

Exact profiling commands after the harness creates `profile-generated`:

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go build -o /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/profile-bin /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/profile-generated
GO4T_PERF_CPU_PROFILE=/workspace/go4t/bench/perf/.work/results-20261104/wordfreq/cpu.pprof /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- bash -c 'input=$1; shift; exec "$@" < "$input"' go4t-perf-input /workspace/go4t/bench/perf/.work/results-20261104/input/wordfreq.txt timeout --kill-after=5s 175s /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/profile-bin
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go tool pprof -top -nodecount=10 /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/profile-bin /workspace/go4t/bench/perf/.work/results-20261104/wordfreq/cpu.pprof
```

All three exited 0. `commands.log` records the expanded commands for every build
and all 30 timed executions, including captured stdout/stderr paths; that log
is also embedded in the retained JSON. The 175-second per-process deadline is a
safety bound, not a CI performance budget. Failures retain their output and exit
status and make the harness exit nonzero. Profiling runs after timing, so it does
not affect the table's elapsed times or binary sizes.

Generated inputs, binaries, Go sources, profiles, and local logs stay in ignored
`bench/perf/.work/` for inspection. Only the paired sources, reusable harness,
small retained evidence JSON, and this report belong in the source tree.
