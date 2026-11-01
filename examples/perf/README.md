# Native scalars and bounded specialization

Run from the repository root:

```sh
go run ./cmd/go4t run examples/perf/main.gt
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
go run ./cmd/go4t build --emit-go "$output" --package perf examples/perf/main.gt
# Read "$output/main.go".
rm -rf "$output"
```

The emit command produces one Go file per module; `main.go` contains the entry
module's native functions, named `perf_addInts_` and `perf_sumPair__special1`.
The excerpts below are the pre-module slice's original execution receipt.

```go
//line examples/perf/main.gt:6
func addInts_(a_ int64, b_ int64) int64 {
//line examples/perf/main.gt:6
	return (a_ + b_)
}
```

The parameters and result are `int64`, and addition is a native Go `+`, not
`rt.AddInt`. `countDown_` likewise has `int64` loop parameters and uses the
existing tail-call loop with simultaneous parameter reassignment.

`sumPair` retains its generic dictionary entry point, while the concrete call
uses this separately generated copy:

```go
//line examples/perf/main.gt:7
func sumPair__special1(a_ int64, b_ int64) int64 {
//line examples/perf/main.gt:7
	return any((Combine_Int{}).Combine(a_, b_)).(int64)
}
```

The copy binds the concrete dictionary statically and removes the hidden
`Combine` parameter. Dictionary methods retain the erased `any` ABI, so that
method call is an intentional boxing boundary. The generic entry remains for
calls whose dictionaries are not concrete or whose specialization budget is
exhausted. See [SPEC §8.1](../../SPEC.md#81-go-generation-and-performance-informative)
for limits and semantics, and [§8.6](../../SPEC.md#86-performance-evidence-historical-not-a-portability-guarantee)
for historical host measurements and budgets.
