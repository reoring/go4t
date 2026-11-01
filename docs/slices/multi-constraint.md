# Multi-constraint slice

## Grammar and types

Replace the type-parameter grammar with:

```ebnf
TParam := Ident [ '[' '_' { ',' '_' } ']' ] [ Ident { '+' Ident } ]
```

The constraint list applies to functions, externs, and class-method type
parameters. A class's own parameter still expresses inheritance through its
superclass declaration, not through a trailing constraint list. Constraint
lists use explicit `+` separators; adjacent class names without `+` are no
longer parsed as a constraint list.

The parser preserves the list in `ast.TypeParam.Constraints`. The checker
preserves parameter declaration order and, within each parameter, class
order in `Scheme.Constraints`. No superclass expansion or reordering is
performed on declared schemes.

Repeated classes on one parameter are rejected with
`duplicate constraint <Class> for type parameter <Param>`. This applies to
function and method signatures and to instance `given` contexts. The same
class may constrain different parameters. Each class must exist and have
the parameter's kind, including higher-kinded parameters.

## Dictionary resolution and representation

A rigid requirement `C[T]` succeeds when any constraint of `T` is `C` or a
subclass of `C`. A class and its superclass can both be declared; the most
derived available dictionary satisfies both slots, irrespective of class
order. Concrete call sites likewise supply the subclass dictionary to both
slots: a call to `[A Eq + Ord]` at `Int` passes `Ord_Int{}` for both `Eq`
and `Ord`. Instance `given` contexts follow the same rule. This behavior
was confirmed with the primary during implementation.

Unrelated constraints on the same parameter remain independent. A missing
rigid requirement lists every declared constraint in order, for example:
`no instance Ord[A] (type parameter A is constrained only by Eq + Show)`.
Diagnostics retain the `file:line:col: message` form.

Generated Go uses one dictionary argument per declared constraint:

- One constraint on a parameter: `dict_<T>` (unchanged).
- Several constraints on a parameter: `dict_<T>_<Class>`.

The same naming rule applies to top-level functions, class interfaces,
instance method parameters, instance context fields, and generalised local
bindings. Receiver contexts and explicit method/local parameters have
separate naming scopes. Call-site dictionaries retain the scheme's slot
order, including higher-order references and superclass upcasts. The
uniform `any` value ABI and the `dict_`/`ext_` reserved-prefix rules are
unchanged.

Let-generalisation now retains independent constraints on one inferred
parameter. For example, `let f = func(x) => show(x) + show(compare(x, x))`
infers `Show` and `Ord`; repeated obligations are deduplicated and implied
superclass obligations retain the existing strongest-class simplification.
Value restrictions are unchanged.

## Prelude signatures and removed limitations

No prelude names or signatures are added or changed. Removed the
one-constraint-per-parameter limitation for explicit type parameters,
inferred local schemes, and instance contexts. Obsolete checker rejection
tests were removed; the existing parser fixture was migrated to `+`.

## Verification

All commands ran from this slice's worktree. Go tests and CLI commands used
the VM-wide serialization wrapper with `SERIALIZE_VM_TESTS_MODE=shared`
(the runner selected its exclusive tier under memory pressure).

- Initial `go test ./parser ./types -run MultiConstraint -count=1` failed on
  the missing `+` grammar and the explicit/let/context single-constraint
  rejections, establishing the regression cases.
- `gofmt -w` on the changed Go files completed successfully.
- `go test ./parser ./types ./codegen` passed all three owning packages.
- `go run ./cmd/gofx run examples/multi-constraint/main.gfx` exited 0;
  observed output is recorded in the example README.
- `go run ./cmd/gofx check examples/wc/main.gfx` ran once, exited 0, and
  produced no diagnostic output.

Codegen tests also parse and type-check generated Go through the existing
source-test harness. They cover ordered dictionary signatures and call
arguments, superclass reuse, method constraints, receiver contexts,
superclass instance contexts, constrained function values, generalised
lets, and nested dictionary captures.

## Deviations

None. Multiple constraints in instance contexts follow the same rule, as
confirmed by the primary. `SPEC.md` and `DESIGN.md` are intentionally
unchanged for primary integration.

## Duplicated names

None.

## Unresolved

`DictRef` identifies a parameter by its name and class, not its rigid
variable identity. If an instance-head parameter and a method's own
parameter have the same name and class constraint, existing codegen can
confuse receiver and explicit dictionaries. This predates this slice;
tests for separate receiver/method constraints use distinct parameter
names. The primary explicitly directed that this issue be documented, not
fixed in this slice.

Minimal repro program (source-inspected; not executed in this slice):

```gofx
package main
type Box[A] = Box(A)
class Render[X] { render[A Show](x X, a A) String }
instance Render[Box[A]] given Show[A] {
  render(x, a) {
    match x { Box(value) => show(value) + show(a) }
  }
}
func main() IO[Unit] { println(render(Box(1), true)) }
```

The intended output is `1true`. [INFERENCE] Both rigid parameters produce
the same `DictRef{Param: "A", ParamClass: "Show"}` identity, so the explicit
method dictionary can be selected for `show(value)` instead of the
receiver's dictionary, invoking the `Bool` implementation on an `Int`.

