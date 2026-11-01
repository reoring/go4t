# Tail calls and source locations

Run from the repository root:

```sh
go run ./cmd/go4t run examples/codegen-quality/main.gt
```

Observed output:

```text
500000500000
```

`sumThrough` adds every integer from 1 through 1,000,000 using an explicit self-tail-recursive helper. Generated Go runs the helper as a loop, evaluating both next arguments before updating the parameters. Type-class dictionary parameters, when present, are unchanged across iterations.

The codegen regression `TestMillionElementFoldWithoutStackGrowth` separately constructs a list of one million elements, folds its sum, checks its length, reverses it, and checks the reversed head and length. It runs generated Go with a 256 KiB maximum goroutine stack and a 120-second deadline, proving that success does not depend on Go automatically growing the stack. Its expected and observed output is:

```text
500000500000 1000000 1000000 1000000
```

## Panic source positions

`panic.gt` deliberately divides by zero in a match arm. It is not named `main.gt`, so the successful-example CI loop does not execute this expected-failure fixture.

```sh
go run ./cmd/go4t run examples/codegen-quality/panic.gt
```

Observed trace excerpt (the machine-specific runtime file path is omitted):

```text
panic: runtime error: integer divide by zero

goroutine 1 [running]:
...
main.main_.divideBy_.func1(...)
	examples/codegen-quality/panic.gt:5
main.divideBy_(...)
	examples/codegen-quality/panic.gt:8
main.main_()
	examples/codegen-quality/panic.gt:11 +0xc
main.main()
	examples/codegen-quality/panic.gt:10 +0x13
exit status 2
```

Line 5 is the division-by-zero match arm in the original `.gt` file, not a generated Go line. The compiled program exits 2; the outer `go run` reports that exit and itself exits 1. This panic happens while constructing `main`'s IO value, before `rt.RunMain` executes the action.

## Focused checks

```sh
go test ./codegen -count=1
go run ./cmd/go4t check examples/wc/main.gt
```

Both passed. The package tests also cover formatting, directives before generated functions and match arms, argument swapping, shadowing, closure captures, named recursive lambda lowering, and unchanged dictionaries.
