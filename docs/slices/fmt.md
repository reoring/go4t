# fmt — Wave B specification delta

## CLI

```text
gofx fmt [-w|-d|-l] FILE...|DIR
```

- No flag: print canonical source, leaving inputs unchanged.
- `-w`: rewrite only changed files, preserving existing permission bits.
- `-d`: print a deterministic full-file unified diff for each changed file;
  unchanged files produce no output. Missing final newlines are represented by
  the standard `\ No newline at end of file` marker. Exit 0 on success.
- `-l`: print each unformatted path once; exit 1 if at least one path is printed,
  otherwise 0. The difference-only status does not print an error to stderr.
- The flags are mutually exclusive; repeated flags, unknown flags and missing
  input report `gofx:1:1: usage: gofx fmt [-w|-d|-l] FILE...|DIR` and exit 1.
- A directory recursively contributes regular `.gfx` files. Directory traversal
  does not follow symlinks. Explicit files are accepted independently of their
  extension. Duplicate absolute paths are removed; output is sorted by path.
- `--` ends flag parsing. Multiple files and directories can be combined.
- Formatting parses syntax only; no main, type checking, prelude loading or
  foreign-package loading is required. Invalid syntax and filesystem errors
  retain the CLI's positioned diagnostics. An invalid file is not overwritten.
  Multiple-file writes are not a transaction: a later error does not undo an
  earlier successful write.

## Canonical source

There is no new language grammar or runtime behavior.

- Indentation is two spaces. Top-level declarations are separated by one blank
  line. Blocks are multiline, and source ends with a newline.
- Match arms are one per line. A single-line arm has no leading `|`; multiline
  arms have `|`. Pattern guards and as-patterns retain their structure.
- Record literals, updates, constructors and patterns remain inline when they
  fit within 80 rune columns, including the indentation/current-line prefix.
  Longer records put each field on its own line. Interior comments also force
  a multiline record so the comments stay with their fields.
- Commas separate elements but never trail the last element. Operators are
  surrounded by spaces; unary operators remain next to their operand.
- A `|>` chain exceeding 80 columns is split with the initial expression on its
  own line and one `|>` step per subsequent line, indented one additional level.
- Parentheses preserve precedence, associativity, negative-expression statement
  boundaries and the distinction between a bare binding and a variable pattern.
- Strings retain decoded values, including literal dollars and interpolation;
  integral Float values retain a Float spelling such as `2000.0`.
- Comments retain their text and order. Leading comments attach to the next
  syntax node; same-line comments remain trailing. Positions map comments to
  AST-produced output, rather than copying source tokens or source whitespace.
  Comments inside interpolation are collected and preserved too. Trailing
  whitespace in a comment is removed.

## Implementation surface

```go
format.Source(src []byte) ([]byte, error)
format.File(f *ast.File) []byte
```

`Source` parses a complete file and returns the parser error unchanged on
failure. `File` accepts an unchecked, valid parsed AST and prints from that AST;
its input is not modified. The printer does not require retained source text.

Additive AST/lexer metadata:

```go
ast.Comment { Pos, End ast.Pos; Text string; Trailing bool }
ast.File.Comments []ast.Comment
ast.File.PackagePos, ast.File.PackageEnd ast.Pos
lexer.Lexer.Comments []ast.Comment
ast.Call.Pipe *ast.Binary
ast.Lambda.Partial, ast.Lambda.Section bool
ast.Ident.Placeholder bool
ast.Interp.Starts []ast.Pos
```

Comment text includes `//`, but not the newline. Positions use the existing
one-based rune columns and exclusive ends. Lexer comments remain outside the
token stream, including during layout lookahead, so semicolon insertion and
existing parser consumers are unchanged. `parser.Parse` transfers the collected
comments into the file.

The parser still lowers pipelines to calls and placeholders to capture-free
lambdas for checking/code generation. Surface metadata retains the original
pipeline operands and distinguishes partial calls from operator sections.
Pipeline lowering no longer mutates the right-hand call when prepending its
argument, so the original surface tree remains intact. Existing AST walkers
continue visiting the lowered semantic tree rather than visiting the surface
metadata a second time. `Interp.Starts` records the expression position
immediately after each `${`, for comments attached to interpolation openings.

## Tests and observed verification

All commands below ran in `.wt/fmt` through
`/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --`.
Only the four owning packages were tested; no full repository suite or vet ran.

- `go test ./format ./lexer ./parser ./cmd/gofx -count=1`: format, lexer and CLI
  passed; a newly added parser assertion initially expected column 23 instead of
  the actual column 22. Correcting that assertion was followed by
  `go test ./parser -count=1`, which passed. The parser also passed its full
  package run before that assertion was added.
- Formatter goldens cover indentation, declaration spacing, operators,
  precedence, single/multiline arms, records, trailing-comma removal, pipelines,
  leading/trailing/between-arm comments and interpolation comments. Width tests
  cover 79, 80 and 81 columns.
- Every `examples/**/*.gfx` file and `prelude/prelude.gfx` is checked for
  `fmt(fmt(x)) == fmt(x)` and position-insensitive structural parse equality.
  The comparison retains semantic fields and alpha-normalizes only synthesized
  placeholder bindings, whose names contain source positions. Surface metadata,
  checker annotations and display filenames are not semantic tree structure.
  Comment count, text, order and leading/trailing classification are checked
  separately. Additional cases cover local functions, patterns, negative
  statement boundaries, interpolation, sections and nested pipelines.
- Lexer tests check comment positions, Unicode columns, trailing classification
  and unchanged tokens. Parser tests check comment transfer, package spans,
  interpolation comments and retained pipeline/placeholder metadata.
- CLI subprocess tests cover stdout, `-w`, `-d`, `-l`, statuses, bad flags,
  positioned errors, invalid-input preservation, recursive directories,
  duplicate paths, permissions and diff newline markers.
- `go run ./cmd/gofx fmt -d examples/fmt/before.gfx` produced the diff retained
  in the example README, before the final reformatting pass.
- `go run ./cmd/gofx fmt -w examples prelude/prelude.gfx` exited 0. This includes
  every example `.gfx`, including intentionally type-invalid diagnostics and
  test fixtures, but deliberately excludes the separately owned `selfhost/`.
- `go run ./cmd/gofx fmt -l examples` and
  `go run ./cmd/gofx fmt -l prelude/prelude.gfx` both exited 0 with no output.
- `go run ./cmd/gofx run examples/fmt/before.gfx` exited 0 and printed `42`.
- All 31 existing `examples/*/main.gfx` programs ran successfully after the
  rewrite, using `go run ./cmd/gofx run examples/<name>/main.gfx` in a sequential
  loop. Typed externs also received `examples/typed-extern/gostd.gfx`; they
  printed `GOFX MEETS GO`, `nanana`, and `[typed, externs, keep, lists]`.
  HTTP returned `status: 200` and `twice: 23`. Records retained its intentional
  `None`-defaulted-to-Unit warning.
- The first loop's piped stdin was not inherited through the serialization
  runner, so wc initially printed zero counts. Running the actual redirected
  command inside the runner fixed the invocation, without changing code:
  `bash -c 'go run ./cmd/gofx run examples/wc/main.gfx < examples/wc/input.txt -- 3'`.
  It exited 0 and printed:

  ```text
  lines: 12, words: 60
     24 the
      5 garden
      5 leaves
  ```

- `go run ./cmd/gofx check examples/wc/main.gfx` ran once and exited 0 without
  diagnostics.
- `gofmt -w` was limited to changed Go files. All 32 smoke-generated example Go
  directories were removed; the IO example's appended log was restored to its
  pre-run contents. No commit, push, live operation or non-target source edit
  was performed.

## Integration and deviations

The formatter is implemented against the assigned post-Wave-A AST. The modules
slice owns `ast.Import`/`File.Imports` and import parsing, which do not exist in
this worktree's base. Qualified names already print via existing AST names and
field expressions; Main must integrate import declarations with the modules
slice's exact AST rather than introduce an alternate module representation.

The LSP slice was given and reused the exact comment metadata/capture API; Main
should deduplicate those shared hunks and retain this slice's interpolation
comment collection. LSP's independent `File.Source` addition is not needed by
the formatter. Other source changes are mechanical formatting; existing example
READMEs remain historical execution receipts unless explicitly updated here.

`before.gfx` intentionally ends canonical after demonstrating its original diff,
so the requested `fmt -l examples` acceptance remains empty. This reconciles the
before/diff demonstration with the final all-example rewrite. No other behavior
contract deviations or known baseline formatting gaps remain.
