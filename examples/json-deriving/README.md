# JSON deriving

`Order` and `Item` derive Encode and Decode as objects, including nested lists and
an optional note. `Status` demonstrates nullary and positional tagged constructors.
The example serializes and parses an order before decoding it, prints two-space
JSON, round-trips a shipped status, and reports a malformed nested field's path.

Run from the repository root (or this slice's worktree):

```sh
go run ./cmd/gofx run examples/json-deriving/main.gfx
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

Exit status: 0. No stderr diagnostics. Json's language declarations are temporarily
in the implicit prelude; module integration moves them to `std/json`.

Focused package checks:

```sh
go test ./rt ./types ./prelude ./codegen
go run ./cmd/gofx check examples/wc/main.gfx
```

Both passed. In the development environment, commands above were invoked through
`/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --` with
`SERIALIZE_VM_TESTS_MODE=shared`; the runner selected exclusive mode when required
by memory admission.
