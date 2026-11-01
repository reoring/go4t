# Original client-go typed-family oracle

This executable belongs to the existing isolated oracle Go module. It intentionally executes original `k8s.io/client-go v0.37.2`, not the native port. It changes neither the parent nine-scenario oracle nor the production implementation.

## Executed capture

Working directory: `ports/client-go/oracle`.

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --class short -- go run -mod=readonly ./families -cases ../kubernetes/regression/family-cases.json -output families/expected.ndjson
```

Observed successful completion in 2.67 seconds:

```text
family oracle: 162 actual operations, 55 groups, 147 Get / 13 Create / 2 Evict; schema_version=1
```

`expected.ndjson` is the actual captured stdout, one row for every operation, sorted by qualified concrete Go resource type. The executable does not read that file. `-output` writes a capture only after every operation succeeds; JSON rows also go to stdout, with the count diagnostic on stderr. Failures return nonzero without publishing a partial capture.

## Coverage and provenance guards

- Startup requires Go build information identifying un-replaced client-go v0.37.2. The manifest must identify revision `72c52dd0cc3b9099b36af11290b0304045dd1ada` and schema version 1.
- Before running, the executable independently reflects the actual upstream `kubernetes.Interface` and all of its group/resource interfaces. It excludes only the discovery accessor and each group's RESTClient accessor. It compares the resulting 162 resources / 55 groups with the case manifest, rejecting missing, extra or duplicate entries.
- Method selection reflects each actual resource interface, choosing Get, then List, then Create, then Evict. The fixed manifest contains 147 Get, 13 Create, 2 Evict and no List selections. Any different count or manifest-selected method fails.
- Each case's expected concrete object pointer type comes from the upstream public method signature, not the manifest's response. Its fully qualified name must match the resource entry. Returned values must implement `runtime.Object` and have exactly that concrete pointer type, checked with reflection before observing metadata and GVK.
- The case file supplies only controlled input/config/HTTP response fixtures and accessor selection. Real `kubernetes.NewForConfigAndClient` creates the clientset; reflection invokes its real public methods. A supplied HTTP client with the capture RoundTripper prevents DNS/network access. Requests outside `http://family.invalid`, duplicate requests, invalid JSON bodies and missing requests fail the capture.
- Create/Evict objects are newly allocated values of the upstream method's concrete argument type and populated from the input fixture. Every HTTP response and body reader is newly allocated. No request, method result or cast result is synthesized from an expected trace row.

## Trace schema v1

The authoritative shared schema is `local://clientgo-family-trace-schema.md`; the case manifest is `ports/client-go/kubernetes/regression/family-cases.json`.

Each NDJSON line has exactly:

```json
{
  "schema_version": 1,
  "resource": "k8s.io/api/core/v1.Pod",
  "operation": "Get",
  "requests": [
    {
      "method": "GET",
      "path": "/api/v1/namespaces/client-namespace/pods/request-name",
      "query": {"resourceVersion": ["fixture-rv"]},
      "body": null,
      "accept": "application/json",
      "content_type": ""
    }
  ],
  "result": {
    "go_type": "k8s.io/api/core/v1.Pod",
    "api_version": "",
    "kind": "",
    "name": "response-name",
    "namespace": "response-namespace",
    "resource_version": "response-rv"
  },
  "cast_ok": true,
  "error": null
}
```

The example is an observed Pod row shape. Get/Create results in this capture have **empty api_version and kind** despite the response fixture containing GVK. This is the original typed decoder's observed behavior, not normalization. Filling these values from a descriptor would falsify the result trace. Both Evict results are null with cast_ok true because their actual API returns only error.

Request path, multi-valued query, parsed body and Accept/Content-Type come directly from the actual RoundTripper request. A bodyless request records null; empty query records `{}`. JSON parsing uses `json.Number`, preserving exact numeric lexemes without conversion through binary64. No request fields, returned metadata, GVK, or error strings are normalized. Object-key ordering is JSON encoder ordering; row ordering is qualified resource-name ordering. Comparison should parse rows and compare complete values keyed by `resource`, as specified by the shared schema.

Fixed configuration is Host `http://family.invalid`, JSON request/Accept content types, QPS -1, and no timeout query. Namespaced accessors receive `client-namespace`; Get uses `request-name` and resourceVersion `fixture-rv`. Create uses input-name/input-namespace with fieldManager family-fixture, dryRun All, fieldValidation Strict. Evict has the same input metadata but no create options.

Both observed policy Evict calls use `/api/v1/namespaces/input-namespace/pods/input-name/eviction` and empty query, even though the getter namespace is client-namespace. Their body retains the respective policy/v1 or policy/v1beta1 GVK. All 162 operations produced one request and completed successfully.

## Limits

This capture establishes original-library behavior for the shared family fixture only. It does not assert native family parity; `ClientGoGeneratedVerify` owns that comparison after native execution. It does not test all verbs for each resource, live API-server behavior, discovery, fake clients, watches, streaming, or field management. No tests/suites, formatters, or linters were run, and no files outside `oracle/families/` were edited for this assignment.
