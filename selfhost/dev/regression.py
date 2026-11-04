"""Focused compiler boundary smoke checks; no generated fixtures survive the run."""
from pathlib import Path
import argparse
import subprocess
import tempfile

from run import ROOT, checked


NEGATIVE = {
    'go-import': ('import "go:strings"\n', 'selfhost does not bind Go packages'),
    'alias-cycle': ('alias Loop = List[Loop]\n', 'type alias cycle involving Loop'),
    'duplicate-bound': ('func f[A Show + Show](x A) A { x }\n', 'duplicate constraint on type parameter A'),
    'float-exponent': ('func f() Float { 1e+ }\n', 'bad float literal: exponent requires digits'),
    'bare-placeholder': ('func f() Int { _ }\n', '_ is only valid as a placeholder in a call or operator section'),
    'interpolation': ('func f() String { "${missing}" }\n', 'unknown identifier missing'),
}

POSITIVE = '''package main
import "std/prelude"
alias Number = Int
func make() func(Number) Number { func(x) => x + 1 }
func lifted() IO[Number] { 7 }
law identity(x Int) { x == x }
proof identity(x) { refl }
func main() IO[Unit] {
  do {
    n <- lifted()
    let recurse = func(i, x) => if i == 0 { show(x) } else { recurse(i - 1, x) }
    println("${make()(n)}: " + recurse(2, true))
  }
}
'''


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--stage1', type=Path, default=ROOT / '.tmp/bootstrap/stage1/selfhost')
    args = parser.parse_args()
    compiler = args.stage1.resolve()
    with tempfile.TemporaryDirectory(prefix='.regression-', dir=ROOT / 'selfhost/dev') as directory:
        directory = Path(directory)
        source = directory / 'main.gt'
        generated = directory / 'main.go'
        for name, (declarations, expected) in NEGATIVE.items():
            source.write_text('package main\n' + declarations + 'func main() IO[Unit] { ioPure(unit()) }\n')
            process = subprocess.run([str(compiler), str(source), '-o', str(generated)], cwd=ROOT,
                                     text=True, capture_output=True, timeout=30)
            if process.returncode == 0 or expected not in process.stderr:
                raise AssertionError(f'{name}: exit={process.returncode}; stderr={process.stderr!r}')
            print(f'PASS {name}: {expected}')
        source.write_text(POSITIVE)
        subprocess.run([str(compiler), str(source), '-o', str(generated)], cwd=ROOT, check=True, timeout=30)
        binary = directory / 'program'
        checked(['go', 'build', '-o', str(binary), str(generated)])
        process = subprocess.run([str(binary)], cwd=ROOT, text=True, capture_output=True, check=True, timeout=30)
        if process.stdout != '8: true\n':
            raise AssertionError(process.stdout)
        print('PASS aliases, lambda tails, implicit IO, erased laws/proofs, recursive dictionaries: 8: true')


if __name__ == '__main__':
    main()
