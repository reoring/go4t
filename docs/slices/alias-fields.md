# Slice: alias-fields

## Grammar and namespace delta

`alias` is a keyword. A top-level declaration may be:

```text
AliasDecl := 'alias' Ident [ '[' Ident { ',' Ident } ']' ] '=' Type
```

Examples:

```gofx
alias Name = String
alias Pair[A] = (A, A)
alias Handler = func(Request) Response
```

Aliases share the type namespace with data types, classes, builtins, and builtin
tuple constructors; duplicates are errors in either declaration order. Alias
parameters have kind `*`, like data-type parameters, and must be distinct.
Parameters shadow global type names inside their scope. Aliases do not introduce
constructors or values.

## Type semantics

- Alias bodies expand transparently in `types/resolve.go`, before type inference,
  field checks, instance registration, and code generation. No alias-specific
  semantic or runtime type survives expansion.
- Every alias use must provide exactly its declared number of parameters. Unlike
  data type constructors, parameterized aliases cannot themselves be partially
  applied. A fully applied alias may expand to an arrow-kinded type, for example
  `alias Boxes[E] = Box[E]` when `Box` has two parameters.
- Arguments are substituted by rigid parameter identity in the alias's own
  declaration scope, not by textual substitution or the caller's local scope.
- Forward references and chains are allowed. Every declaration is validated,
  including unused aliases; direct and indirect alias cycles are errors. Recursive
  data types may still be named by aliases.
- Diagnostics display expanded types. For example, a function returning `Name`
  but producing `1` reports `type mismatch: expected String, got Int`.

Diagnostic contracts:

```text
arity mismatch for alias Pair: want 1 arguments, got 0
type alias cycle involving Loop
duplicate declaration Name
duplicate type parameter A
instance head must be a type constructor
```

Existing unknown-type and kind diagnostics are retained. Checker errors retain
source file and one-based line/column positions.

### Instance heads: approved interpretation

After expansion, an alias instance head must have the existing form: a bare type
constructor, or a constructor applied to distinct fresh variables from the
written instance head. Every written variable must occur exactly once as a direct
argument. Argument permutations are allowed, including `given` dictionaries;
partially applied constructor heads retain the existing kind rules.

Thus `alias Name = String` can name an instance for `String`, and
`alias Flipped[A, B] = Box[B, A]` can name a generic `Box` instance. The resulting
instance is keyed by the expanded constructor, so a second instance under the
original constructor name is a duplicate.

Function/variable heads, specialized or nested arguments, repeated variables, and
erased variables report `instance head must be a type constructor`. In particular,
`alias Pair[A] = (A, A)` is a valid transparent type but not a valid generic instance
head: it repeats `A` rather than describing all tuples with two independent
parameters. Main explicitly approved this boundary: broadening constructor-keyed
instance resolution into specialization/unification matching is outside this slice.

## Common-field access and updates

`x.f` and `x{f: e, ...}` now work for multi-constructor data types when every
constructor declares each requested field with the same alias-expanded type.
Field positions may differ. Type equality is checked on the constructor
declarations using the data type's rigid parameters; access does not constrain
inference variables to make different declared field types agree. Substituting
the receiver's type arguments then yields the field's instantiated type.

- Missing-field checks visit constructors in declaration order and name the first
  constructor lacking a requested field: `unknown field id in constructor Empty`.
- Missing fields are checked before incompatible field types.
- Incompatible types identify both constructors and types, for example:
  `field id has type String in constructor Rect; expected Int from constructor Circle`.
- Non-data receivers report `field id requires a record type`.
- Duplicate replacement fields, replacement type errors, and the existing
  `cannot infer record type of x.f; annotate` diagnostic remain errors.
- Updates preserve the constructor and every untouched field, allocate one result
  record, and leave the input immutable. Receiver and replacement expressions
  are evaluated once, left to right in source order.
- Code generation uses the same pointer constructor structs and `F<n>` layouts
  as match lowering, with a Go type switch for multi-constructor operations.
  Single-constructor accesses/updates retain their direct assertion fast path.

## Implementation contract surface

- `ast.AliasDecl`: `Pos`, `End`, `Name`, `TParams []string`, `Type ast.Type`;
  implements declaration membership and `GetPos`, `GetEnd`, `SetEnd` for the
  diagnostics slice's shared span interface.
- `types.RecordCase`: `Ctor *Ctor`, `Indexes []int`.
- `types.Ann.RecordCases []RecordCase`: set only for multi-constructor field
  access/update, in constructor declaration order. Indexes follow source field
  order; access has one index. Existing `Ctor` and `FieldIndex`/`FieldIndexes`
  describe the first constructor and continue to support single-constructor code.
- Alias instance bodies are associated with their resolved instance without
  rewriting the parsed head; checking the same parsed files again is supported.
- Uniform `any` ABI unchanged. No runtime or prelude additions.

## Removed limitations

Type synonyms no longer require wrapper constructors. Named field access and
immutable updates no longer require a single-constructor data type when the
requested fields satisfy the common-field rule. Obsolete single-constructor-only
record rejection tests were removed and replaced by missing-field, incompatible
type, non-record receiver, and successful multi-constructor tests.

## Verification

Run from `.wt/alias-fields`, through
`/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --`:

- `go test ./parser -run '^TestAliasDeclarations$' -count=1` — passed. The test
  waited for the VM lock and ran after syntax implementation; no red result was
  observed for this invocation.
- `go test ./lexer ./parser ./types ./codegen` — all four packages passed.
  Codegen tests parse and type-check the generated Go, including both constructor
  cases, generic fields, function/tuple aliases, and alias-headed instances.
- `go run ./cmd/gofx run examples/alias-fields/main.gfx` — passed; observed output
  is in the example README. Both constructors, generic fields at different
  positions, untouched fields, and immutable originals are exercised.
- `go run ./cmd/gofx check examples/wc/main.gfx` — exit 0, no diagnostics.

`gofmt -w` was run only on changed Go files. No full-suite test, vet, commit, push,
or live operation was performed. Generated example Go output was removed after
execution; the CLI regenerates it on demand.

## Deviations

No unapproved deviations. The instance-head interpretation above was explicitly
approved by Main and preserves the existing instance model.

## Duplicated names

None. No names owned by another slice were declared.
