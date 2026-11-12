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
    'leading-minus': ('func f(x Int) Int {\n  let y = x\n  -y\n}\n',
                      'ambiguous "-" at the start of a line: end the previous line with "-" to subtract, or write (-x) for a negative value'),
    'separator-repeat': ('func f() Int { 1__0 }\n', 'bad number literal 1__0: "_" must separate digits'),
    'separator-trailing': ('func f() Int { 10_ }\n', 'bad number literal 10_: "_" must separate digits'),
    'prefix-empty': ('func f() Int { 0x }\n', 'bad number literal 0x: expected hexadecimal digits after 0x'),
    'prefix-digit': ('func f() Int { 0b2 }\n', 'bad number literal 0b2: expected binary digits after 0b'),
    'number-letter': ('func f() Int { 123abc }\n', 'bad number literal 123abc: a number cannot be followed by a letter'),
    'prefix-letter': ('func f() Int { 0x1G }\n', 'bad number literal 0x1G: a number cannot be followed by a letter'),
    'integer-overflow': ('func f() Int { 9223372036854775808 }\n', 'integer literal 9223372036854775808 is out of range for Int'),
    'negative-overflow': ('func f() Int { -9223372036854775809 }\n', 'integer literal 9223372036854775809 is out of range for Int'),
    'float-separator': ('func f() Float { 1_0.0 }\n', 'digit separators and base prefixes are not supported in Float literals'),
    'float-prefix': ('func f() Float { 0x1.2 }\n', 'digit separators and base prefixes are not supported in Float literals'),
    'unfinished-law': ('law identity(x Int) { x == x\n', 'unterminated law or proof declaration'),
}

POSITIVE = '''package main
import "std/prelude"
alias Number = Int
func make() func(Number) Number { func(x) => x + 1 }
law before(x Int) { x == x }
proof before(x) { refl }
func lifted() IO[Number] { between(7) }
law identity(x Int) { x == x }
proof identity(x) { refl }
func between(x Int) Int { x }
law after(x Int) { between(x) == x }
proof after(x) { refl }
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
