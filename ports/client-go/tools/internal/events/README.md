# Shared event mechanics

Native counterpart of the recorder capability interfaces in client-go v0.37.2 `tools/internal/events/interfaces.go`, plus common implementation mechanics for tools/record and tools/events. Apache-2.0.

`EventRecorder`, `EventRecorderLogger`, and `AnnotatedEventRecorder` retain eventf, annotatedEventf and withLogger through typed callbacks over optional canonical runtime.Objects, canonical annotation maps and already-formatted Strings. Logger is an explicit effectful callback. It is never a hidden no-op logger.

`Hub[A]` and `Listener[A]` implement bounded non-blocking fanout. Each listener has an amortized FIFO (front/back lists), shared-runtime mutex/condition and an owned worker task. Full listeners drop delivery without blocking other listeners. Explicit stop drops pending listener deliveries; hub shutdown rejects new events and drains accepted deliveries, then joins workers. Context cancellation and callback failures run cleanup and unlink listeners. State synchronization uses only support/sync.

The host boundary in `ports/client-go/host/events/effects.go` contains external codec/conversion primitives only: time.Time/nanoseconds, hexadecimal formatting, UUID entropy, external DNS-name validation, CommonAccessor, strategic merge patch encoding, hostname, default stderr logging, and exact external StatusError extraction. Its public Go import path permits emitted consumers outside the tools tree and external consumer modules. It does not implement event correlation, scheduling, retry, throttling or registry policy, and imports no client-go implementation.

`broadcast_test.gt` prepares deterministic FIFO/shutdown and slow-listener overflow traces. No checks were run.
