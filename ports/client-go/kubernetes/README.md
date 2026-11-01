# Native Kubernetes typed clients

This directory ports the pinned `k8s.io/client-go/kubernetes` tree at
`v0.37.2`, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`.
The request and fake-action algorithms are native gofx. Production code does
not import or delegate to the original client-go package.

## Generated scope and provenance

- 162 typed resources across all 55 pinned group/version clients, with matching
  fake resource and group clients.
- An explicit clientset assembles all 55 groups and discovery in upstream order.
- `scheme.Registry` replaces upstream package globals. It contains an initialized
  canonical runtime Scheme, codecs, parameter codec, negotiated serializer and
  universal decoder. All 55 external API registration callbacks are sequenced
  explicitly in gofx. The small host file only adapts external API registration,
  constructors, codec interfaces and field selectors.
- `coverage.json` maps all 1,537 exported production declarations to native
  capabilities and records provenance for all 634 upstream files (631 Go files
  and three ownership metadata files). This is a source map, not proof of tests.
- The shared `tools/portgen/resources.json` contains 163 records: the 162 typed
  resources plus the lister-only ImageReview. `typed_client` distinguishes them.
  It includes exact external object/list identities, native binding references,
  apply types, scope, verbs, serializer preference, generator markers, original
  signatures, extension request policies, fake action provenance and source
  hashes. It is extracted from pinned sources, never discovery responses.

Regenerate from the repository root with:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 tools/portgen/TypedClients/regenerate.py
```

The script first emits the shared resource descriptor, then real clients,
scheme registrations, fake clients, clientsets and exported-symbol coverage.
It reads the pinned source checkout at `../../.tmp/client-go-v0.37.2` relative
to this worktree root, canonical dependency bindings and the upstream inventory.
The generator runs the standard-library-only `route_literals.go` helper to
extract fluent `AbsPath` and `Resource` overrides using the Go AST. Formatting
cannot alter those route overrides; conflicting literals fail generation.
No product compiler, linter, formatter, product build or test is run by generation.

For isolated byte comparison, pass `--output-root /absolute/shadow-root`.
This writes the generated `ports/client-go/kubernetes` tree and
`tools/portgen/TypedClients/groups.json` under that root without replacing
live outputs. It reuses the current source repository's resource descriptor,
dependency bindings and upstream inventory; it does not re-extract resources.

## Native surface

`kubernetes.Interface` (also `Clientset`) is a typed capability record. For example:

```gofx
let pods = client.coreV1().pods("team-a")
result <- pods.get("pod-a",meta.newGetOptions())
```

`newForConfig(environment,config)` and
`newForConfigAndClient(environment,config,httpClient)` return
`IO[Outcome[Option[Clientset]]]`. All group clients share the exact supplied
canonical `net/http.Client`; its timeout, redirect callback, cookie jar and
transport identity are retained. One scheme registry is shared by clientset
construction. A positive QPS without a rate limiter requires a positive burst;
otherwise construction returns the upstream error:

`burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0`

`newClientset(environment,registry,restInterface)` provides dependency injection.
Every group also exports `newClient`, `newForConfig`,
`newForConfigAndClient`, `newForConfigAndClientWithRegistry` and
`newForConfigOrDie`. Group `restClient` is `Some` for real clients and `None`
for fake clients. `isWatchListSemanticsUnSupported` is false on a real clientset
and true on a fake one; informer feature registries remain explicit at their
own construction boundary.

Resource records expose the upstream methods as lowerCamel callbacks. Ordinary
get/create/update/status/patch operations return `IO[Outcome[APIObject]]`;
list returns the exact canonical list type; delete returns `Outcome[Unit]`;
watch uses the shared `support/watch.Watch`. Apply accepts an optional native
apply configuration and returns an optional object, preserving validation
failures before requests. Adapters reuse canonical ObjectType descriptors
without creating empty witnesses for conversion. List setters preserve nil
items when fake label filtering produces no matches.

## Expansion distinctions

All generated subresource methods and handwritten expansions are retained:
Pod binding, versioned evictions, logs and proxy; service proxy; namespace
finalization; node status patch; event namespace operations, selectors, search
and sink; deployment rollback; scale, approval, ephemeral containers, resize
and token requests. Input and result types are independent, including versioned
Scale objects. Explicit parent names remain independent of body metadata.
Rollback reads the canonical `DeploymentRollback.Name` field directly, not
`ObjectMeta`: the pinned extensions/v1beta1 expansion supplies that name to the
request path. The pinned apps/v1beta1 `DeploymentExpansion` is empty.
Both policy eviction versions POST to the absolute `/api/v1` pods route, with
the input eviction's namespace and name, not the policy client's API base or
the `evictions` resource.

Important real/fake differences follow their original sources rather than
forcing the fake to resemble HTTP:

- Real Pod Bind uses the client namespace. Fake Bind records the binding body
  namespace and ignores the create options, as upstream does.
- Real Resize uses its explicit parent name. Fake Resize records the body object
  and its update options, without promoting the explicit name into the action.
- Fake namespace Finalize records a **create** action with subresource `finalize`
  and ignores the update options; real Finalize is a PUT.
- ApplyScale accepts a configuration with no metadata/name and uses its explicit
  parent name. Ordinary Apply retains its name requirement.
- Real event namespace operations reject mismatches before sending a request;
  fake event operations preserve their different namespace selection policy.
  These expansion results are optional, including early failures and nil reactor
  objects. Core `WithContext` names use the native implicit IO context.
- Fake lists apply label filtering, not field filtering. Native generic fake
  algorithms own the action-copy and reactor-sharing rules.
- Fake log requests record the typed PodLogOptions payload, return `fake logs`
  by default, validate reactor results as runtime.Unknown, and defer reactor
  errors until execution. Each request execution receives a fresh response body.
  Reactors retrieve the canonical options object with
  `testing.getValue(action,testing.runtimeObjectPayloadKey())`, then use the
  canonical PodLogOptions descriptor to view it. The public immutable key is
  shared by producers and reactors; no private per-Pod key is required.
- Proxy callbacks return optional `rest.ResponseWrapper`, retaining the absence
  of a fake proxy reactor.

## Fake clientsets

`kubernetes/fake.Clientset` holds `client:kubernetes.Interface`,
`fake:testing.Fake` and `tracker:Option[testing.ObjectTracker]`.
`newEmptyClientset(environment)` has no tracker or default reactors.
`newSimpleClientset(environment,objects)` installs the simple tracker and
object/watch reactors. `newClientset(environment,objects)` installs the
field-managed tracker with the native apply type converter. Both populated
constructors propagate object-add failures as IO failures, matching upstream's
panic-on-invalid-initial-object behavior.

## Regression and acceptance status

`regression/typed_test.gfx` retains focused consumer-level checks for Bind,
Finalize, Resize, nameless ApplyScale, deferred fake-log errors, real explicit
parent paths and event namespace errors. The separate oracle contains nine
scenarios spanning these operations, fake action mutation, apply builders and
managed-field conflicts; the parity/examples owner owns their native comparison
programs.

`regression/family/main.gfx` is generated by
`tools/portgen/TypedClients/family.py` and executes 162 real-client operations
across all 55 groups. Its observer records actual returned GVK and metadata;
it does not require response-fixture GVK to survive decoding. Exact GVK and
request parity are compared against the immutable original-Go family rows by
the verification owner, not inferred from descriptors or fixture contents.

Generation has been executed. No parser check, typecheck, product build,
formatter, linter, test or oracle comparison has been run by this owner. The
source/provenance counts above must not be interpreted as integrated acceptance.
The supervisor must run the focused native regressions and nine oracle
comparisons after all sibling dependency contracts have landed.
