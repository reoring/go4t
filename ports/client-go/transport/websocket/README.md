# Native websocket upgrade transport

Source authority: client-go v0.37.2
`transport/websocket/roundtripper.go`, revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada` (Apache-2.0).

| Upstream public symbol | Native capability |
| --- | --- |
| `ConnectionHolder` | `ConnectionHolder` with canonical Gorilla connection identity |
| `RoundTripper` | `RoundTripper {tlsConfig,proxier,holder}`, constructed with `newRoundTripper` |
| `RoundTripper.Connection` | `connection(value.holder)` |
| `RoundTripper.DataBufferSize` | `dataBufferSize()` (32768) |
| `RoundTripper.TLSClientConfig` | `tlsClientConfig(value)` |
| `RoundTripper.RoundTrip` | `roundTrip(holder,tls,proxy,request)`; `asTransport(value)` supplies native Transport capability |
| `RoundTripperFor` | `roundTripperFor(rest.Environment,rest.Config)` |
| `Negotiate` | `negotiate(transport,holder,request,protocols)` |

`websocket.gt` owns scheme validation/conversion, offered and selected
subprotocols, handshake failure classification, bounded error response reading,
status-error interpretation, and request/response body lifetimes. The request is
cloned before removing the protocol header or changing the socket scheme.
`http`/`https` are mapped to `ws`/`wss`; other schemes fail. Failed handshakes are
wrapped with the external `httpstream.UpgradeFailureError` type, including a
maximum 64 KiB response body and decoded Kubernetes Status where possible.
`upgradeFailureCause` exposes that upstream error's Cause field without falsely
claiming that the external type implements Unwrap.

The host conversion of offered protocols passes `Some(protocols)` to the
canonical slice converter. A bare native list denotes neither a present nor an
empty Go slice in that ABI and must never be used for this conversion.

The selected subprotocol must occur in the offered list. Otherwise the socket is
closed before returning the upgrade failure. A failed response-body close also
closes the negotiated socket. Native Transport wrappers run on the same
canonical request/response/socket identities. This aggregate is single-session,
as is the pinned upstream transport; callers must not concurrently reuse it.

`host/effects.go` contains Gorilla dial/frame effects, deadlines, external status
codec conversion and external error constructors only. It does not implement a
remotecommand protocol or portforward controller. `writeFrame` preserves partial
write counts and writer-close/flush errors; a zero deadline leaves an explicitly
configured socket write deadline untouched.

## Focused verification

`websocket_test.gt` uses real HTTP upgrade servers to cover successful protocol
selection, rejected/non-offered protocols, HTTP 403 upgrade classification and
structured Status causes. Additional real websocket channel streaming is in
`tools/remotecommand/loopback.runWebsocket`.

The independent `ClientGoStreamingVerify` worker ran
`go run ./cmd/go4t test ports/client-go/transport/websocket` under the serialized
VM test wrapper: **3 passed, 0 failed**, exit 0. It also observed successful
real websocket negotiation for each v1-v5 protocol, a successful real HTTP 403
websocket-to-SPDY fallback, and the published remotecommand loopback returning
true after the final cancellation and byte-boundary fixture changes.

The successful-negotiation regression caught a missing Option wrapper in the
host slice conversion; the correction preserves the actual offered protocols
rather than weakening protocol validation. The implementation owner inspected
the verifier's evidence but did not execute checks.
