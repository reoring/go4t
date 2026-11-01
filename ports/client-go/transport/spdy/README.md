# Native SPDY transport

Port of `transport/spdy/spdy.go` from client-go v0.37.2, revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. Kubernetes-derived source remains
Apache-2.0 licensed. HTTP upgrade framing is the external
`k8s.io/streaming/pkg/httpstream/spdy` v0.37.2 implementation, not client-go.
Request construction, protocol offer ordering, response lifetime, REST wrapper
composition, and error policy are in `spdy.gt`.

## Public capability mapping

| Upstream symbol | Native equivalent |
| --- | --- |
| `Upgrader`, `Upgrader.NewConnection` | `Upgrader`, `upgrader.newConnection(response)` |
| `RoundTripperFor` | `roundTripperFor(rest.Environment,rest.Config)` returning `Outcome[Option[Transports]]` |
| `NewDialer`, `NewDialerForStreaming` | `newDialer(upgrader,canonical http.Client,method,address)` |
| `dialer.Dial`, `streamingDialerAdapter.Dial` | `Dialer.dial(protocols)` |
| `NewUpgraderForStreaming`, `compatUpgraderAdapter.NewConnection` | `newUpgraderForStreaming(canonical streaming.UpgradeRoundTripper)` |
| `Negotiate`, `NegotiateStreaming` | `negotiate(upgrader,client,request,protocols)` |
| `streamingStreamAdapter` / `compatStreamAdapter`: `Read`, `Write`, `Close`, `Reset`, `Headers`, `Identifier` | `fromStream`; `Stream.reader.read`, `writer.write`, `writer.close`, `reset`, `headers`, `identifier` |
| `streamingConnectionAdapter` / `compatConnectionAdapter`: `CreateStream`, `Close`, `CloseChan`, `SetIdleTimeout`, `RemoveStreams` | `fromConnection`; `Connection.createStream`, `close`, `awaitClose`, `setIdleTimeout`, `removeStreams` |

The two upstream compatibility families intentionally collapse into one native
canonical streaming family; no obsolete client-go/apimachinery adapter types or
forwarding shims remain. `Stream.identity` retains the canonical external stream
for exact removal. Upgrader handles also use the canonical external interface.

`Reader` and `Writer` are cancellation-aware IO capabilities. A blocking custom
reader must honor IO cancellation or unblock when `close()` is called. The
canonical `fromReader` adapter is suitable for finite in-memory readers and
closable blocking readers; it cannot manufacture interruptibility for arbitrary
foreign `io.Reader` implementations that have no cancellation/close operation.
No detached goroutine is used to conceal such a reader. Reads/writes preserve
partial bytes/counts plus canonical errors. `copy` mirrors `io.Copy` short-write
handling; `writeAll` is an explicit complete-write helper.

The supplied HTTP client is never replaced/copied by `newDialer` or `negotiate`.
REST wrappers are built over the real upgrading transport, preserving wrapped
transport identity. Offered protocols retain order and repeated header values;
response bodies are closed on success and failure after `Client.Do` succeeds.
SPDY keepalive is configured at five seconds.

## Focused verification

`spdy_test.gt` contains real HTTP/SPDY upgrade negotiation and partial-I/O/short
writer regressions. `testsupport/server.go` is a loopback server only.

The independent `ClientGoStreamingVerify` worker ran
`go run ./cmd/go4t test ports/client-go/transport/spdy` under the serialized VM
test wrapper: **3 passed, 0 failed**, exit 0. Its additional real-handshake probe
negotiated every protocol from v1 through v5 over SPDY successfully. Independent
byte probes also passed for UTF-8/NUL one-byte `writeAll` calls, partial-read
errors and short-write counts. Those probe helpers were removed after evidence
capture; the permanent regression sources remain in this module.

These results are focused streaming evidence, not whole-repository or live
Kubernetes acceptance. The implementation owner did not execute checks.
