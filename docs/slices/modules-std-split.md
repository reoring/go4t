# Modules standard-library split authority

Worker target: only `std/**/*.gfx`, `std/embed.go`, and existing `examples/**/*.gfx` in `/workspace/wip/gofx/.wt/modules`. Do not edit the old prelude, Go tests, or any other Go source. No recursive delegation, commits, tests/builds/formatters. Parent owns integration and verification.

## Exact Go interface

Create package `std` in `std/embed.go`:

```go
package std
import "embed"
//go:embed */*.gfx
var FS embed.FS
```

No other Go API is needed. Embedded FS names are `prelude/prelude.gfx`, `list/list.gfx`, etc. Every module contains one file with `package <last path element>` and imports immediately after package. Modules listed below must all exist.

## Declaration ownership

Move each declaration from `prelude/prelude.gfx` verbatim except qualified references required below; do not duplicate definitions. Line ranges below refer to the unchanged Wave A source and include complete declarations.

- `std/prelude/prelude.gfx`: classes at 3–30 and 34–36; Ordering at 32; Option at 38; Option instances 50–67 and 101–114; Ordering/Option Show 199–205; tuple Show/Debug 252–262; primitive extern helpers 268–277; min/max 299–300; primitive/tuple instances 385–456; map/flatMap 467–473; fst/snd 518–524. Everything here is exported through explicit `prelude.name`, but only the implicit list below is automatically visible.
- `std/list/list.gfx`: List at 46; List instances 82–99, 116–132, 243–250; foldl/toList/foldMapList 134–144; forEach/mapM 475–487; fold/length/append/filter/reverse 489–516; zip/head/tail/take/drop/sum/concatMap/elem/insertBy/sortBy/sort/maximum/minimum 526–621; all list extras 767–976.
- `std/result/result.gfx`: Result and Show at 40–44; Result Functor/Monad 69–80; Option helpers 183–197; Result helpers 207–237. Option itself remains prelude.
- `std/io/io.gfx`: ioFailWith 239–241; externs 264–267; externs 358–377; IO instances 458–465. `tryIO` lives here, not result.
- `std/float/float.gfx`: integer numeric helpers 278–280; all float externs/instances 282–297 and 302–316.
- `std/string/string.gfx`: externs 318–349.
- `std/regex/regex.gfx`: externs 351–356; keep existing names (`regex.regexMatch`, not a new rename).
- `std/conc/conc.gfx`: externs 378–383.
- `std/map/map.gfx`: 625–681 plus Show/ Eq/Functor Map instances 733–737, 747–749, 759–761.
- `std/set/set.gfx`: 683–720 plus Show/Eq Set 739–741, 751–753.
- `std/array/array.gfx`: 722–731 plus Show/Eq/Functor Array 743–745, 755–757, 763–765.
- `std/state/state.gfx`: State type at 48 and 146–181.
- `std/lazy/lazy.gfx`: 978–987.

State/lazy preserve existing Wave A behavior outside the deliberately narrow prelude. No definitions may be dropped.

## Prelude reexports and qualification

Parent supplies reexport linking in resolver: these exact names are implicitly visible in every file:

`Functor Monad Show Debug Eq Ord Foldable Traversable Ordering Option Result List Unit map flatMap fold filter length append reverse head tail show debug println print eprintln ioPure ioFail unit fst snd compare eq min max toFloat`

Reexporting a data type includes its constructors: LT EQ GT None Some Err Ok Nil Cons. Builtin types Int Float String Bool IO Map Set Array Fiber and tuple types remain compiler builtins. The other class methods are NOT implicit: use `prelude.fmap`, `prelude.pure`, `prelude.bind`, `prelude.foldr`, `prelude.traverse` when referencing them from another module. Instance method declaration names stay unqualified (`fmap`, `pure`, `bind`, etc.). Constructors and listed names may remain unqualified; declarations within the same module stay unqualified. `prelude` imports are explicit wherever qualified references are used. No dot import syntax.

`std/prelude` reexports `List` and its constructors/core operations from std/list; `Result` and constructors from std/result; IO core from std/io; toFloat from std/float. Parent resolves this without language-level export syntax. Prelude's own references to those listed names also stay unqualified. Add `import "std/prelude"` to std files that use nonimplicit class methods and qualify those references, except prelude's own bodies.

Other cross-module references must use imports and qualifiers: e.g. Map groupBy uses `result.fromOption`, Show Map/Set/Array uses `string.join`. Retain existing operation spellings (e.g. `map.mapInsert`, `float.powInt`). Each explicit import is file-scoped.

## Exact example migration rule

For EVERY existing `examples/*/main.gfx`, `examples/typed-extern/gostd.gfx`, `examples/cli-tools/failing.gfx`, and `examples/diagnostics/bad_*.gfx`:

1. Resolve the old prelude identifier against the ownership list above.
2. If it is on the implicit list (or is one of the constructor/builtin names above), leave it unchanged.
3. If it is a local declaration, type parameter, lambda/function parameter, let/pattern binding, field label, quoted text, comment, extern Go target, or instance method declaration, leave it unchanged.
4. Otherwise replace each reference to the old prelude name by `<module>.<name>` and add `import "std/<module>"` once after package. Include qualified types and patterns for State/Lazy. For prelude-only nonimplicit methods use `import "std/prelude"` and `prelude.fmap` etc.
5. Use `import collections "std/map"` and qualifier `collections` when a source already needs unqualified prelude `map`; use the same alias whenever import default collides with a local declaration. Imports are namespaces rather than values, but explicit aliases make examples clear. Similarly alias `std/float` only if needed.

This is a lexical-reference migration, NOT global regex replacement over strings/comments/declaration names. Bodies and observable behavior otherwise remain unchanged. Preserve diagnostics fixtures' intended failures. Do not create modules example; parent owns it. Do not touch example READMEs; parent owns docs updates.

## Acceptance supplied to parent

Report complete files created/edited, ownership distribution, and any ambiguity requiring parent resolution. Parent runs all non-selfhost tests and all examples after integrating parser/resolver/codegen.
