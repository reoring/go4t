# Raw native watch leaf

`Event` has an arbitrary string `eventType` and an optional canonical external `runtime.Object`. No unknown-string, nil-object, or Status filtering occurs here.

`Watch` uses a private positional constructor holding `sync.Channel[Event]` and its producer-owned stop callback. Public operations are `newWatch`, `next`, `receiveCase`, and `stop`. `None` from `next` means closed and drained. A `receiveCase` consumes only when it wins the shared mixed-select operation.

`stop` invokes the callback under `sync.uninterruptible`. Producers must supply an idempotent callback, stop any blocked send via their cancellation/stop channel, and join actual completion where they own a task. The leaf does not close a caller-owned channel or invent a second lifetime owner. Cancellation while receiving is an IO cancellation; higher-level Until translates it while retaining its last event.

This leaf is shared by Rest, Cache and WatchPager. It imports no concrete client/cache/transport module.
