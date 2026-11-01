# json-deriving — Wave B specification delta

## Surface and placement

```gofx
type Json = JNull | JBool(Bool) | JNum(Float) | JStr(String) | JArr(List[Json]) | JObj(Map[String, Json])
class Encode[A] { encode(a A) Json }
class Decode[A] { decode(j Json) Result[String, A] }
toJson(j Json) String
toJsonPretty(j Json) String
parseJson(s String) Result[String, Json]
```

All language declarations, instances, and private `_json*` helpers live in
`std/json/json.gfx`. Import `codec "std/json"` and qualify names, including
`deriving codec.Encode, codec.Decode`; no JSON name is an implicit prelude export.
Runtime implementation is `rt/json.go`. No CLI syntax changes. Canonical intrinsic
identities bind synthesized helper calls and runtime shapes to std/json, while
ordinary source resolution still enforces imports and private-name visibility.
Derived constructor tags and error type labels retain source spelling across
module alpha-renaming, using the same name map as derived Debug.

`toJson` is compact; `toJsonPretty` uses two spaces, without a trailing newline.
Strings use JSON escaping, including encoding/json's HTML and U+2028/U+2029 escaping.
`parseJson` uses `encoding/json.Decoder` with `UseNumber`, consumes exactly one
value, accepts surrounding whitespace, and returns `Err("invalid JSON: " + reason)`
for malformed/trailing input or a number outside Float's finite range. Exponents
are accepted; numerical spellings are normalized on printing.

## Encoding and decoding

Prelude instances exist for **Int, Float, String, Bool, Unit, Option[A], List[A],
Array[A], Map[String, V], tuples of every arity >= 2, and Json itself**. Container
instances require the corresponding class for their elements/values. Json's
instances are identity / `Ok(j)`.

- Int and Float encode as numbers; String and Bool use their JSON counterparts.
- Unit encodes as null and decodes only null.
- None encodes as null; Some encodes its contained value without a wrapper. Null
  always decodes to None. Consequently Some(unit()), Some(None), and Some(JNull)
  are intentionally indistinguishable from None in this representation.
- List, Array, and tuples encode as arrays. Tuple decoding requires exact length.
- Map encodes as an object with lexicographically sorted String keys, including
  maps built with a custom comparator through the existing `By` APIs.
- Parsed objects preserve input member order when printed as Json. Duplicate
  keys keep their first position and last value, consistently with a unique-key
  Map payload. Derived records preserve declaration order, independently of
  input object order during decoding.
- Parsed/derived object Maps have optional immutable source-order metadata.
  Ordinary Map traversal still uses key order. Ordinary Map operations discard
  the metadata rather than mutate it; Encode[Map] always prints sorted keys.
- Numbers are Float throughout Json. Int encoding uses the existing `toFloat`:
  integers beyond binary64's exact range can round. Int decoding requires a
  finite, integral Float in [-2^63, 2^63); it never truncates or overflows.
  In particular, encoding maxInt() rounds to 2^63, which cannot decode as Int.
- JSON cannot represent NaN or infinity. Printing either panics with
  `cannot encode non-finite JSON number`; the String-returning print API does
  not silently substitute null. Parsing never constructs non-finite numbers.

## Deriving

The existing clause accepts `deriving Encode, Decode`, independently or together
with other supported classes. Expansions in `types/derive.go` are ordinary checked
instances; no reflective runtime decoding is used. Each type parameter receives
`given Encode[A]` / `given Decode[A]`, including phantom parameters. Recursive
and nested fields use their ordinary class dictionaries.

| Constructor shape | JSON representation |
| --- | --- |
| Sole record `Item{sku: String, qty: Int}` | `{"sku":"x","qty":2}` |
| Record in a sum `Named{name: String}` | `{"$tag":"Named","name":"x"}` |
| Positional `Shipped(String)` | `{"$tag":"Shipped","$values":["track"]}` |
| Nullary `Pending` | `{"$tag":"Pending"}` |

Tags precede fields or `$values`. Positional payload arrays require the exact
constructor arity. Records require every declared field, including Option fields
(use explicit null for None); unknown object fields are ignored. All derived
forms require an object, even single positional/nullary constructors. The existing
grammar still rejects zero-field record syntax; use a nullary constructor.

Function fields, including functions nested in a syntactic tuple or container,
reject derivation with `cannot derive Encode for T: field of function type` (or
Decode). Other unsupported fields, including aliases hiding a function, fail normal
instance resolution. Duplicate deriving classes and explicit-instance conflicts
retain the existing diagnostics.

## Decode errors and paths

Errors use `expected <what> at <path>`, rooted at `$`. Field segments use `.name`
for identifiers and JSON-quoted bracket notation for other keys; indexes are
zero-based. Container/derived decoders preserve the first error and prefix its
path without losing nested segments. Examples:

```text
expected String at $.items[2].sku
expected Int at $["a.b"]
expected field at $.sku
expected array of length 2 at $
expected Status constructor at $["$tag"]
expected String at $["$values"][0]
```

Primitive expectations are `Int`, `Float`, `String`, `Bool`, or `null`. Shape
expectations are `array` / `object`; missing fields report `field` at the missing
field's path. Unknown tags report `<Type> constructor`. Record fields are decoded
in declaration order; lists/arrays/tuples in index order; Map values in key order.

## Checker and runtime integration

Main approved these supporting contract extensions:

1. **Concrete nullary builtin arguments in direct instance heads**, e.g.
   `instance Encode[Map[String, V]] given Encode[V]`. Int, Float, String, Bool,
   and Unit are concrete; other names remain fresh head variables. Arbitrary
   user-defined concrete arguments are not supported: resolving a user's type
   `A` globally would accidentally specialize the prelude's generic `A` heads.
   Supporting them requires a separate scope/syntax decision. Coherence remains
   one instance per (class, outer constructor), so specialized and generic heads
   cannot overlap. Alias-head expansion retains its previous restrictions.
   `types.Instance.HeadArgs` retains concrete arguments and variable positions;
   method checking, dictionary matching, and superclass dictionaries use it.
2. **Unbounded tuple instances**: the pair instances live in `std/json`;
   `expandJSONTuples` materializes ordinary Encode/Decode instances for other
   tuple arities appearing in the input AST. It is deterministic, repeat-check
   safe, and preserves explicitly supplied tuple instances.
3. **Optional immutable order metadata on rt mapValue**: only parsed/derived
   objects carry it, while ordinary Map semantics remain unchanged.

`rt.ShapeFuncs` adds JNull `func() any`, JBool/JNum/JStr/JArr/JObj `func(any) any`,
and JsonView `func(any) (kind string, value any)`. View kinds are `null`, `bool`,
`number`, `string`, `array`, and `object`. `codegen/json.go` registers the generated
ADT constructors/destructor after the List/Option and Result initializers, so
shape assignment cannot erase them; generated function literals retain `//line`.
All extern functions keep the uniform any ABI. Module integration must preserve
these intrinsic ctor lookups and resolve generated `_json*` helper references.

## Verification and cleanup

Commands run from this worktree with prefix:

```text
env SERIALIZE_VM_TESTS_MODE=shared /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --
```

- Red: `go test ./rt -run TestJSON -count=1` failed because JSON shapes/functions
  did not exist.
- `go test ./rt ./types ./prelude ./codegen`: passed after fixing shape-literal
  source directives and removing an invalid empty-record test fixture.
- `go run ./cmd/gofx run examples/json-deriving/main.gfx`: round-trip true, pretty
  order, both Status forms, and `expected String at $.items[2].sku` (full output in
  the example README).
- `go run ./cmd/gofx check examples/wc/main.gfx`: exit 0, no diagnostics.

Tests cover parser/printing escapes, Unicode, exponent and invalid numbers,
ordered/duplicate/nested objects, Map order and persistence, builtin round-trips,
all deriving shapes, generic recursion, tuple arities, concrete-head rejection,
repeat checking, function-field diagnostics, and exact nested decode paths.
Only written Go files were gofmt'd; no full suite, vet, commits, or pushes. The
example's generated main_gen output is removed. No temporary scripts remain.

No unresolved implementation blocker. The std/json move is the expressly required
module-integration step; numeric and Option representation losses above are
inherent in the fixed JSON representation, not hidden round-trip guarantees.
