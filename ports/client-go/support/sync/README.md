# Native synchronization effects

`sync.gfx` is the typed API. `internal/hostsync` supplies generic Go effects through
opaque erased handles and `rt.Shapes`; no client-go policy lives in the host.
The authoritative signature list is the Shared prerequisite contracts in
`../../DESIGN.md`.

References serialize individual reads/writes, not multi-operation transitions.
Use `withLock` for transitions. Locks are non-reentrant. Condition operations
belong inside the associated `withLock` action; `wait` registers before releasing
ownership and always reacquires before returning, including cancellation. Use
predicate loops in gofx. Condition calls are valid only in the sequential
`withLock` action that owns the mutex. `start`/`startDetached` assign fresh task
identities: a child cannot inherit the parent's lock ownership, but can acquire
its own lock. Arbitrary `std/conc` or foreign goroutine forks do not transfer
ownership either; do not pass the lock action's context to them. The host does
not enforce general goroutine identity. Acquisition and blocking operations
observe cancellation; release runs on failure and panic.

Channels use native buffered/rendezvous communication. `selectReceive` commits
exactly one receive (indices are zero-based), without speculative losing receives.
An empty selection waits for cancellation. Closing preserves buffered values,
then receives return `None`. Negative capacity fails with
`newChannel: negative capacity`; sends on closed channels and repeated closes
propagate typed `Panic` failures. Select ties have no ordering guarantee.
Close first wakes blocked senders, then waits for active native sends to leave
before closing the underlying channel. This preserves closed-send failures and
close-unblocks-send without a native send/close race.

`start` inherits cancellation/deadlines; `startDetached` retains context values
without them. `cancelTask` requests cancellation, while `join` waits for the
actual cached outcome, or the joining caller's cancellation. No timeout abandons
cleanup. `uninterruptible` also accepts an already-cancelled context. Task errors
and panics propagate; a task that explicitly completes cleanup successfully may
return that success after cancellation. Detached tasks require an explicit
lifecycle owner to cancel and join them.

Clock durations are signed integer nanoseconds. Real instants retain Go's
monotonic component; fake instants start at the requested Unix nanosecond.
Timers have one buffered notification, never close their channel, and nonpositive
durations fire immediately. Explicitly closing a timer channel leaves it
closed; later expiry cannot deliver into it. Stop/reset return whether the timer
was active and remove any pending stale notification. Fake advance emits each
due timer's deadline, not the final advanced instant; equal deadlines have no
ordering guarantee. `advance` rejects real clocks (`advance: requires fake clock`)
and negative durations (`advance: negative duration`). Condition misuse fails
with `condition: associated mutex is not held`.

A throwaway end-to-end gofx smoke exercised atomic competing increments,
condition wakeup, lossless selection, closed-channel draining, rendezvous and
cancelled empty receives, detached cleanup and strict join beyond 100ms,
failure/panic propagation, real timers and fake 25/50 microsecond timers with
reset/stop. Its source and generated artifacts were removed after success.
Focused host regression tests are retained in `internal/hostsync/sync_test.go`.
The explicitly assigned `go test -race ./internal/hostsync` boundary verification
exercises closed-send races, task ownership, typed failures and cancellation.
