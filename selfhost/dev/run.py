"""Build/run a slice driver against the current compiler in one temporary module.

Usage: python3 selfhost/dev/run.py types|infer|parser|lexer|decls|cg_expr|cg_decls
"""
from pathlib import Path
import argparse
import re
import shlex
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
RUNNER = Path('/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests')


def checked(command: list[str]) -> None:
    if RUNNER.is_file():
        command = [str(RUNNER), '--class', 'short', '--', *command]
    subprocess.run(command, cwd=ROOT, check=True, timeout=180)


def build_driver(driver: str, output: Path, compiler: Path) -> None:
    source_list = re.search(r'SRC=\((.*?)\)', (ROOT / 'selfhost/bootstrap.sh').read_text(), re.S)
    if source_list is None:
        raise RuntimeError('bootstrap source inventory is missing')
    sources = shlex.split(source_list.group(1))
    with tempfile.TemporaryDirectory(prefix='.driver-', dir=ROOT / 'selfhost/dev') as directory:
        stage = Path(directory)
        for source in sources:
            text = (ROOT / source).read_text()
            if source == 'selfhost/main.gt':
                text = text.split('func main() IO[Unit]', 1)[0]
            (stage / Path(source).name).write_text(text)
        text = (ROOT / f'selfhost/dev/{driver}_main.gt').read_text()
        artifacts = output.parent / 'fixtures'
        text = text.replace('selfhost/dev/out/', f'{artifacts}/out/')
        text = text.replace('selfhost/dev/parser_negative_', f'{artifacts}/parser_negative_')
        for subdir in ('cgdecls', 'cgdecls_features', 'cgexpr_gen'):
            (artifacts / 'out' / subdir).mkdir(parents=True, exist_ok=True)
        (stage / 'driver.gt').write_text(text)
        generated = stage / 'generated'
        checked([str(compiler), 'build', '--emit-go', str(generated), str(stage)])
        checked(['go', 'build', '-o', str(output), str(generated)])


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('driver', choices=('types', 'infer', 'parser', 'lexer', 'decls', 'cg_expr', 'cg_decls'))
    parser.add_argument('--go4t', type=Path, default=ROOT / '.tmp/bootstrap/go4t')
    args, program_args = parser.parse_known_args()
    with tempfile.TemporaryDirectory(prefix='.run-', dir=ROOT / 'selfhost/dev') as directory:
        output = Path(directory) / 'driver'
        build_driver(args.driver, output, args.go4t.resolve())
        subprocess.run([str(output), *program_args], cwd=ROOT, check=True, timeout=120)


if __name__ == '__main__':
    main()
