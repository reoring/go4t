# Complete client-go port to gofx

## Authority and ownership

- User goal: rewrite the entire Kubernetes client-go library in gofx as an experiment; use parallel `worker-ultrafast` agents on `openai-codex/gpt-6.1-sol`.
- The primary is supervisor only: authority selection, domain design, task ownership, acceptance decisions, and orchestration. All executable source changes belong to workers.
- Repository: https://github.com/reoring/gofx.git.
- Worktree: `/workspace/wip/gofx/.wt/client-go`, branch `experiment/client-go-gofx`, starting revision `de2d9100d39af9f49e68bd1e6eba535062fef56e`.
- Upstream target: https://github.com/kubernetes/client-go, latest stable release `v0.37.2`, commit `72c52dd0cc3b9099b36af11290b0304045dd1ada`.
- Read-only upstream checkout: `/workspace/wip/gofx/.tmp/client-go-v0.37.2`.
- The upstream README recommends release tags. Unreleased master was inspected at `b51563e839b761a4b85dca15faf4a9bac49a84c0` but requires Go 1.27. The selected stable release requires Go 1.26; the observed host toolchain is Go 1.26.1. Master is not the parity target.
- Current language authority: this worktree's `SPEC.md`, especially sections 2.2, 3, 6.3, 8.3, 8.7 and 9. Root `DESIGN.md` is historical background, not an override of SPEC.

## Meaning of complete

All client-go implementation packages, generated API families, fake clients, testing facilities, utilities, authentication plugins, examples, and platform-specific behavior in the pinned source belong in the inventory. No package may disappear because a binder cannot express its signature. Upstream tests and fixtures define observable behavior and must be carried into the parity evidence.

This is a native gofx library, not a promise of Go source-level drop-in compatibility. The public capabilities, configuration precedence, wire behavior, lifecycle, error distinctions, ordering, cancellation, and extension points must be preserved. An exported upstream symbol must map to a real implementation or an explicitly documented equivalent native API; unsupported placeholders do not count.

Bindings that call the original client-go implementation, embedded Go copies of client-go algorithms, stubs, fake fallbacks, source-text translation that cannot compile, and empty package markers are not a port. The production dependency graph must not import `k8s.io/client-go`. The pinned upstream may be used only by an isolated oracle/test harness.

## Domain model first

- **Client configuration** owns resolved endpoint, credentials, TLS, serialization, throttling, timeout, and impersonation settings. Configuration loading/merging is separate from request execution.
- **Request** is an immutable description of verb, group/version/resource, namespace/name, subresource, query, headers, body, and execution policy. Builders transform descriptions; an executor owns network effects and response lifetime.
- **Resource identity and object** carry group/version/kind, metadata, and typed or unstructured content. Kubernetes API and apimachinery are upstream dependencies, not client-go implementation; their external contracts can remain dependencies.
- **Response and failure** preserve Kubernetes status details, retry hints, warnings, negotiated media type, and stream state. Reducing structured errors or integers to lossy strings/floats is forbidden.
- **Watch** has a single lifecycle owner and an ordered event stream. End-of-stream, cancellation, expiration, retryable transport failure, bookmark, and API error are distinct outcomes.
- **Cache aggregate** owns indexed objects and resource versions. Delta queues, reflectors, shared informers, and listeners have separate responsibilities and declared transitions. Each projection has one writer.
- **Work queue aggregate** owns pending, dirty, processing, delayed/retry, and shutdown/drain state. Its behavior must retain upstream deduplication and concurrent-producer/consumer invariants.
- **Lease/election aggregate** owns observed lease identity, acquisition, renewal, loss, and release. The algorithm is gofx; persistence and clocks are ports.
- **Generated resource family** is a descriptor plus typed object and apply-configuration definitions. Clientsets, typed clients, fake clients, listers, informers, and apply builders are generated from the complete upstream inventory, not hand-enumerated subsets.
- **Extension ports** are declared functions/records or type classes. New authentication providers, codecs, resources, metrics sinks, and transports must register adapters without editing unrelated consumers.

## Implementation boundaries

- Target module root: `ports/client-go/`. Preserve upstream-relative domain paths where possible.
- Pure algorithms, configuration policy, request policy, retry decisions, cache/queue/election transitions, generated API behavior, and fake-client behavior are implemented in `.gfx`.
- Go host code is limited to generic effects not currently expressible in gofx: synchronization primitives, reference/channel mechanics, socket/TLS/stream I/O, subprocess execution, cryptography, and external dependency codecs/conversions. It must not implement client-go policy or call client-go.
- Effects are explicit `IO`; context is the existing implicit cancellation environment. No impure extern masquerading as a pure function.
- First-class callbacks and aggregate state must remain typed in the source API. Do not use `dyn.Value` as a universal replacement for typed clients.
- Exact shared signatures and file ownership are frozen before their consumer implementation wave. An evidence worker may report requirements but cannot choose authority, redefine acceptance, or narrow scope.
- New language/runtime facilities are introduced only for an observed requirement that cannot be implemented through the existing declared ABI; one worker owns each compiler/runtime boundary.

## Work sequence and single-writer map

1. Inventory owner: complete source/symbol/dependency/test inventory and reproducible inventory tooling only.
2. Runtime evidence owner: read-only extraction of concurrency/lifecycle requirements from workqueue and cache, matched against existing gofx effect/FFI contracts.
3. Generation evidence owner: read-only extraction of all generated-family templates, cross-package data/API contracts, and exact parity probes.
4. Supervisor freezes shared contracts and assigns implementation slices by upstream domain. Shared primitives and manifests are established and committed before dependent fan-out.
5. Independent implementation slices run together using the user-requested `worker-ultrafast`/Sol 6.1. Workers do not recursively delegate, commit, push, stash, or run formatters, linters, builds, or suites mid-flight.
6. An integration worker owns cross-package glue after individual ownership is released. The supervisor accepts the result from direct evidence, not worker claims.
7. Verification runs under supervisor orchestration once the integrated tree is ready. A specifically assigned narrow PoC may run earlier; it is not a whole-suite run.

The inventory will determine the exhaustive implementation slice ledger. No unassigned package is implicitly out of scope.

## Acceptance

- Inventory completeness: every upstream implementation package, generated family and exported capability is accounted for against the pinned revision; no truncated directory listing can support a complete claim.
- Build/typecheck: all gofx modules and generated Go artifacts compile with the declared dependency graph.
- Behavioral parity: upstream expectations for configuration precedence, requests, codecs/statuses, watches/relist/bookmarks, queues/draining, caches/informers, fake actions, leader election, authentication, streaming and utilities are exercised.
- Regeneration: clean regeneration produces the committed bytes; generated sources have source/version provenance and preserve upstream Apache 2.0 notices.
- Runtime proof: run the port through Kubernetes API operations and failure paths, including cancellation and concurrent watch/cache/queue use. Tests alone are insufficient.
- Honest reporting: source inspected, user-reported results, worker claims and directly exercised evidence stay distinct. No claim of full completion while a package, capability, or required verification remains unaccounted for.

## Shared prerequisite contracts

The exhaustive inventory now records 2,531 files, including 2,453 Go files, 2,001 generated files and 157 test files. Its 401 package entries distinguish external test packages from implementation packages. These are scope counts, not implementation progress. `upstream-inventory.json` and `tools/inventory/main.go` are the reproducible manifest and its owner.

### Module and host-effects boundary

- Go and gofx module name: `github.com/reoring/gofx/ports/client-go`. Local Go dependency `gofx v0.0.0` is replaced by `../..`; Go version is 1.26.0. The module's own source is imported using upstream-relative paths.
- Native gofx callbacks are the public contract. Exact Go-interface drop-in compatibility is not required; existing `--emit-go` limitations do not justify dropping native generic/callback APIs.
- `support/sync` is the only generic synchronization surface; `internal/hostsync` implements its host effects. Types: `Ref[A]`, `Mutex`, `Condition`, `Channel[A]`, `Task[A]`, `Clock`, `Instant`, `Timer`, all opaque nominal handles.
- All allocation, reads of mutable state, mutation, blocking, notifications, task operations and clock operations return `IO`. `newRef[A](value A) IO[Ref[A]]`, `readRef[A](ref Ref[A]) IO[A]`, `writeRef[A](ref Ref[A], value A) IO[Unit]`. References alone do not make multi-step transitions atomic.
- `newMutex() IO[Mutex]` and `withLock[A](mutex Mutex, action IO[A]) IO[A]` provide cancellation-aware acquisition and exception-safe release. `newCondition(mutex Mutex) IO[Condition]`, `wait(condition Condition) IO[Unit]`, `signal(condition Condition) IO[Unit]`, `broadcast(condition Condition) IO[Unit]`; wait atomically registers/releases/reacquires the associated mutex, including cancellation paths. Caller uses condition operations inside withLock. Algorithms and predicates remain gofx.
- `newChannel[A](capacity Int) IO[Channel[A]]`, `send[A](channel Channel[A], value A) IO[Unit]`, `receive[A](channel Channel[A]) IO[Option[A]]`, `close[A](channel Channel[A]) IO[Unit]`. Zero capacity is rendezvous, negative capacity is an error; None is closed-and-drained, never a data value. Native select is `selectReceive[A](channels List[Channel[A]]) IO[(Int, Option[A])]`; it atomically consumes one winning source, never consumes a losing source, and handles cancellation without spinning. Heterogeneous callers explicitly map events to a common sum type.
- Tasks are distinct from std/conc Fiber: `start[A](action IO[A]) IO[Task[A]]` is parent-linked; `startDetached[A](action IO[A]) IO[Task[A]]` preserves context values but not parent cancellation/deadline; `cancelTask[A](task Task[A]) IO[Unit]`; `join[A](task Task[A]) IO[A]` waits for actual completion or caller cancellation, not task-cancel acknowledgement. `uninterruptible[A](action IO[A]) IO[A]` supplies cleanup context without cancellation while preserving values. No arbitrary 100ms abandonment in strict joins.
- `realClock() IO[Clock]`, `fakeClock(startNanos Int) IO[Clock]`, `now(clock Clock) IO[Instant]`, `unixNanos(instant Instant) Int`, `elapsed(start Instant, end Instant) Int`, `add(instant Instant, nanos Int) Instant`, `compareInstant(a Instant,b Instant) Ordering`; Instants preserve real monotonic readings. `newTimer(clock Clock, nanos Int) IO[Timer]`, `timerChannel(timer Timer) IO[Channel[Instant]]`, `stopTimer(timer Timer) IO[Bool]`, `resetTimer(timer Timer,nanos Int) IO[Bool]`, `advance(clock Clock,nanos Int) IO[Unit]` for fake clocks. Nanosecond durations, no Float conversion.
- Handle construction and tuple/Option bridging use the existing erased extern ABI and rt.Shapes; no compiler changes or client-go policy are authorized in this prerequisite.

### External typed dependencies

- `k8s.io/api` and `k8s.io/apimachinery` stay external dependencies at v0.37.2. Their types and codecs must not be replaced with lossy Json/dyn representations.
- One canonical dependency binding graph lives under `deps/`; all generated families reuse it. Independent nominal runtime.Object/selector handles per consumer are forbidden.
- Source-facing object types remain distinct by their actual Go package and type, including versioned Scale types. Opaque handles are acceptable for external dependency objects only when the complete field/constructor/codec access needed by client-go is exposed. An opaque handle is not permission to omit its functionality.
- The generator must preserve nil/presence, exact integer/byte values, embedded-field lookup, recursive graphs and cross-package identity. Record mutation cannot be implemented by a pointer-receiver bind that silently mutates a discarded copy. Immutable object access/copy-on-write and explicitly effectful identity mutation must be distinguished.
- Generator outputs and descriptors must account for every exported upstream capability; source mapping is required. No general Go-to-gofx transpiler that preserves hidden client-go Go bodies is allowed.

### Prerequisite owners

- `ClientGoRuntime`: module manifests, `support/sync/**`, `internal/hostsync/**`, and a throwaway primitive smoke. No networking, API-domain logic or compiler edits.
- `ClientGoDependencyContract`: read-only extraction of the exact external API type/codec/selector graph into a machine-readable proposed binding manifest and bounded evidence. No shared manifest or product-binding edits until the supervisor accepts the canonical schema. This is independent of synchronization implementation.

Before the main implementation fan-out, the supervisor will accept and commit these shared contracts and verified runtime primitives, then assign every implementation package from the inventory to one owner.

## Accepted native API decision and canonical dependency schema

The user explicitly selected **gofx-facing APIs**, not unchanged Go-source compatibility. This does not remove any client-go functionality from scope.

- `ownership.json` assigns all 401 package records / 388 source directories to 22 owners, using longest matching upstream path prefix; external test companions stay with the source directory. It is the exhaustive slice ledger. Pending means unimplemented, never inferred complete from inventory tooling.
- `deps/types` owns canonical nominal external types. `deps/<actual Go import path>` reuses them through aliases; deterministic fully-qualified identity hashes prevent name collisions. Only actual Go aliases share nominal identity.
- External structured values may be host-backed typed snapshots, with complete usable typed constructor/field access/update functions. Operations are named `newPod`, `podGetObjectMeta`, `podWithObjectMeta`, consistently including receiver type and `Get` for getters. No untyped typed-client surface.
- Pointer values preserve Option presence. Nil versus empty slices/maps and exact bytes/integers remain distinct. Native Map is used only with faithful Ord keys; otherwise a canonical `ForeignMap[K,V]` retains actual Go key equality. UInt64 uses checked high/low components in [0,2^32-1], not signed truncation.
- Field access does not serialize or copy whole objects. Snapshot updates copy the containing structure; mutation methods return the updated snapshot and retain every original result. API DeepCopy is used where a mutating method would otherwise alter shared descendants.
- Identity-bearing external services (HTTP clients/transports, readers/writers, decoders, locks, stateful factories) are **not snapshots** and must never be copied. Their operations run in IO on the original identity. Type metadata explicitly distinguishes snapshot and identity, with source-backed decisions.
- External call adapters preserve all return values, including partial result plus optional original error. `DecodeOutcome` retains object, optional GVK and original structured error even on failure, using a fresh decode destination. Error predicates and wrapping identity are retained, not replaced with message-only errors.
- `oracle/` is an isolated Go module deliberately importing the pinned original client-go to collect differential traces. Production dependencies may not import it. Its first captured fixtures exercise real HTTP request construction, fake actions, apply builders and managed-field conflicts; they do not constitute native-port verification.
- Shared runtime smoke and focused race checks have passed after correcting send/close synchronization and child condition-ownership leakage. Condition use is scoped to the sequential owning withLock action; arbitrary foreign/std-conc goroutine ownership is not claimed to be enforced.

For native client-go domain APIs, retain upstream module boundaries and exported capabilities, with lowerCamel free functions and explicit receiver arguments in place of Go methods. Use immutable records/sums for decisions and callback-capability records for extensible interfaces; stateful aggregates carry typed host primitive handles. Public cross-domain signatures must be published to their consumers and accepted by the supervisor before incompatible changes. Domain workers do not select alternate dependency types or invent parallel representations.
