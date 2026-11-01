# JSON deriving

`Order` and `Item` derive Encode and Decode as objects, including nested lists and
an optional note. `Status` demonstrates nullary and positional tagged constructors.
The example serializes and parses an order before decoding it, prints two-space
JSON, round-trips a shipped status, and reports a malformed nested field's path.

JSON numbers use the runtime's `encoding/json` formatter, not `Show[Float]`.
Whole floats therefore remain JSON integers such as `38495` in encoded output,
even though user-facing `show(38495.0)` now returns `38495.0`; non-finite numbers
are rejected rather than emitting invalid JSON `NaN` or `+Inf`.

Run from the repository root (or this slice's worktree):

```sh
go run ./cmd/go4t run examples/json-deriving/main.gt
```

Observed stdout:

```text
round-trip: true
{
  "id": 42,
  "customer": "Ada",
  "items": [
    {
      "sku": "GO-1",
      "qty": 2,
      "price": 12.5
    }
  ],
  "note": "Leave at reception"
}
{"$tag":"Pending"}
{"$tag":"Shipped","$values":["TRACK-7"]}
expected String at $.items[2].sku
```

Exit status: 0. No stderr diagnostics. The source imports `codec "std/json"` and
`"std/prelude"`; JSON calls and deriving classes are qualified with `codec.`,
and Result composition uses `prelude.bind`. The output is unchanged by the move.

Focused package checks:

```sh
go test ./rt ./types ./std ./codegen
go run ./cmd/go4t check examples/wc/main.gt
```

Both passed. In the development environment, commands above were invoked through
`/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --` with
`SERIALIZE_VM_TESTS_MODE=shared`; the runner selected exclusive mode when required
by memory admission.
