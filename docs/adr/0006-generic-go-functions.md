# ADR 0006: call-site instantiation of generic Go functions

Date: 2026-11-04
Status: Accepted / Implemented (2026-11-04; bounded function scope below)

## Context

ADR 0003 binds a Go package during import, before checking its callers. At the
baseline below, generic Go functions remained unreachable under SPEC §8.
Generic go4t extern declarations already existed for erased Go targets.

### Baseline implementation

- `extern/load.go:Loader.Load` reads Go export data and retains `go/types.Signature`.
  `extern/mirror.go:mirrorSet.call` rejects nonempty `TypeParams` or
  `RecvTypeParams` with `generic function`. `extern/shim.go:BindWithShim` prints
  `// skipped: Name (generic function)`; its receiver check also excludes generic
  named types. Removing the signature check alone does not enable their methods.
- `cmd/go4t/bind_shim.go:prepareShimDir` resolves a receiving Go module and package.
  `bind --shim DIR` writes one `go4t_shim_<pkgbase>.go` and prints declarations;
  `cmd/go4t/bind.go` calls `BindWithShim` directly, without the import-time cache.
- `cmd/go4t/goimport.go:goImportBinder.Bind` generates or retrieves one binding per
  package, parses it, and writes its shim beneath `<out>/go4tbind/<import path>`.
  Check-only loading uses a placeholder shim path and does not write build output.
  `modules` uses the `GoBinder` port, not an `extern` dependency.
- `cmd/go4t/bindcache.go:bindingCacheKey` includes compiler identity, Go version,
  export-data SHA-256, import path, Prefix/Package/Float, the sorted Prelude set,
  and shim path/name. Entries hold `bindings.gt`, `shim.go`, and `records.json`.
  Caller types are absent. `TestBindingCacheKey` fixes these invalidation inputs.
- The call path is `codegen/expr.go:identCall` → generated extern wrapper →
  `codegen/extern.go:emitExtern` → original Go function or erased shim → Go call.
  `emitExtern` uses `extern.Match` against the declaration's scheme, not a distinct
  wrapper per call-site instantiation. `codegen/foreign.go:foreignShapes` registers
  record conversions; it does not dispatch calls and currently excludes generic
  record declarations. Ownership keys are `<Go import path>.<Go type name>`.
- The checker **retains instantiated go4t types at codegen time**:
  `types/expr.go:ident` sets `Ann.Inst`, call inference unifies arguments and the
  expected result, and `types/resolve.go` zonks `Ann.Type` and `Ann.Inst`.
  Monomorphic uses have concrete types. A polymorphic function body can retain
  rigid `TVar`s; unresolved ordinary metas otherwise default to Unit. These are
  not exact Go type arguments: several Go integers map to Int, and Go constraints
  are not represented by go4t Eq/Ord dictionaries.
- `codegen/specialize.go` is a dictionary optimization, not general monomorphisation:
  a body, class constraints, at most 40 AST nodes, and at most 8 copies are required.
  Copies are keyed by concrete dictionaries, not all type arguments; unconstrained
  arguments stay erased. Externs have no body. `specialize_test.go` explicitly
  tests erased fallback and sharing a copy across different unconstrained types.

### Measured surface

Environment: Go 1.26.1, linux/amd64; apimachinery and client-go v0.34.0, matching
`examples/node-drain-typed/go.mod`. Repository starting revision: `c2a89b1`.
Measurements use the working tree, not an implementation of this proposal.

An isolated module `genericsadr` at `/tmp/go4t-generics-adr-lhuI8y` requires both
Kubernetes modules at v0.34.0. Commands, from the repository root:

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go run ./cmd/go4t bind slices --shim /tmp/go4t-generics-adr-lhuI8y/slices
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go run ./cmd/go4t bind maps --shim /tmp/go4t-generics-adr-lhuI8y/maps
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go run ./cmd/go4t bind sync --shim /tmp/go4t-generics-adr-lhuI8y/sync
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- env GOFLAGS=-mod=mod go run ./cmd/go4t bind k8s.io/apimachinery/pkg/util/sets --shim /tmp/go4t-generics-adr-lhuI8y/sets
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- env GOFLAGS=-mod=mod go run ./cmd/go4t bind k8s.io/client-go/util/workqueue --shim /tmp/go4t-generics-adr-lhuI8y/workqueue
```

All five commands exit 0. Counts use exact `// skipped: ... (generic function)`
lines; names containing `.` are methods, not package-level functions. Extern
counts include generated accessors, constructors, and upcasts. Summary:

```text
slices: generic functions=40, generic methods=0, generic skips=40, all skips=40, externs=0
maps: generic functions=10, generic methods=0, generic skips=10, all skips=10, externs=0
sync: generic functions=2, generic methods=0, generic skips=2, all skips=3, externs=46
k8s.io/apimachinery/pkg/util/sets: generic functions=9, generic methods=16, generic skips=25, all skips=25, externs=86
k8s.io/client-go/util/workqueue: generic functions=14, generic methods=69, generic skips=83, all skips=90, externs=62
TOTAL: generic functions=75, generic methods=85
```

Top skipped entry points, selected for collection and controller use, not ranked
by usage frequency (verbatim output excerpts, grouped in the package order above):

```text
// skipped: BinarySearch (generic function)
// skipped: Clone (generic function)
// skipped: Contains (generic function)
// skipped: Sort (generic function)
// skipped: Sorted (generic function)
// skipped: Clone (generic function)
// skipped: Equal (generic function)
// skipped: Keys (generic function)
// skipped: Values (generic function)
// skipped: OnceValue (generic function)
// skipped: OnceValues (generic function)
// skipped: Insert (generic function)
// skipped: KeySet (generic function)
// skipped: List (generic function)
// skipped: New (generic function)
// skipped: DefaultTypedControllerRateLimiter (generic function)
// skipped: NewTyped (generic function)
// skipped: NewTypedRateLimitingQueue (generic function)
// skipped: NewTypedRateLimitingQueueWithConfig (generic function)
```

The other set functions are `ByteKeySet`, `Int32KeySet`, `Int64KeySet`, `IntKeySet`,
`StringKeySet`. Other workqueue functions are `DefaultQueue`,
`DefaultTypedItemBasedRateLimiter`, `NewTypedDelayingQueue`,
`NewTypedDelayingQueueWithConfig`, `NewTypedItemExponentialFailureRateLimiter`,
`NewTypedItemFastSlowRateLimiter`, `NewTypedMaxOfRateLimiter`, `NewTypedWithConfig`,
`NewTypedWithMaxWaitRateLimiter`, `TypedNewDelayingQueue`.

The generic reason wins before other mirror checks; 75 skips do not imply 75
usable bindings after instantiation. Source signatures expose further limits:

- Five slices functions take iterators: `AppendSeq`, `Collect`, `Sorted`,
  `SortedFunc`, `SortedStableFunc`. Maps `Collect` and `Insert` also take iterators.
  `mirrorSet.resolve` rejects iterator parameters; only results are materialized.
- Set `New`, `KeySet`, `Insert`, `List` use `Set[T]`. Every skipped workqueue
  function uses instantiated named types. Both named-value and handle resolution
  currently reject them, including `Typed[any]` in already nongeneric functions.
- `sync.OnceValue` and `OnceValues` capture a `sync.Once` and cached results.
  A returned function can therefore observe shared state without a visible handle
  argument. Merely removing the generic skip would expose a pure-looking closure.
- In-place operations do not become functional operations. `slices.Sort` and
  `Reverse`, and maps `Copy` and `DeleteFunc`, mutate boundary copies and return
  Unit; their changes do not update the caller's List/Map. Enabling them does not
  replace `std/list.sort`, which returns a sorted list.

## Decision

Accept **A: call-site monomorphisation**, with conservative IO and separate
template/instance cache identities. The implemented function scope and departures
from the original broader proposal are recorded below.

### A. Call-site monomorphisation — accepted

After inference, request one `F[T1, …]` shim wrapper per distinct concrete Go type
 tuple. Import loading exposes generic signature/constraint metadata; a second
shim pass runs after checking and instantiation planning, before Go emission.

[INFERENCE] With existing concrete mirrors, candidate function reach is slices
35/40, maps 8/10, sync 2/2, sets 5/9, workqueue 0/14. Supporting instantiated named
types and their methods, as required by this option, additionally targets the
remaining 4 set functions, 14 workqueue functions, and 85 receiver methods.
Thus 68/75 function names have candidate instantiations without iterator-input
support; this is a signature-based projection, not a successful binding count.

Concrete Go constraints and nominal handles stay intact. Copy boundaries and
handle-argument IO remain mandatory; conservative IO also covers hidden closure
state. The existing optimizer is insufficient: required instantiation cannot fall
back to an erased generic Go call after eight copies.

[INFERENCE] High cost: roughly 20–25 source files plus focused tests. Owners are
`extern/{load,mirror,shim,methods,upcasts}.go` (templates, constraints, concrete
mirrors), `ast/ast.go`, `modules/load.go`, `types/{types,modules,expr,resolve}.go`
(metadata and obligations), `codegen/{codegen,expr,extern,foreign,mirror}.go` and a
required-instantiation planner, `cmd/go4t/{goimport,bindcache,main,tools}.go`, and
`lsp/server.go` (the same validation in check-only paths). No call syntax changes.
The package cache remains reusable for templates; emitted shims need a second,
request-dependent key.

### B. Instantiate at `any` and box the boundary — not recommended

Use `[]any`/`map[any]any` for structural type parameters, and `any` for eligible
element parameters. Go accepts `any` for `comparable`, but equality or hashing
still panics for dynamic slices, maps, or functions. go4t Eq does not prove Go
comparability. `any` currently maps to `dyn.Value`, not an arbitrary go4t value
or a handle-preserving box.

[INFERENCE] A strict any-only policy, excluding comparable equality, has 25/40
slices candidates: seven ordered functions, four comparable functions, and four
additional iterator-input functions remain out. The four comparable functions
(`Compact`, `Contains`, `Equal`, `Index`) require statically safe scalar values or
an additional checked boundary. Maps gain 0/10 under the current scalar-key mirror:
`map[any]any` is not mirrorable. Sync has 2/2 signature candidates and the legacy
set key extractors have 5/9; typed sets/workqueues still need instantiated named
mirrors. Boxing alone does not unlock the measured 85 receiver methods.

Handles must not disappear into dyn values and escape IO classification. Returned
closures require the same effect decision as A. Ordered constraints cannot use
`any`; adding fixed scalar variants or type-directed dispatch converges on A.

[INFERENCE] Medium cost: about 5–8 source files around `extern/{bind,mirror,shim,
match}.go`, runtime dynamic conversion if its domain changes, and binding-cache
metadata, plus tests. A fixed set of boxed wrappers stays package-deterministic;
compiler identity invalidates old cache entries. Caller-selected variants require
A's request key. A new generic box ABI is not justified by the smaller reach.

### C. Explicit user instantiation — not recommended as a prerequisite

Add expression type application, for example `slices.sort[Int](xs)`. go4t has type
parameters on declarations and type constructors, but no expression type
application: `parser.postfix` accepts calls, projections, and record updates only.
The example also needs a rule for omitted structural parameters: Go Sort has
`S ~[]E, E cmp.Ordered`, not just one type parameter.

[INFERENCE] Candidate reach matches A's 68/75 functions and conditional 85 methods;
explicit types additionally resolve otherwise ambiguous constructor calls. They
do not supply iterator-input conversion, instantiated named mirrors, or an effect
model. Handles, callbacks, and IO obey exactly A's rules.

[INFERENCE] Highest cost: A's backend plus approximately 6–10 syntax/tooling files,
including `ast/{ast,walk}.go`, `parser/parser.go`, `format/format.go`,
`modules/load.go`, `types/{expr,index}.go`, and editor presentation, plus tests.
Explicit tuples still vary by caller, so the final per-package shim cache requires
a normalized request set. Syntax does not restore the existing package-only key.

### D. Keep skips; use native std equivalents — insufficient alone

[INFERENCE] New Go binding reach is 0/75 functions and 0/85 methods. Existing
`std/list.sort`, `sortBy`, `reverse`, `findIndex`, `std/map` operations, and
`std/set` operations already cover substantial collection intent without Go
mutation semantics. They do not provide a typed client-go queue or an equivalent
of a shared OnceValue cache.

Pure persistent operations preserve the law boundary. Shared memoization or queue
state still requires IO; it cannot be hidden in a pure std implementation. Existing
untyped workqueue bindings are useful but not new typed reach.

[INFERENCE] Low collection cost: `std/{list,map,set}/*.gt` and their focused tests;
zero changes if the existing operations suffice. Stateful equivalents additionally
need runtime IO support and must not duplicate client-go infrastructure. The
binding pipeline and package cache are unchanged. Retain these native operations
as the preferred collection API even if A is accepted.

## Rules

The accepted design rules follow. The bounded implementation deviations below
take precedence over the original named-type and local-helper scope.

1. **Preserve Go identity.** Carry original signatures, constraints, and owner
   metadata through the binder/checker port. Validate concrete tuples with Go's
   constraint rules, not go4t Eq/Ord. Use canonical Int → `int64`, Float →
   `float64`, Bool → `bool`, String → `string` when no exact foreign owner type
   applies; List/Map compose these mappings. Preserve an explicitly known foreign
   record/handle's Go identity. A lossy mirror alone cannot recover a named Go
   type or satisfy its method-set constraint; reject rather than guess.
2. **Resolve before emitting.** Check generic calls and function values, including
   partial applications and uses inside polymorphic helpers. Propagate required
   concrete tuples through their reachable callers; retain ordinary erased code
   where no foreign instantiation is needed. No dictionary-optimizer budget may
   silently remove a required instance. Diagnose an unresolved tuple, unsupported
   constraint/mirror, or unbounded polymorphic instantiation at its source use.
   Do not let default-to-Unit invent a foreign type argument. `check` and LSP run
   the same obligation validation without writing shims or building Go code.
3. **Include instantiated named types.** Apply ADR 0003's record-versus-handle
   predicate after substitution. Keep declarations in their owning binding module;
   include the canonical Go argument tuple in nominal identity, record keys,
   wrapper names, and converter keys. `Set[Int]` and `Set[String]`, or two distinct
   Typed queues, must not alias. Instantiate receiver methods/accessors/upcasts
   with the same tuple. Private state remains opaque; pointers to handles do not
   become copied records. Do not count typed workqueue reach without this work.
4. **Declare effects before inference.** All newly admitted generic foreign calls
   return IO, including constructors; callback invocations at this boundary are
   also IO in both directions. This conservative rule avoids type-dependent
   effect changes after checking and covers retained closure state. For example,
   the proposed go4t shape is `onceValue[A](f func() IO[A]) IO[func() IO[A]]`.
   Execute conversion and the Go call inside the action, with cancellation checked
   first. Preserve existing callback context and failure conventions; test retained
   callbacks separately from immediate invocation. Keep handle reads, nested
   handle inputs, setters, and returned stateful functions outside pure laws.
5. **Do not invent mutation results.** Preserve copy boundaries and Go results:
   `Sort` stays an action returning Unit, not a new List-returning API. Iterator
   parameters remain unsupported; iterator results remain materialized Lists.
   Map-key restrictions and the three-payload-result limit remain unchanged.
6. **Split cache identity.** Cache package templates under the existing base key
   plus the metadata schema version. Cache concrete output under that base key
   plus a sorted, deduplicated set of `(owner, symbol, canonical Go type tuple)`
   requests, concrete mirror identities and their owner export-data fingerprints,
   and the effect/mapping policy version. Preserve shim path/name and all existing
   key inputs. Include receiver/type requests in the same manifest. Publish the
   complete immutable entry atomically; never append caller-specific wrappers to
   the old package-only entry. Reordering or repeating calls does not invalidate
   it; a new tuple does. Nongeneric package entries retain their current behavior.

**Explicit deviations from current rules:** SPEC §8 and ADR 0003's blanket generic
skip becomes validated concrete instantiation; the complete Binding ceases to be
independent of callers; generic named identities include arguments; newly admitted
generic calls, constructors, and callbacks use conservative IO even when ADR
0003's signature/handle predicate alone would classify them as pure. Nongeneric
bindings and trusted runtime externs do not change under this proposal.

## Consequences

- Go generics become reachable without introducing expression type application or
  erasing constraints into dyn values. Collection operations still favor std.
- Package-template cache hits survive caller edits. Concrete shim cache entries,
  generated code size, and build work grow with distinct requested tuples.
- Generic foreign calls require IO even for copied scalar collections. This buys
  a stable, sound effect scheme; relaxing it needs a separate purity decision.
- Source inference supplies concrete arguments without adding expression type
  application. Unresolved, unsupported, or invalid instances are rejected before
  code generation; ordinary default-to-Unit cannot invent a Go argument.
- The baseline skip counts remain historical evidence. Registering a template
  does not prove that every instantiation of that symbol is mirrorable.

## Implemented scope and deviations

- Import-time binding registers every generic package function as a typed
  template. `ast.ForeignBindings` is the binder/checker port; `modules` does not
  depend on `extern`. Cache hits restore this port without rebuilding package
  declarations. Exact Go signatures and constraints are loaded on demand.
- Required instantiation runs before `types.Check` returns. Codegen consumes
  ordinary monomorphic externs and helper copies, rather than owning a second
  independent planner. Check and LSP therefore use the same validation. Check
  does not write build shims.
- Top-level polymorphic helpers specialize transitively, including constrained
  helpers, partial applications, and function values. Required copies have no
  40-node/8-copy optimization budget. Changing types along a recursive expansion
  is diagnosed as unbounded polymorphic instantiation. A generalized local
  lambda whose foreign arguments remain rigid requires a concrete annotation;
  it is diagnosed rather than silently emitted through an erased fallback.
- **Named-type scope is narrower than Rule 3:** generic receiver methods and
  instantiated named structs/interfaces remain unsupported. Generic named
  collection aliases use existing structural mirrors when possible. Typed
  workqueue construction/add/get/shutdown is not claimed; its generic functions
  are registered templates, not usable typed-queue bindings.
- Iterator parameters remain unsupported, including `slices.sorted`; iterator
  results such as `maps.keys` materialize Lists. Scalar-map-key and result-arity
  restrictions remain. `slices.sort` changes only its boundary copy.
- Generic calls and callbacks are IO in both directions. Conversion and calls
  run after cancellation checks inside the action. Context/error/optional-result
  conventions come from the original declaration: substituting a type parameter
  with Bool does not turn a tuple into Option.
- The package key includes generic metadata schema version 1. Concrete wrappers
  occupy separate, atomically published per-tuple companion entries beneath the
  base cache entry, rather than one program-set entry. Their keys include the
  exact Go tuple, effect policy, and foreign argument-owner export fingerprints;
  the base key supplies compiler, toolchain, package export, options, and shim
  identity. Adding, removing, repeating, or reordering calls leaves the package
  template bytes unchanged. Only used tuples are imported by generated programs.

### Post-change binding surface

The five baseline commands were repeated in the isolated module
`/tmp/go4t-generics-impl-4hwcrk`, requiring apimachinery/client-go v0.34.0.
Each exited 0. Kubernetes commands used `GOFLAGS=-mod=mod`.

| Package | All skips before → after | Generic skips before → after | Templates | Extern declarations after |
| --- | ---: | ---: | ---: | ---: |
| slices | 40 → 0 | 40 → 0 | 40 | 40 |
| maps | 10 → 0 | 10 → 0 | 10 | 10 |
| sync | 3 → 1 | 2 → 0 | 2 | 48 |
| k8s.io/apimachinery/pkg/util/sets | 25 → 16 | 25 → 16 | 9 | 95 |
| k8s.io/client-go/util/workqueue | 90 → 76 | 83 → 69 | 14 | 76 |

All 75 generic package functions are registered; all 85 generic receiver methods
remain skipped. This is a template count, not 75 successfully instantiated APIs.
Workqueue still has 69 generic-method skips and 7 other skips; its 14 generic
function templates do not provide instantiated named queue types.
