# Canonical partial outcome

`Outcome[A] { value: A, error: Option[deps/support.Error] }` preserves a value even when a domain operation fails. It is not an exclusive Either. Pointer-like absence belongs in `A` as `Option[T]`; no-value operations use `Unit`.

`success`, `failure`, and `from` construct values. `value`, `error`, `isSuccess`, and `mapValue` inspect or transform them without erasing the original error. `mapValue` intentionally also maps partial values on failure.

This leaf imports only the canonical error dependency. Network cancellation, deadline, and runtime failures remain in `IO`; domain errors live in Outcome. It imports no REST client, transport, kubeconfig, or other concrete client domain.

No verification was run by the implementation worker. Consumers must share this declaration rather than create nominally distinct outcome records.
