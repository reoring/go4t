# Isolated client-go behavioral oracle

This is an executable, separate Go module used only for differential evidence. It intentionally imports original `k8s.io/client-go v0.37.2`. It is **not** a dependency or implementation adapter for the native production port. No parent module or production manifest is changed. `go.mod`/`go.sum` pin its independent dependency graph; startup rejects a different or replaced client-go dependency using Go build information.

Upstream revision metadata: `72c52dd0cc3b9099b36af11290b0304045dd1ada` (v0.37.2). The executable validates the module version, not a git checkout; the revision is the pinned release provenance, not a runtime git measurement.

## Capture

From this directory, the exact initial capture command was:

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go run -mod=mod . -output expected.json
```

It exited successfully after executing every scenario and wrote `expected.json`. It completed in 16.79 seconds including lock admission/build/download. This is an original-client-go trace, not verification of the native port. No suites, formatter, or linter were run.

For a subsequent capture with the committed dependency graph, the equivalent read-only-module command is:

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go run -mod=readonly . -output expected.json
```

Only the first command was exercised for this capture. `-output` is optional; JSON is always written to stdout. Errors go to stderr and produce exit status 1. The output file is written only after all scenarios succeed. The harness asserts fixture identity/presence prerequisites, expected HTTP method/path and exchange count, API success/errors, and fake/builder-specific invariants. It does not read `expected.json`, and it does not emit hardcoded expected request/action traces.

The HTTP server is a deterministic transport fixture returning known API objects/statuses; the **original typed client** builds and serializes every observed request, deserializes responses, and constructs API errors. Original fake clientsets and reactors execute the non-HTTP scenarios. A local HTTP fixture is not an API-server semantic emulator and supplies no native-port parity claim.

## Trace schema and exact normalization

The envelope has `schema_version: 1`, `upstream_version`, `upstream_revision`, and ordered `scenarios`. Each scenario is `{id, events:[{kind, fields:object}]}`. Scenario/event order is execution order. Map keys are deterministically encoded by Go's `encoding/json`. Snapshots use `json.Decoder.UseNumber`, so integers are not converted to floating point. Object observations are frozen immediately; later reactor/builder mutation cannot change an earlier event.

**No captured header, body, error message, API status, or field value is normalized or substituted.** In particular:

- HTTP events capture original method, escaped request path, raw query, parsed multi-value query, all headers exposed in the server's `Request.Header`, and the exact request-body bytes as a JSON string. Newlines in serialized bodies are retained.
- Host/ephemeral listening port, connection addresses, TLS/connection state and response headers (including automatic HTTP Date) are not fields of the request-event schema. They are not rewritten to invented fixed values. No random/time header is present in the captured request headers, so the allowed header-normalization rule needs no active normalization.
- User-Agent and JSON negotiation are explicit, deterministic client fixture configuration, not post-capture header rewrites. HTTP client timeout is a fixed 10 seconds and its observed `timeout=10s` query remains captured; the list/delete-collection fixture overrides this with 21 seconds.
- Typed result and fake action object observations contain their complete JSON representation. Fake action events also record concrete Go action type, verb, GVR, namespace, subresource and typed options; ListAction includes GVK and both selectors.
- `managed_object` is explicitly a **projection**, not a full ConfigMap observation: it records Data and, per ManagedFields entry, Manager, Operation, APIVersion, FieldsType, FieldsV1 and Subresource. ManagedFields.Time and other ConfigMap metadata are outside that event's schema. No recorded timestamp is zeroed or replaced, and ownership list order is preserved. This keeps the evidence focused on the requested ownership/conflict behavior without treating wall-clock timestamps as deterministic outputs.
- API errors retain concrete Go type, observed message, complete `metav1.Status` including code/reason/details/causes, and cancellation/deadline predicates. Messages and details are not collapsed into text-only placeholders.

## Executed scenarios and source anchors

All upstream-relative anchors below refer to the v0.37.2 checkout documented in `local://clientgo-generation-evidence.md`.

| Scenario ID | Actual original API calls and observed scope | Source/oracle anchors |
|---|---|---|
| `typed_pod_crud_status_collection_patch` | Namespaced Pod Create/Get/List/Update/UpdateStatus/Patch/Delete/DeleteCollection, then 404 Get. Captures all 9 HTTP requests plus decoded results/errors. Delete preconditions/dry-run body, list selectors/timeout/pagination and patch bytes are retained. | `gentype/type.go:157-292`; `kubernetes/typed/core/v1/pod.go:40-55`; timeout fixture precedent `kubernetes_test/timeout_test.go:35-59` |
| `typed_resize_explicit_parent_name` | Pod UpdateResize with parent name and client namespace deliberately different from body metadata. | `kubernetes/typed/core/v1/pod.go:96-109` |
| `apply_scale_without_configuration_name` | Deployment ApplyScale with neither configuration metadata nor name. Captures successful PATCH to explicit parent's scale subresource, apply content type and force/manager query. | `kubernetes/typed/apps/v1/deployment.go:118-138` |
| `real_and_fake_bind_namespace` | Original real Bind through HTTP versus original bare fake Clientset Bind. The real request uses client namespace; fake action uses binding namespace and does not retain the supplied CreateOptions. | real `kubernetes/typed/core/v1/pod_expansion.go:48-50`; fake `.../fake/fake_pod_expansion.go:37-46` |
| `fake_namespace_finalize` | Original bare fake Namespace Finalize records create/finalize with empty CreateOptions despite caller's UpdateOptions. | `kubernetes/typed/core/v1/fake/fake_namespace_expansion.go:27-39` |
| `fake_list_label_not_field_filter` | Original simple tracker with same-namespace label-match/field-match disagreement plus another-namespace label match. The returned Pod matches label, not field selector; action retains both selectors. | `gentype/fake.go:154-181`; `testing/fixture.go` tracker |
| `fake_action_snapshot_and_reactor_mutation` | Two original fake reactors: first mutates SelfLink, second observes mutation. Recorded action and caller's input remain unchanged; result includes mutation. | `testing/fake_test.go:28-166`, TestOriginalObjectCaptured and TestReactorChangesPersisted |
| `apply_builder_presence_merge_append` | Original Pod builder unset versus explicit zero/false/empty string; map overwrite/merge, finalizer append, nested Container append; nil owner reference panic. | `applyconfigurations/core/v1/pod.go:218-311`; applyconfiguration-gen field templates |
| `field_managed_clientset_conflicts` | Original field-managed Clientset create, two managers, structured conflict, owned-field update, force takeover/removal of omitted field. Data and field ownership projection recorded at every successful stage. | `kubernetes/fake/clientset_generated_test.go:66-134`, TestManagedFieldClientset |

The trace contains 9 scenarios and 43 events. Observed errors include `pods "missing" not found` with Status code 404/NotFound and `Apply failed with 1 conflict: conflict with "manager-1": .data.k1` with code 409/Conflict and FieldManagerConflict cause. These strings come from the executed original APIs, not fabricated oracle outputs.

## Deliberate evidence limits

This capture covers the assigned scenarios only. It does not exercise a live Kubernetes API server, native go4t code, watches/informers/listers, CBOR negotiation, protobuf requests, exec/proxy/log streaming, apply extraction, every generated resource family, or a second-run byte comparison. All requests here explicitly negotiate JSON; successful request serialization and fake ownership behavior do not imply server-side validation/defaulting parity. Bare fake clientsets in Bind/Finalize/reactor scenarios intentionally isolate the original action recording path without adding object-tracker/server behavior.
