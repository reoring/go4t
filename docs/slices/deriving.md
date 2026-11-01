# Slice: deriving

## Language / SPEC delta

The deriving clause's grammar is unchanged. Supported names are now `Eq`, `Ord`,
`Debug`, `Functor`, `Foldable`, and `Traversable`; duplicate names and duplicate
hand-written instances remain errors. `deriving Show` is rejected with exactly:

```text
deriving Show is not supported; derive Debug and write instance Show if a display form is needed
```

`Debug` derivation emits ordinary `ast.InstanceDecl`s before checking, as the
existing `Eq` and `Ord` expansions do. It renders nullary `C`, positional
`C(f1, f2)`, and record `C{a: f1, b: f2}` using each field's `Debug`. Every type
parameter receives a `Debug` constraint, including phantom parameters, matching
existing Eq/Ord derivation. Generated expressions retain the declaration position.

Higher-kinded derivations fix all leading type parameters and map only the last:
`type T[E, A] ... deriving Functor` produces `instance Functor[T[E]]` without
constraints on `E`. A declaration with no type parameter is rejected. The last
parameter may occur directly, in tuples (any arity), as the last argument of a
type with the corresponding instance, or recursively in the declaring type.
Unmapped fields are preserved for mapping/traversal and ignored for folding;
function-valued fields not containing the mapped parameter are therefore legal.

Mapped occurrences inside function types or non-last arguments are rejected at
the declaration, naming the constructor and the record field name or one-based
positional field number. Example diagnostics:

```text
cannot derive Functor for T: requires a last type parameter
cannot derive Traversable for T: constructor T field run: type parameter A occurs in a function type
cannot derive Foldable for T: constructor T field 1: type parameter A occurs as a non-last argument of Pair
```

Nested mapping/folding/traversal uses the nested type's ordinary class instance;
missing instances are normal checker errors. Folding associates right and visits
constructor/tuple fields in declaration order. Traversal sequences effects in that
order and reconstructs the original shape, short-circuiting according to the
supplied Monad. `Traversable` requires its `Functor` superclass instance; it does
not automatically derive it. Method-local `M[_] Monad` dictionaries already work
in the checker/code generator; focused tests now cover them in prelude and derived
instances, including `State[List[Int]]` as a partially applied Monad.

## Prelude additions and preserved behavior

```gofx
class Debug[A] { debug(a A) String }
class Foldable[F[_]] { foldr[A, B](fa F[A], init B, f func(A, B) B) B }
class Traversable[F[_]] : Functor[F] {
  traverse[M[_] Monad, A, B](fa F[A], f func(A) M[B]) M[F[B]]
}
func foldl[F[_] Foldable, A, B](fa F[A], init B, f func(B, A) B) B
func toList[F[_] Foldable, A](fa F[A]) List[A]
func foldMapList[F[_] Foldable, A, B](fa F[A], f func(A) List[B]) List[B]
extern func debugString(s String) String = "gofx/rt".DebugString
```

- `Debug`: structural instances for `Int`, `String`, `Bool`, `Unit`, `Ordering`,
  `Option[A]`, `List[A]`, and `(A, B)`, matching the existing prelude tuple instance
  arity. `Unit` is `()`; strings are quoted with Go escapes using `strconv.Quote`.
  Lists are structural `Cons(a, Cons(b, Nil))`, not the display list form.
- `foldr`: right fold for `List` and `Option`; empty input returns `init`.
- `traverse`: effectful shape-preserving map for `List` and `Option`; empty input
  uses `pure` and does not call the function.
- `foldl`: left fold expressed through the Foldable right fold and continuations.
- `toList`: flatten mapped values in fold order through Foldable.
- `foldMapList`: map each folded value to a list and concatenate in fold order.
- `debugString`: any-ABI runtime primitive returning a Go-quoted string.

`Ordering` and `Option` now derive Debug rather than Show and have explicit Show
instances preserving the previous output. All previous prelude Show behavior is
unchanged: String remains identity, lists remain `[a, b]`, pairs `(a, b)`, Option
`None`/`Some(a)`, Ordering `LT`/`EQ`/`GT`, and Int/Bool retain their representations.
Interpolation still requires Show. Cards uses explicit `debug(...)` interpolation;
routes has an explicit Station Show delegating its enum label to Debug, retaining
compact displayed lists.

## Verification

All commands ran from `.wt/deriving`. Go tests and CLI commands were prefixed by
`env SERIALIZE_VM_TESTS_MODE=shared /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --`.
The runner downgraded to exclusive mode when memory admission required it.

- `go test ./types ./prelude ./rt ./codegen` — passed all four packages. Includes
  recursive/nested/tuple/fixed/phantom derivations, repeat checking of the same AST,
  diagnostics, method-local Monad dictionary resolution, escaped strings, unchanged
  Show rendering, left/right fold order, traversal order via State, and Option
  short-circuiting with a divide-by-zero sentinel in the skipped continuation.
- `go run ./cmd/gofx run examples/deriving/main.gfx` — passed; exact observed output
  is in `examples/deriving/README.md`.
- `go run ./cmd/gofx check examples/cards/main.gfx` — passed.
- `go run ./cmd/gofx check examples/routes/main.gfx` — passed.
- `go run ./cmd/gofx check examples/wc/main.gfx` — passed once.
- `gofmt -w types/derive.go types/pass10_test.go types/deriving_test.go prelude/prelude_test.go prelude/deriving_test.go rt/debug.go rt/debug_test.go codegen/pass10_test.go` — completed.

No full suite, vet, live access, commits, or pushes. Generated-program regression
files use `t.TempDir`; no throwaway scripts or persistent generated artifacts added.

## Deviations

None within this slice. `Float` and `Result` are absent from its baseline and were
not duplicated solely to add instances. Integration decisions: FloatNum adds
`Debug[Float]` and a duplicate Debug class (drop that duplicate); Main will migrate
IoEffects' Result to `deriving Eq, Ord, Debug` and an explicit Show preserving
`Err(e)`/`Ok(a)` with fields displayed by Show.

## Duplicated names

None declared here. The Debug class duplicate supplied by FloatNum must be removed
at integration; the class in this slice is the owner.

## Unresolved

By Main's explicit instruction, `selfhost/` belongs to another concurrent project
and was not edited. `selfhost/ast.gfx` and `selfhost/ty.gfx` still contain deriving
Show and will fail the new diagnostic until their owner migrates them. Lexer and
parser tests containing deriving Show intentionally remain: the syntax still
parses, and rejection belongs to type checking.

Diagnostics' independently assigned hint still mentions `(or 'Show')`; its owner
confirmed that exact contradictory wording is mandated by its assignment. Main
has been notified. No deriving implementation blocker remains.
