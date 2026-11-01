# Float and numeric helpers

Computes the mean and population standard deviation of a Float list, formats results, parses decimal input, and demonstrates powers, generic min/max, rounding, and IEEE division.

From the repository root:

```sh
go run ./cmd/gofx run examples/float-num/main.gfx
```

Observed output:

```text
mean=5.00 stddev=2.00
literals=1.5, 2000.0, 0.001
parse=Some(12.5) invalid=None
pow=8.0 powInt=243
ints min=3 max=8; floats min=1.5 max=2.5
truncate=-2 floor=-3 ceil=-2 round=-3 abs=2.75
division=+Inf, -Inf; NaN=true ordered=true equal=false
```

The sample is nonempty; its population variance divides by the sample count. Empty input consequently has an IEEE NaN mean rather than a division-by-zero panic. Float values and Int values are distinct types: use `toFloat` when converting a count.

The verification invocation used the VM-wide serialization wrapper:

```sh
SERIALIZE_VM_TESTS_MODE=shared /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests -- go run ./cmd/gofx run examples/float-num/main.gfx
```
