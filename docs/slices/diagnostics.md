# Diagnostics slice

## Grammar and source spans

- No new expression or declaration syntax. Existing `_name` identifiers are now accepted as value bindings, including as-pattern names; `_` alone remains a wildcard.
- `ast.Pos` remains a one-based line/rune-column pair. Declarations, expressions, patterns, syntactic types, parameters, blocks, arms and binding statements carry an exclusive `End ast.Pos`.
- `ast.Node` exposes `GetPos() Pos`, `GetEnd() Pos`, and `SetEnd(Pos)`; `Decl`, `Expr`, `Pattern` and syntactic `Type` implement it. Existing `Pos` fields and diagnostic start positions are preserved. `ExprBase` supplies expression accessors.
- `lexer.Token.End` is the cursor immediately after the token. The parser captures the last consumed token's end before consuming separators or following whitespace. Decoded string length is not used as source length. Interpolation expressions retain their original-file coordinates.
- Synthesized nodes may have zero `End`; they are not mistaken for user-authored unused bindings. `ast.Walk` visits syntax without traversing semantic annotations, and `ast.EndAt` finds the smallest span beginning at an error position.

## Checker and command diagnostics

- `types.Error` now carries `Pos`, `End`, `Msg`, and `File`; its `Error()` string remains `line:col: message`.
- `Program.Warnings` remains the existing channel for nonfatal, source-ordered diagnostics. A successful `check`, `build` or `run` prints warnings and continues.
- CLI errors remain `file:line:col: message`; warnings remain `file:line:col: warning: message`. Each is followed by the source line and a `^~~~` underline. Exclusive end columns are clamped to the first source line; unavailable spans produce one caret. Rune columns, indentation tabs, CRLF lines and EOF positions are handled. If the named source cannot be read, the positioned message remains available without an excerpt.
- Missing-match diagnostics are `non-exhaustive match; missing: <pattern>, ...`, with at most five witnesses. Constructor and tuple fields are specialized recursively, record fields use their positional constructor order, as-patterns preserve their inner coverage, and guarded arms do not cover any cases. Absent constructor fields are rendered `_` without recursively unfolding their types. Int/String literal holes use a concrete absent value; unconstrained columns use `_`.
- `Bool` is a finite domain: unguarded `true` and `false` together are exhaustive, including when nested in constructors or tuples. Int and String still need wildcard/default coverage.
- Missing instances on user-defined data types append:
  - Show: `hint: write 'instance Show[T]' (or 'deriving Debug' for a structural form)`.
  - Debug/Eq/Ord: `hint: add 'deriving <Class>' or write 'instance <Class>[T]'`.
  - `T` is the actual rendered type, including arguments. Builtins, function types, rigid type parameters and other classes do not receive deriving hints. Recursive instance-context diagnostics retain the underlying hint and the requiring-instance context.
- Unused named lets, lambda parameters, pattern variables and named `do` binds warn `x is unused; prefix with _ to silence`. Top-level function/instance-method parameters are exempt. Names beginning `_` are exempt. Guards and nested closures count as uses; shadowed bindings have independent usage records. Rechecking the same AST does not retain old usage state.

## Prelude signatures

None added or changed. Three genuinely unused prelude lambda parameters (`put`, `forEach`, `length`) were prefixed with `_`, so checking ordinary programs does not acquire prelude warnings. `examples/wc/main.gfx` needs no source change.

## Removed limitations and approved deviations

- Match errors now identify missing values instead of merely reporting non-exhaustiveness; checker errors/warnings now underline source spans.
- The previous conservative rule rejecting exhaustive Bool literal matches is removed. Main explicitly approved finite Bool coverage and replacement of the obsolete negative test.
- The original Pass 12 Show hint suggested deriving Show, which the deriving slice removes. Main explicitly overrode that wording with the Show hint above. No unsupported deriving Show suggestion remains.

## Duplicated names

No production declarations were duplicated. The diagnostics unit test declares a local `Debug[A] { debug(a A) String }` fixture because the unmerged worktree does not yet contain the deriving slice's prelude class; it does not modify the production prelude.

## Integration seams

Merge seams requiring explicit integration: ast.Node adds GetPos/GetEnd/SetEnd to Decl/Expr/Pattern/Type; ast.Walk(new) needs AliasDecl RHS and FuncDecl cases in block/do loops. AliasFields already provided methods. parser.funcDecl itself now captures End, covering localFunc. Checker usage tracker is locals.uses map[string]*localUse, b.trackLocal(env,name,pos,end); local function parameters need tracking only when functionBody outer!=nil; recursive publication must preserve own use record. Float needs literalPattern + absent-float witness handling in coverage.go. CLI warnings now printWarning(file,*types.Error), diagnostic moved into diagnostics.go. Prelude put/forEach ignored→_ignored and length lambda x→_x remove 3 real warnings; no wc edit needed.

## Observed verification

All commands were run from the diagnostics worktree.

```sh
env SERIALIZE_VM_TESTS_MODE=shared /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests -- go test ./lexer ./parser ./types ./cmd/gofx
```

Final result: exit 0; lexer, parser, types, and cmd/gofx all passed. Tests cover
exclusive spans (including strings, interpolation and nested syntax), nested
constructor/record/tuple/literal witnesses and the five-witness cap, all four
class hints, unused-local scoping/exemptions and spans, repeated checking, and
caret/error/warning rendering for check/build/run. The regression baseline
failed for the missing diagnostics; after implementation one extra recursive
suffix test caught unnecessarily expanded list witnesses, fixed by keeping
unconstrained suffixes as wildcards. One wrapper invocation timed out while
waiting for another worktree's VM lock, before tests started; the subsequent
bounded invocation ran successfully.

- `go run ./cmd/gofx run examples/diagnostics/main.gfx`: exit 0, `count: 3`.
- `go run ./cmd/gofx check examples/wc/main.gfx`: exit 0, no output or warnings.
- `go run ./cmd/gofx check examples/diagnostics/bad_missing_ctor.gfx`: expected exit 1.
- `go run ./cmd/gofx check examples/diagnostics/bad_missing_nested.gfx`: expected exit 1.
- `go run ./cmd/gofx check examples/diagnostics/bad_missing_tuple.gfx`: expected exit 1.
- `go run ./cmd/gofx check examples/diagnostics/bad_missing_literal.gfx`: expected exit 1.
- `go run ./cmd/gofx check examples/diagnostics/bad_hint.gfx`: expected exit 1.
- `go run ./cmd/gofx check examples/diagnostics/bad_unused.gfx`: exit 0, three warnings.

Exact example diagnostics and caret output are recorded in
`examples/diagnostics/README.md`. No repository-wide tests, build, lint, or
formatter were run; only changed Go files were formatted with `gofmt`.
