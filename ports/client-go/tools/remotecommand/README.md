# Native remotecommand

Source: every implementation file in client-go v0.37.2
`tools/remotecommand`, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`,
Apache-2.0. Protocol orchestration is native `.gfx`; Go is restricted to immutable
Status JSON decoding, structured exit-error conversion, byte operations, and
isolated loopback servers.

## Public symbol mapping

| Upstream declaration | Native equivalent |
| --- | --- |
| `StreamOptions` | `StreamOptions {stdin,stdout,stderr,tty,terminalSizeQueue}` |
| `Executor` | `Executor {streamWithContext}` |
| `Executor.Stream`, `Executor.StreamWithContext`, corresponding methods on each concrete executor | `stream`, `streamWithContext`, or the capability callback; cancellation context is implicit IO |
| `TerminalSize` | `TerminalSize {width,height}` |
| `TerminalSizeQueue`, `Next` | `TerminalSizeQueue {next,close}`; explicit close enables joined cancellation |
| `FallbackExecutor`, `NewFallbackExecutor` | `Executor`, `newFallbackExecutor(primary,secondary,predicate)` |
| `NewSPDYExecutor` | `newSPDYExecutor(environment,config,method,address)` |
| `NewSPDYExecutorForTransports` | `newSPDYExecutorForTransports(transport,upgrader,method,address)` |
| `NewSPDYExecutorRejectRedirects` | `newSPDYExecutorRejectRedirects(transport,upgrader,method,address)` |
| `NewSPDYExecutorForProtocols` | `newSPDYExecutorForProtocols(transport,upgrader,method,address,protocols,rejectRedirects)` |
| `NewWebSocketExecutor` | `newWebSocketExecutor(environment,config,method,address)` |
| `NewWebSocketExecutorForProtocols` | `newWebSocketExecutorForProtocols(environment,config,method,address,protocols)` |
| `readerWrapper.Read` | `spdy.Reader.read`; native copying calls Read rather than foreign WriterTo shortcuts |
| `wsStreamCreator.CreateStream` | native `WSMux` stream creation used by `streamWebSocket` |
| websocket `stream.Read`, `Write`, `Close`, `Reset`, `Headers`, `Identifier` | `spdy.Stream.reader`, `writer`, `reset`, `headers`, `identifier`; stream close sends binary `[255,id]` |

There are no exported declarations in upstream `errorstream.go`, `v1.go` through
`v5.go`, or `doc.go`. Their behavior lives in `streamProtocol`,
`streamProtocolReady`, `decodeError`, and the websocket multiplexer.

## Wire and lifecycle contract

* SPDY offers v5, v4, v3, v2, v1 in that order; an empty negotiated protocol means
  v1. Unknown versions are explicit errors, never substituted with websocket.
* Websocket defaults to **only v5** for upstream-compatible version skew. An
  explicit protocol list supports v1-v5. All ordinary messages are binary and
  carry the same stdin/stdout/stderr/error/resize channel identifiers.
* Error, stdin, stdout, stderr and resize streams are all created before any
  copying/demultiplexing starts. TTY suppresses stderr. Resize is enabled from v3
  and encodes `Width`/`Height` JSON plus newline.
* v1 uses its early error/output completion behavior; v2 adds stdin half-close
  and remote-stdin draining; v3 adds resize; v4/v5 decode Status and uint8 exit
  causes. A nonzero status is not reduced to an opaque string: `exitCode(error)`
  retrieves the structured code, including through error wrappers.
* stdout/stderr drain the remaining remote stream after destination errors.
  Partial reads/writes retain bytes/counts and original errors. Input callbacks
  are cancellation-aware, and explicit close is required to unblock foreign
  blocking sources. Finite/closable canonical readers have adapters in spdy.
* All native producers are parent-linked tasks with lexical ownership. Cleanup
  closes the connection and input/resize sources before canceling and joining
  tasks; no timeout abandons a live producer. Websocket writes are serialized;
  the single demultiplexer and five-second heartbeat are separately owned and
  joined. Heartbeats use a 61-second read deadline and 60-second data-write limit.
* Websocket read-side closure is a separate, locked, uninterruptible transition
  from write half-close. Demultiplexer EOF, reader close and stream reset share
  that transition, close the channel once, and retain the first terminal error.
* Fallback occurs only after a primary error for which the supplied predicate
  returns true. A successful execution never invokes the secondary.
* Constructors consume explicit REST configuration; they never load a real user
  kubeconfig or split/execute command strings.

## Regression and integration entrypoints

`remotecommand_test.gfx` covers version policy, structured exits, fallback
predicates, TTY/resize and actual task completion on cancellation.
`loopback.run() IO[Bool]` exercises v1-v5 over real TCP/SPDY sockets and v5 over
real websocket sockets; payloads include Japanese/accented UTF-8, NUL and
non-UTF8 bytes. Input reads are capped at two bytes, splitting code points
across reads. It checks exit
codes, stdout integrity and TTY stderr suppression. The v3/v5 TTY cases send a
one-shot 123-by-45 resize and the real server decodes and asserts those dimensions.
`runVersion` and `runWebsocket` expose narrower variants. Fixtures bind only
loopback, have 10-second socket deadlines and explicit cleanup. Heartbeats bind
the effectful `sync.timerChannel` operation before receiving from the channel.

`runCancellation()` uses a real TCP/SPDY connection and a blocking but explicitly
closable `io.PipeReader`. It waits for all three server-side streams and for stdin
Read to begin before canceling the native child context. Success requires a
joined protocol task reporting cancellation, an observed input close, and actual
server completion. It is also included in the combined `run()` entrypoint.

## Focused verification

The independent `ClientGoStreamingVerify` worker ran
`go run ./cmd/gofx test ports/client-go/tools/remotecommand` under the serialized
VM test wrapper: **6 passed, 0 failed**, exit 0, including the permanent
idempotent-read-close regression. It invoked the published
`tools/remotecommand/loopback.run()` after adding the real cancellation fixture
and strengthened UTF-8/partial-read payloads: **true**, exit 0.

Additional independent probes passed real SPDY and websocket negotiation for
each v1-v5 protocol, real HTTP 403 websocket-to-SPDY fallback, one-byte
`writeAll` with UTF-8/NUL, partial-read error preservation and short-write byte
counts. Probe helpers were removed after evidence capture. The permanent
protocol, cancellation, closure and loopback fixtures remain.

The implementation owner inspected these verifier results but did not run
checks. Whole-repository and live Kubernetes acceptance remain separate gates.
