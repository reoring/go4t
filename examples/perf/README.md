# Native scalars and bounded specialization

Run from the repository root:

```sh
go run ./cmd/gofx run examples/perf/main.gfx
```

Observed output:

```text
native: 42
specialized: 42
tail loop: 500000500000
```

Inspect the generated Go without leaving build output in the repository:

```sh
output=$(mktemp -d)
go run ./cmd/gofx build --emit-go "$output" --package perf examples/perf/main.gfx
# Read "$output/main.go".
rm -rf "$output"
```

The emit command succeeded with no stdout. Its generated file contains:

```go
//line examples/perf/main.gfx:6
func addInts_(a_ int64, b_ int64) int64 {
//line examples/perf/main.gfx:6
	return (a_ + b_)
}
```

The parameters and result are `int64`, and addition is a native Go `+`, not
`rt.AddInt`. `countDown_` likewise has `int64` loop parameters and uses the
existing tail-call loop with simultaneous parameter reassignment.

`sumPair` retains its generic dictionary entry point, while the concrete call
uses this separately generated copy:

```go
//line examples/perf/main.gfx:7
func sumPair__special1(a_ int64, b_ int64) int64 {
//line examples/perf/main.gfx:7
	return any((Combine_Int{}).Combine(a_, b_)).(int64)
}
```

The copy binds the concrete dictionary statically and removes the hidden
`Combine` parameter. Dictionary methods retain the erased `any` ABI, so that
method call is an intentional boxing boundary. The generic entry remains for
calls whose dictionaries are not concrete or whose specialization budget is
exhausted. See `docs/slices/perf.md` for limits, semantics, and measured budgets.
