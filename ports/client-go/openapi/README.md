# Native OpenAPI clients

Source authority: Kubernetes client-go v0.37.2, revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. `provenance.json` records every
owned upstream source, declaration, and fixture hash. Kubernetes-derived code
is Apache-2.0. The independent forked HTTP cache retains its own LICENSE.

## Public capability mapping

| Upstream | Native implementation |
| --- | --- |
| Client, ClientWithContext; Paths, PathsWithContext | `Client.paths` / `paths(rest.Interface)` |
| NewClient, NewClientWithContext | `newClient(rest.Interface)` |
| ToClientWithContext | Not needed: all native calls use the implicit IO context, including cancellation |
| GroupVersion, GroupVersionWithContext | `GroupVersion{schema,serverRelativeURL}` |
| Schema, SchemaWithContext; ServerRelativeURL | `schema: func(String) IO[Outcome[String]]`; `serverRelativeURL: func() IO[String]` |
| ToGroupVersionWithContext | Not needed: the single native capability always carries contextual IO |
| ContentTypeOpenAPIV3PB | `contentTypeOpenAPIV3PB()` |
| NewTypeConverter | `newTypeConverter(Client,preserveUnknownFields)` |
| cached.Client/NewClient/NewClientWithContext | `cached.newClient(Client) IO[Client]` |
| cached.Paths/PathsWithContext | Returned client's `paths` callback |
| cached groupversion.Schema/SchemaWithContext | `cached.newGroupVersion`, keyed by content type |
| openapitest.FakeClient/NewFakeClient/Paths | `FakeClient`, `newFakeClient`, `fakeClient` |
| openapitest.FakeGroupVersion/Schema | `FakeGroupVersion`, `fakeGroupVersion` |
| openapitest.NewFileClient/NewEmbeddedFileClient | `newFileClient`, `newEmbeddedFileClient` |
| fileGroupVersion.Schema/ServerRelativeURL | Returned `GroupVersion` callbacks; URL callback returns the exact filename |

All fallible calls preserve `support/outcome.Outcome` and canonical errors.
Byte strings are exact bytes, not interpreted schema trees. `decodeV2` returns
a canonical gnostic OpenAPI v2 Document; `decodeV3` returns a canonical
kube-openapi spec3.OpenAPI. The type converter merges complete canonical
`spec.Schema` objects in native code, then calls the external managedfields
constructor. The structured-merge-diff type converter identity is shared with
Apply/Fake, not recreated as a JSON-only model. External document handles are
treated as read-only after decode; native retrieval/cache operations never
mutate schema objects.

Discovery removes the client's root path once. `/openapi/v3` URLs use the
AbsPath/Param branch; other returned URLs use RequestURI. Both preserve the
REST client's base path. RequestURI replaces the previous request prefix
and query, as pinned `rest/request.go:393-405` and
`rest/request_test.go:306-314` require. Query values, including hash, are
retained. Accept is exactly the requested content type. V2 requests use the
upstream protobuf media type and `/openapi/v2` path.

Cached clients serialize first retrieval and cache both success and failure.
Each group serializes access and caches both bytes and errors per content
type. Context cancellation remains an IO failure, not a cached synthetic
schema error. A cached client intentionally does not refresh discovery;
construct a new one to observe a new root. Uncached root operations fetch
paths every time, so changed hashes select new HTTP cache keys.

`serverRelativeURL()` is a lazy IO capability, independent of schema retrieval.
Real providers return the exact discovered URL after the existing client-root
prefix removal; file providers return the exact schema filename. Cached groups
forward the callback without invoking it during construction, discovery, or
schema access, and without memoizing its result or failure.

## Fixtures and regression sources

`openapitest/testdata` contains all five shipped JSON files, byte-for-byte,
without replacing them with smaller schemas. `provenance.json` records their
SHA-256 hashes. The embedded filesystem serves those same bytes. The fixture
client preserves filename-to-path transformation and JSON-only media errors.
The native fake preserves the pinned `fakeclient.go:81-84` behavior:
construction and JSON schema access succeed, but `serverRelativeURL()` raises
`io.Panic("unimplemented")` at access time. It never returns an empty fallback.

`client_test.gfx` covers outbound prefix/query/media behavior through a real
REST request executor with a callback HTTP transport. `openapitest/client_test.gfx`
proves delayed fake URL panic and exact file/embedded filenames; run file tests
from `ports/client-go` so `openapi/openapitest/testdata` resolves directly.
Cached tests cover once-only discovery, per-media error caching, lazy URL
forwarding on every access, and fake/file consumers. `openapi3/root_test.gfx`
covers path parsing, root refresh, ordering, canonical schema parsing,
conversion using shipped schemas, and typed not-found errors. HTTP cache
regressions cover stale/fresh precedence, validators, 304 replacement, EOF
commit, only-if-cached, Vary, no-store, and hash-separated cache keys.

Verification is supervisor-owned. The syntax verifier reported a passing
parse of `httpcache/cache.gfx` after its alias and missing-brace corrections.
The behavioral verifier subsequently reported extern ABI failures before
tests executed. Owned host bridges now use the fully erased all-`any` ABI
for compound returns, per SPEC §8.3; mixed primitive/erased signatures are
not supported. Error bridges use the canonical `ErrorFromGo`,
`ErrorOptionFromGo`, and `ErrorToGo` helpers. The independent verifier then
reported HTTP cache **7 passed, 0 failed** (`artifact://654`). OpenAPI's
prefix/hash test passed; an incorrectly authored bypass-prefix test failed
(`artifact://656`). Direct inspection of the pinned REST source/test showed
that preserving the base path is intentional. With Main's approval, the
test now requires `/proxy/elsewhere/schema?hash=2` and verifies replacement
of a seeded prefix and query. Production routing was not changed. The
corrected failed-only test awaits independent verification.

Final capability reconciliation validation used the current-worktree compiler,
with each command serialized in the short class from `ports/client-go`:
`go run ../../cmd/gofx test openapi` passed 2 tests;
`go run ../../cmd/gofx test openapi/openapitest` passed 2 tests;
`go run ../../cmd/gofx test openapi/cached` passed 4 tests.
An independent public consumer passed four additional proofs: fake schema
before/after the exact `io.Panic("unimplemented")`, cached embedded-file schema,
cached disk-file schema, and matching real/cached URL access. It called public
capabilities directly, not existing test functions. Its source and generated
output were archived outside the repository and removed after execution.
An earlier run stopped before executing tests on the shared ClientConfig
`FormatBytes` extern ABI mismatch; the owning worker corrected that boundary
before the successful runs. Existing warning output was not suppressed.
