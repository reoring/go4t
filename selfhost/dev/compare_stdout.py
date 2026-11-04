"""Apply bootstrap's explicit exact-output or nondeterministic line-count policy."""
from pathlib import Path
import sys


def compare(expected: bytes, actual: bytes, *, nondeterministic: bool = False) -> str | None:
    reference = expected.splitlines(keepends=True)
    observed = actual.splitlines(keepends=True)
    if len(reference) != len(observed):
        return f"line counts: reference={len(reference)}, stage1={len(observed)}"
    if nondeterministic:
        return None
    for number, (wanted, value) in enumerate(zip(reference, observed), 1):
        if wanted != value:
            return f"line {number}: expected {wanted!r}; stage1 {value!r}"
    return None


def main() -> int:
    name, mode, reference, actual = sys.argv[1:]
    if mode not in ("deterministic", "nondeterministic"):
        raise ValueError(f"unknown comparison mode: {mode}")
    nondeterministic = mode == "nondeterministic"
    error = compare(Path(reference).read_bytes(), Path(actual).read_bytes(), nondeterministic=nondeterministic)
    if error:
        print(f"FAIL output {name}: {error}")
        return 1
    if nondeterministic:
        print(f"ok (nondeterministic: exit+line count) {name}")
    else:
        print(f"ok   {name}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
