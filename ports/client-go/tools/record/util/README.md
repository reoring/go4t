# Event utility policies

Native mapping of client-go v0.37.2 tools/record/util (Apache-2.0):

- `ValidateEventType` → `validateEventType`: only Normal and Warning.
- `IsKeyNotFoundError` → `isKeyNotFoundError`: exact external StatusError with HTTP status code 404, not a message comparison.
- `GenerateEventName` → `generateEventName`: `<reference-name>.<hex-unix-nanoseconds>` when valid under Kubernetes DNS-subdomain validation; otherwise a fresh UUID.

Validation choice and naming fallback remain native. DNS validation and UUID entropy are external effects under the shared event host boundary. No validation command was run for this assignment.
