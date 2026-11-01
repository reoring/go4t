# Native client-go for gofx

This is an experimental native gofx rewrite of client-go **v0.37.2**, pinned to
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. Native APIs use explicit environments,
immutable request builders, typed external Kubernetes objects, `IO` effects, and
`support/outcome.Outcome` for results that can contain both a value and an error.
It is not a Go-source-compatible replacement.

**Status: ACCEPTED EXPERIMENTAL NATIVE PORT against the recorded scopes below.**
The primary has accepted integration; this is not production certification or a
claim that every original Go test, generated codec or platform matrix was run.
Final source reconciliation accounts for all 17,033 pinned production exports,
all 388 source directories and all 401 package records assigned to 22 owners,
with no unmatched production keys or missing referenced native files. This is
source accounting, not proof of every branch or upstream behavior.
`ownership.json` separates the inventory baseline from delivery:
`native-source-present` records implementation presence; `integration-accepted`
records primary acceptance of the documented evidence, not universal verification.
`DESIGN.md` remains the authority.

Observed evidence includes a strict differential pass for 162 typed operations
across 55 groups and all nine post-cutover oracle scenarios: every expected field
and ordered event matched the immutable original-client-go oracle. Deterministic
shadow regeneration was byte-identical for 116 typed, 220 informer, 301 lister,
110 apply and 233 dependency files. The generated surface includes 952 apply
records / 3,603 fields, 147 informer families and 150 lister families; reconciliation
also records 75 manual host adapters. These scopes are not a count of original Go
tests executed and must not be summed with repeated fixture or consumer runs.

A development-only Kubernetes v1.37.0 Kind proof recorded 29 assertions and
confirmed namespace cleanup; the owned cluster was deleted afterward. This is bounded runtime
evidence contributing to experimental acceptance, not production approval.

## Native composition

`examples/environment.gfx` shows the actual composition:

- one explicit feature registry with an isolated feature environment;
- `transport.newEnvironment`, a real clock, authentication provider registry,
  exec cache/environment, and token-bucket rate limiter;
- `rest.newEnvironment(...)` and typed `kubernetes.newForConfig(environment, config)`;
- `client.coreV1().pods(namespace).list(options)` and analogous generated
  resource capabilities, retaining canonical dependency object types.

The example environment does not load a user's kubeconfig, service account, or
process feature overrides. The in-cluster workflow uses injectable
`rest.inClusterConfigWith`; the out-of-cluster workflow feeds controlled
kubeconfig bytes to `clientcmd.restConfigFromKubeConfig`. The resolved in-cluster
file references are asserted before being replaced with the explicitly isolated
loopback endpoint and inline token for the HTTP exercise.

## Shipped workflows

From the **worktree root**, invoke the current compiler source rather than an
unrelated installed `gofx`:

```sh
timeout 180s go run ./cmd/gofx run ports/client-go/examples/in-cluster-client-configuration/main.gfx
timeout 180s go run ./cmd/gofx run ports/client-go/examples/out-of-cluster-client-configuration/main.gfx
timeout 180s go run ./cmd/gofx run ports/client-go/examples/create-update-delete-deployment/main.gfx
timeout 180s go run ./cmd/gofx run ports/client-go/examples/dynamic-create-update-delete-deployment/main.gfx
timeout 180s go run ./cmd/gofx run ports/client-go/examples/fake-client/main.gfx
timeout 180s go run ./cmd/gofx run ports/client-go/examples/workqueue/main.gfx
timeout 180s go run ./cmd/gofx run ports/client-go/examples/leader-election/main.gfx
```

The primary-supervised terminal run observed exit 0 for all seven actual shipped
CLIs, approximately 80–94 seconds each. These are scoped workflow passes, not
full-port acceptance. Full native graphs have required approximately 14 GB of
memory. Run each command with a hard-exclusive execution slot, serialized against
other compiler/build/test commands, and a 180-second deadline. Do not disable
optimizations, delete tests, or substitute a reduced graph to obtain acceptance.

These are runnable controlled examples, not commands for a real cluster. HTTP
servers bind loopback; they assert routes and collect actual native requests.
Typed and dynamic deployment workflows create, refetch/retry conflicts, update,
list, and delete with foreground propagation. Dynamic updates retain unrelated
unstructured fields. The fake-client workflow uses a native fake tracker and
shared informer, waiting for watch establishment before creating a Pod. The
controller exercises HTTP list/watch, the native informer cache, a rate-limiting
workqueue, and cancellation/joins. Leader election writes and releases a Lease
through the native typed client against a local protocol fixture.

The seven upstream root documentation examples map to functions in
`example_test.gfx`: `exampleInClusterConfiguration`,
`exampleOutOfClusterConfiguration`, `exampleHandlingAPIErrors`,
`exampleUsingDynamicClient`, `exampleUsingInformers`, `exampleServerSideApply`,
and `exampleLeaderElection`. They run controlled scenarios instead of inspecting
local credentials or waiting indefinitely for OS signals.

## Controlled integration acceptance

The primary reports four retained `util/testing` passes and a passing root
documentation run (91 seconds), accepted with the recorded integration evidence.
Reproduction commands follow; use the worktree root and the same hard-exclusive,
serialized 180-second bounds:

```sh
timeout 180s go run ./cmd/gofx test ports/client-go/util/testing
timeout 180s go run ./cmd/gofx test ports/client-go/kubernetes_test
timeout 180s go run ./cmd/gofx test ports/client-go/example_test.gfx
timeout 180s go run ./cmd/gofx run ports/client-go/examples/integration/main.gfx
```

The integration entrypoint performs actual config/clientset HTTP calls, typed
and dynamic CRUD, SSA, fake tracker/watch/informer delivery, cache/queue
processing, Lease election/release, User-Agent checks, list/watch timeout
encoding, structured NotFound, in-flight HTTP cancellation, fake/real GVK
behavior, and the Streaming owner's socket-backed remotecommand and port-forward
fixtures. Every stage asserts observed results before printing success.
`util/testing` includes native HTTP handler validation/count/skip/lifecycle tests,
a temporary-directory cleanup test, and an external kube-openapi V3 service with
native request counters and controlled schema input.

Programs impose bounded contexts and use bracket/ensuring cleanup for owned
servers and tasks. The primary-supervised failure-cleanup consumer exited 0,
checking the original assertion, cancellation/join completion and listener
closure. This is scoped cleanup evidence, not proof of every failure branch.

## Full differential trace

The native trace program runs all nine scenarios in the same order and schema as
`oracle/expected.json`. It never reads the expected file and never invokes the
original client-go. Runtime JSON comes from actual returned objects and errors;
HTTP request events come from local server observations, fake actions from the
native fake action ledger, and managed-field ownership from returned ConfigMaps.
Only upstream's documented managed-field timestamp exclusion is applied.

To reproduce the observed trace from the worktree root, using the same exclusive
execution rule:

```sh
timeout 180s go run ./cmd/gofx run ports/client-go/examples/parity/main.gfx > /tmp/clientgo-native-trace.json
timeout 180s go run ./tools/portgen/ParityExamples/compare.go \
  ports/client-go/oracle/expected.json /tmp/clientgo-native-trace.json
```

The comparator checks every field, type, array length, scenario, and ordered
event, reports all mismatches, preserves JSON integer precision, rejects trailing
JSON, and exits nonzero on any difference. It does not normalize away headers,
queries, action options, GVKs, error messages, or managed-field ownership. All nine
scenarios must pass together; a successful subset is not full parity.

The nine scenarios cover typed Pod CRUD/status/patch/delete collection/error,
explicit-parent resize, unnamed Scale apply, real/fake Bind namespace behavior,
fake Namespace Finalize, label-versus-field fake list selection, shared reactor
mutation versus recorded snapshots, builder presence/map merge/list append/nil
panic, and field-managed clientset conflicts/forced ownership/omission.

## Scope and provenance

Examples, root documentation examples, `kubernetes_test`, and `util/testing` are
ported from the pinned upstream source/tests and retain Apache-2.0 attribution.
Public host paths include `host/sync`, `host/deps`, `examples/host` and
`util/testing/host`; the old internal host paths are not compatibility APIs.
Host adapters provide generic synchronization, loopback, file, URL, external
object-codec and kube-openapi service effects. Client-go request, controller,
fake, election and apply behavior belongs to native APIs, not delegated
original-client-go calls.

## Final integration evidence and limits

The primary reports these final gates passed:

- The production Go dependency graph contained 515 packages from 36 source-host
  roots, with no errors, incomplete packages or original `k8s.io/client-go`
  dependency. This source-host graph is distinct from emitted consumer graphs.
- From this port directory,
  `go test -json -p2 -timeout120s -mod=readonly -count=1 ./...` exited 0:
  51 package terminals (5 passed, 46 with no tests), with 51 test passes.
- From the worktree root,
  `GOWORK=off serialize --exclusive -- go test -p2 -timeout150s ./...` exited 0
  in 82.61 seconds: all 21 packages reached terminal results (15 test packages
  passed, 6 with no tests). `cmd/gofx` took 81.474 seconds and `codegen` 24.793
  seconds; these package timings are not additive.

Before the final port Go check, nine archived, owned duplicate CLI emission
directories were removed. Real source and tests were unchanged. The final checks
used the complete retained source/test graphs, not optimization-disable flags or
test reduction.

These results, the seven shipped CLI passes, root documentation and cleanup
evidence, strict differential comparison, byte-identical regeneration and
development Kind proof support the primary's experimental integration acceptance.
They do not establish every original upstream Go test, every codec, every
failure branch or a platform matrix, and do not certify production use. Counts
from different scopes or repeated runs must not be summed as unique coverage.
Domain READMEs may retain historical implementation-owner snapshots.

The accepted source integration is committed as `5c0eb5a` on
`experiment/client-go-gofx`; compiler fixes are separate preceding commits.
