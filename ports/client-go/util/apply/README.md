# Apply request policy

Port of pinned `util/apply/apply.go` v0.37.2. `newRequest[A]` accepts a REST interface, native typed configuration, typed `Marshal[A]` capability, and the shared feature registry. It returns `IO[Outcome[Option[rest.Request]]]`.

JSON bytes intentionally use `application/apply-patch+yaml`. Only when **both** `ClientsAllowCBOR` and `ClientsPreferCBOR` are enabled does the policy choose the CBOR encoder and `application/apply-patch+cbor`. All four combinations are retained in `apply_test.gt`.

Encoder failures preserve the original cause under `failed to marshal apply configuration: <cause>` and do not allocate a request. The effectful REST `patch` constructor runs only after successful encoding. Typed codecs keep the native apply-configuration type; there is no dyn-value conversion or client-go delegation.

No tests or builds were run by this implementation worker; acceptance belongs to Main.
