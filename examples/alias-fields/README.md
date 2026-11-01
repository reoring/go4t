# Transparent aliases and common fields

From the repository root:

```sh
go run ./cmd/gofx run examples/alias-fields/main.gfx
```

Observed output:

```text
hello Ada
pair: Ada / Ada
circle id=7 r=3 -> circle id=0 r=3; original=7
rect id=9 w=4 h=5 -> rect id=0 w=4 h=5; original=9
generic: L/R updated/updated count=2; originals=left/right
```

`Name`, `Pair[A]`, and `Handler` are transparent aliases for a string, tuple,
and function type. `Shape` supports reading `id` and copying with a replacement
`id` without a match. The generic `Tagged[A]` example puts its common fields at
different positions in the two constructors and updates two fields together.
Updates retain the original constructor and all untouched fields, and do not
modify the input.
