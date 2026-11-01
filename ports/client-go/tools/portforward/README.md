# Native portforward

Source: all implementation files under client-go v0.37.2
`tools/portforward`, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`,
Apache-2.0. Listening, address grouping, stream creation, copy coordination,
request IDs, stop behavior and tunneling policy are native `.gfx`.

## Public symbol mapping

| Upstream declarations | Native equivalent |
| --- | --- |
| `PortForwardProtocolV1Name` | `portForwardProtocolV1Name()` |
| `ErrLostConnectionToPod` | `errLostConnectionToPod()` with stable tagged `errors.Is` semantics |
| `ForwardedPort` | `ForwardedPort {local,remote}` |
| `PortForwarder` | `PortForwarder` aggregate |
| `New`, `NewForStreaming` | `newPortForwarder(dialer,ports,stop,ready,out,errOut)` |
| `NewOnAddresses`, `NewOnAddressesWithContext`, `NewOnAddressesForStreaming`, `NewOnAddressesForStreamingWithContext` | `newOnAddresses`; explicit stop channel and implicit IO context |
| `PortForwarder.ForwardPorts`, `Close`, `GetPorts` | `forwardPorts`, `close`, `getPorts` |
| `FallbackDialer`, `StreamingFallbackDialer` | canonical `spdy.Dialer` capability |
| `NewFallbackDialer`, `NewFallbackDialerForStreaming`, both `Dial` methods | `newFallbackDialer` and `dialer.dial` |
| `TunnelingConnection` | `TunnelingConnection` |
| `NewTunnelingConnection`, `NewTunnelingConnectionWithLogger` | `newTunnelingConnection(name,conn)` using named `klog.Background()`, `newTunnelingConnectionWithLogger(logger,conn)` using canonical `logr.Logger` |
| `TunnelingConnection.Read`, `Write`, `Close` | `read`, `write`, `closeTunnelingConnection` |
| `TunnelingConnection.LocalAddr`, `RemoteAddr` | `localAddr`, `remoteAddr` |
| `TunnelingConnection.SetDeadline`, `SetReadDeadline`, `SetWriteDeadline` | `setDeadline`, `setReadDeadline`, `setWriteDeadline` |
| `PingPeriod` | `pingPeriod()` (10 seconds) |
| `NewSPDYOverWebsocketDialer`, `NewSPDYOverWebsocketDialerWithLogger`, `NewSPDYOverWebsocketDialerForStreaming`, `NewSPDYOverWebsocketDialerForStreamingWithLogger` | `newSPDYOverWebsocketDialer(environment,config,address)` and `newSPDYOverWebsocketDialerWithLogger(logger,environment,config,address)`; both upstream views share one native dialer |
| `tunnelingDialer.Dial`, `streamingTunnelingDialer.Dial` | returned `spdy.Dialer.dial` |
| compatibility adapter `Dial`, `CreateStream`, `Close`, `CloseChan`, `SetIdleTimeout`, `RemoveStreams`, `Read`, `Write`, `Reset`, `Headers`, `Identifier` | one canonical `spdy.Dialer`/`Connection`/`Stream` family; no duplicate compatibility representations |

## Behavioral boundaries

Ports use exact uint16 parsing, including `local:remote`, an omitted local port,
and ephemeral local zero; remote zero is rejected. `localhost` expands to IPv4
and IPv6 with the upstream "at least one succeeds" group. Explicit IPs require
success independently. Addresses are deduplicated and ordered. A bound ephemeral
port is reused for subsequent addresses of the same forwarding pair.
Display formatting is explicit: forwarded pairs use `{local remote}` and listen
addresses use `{address protocol failureMode}`. These multi-field records derive
Eq/Debug, not the scalar-only automatic Show implementation.

Forwarding creates the error stream before the data stream and passes both the
remote port and a monotonic request ID. Data reset precedes the wait for the
error stream. Either a remote error payload or an error reading that stream
closes the multiplexed connection. Listeners and active connection producers
are closed/canceled and joined at shutdown. Readiness is separate from listener
allocation; `getPorts` returns the pinned no-Ready/not-ready errors until ready.

Tunneling negotiates `SPDY/3.1+<protocol>` over a GET websocket upgrade, removes
that exact prefix and supplies a native `net.Conn` capability to the external
SPDY framing implementation. Binary message boundaries are hidden from readers;
message reader state preserves partial reads. Text messages are errors. Writes
preserve partial count and flush errors. Normal websocket close maps to EOF;
closure sends the normal-close control frame once and closes the underlying
socket. Explicit deadlines are not overwritten by tunnel writes.

Logging uses the canonical external `logr.Logger` and klog naming/context
bindings, not an implicit no-op. Negotiation is shared through
`negotiateSPDYOverWebsocket(logger,transport,holder,address,protocols)`,
attaches the supplied logger to the HTTP request context, emits the pinned
V(4) upgrade/negotiated-protocol records, and names the tunnel logger `client`.
Read/write/close messages retain the pinned V(7)/V(8) levels; detailed read
completion fields are emitted only when V(8) is enabled, otherwise V(7)
completion is emitted. Non-normal NextReader errors are logged at V(4).
Read/write data fields retain byte-array representation.

## Regression and integration coverage

`portforward_test.gfx` covers parsing, address-family groups, fallback and
readiness errors. `loopback.run() IO[Bool]` runs actual local TCP listening plus
SPDY error/data streams, verifies stream headers and binary data including
split UTF-8, NUL and 0xff, and checks explicit stop and remote error/connection
loss. The success fixture sends a stop signal; only its cleanup closes the stop
channel. `loopback.runTunnel()`
also checks real websocket negotiation and message boundaries, partial reads,
writes, normal EOF and repeated closure through the native TunnelingConnection.
Its custom thread-safe logr sink runs at both V(8) and V(7), checks negotiation
and operation levels, caller/`client` names, byte/length fields, and exactly one
close record. Test servers are isolated under `loopback/host`; generic
production socket/logger conversion effects are under `host`.

## Focused verification

The independent `ClientGoStreamingVerify` worker ran
`go run ./cmd/gofx test ports/client-go/tools/portforward` under the serialized
VM test wrapper: **4 passed, 0 failed**, exit 0. It invoked the published
`tools/portforward/loopback.run()` after the stop-channel ownership correction
and strengthened UTF-8/two-byte socket-write fixtures: **true**, exit 0.
This includes local TCP forwarding, remote error/connection loss, native tunnel
message boundaries, normal EOF and repeated tunnel close.

The fixture's stop channel has one closer; normal completion sends a signal
instead of closing a second time. The implementation owner inspected verifier
evidence without running checks. Whole-repository and live Kubernetes
acceptance remain separate gates.

After capability reconciliation, the current worktree compiler ran
`go run ./cmd/gofx test ports/client-go/tools/portforward`: **4 passed,
0 failed**, exit 0. A temporary entry point invoking `loopback.runTunnel()`
ran with `go run ../../cmd/gofx run tunnel_logging_proof.gfx` from the port
root and returned **true**, exit 0. Both commands used
`serialize-vm-tests --class short` with a 180-second execution bound and default
optimization. The consumer exercised custom logger levels/names, request
context propagation, actual negotiation and byte/close behavior. Its first
execution exposed a missing Option wrapper at the generic HTTP request
conversion boundary; production and fixture conversions now match the
canonical pointer representation. The temporary entry point was removed.
