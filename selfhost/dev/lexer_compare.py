"""Focused differential driver: run through serialize-vm-tests, not repo-wide tests."""
import difflib
import pathlib
import random
import subprocess
import tempfile
from run import build_driver, checked

ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCES = [
    "selfhost/dev/lexer_main.gt", "selfhost/base.gt", "selfhost/ast.gt",
    "selfhost/ty.gt", "selfhost/core.gt", "selfhost/check.gt", "selfhost/lexer.gt",
]


def run(command, source=None):
    return subprocess.run(command, cwd=ROOT, input=source, capture_output=True,
                          check=True, timeout=90).stdout


def compare(name, actual, expected):
    if actual != expected:
        diff = "".join(difflib.unified_diff(
            expected.decode().splitlines(keepends=True), actual.decode().splitlines(keepends=True),
            fromfile="Go lexer: " + name, tofile="selfhost lexer: " + name))
        raise AssertionError(diff)


# Mirrors the reference lexer's layout matrix, including token-boundary negatives.
CASES = []
previous = ["name", "123", '"text"', '"${x}"', "true", "false", ")", "]", "}", "_", ""]
previous += "package class instance type func extern let do match if else given deriving => <- == != <= >= && || ( [ { , ; . = : | @ + - * / % < > !".split()
for prev in previous:
    CASES.append(prev + "\nnext;")
    for ending in ["", "\n", "\n\n", " // comment\n", " // comment"]:
        CASES.append(prev + ending)

leading = "else | . ) ] given deriving => <- + - * / % == != < <= > >= && ||".split()
leading += "elsewhere else_ else2 elseα else٢ given2 given_ givenα given٢ deriving2 deriving_ derivingα deriving٢ = ! ; , : @ ( [ { }".split()
leading += ['"else"', '"given"', 'other else']
for lead in leading:
    for gap in ["\n", "\r\n \t\n  ", "\n// between\n  ", " // trailing\n // else given | +\n\n\t// another\n "]:
        CASES.append("x" + gap + lead + " next;")

CASES += [
    "f(\n a\n b\n)\nx", "[\n a\n b\n]\nx", "{\n a\n b\n}\nx",
    "f(\n [1,\n 2]\n)\nx", "f(func() => {\n a\n b\n})", "f({\n a\n b\n}\n c\n)\nx",
    "[{\n a\n b\n}\n c\n]\nx", "{\nf(\n a\n b\n)\nx\n}", "(x", "[x", "{x", "({x", "({x}",
    "// comment", "x\nelse", "x\ngiven", "x\n|", "x;\nelse",
    "if a { x }\nelse { y }", "type T =\n A(Int)\n | B\nx",
    "type T = A\n | B\n deriving Eq, Ord, Show\nx", "let xs:\n List[Int] =\n []\nxs",
    "do {\n let p: (Int,\n String) = (1,\n name)\n pure(p)\n}",
    "match xs {\n Some(x) if valid(x)\n && ready()\n => x\n | _ => 0\n}",
    "func(\n (a, b),\n _,\n whole @\n Point{x: x, ..},\n c Int\n) => c", "x\n-x;",
    "Point{\n x: 1\n y: 2\n}\nx", "// heading\n  α // trailing\n// between\n  beta // end",
    "<-< == = => != ! <= >= && || + - * / % ( ) [ ] { } , ; ... : | @ _",
    "_ _name name_2 true false if iffy deriving deriving_ 42", "p.0 p.0.1 p.0.name 0.1",
    "α٢\u00a0_名\nelseα\n٢٣\n_", "a\u0085b\u2003c\u2028d",
]

CASES += [
    r'"hello ${name}: ${1 + 2}!"', r'"${x}${y}"', r'"{${x}}"', r'"{{${x}}}"',
    r'"$$${x} $$ $"', r'"\x24${x}"', r'"a\n${x}\t"', r'"${show({a: 1}.a)}"',
    r'"${show(Point{a: "quoted \"text\""}.a)}"', r'"${"nested"}"',
    r'"${show("{braces}")}"', r'"${show("{{braces}}")}"',
    r'"${show("${Point{a: "x"}.a}")}"', r'"${}"',
    '"${x + // } and " are comment text\n 1}"', r'"a ${x} b ${f(y, "}")} c"',
    r'"{not interp}"', r'"{x}"', r'"{{x}}"', r'"{{{{x}}}}"', r'"\x7bx\u007d"',
    r'"\x7b{{x}}\x7d"', '"{"', '"}"', '"}}"', '"$"', '"$value"', '"end$"',
    '"$$"', '"$$$"', '"$$$$"', '"$${x}"', '"$${"', r'"\x24$"', r'"$\u0024{x}"',
    r'"\x24$${x}"', '"plain"', r'"line\n\t\r\"\\\x41\u03b1\U0001F600"',
    r'"\a\b\f\n\r\t\v\000\377\xFF\u0000\U0010FFFF"',
    r'"${"\q"}"', r'"${"\uZZZZ"}"', r'"${"\q${x}"}"',
    '\n  "abc', '\n  "abc\n', '\n  "abc\\', '\n  "abc\\\n"',
    r'"\q"', '\n $', '\n  "bad ${', '\n  "bad ${x', '\n  "bad ${x"',
    '\n  "α ${x', '\n  "bad $$ ${x', r'"${Point{a: 1}"', r'"${show("x")"',
    r'"${x}', r'"\q${x}"', r'"${x}\q"', r'"${"${}"}"',
    r'"\x0"', r'"\u123"', r'"\U00110000"', r'"\uD800"', r'"\400"', r'"\8"',
    r'"\$"', r'"\$$"', r'"\$${x}"', r'"\q', r'"\q${', r'"${"\q}"}',
    "'", "\\", "&", "😀", "\x00", "\x01", "\x7f", "\u200b", "\ufffd", "\U0010ffff",
]

rng = random.Random(1)
fragments = ["x", "α", "_", "١", "123", "\n", " ", "\t", "\r", "// }\n", "if", "else", "elseα",
             "given", "deriving", '"a"', '"${x}"', '"${"}"}"', *"()[]{},;.=:|_@+-*/%<>!&$"]
CASES += ["".join(rng.choices(fragments, k=rng.randrange(1, 45))) for _ in range(200)]
CASES += ["a" * 20000, "x\n" + "// gap\n\n" * 5000 + "+ y", '"' + "α" * 20000 + '${x}"']

with tempfile.TemporaryDirectory(prefix="go4t-lexer-") as directory:
    directory = pathlib.Path(directory)
    selfhost = str(directory / "selfhost-lexer")
    reference = str(directory / "go-lexer")
    compiler = directory / "go4t"
    checked(["go", "build", "-o", str(compiler), "./cmd/go4t"])
    build_driver("lexer", pathlib.Path(selfhost), compiler)
    checked(["go", "build", "-o", reference, "./selfhost/dev/golex"])
    files = sorted(str(p.relative_to(ROOT)) for p in ROOT.glob("examples/*/main.gt"))
    files += sorted(str(p.relative_to(ROOT)) for p in ROOT.glob("std/*/*.gt"))
    actual = run([selfhost, *files])
    expected = run([reference, *files])
    compare("examples and prelude", actual, expected)
    print(f"Corpus diff: EMPTY ({len(files)} files, {len(actual.splitlines())} output lines)", flush=True)
    for i, source in enumerate(CASES):
        data = source.encode()
        compare(f"case {i}: {source[:80]!r}", run([selfhost, "--stdin"], data), run([reference, "--stdin"], data))
    print(f"Layout/interpolation/error/stress diff: EMPTY ({len(CASES)} cases)", flush=True)
    source = 'α +\n  "value ${x}"'.encode()
    for base in [(7, 11), (0, 0), (-2, 5)]:
        options = ["--base", *(str(x) for x in base)]
        compare(f"base {base}", run([selfhost, *options], source), run([reference, *options], source))
    print("Base-position diff: EMPTY (3 cases)", flush=True)
    for source in SOURCES:
        compare(source, run([selfhost, source]), run([reference, source]))
    print(f"Compiler-input diff: EMPTY ({len(SOURCES)} files)", flush=True)
    default_output = run([selfhost])
    assert b'== examples/parser/main.gt\n' in default_output
    assert b'== interpolation\n' in default_output
    assert b'== unexpected\nERROR 2:2: unexpected character' in default_output
    print("Default inline driver: PASS", flush=True)
