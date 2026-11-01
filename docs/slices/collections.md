# Collections: Pass 12 SPEC delta

## Types and grammar

No new grammar. Three builtin opaque type constructors are always in scope:

| Constructor | Kind | Representation |
| --- | --- | --- |
| `Map[K, V]` | `(*, *) -> *` | Immutable AVL tree, ordered by the key comparator |
| `Set[A]` | `(*) -> *` | Immutable AVL tree, one entry per comparator-equivalence class |
| `Array[A]` | `(*) -> *` | Go slice with copy-on-write updates |

The type constructors are exported as `types.Map`, `types.Set`, `types.Array` and
cannot be redeclared or pattern-matched through constructors. Partial application
`Map[K]` has kind `(*) -> *`. All extern arguments/results retain the uniform `any`
ABI; `Int` indexes and sizes remain `int64` at runtime.

## Prelude API

All operations below are pure. Maps/sets use the key's `Ord` ordering, not Go
comparison or hashing. Inserting a key without `Ord` fails with
`file:line:col: no instance Ord[Key]`. Comparators must define a consistent total
ordering; keys comparing `EQ` designate the same entry.

### Maps

```gofx
emptyMap[K, V]() Map[K, V]
mapInsert[K Ord, V](m Map[K, V], k K, v V) Map[K, V]
mapLookup[K Ord, V](m Map[K, V], k K) Option[V]
mapDelete[K Ord, V](m Map[K, V], k K) Map[K, V]
mapMember[K Ord, V](m Map[K, V], k K) Bool
mapSize[K, V](m Map[K, V]) Int
mapFromList[K Ord, V](xs List[(K, V)]) Map[K, V]
mapToList[K, V](m Map[K, V]) List[(K, V)]
mapKeys[K, V](m Map[K, V]) List[K]
mapValues[K, V](m Map[K, V]) List[V]
mapAdjust[K Ord, V](m Map[K, V], k K, f func(V) V) Map[K, V]
mapAlter[K Ord, V](m Map[K, V], k K, f func(Option[V]) Option[V]) Map[K, V]
mapUnion[K Ord, V](a Map[K, V], b Map[K, V]) Map[K, V]
mapMap[K, V, W](m Map[K, V], f func(V) W) Map[K, W]
mapFold[K, V, B](m Map[K, V], init B, f func(B, K, V) B) B
groupBy[K Ord, A](xs List[A], key func(A) K) Map[K, List[A]]
```

| Operation | Semantics |
| --- | --- |
| `emptyMap` | Empty map, with no key constraint. |
| `mapInsert` | Insert/replace the key and value; preserve the old map. |
| `mapLookup` | `Some(value)` if present, otherwise `None`. |
| `mapDelete` | Remove a key; return the original tree when absent. |
| `mapMember` | Whether the key is present. |
| `mapSize` | Number of distinct keys; constant time. |
| `mapFromList` | Build from pairs; the last pair wins for duplicate keys. |
| `mapToList` | Pairs in ascending key order. |
| `mapKeys` | Keys in ascending order. |
| `mapValues` | Values in ascending key order, not sorted by value. |
| `mapAdjust` | Apply `f` exactly once to an existing value; do not call `f` if absent. |
| `mapAlter` | Call `f` exactly once on the current `Option`; `Some` inserts/replaces, `None` deletes. |
| `mapUnion` | Left-biased union; a duplicate keeps the left key/value. |
| `mapMap` | Map values in ascending-key order without needing `Ord`; keep keys and tree shape. |
| `mapFold` | Left fold in ascending-key order; an empty map returns `init`. |
| `groupBy` | Group under ascending keys, preserving input order within every group. |

Single-key operations use O(log n) comparisons and path-copying space. Size uses a
cached subtree count. Traversal/value mapping is O(n); list construction traverses
right-to-left and prepends without an intermediate slice. `groupBy` prepends then
reverses each group, avoiding quadratic per-group appends.

### Sets

```gofx
emptySet[A]() Set[A]
setInsert[A Ord](s Set[A], a A) Set[A]
setDelete[A Ord](s Set[A], a A) Set[A]
setMember[A Ord](s Set[A], a A) Bool
setSize[A](s Set[A]) Int
setFromList[A Ord](xs List[A]) Set[A]
setToList[A](s Set[A]) List[A]
setUnion[A Ord](a Set[A], b Set[A]) Set[A]
setIntersection[A Ord](a Set[A], b Set[A]) Set[A]
setDifference[A Ord](a Set[A], b Set[A]) Set[A]
```

| Operation | Semantics |
| --- | --- |
| `emptySet` | Empty set without an element constraint. |
| `setInsert` | Add an element; an equivalent existing element is retained. |
| `setDelete` | Remove an element; absent elements leave the set unchanged. |
| `setMember` | Whether an equivalent element exists. |
| `setSize` | Distinct element count; constant time. |
| `setFromList` | Deduplicate; retain the first representative of equivalent elements. |
| `setToList` | Elements in ascending order. |
| `setUnion` | Elements in either set, retaining the left representative on overlap. |
| `setIntersection` | Left elements also present in the right set. |
| `setDifference` | Left elements absent from the right set. |

### Arrays

```gofx
arrayFromList[A](xs List[A]) Array[A]
arrayToList[A](a Array[A]) List[A]
arrayLength[A](a Array[A]) Int
arrayGet[A](a Array[A], i Int) Option[A]
arraySet[A](a Array[A], i Int, v A) Array[A]
arrayMap[A, B](a Array[A], f func(A) B) Array[B]
arrayFold[A, B](a Array[A], init B, f func(B, A) B) B
arrayPush[A](a Array[A], v A) Array[A]
arraySlice[A](a Array[A], start Int, end Int) Array[A]
arraySortBy[A](a Array[A], cmp func(A, A) Ordering) Array[A]
```

| Operation | Semantics |
| --- | --- |
| `arrayFromList` | Preserve list order. |
| `arrayToList` | Preserve array order. |
| `arrayLength` | Element count; constant time. |
| `arrayGet` | Zero-based O(1) access; `None` for negative indexes or `i >= length`. |
| `arraySet` | Copy and replace at a valid index; return the original unchanged outside `[0, length)`. |
| `arrayMap` | Map in ascending index order into a new array. |
| `arrayFold` | Left fold in ascending index order; empty arrays return `init`. |
| `arrayPush` | Copy and append one element. |
| `arraySlice` | Half-open `[start, end)`; clamp both bounds to `[0, length]`; reversed/empty intervals yield an empty array. |
| `arraySortBy` | Stable sort of a copy using `Ordering`; never modify the input. |

Slices may share storage because every update, push, and sort copies before
writing. Empty slices do not retain the original backing storage.

## Instances

- `Show[Map[K, V]] given Show[K], Show[V]`: `{k: v, k2: v2}`; empty `{}`.
- `Show[Set[A]] given Show[A]`: `#{a, b}`; empty `#{}`.
- `Show[Array[A]] given Show[A]`: `#[a, b]`; empty `#[]`.
- `Eq[Map[K, V]] given Eq[K], Eq[V]`: compare ascending pair lists.
- `Eq[Set[A]] given Eq[A]`: compare ascending element lists.
- `Eq[Array[A]] given Eq[A]`: compare elements in index order.
- `Functor[Array]`: `fmap(a, f) = arrayMap(a, f)`.
- `Functor[Map[K]]`: `fmap(m, f) = mapMap(m, f)`; no constraint on the fixed key.

`Eq` and `Show` do not demand `Ord`: building an ordered nonempty map/set through
the standard API already requires it. No `Foldable` instance is added here.

## ABI and helper names

`rt.ShapeFuncs` adds:

```go
Tuple2     func(a, b any) any
Untuple2   func(p any) (a, b any)
OrderingOf func(o any) int
```

Codegen emits `rt_tuple2`, `rt_untuple2`, and (for the standard `Ordering`)
`rt_orderingOf`, registering them in the same `init` as the List/Option shapes.
`rt.Tuple2Of(a, b any) any` constructs generated pair values. `OrderingOf` maps
`LT` to -1, `EQ` to 0, `GT` to 1; an invalid value panics with
`rt.Shapes.OrderingOf: invalid Ordering`. Missing tuple/ordering registrations
panic with `rt.Shapes is not initialized: Tuple2 is required` or
`rt.Shapes is not initialized: OrderingOf is required`.

The prelude's comparator-bearing extern helpers are `mapInsertBy`, `mapLookupBy`,
`mapDeleteBy`, `mapMemberBy`, `mapFromListBy`, `mapUnionBy`, `setInsertBy`,
`setDeleteBy`, `setMemberBy`, `setFromListBy`, `setUnionBy`, `setIntersectionBy`,
and `setDifferenceBy`. Each has the corresponding public operation's arguments
plus a final `cmp func(K, K) Ordering` (or `func(A, A) Ordering`) and no `Ord`
constraint. Public ordered functions pass `func(a, b) => compare(a, b)` explicitly.
These helpers require the same comparator ordering for every operation on a given
map/set; they do not permit mixing differently ordered trees.

## Removed limitations and deviations

- The language now supports ordered maps/sets and random-access immutable arrays.
- No contract deviations. Pass 12 did not specify duplicate-key ingestion,
  array slicing boundaries, or sort stability; their choices are documented above.
- The brittle exact prelude declaration inventory was replaced with required-name
  membership assertions, as assigned by the primary; additions no longer break a
  count/order test unrelated to their semantics.

## Duplicated names

No other slice's gofx type/function names were declared. The shared tuple ABI
(`ShapeFuncs.Tuple2`, `ShapeFuncs.Untuple2`, `Tuple2Of`, `rt_tuple2`, `rt_untuple2`)
is also needed by concurrency/stdlib-extras and follows the primary's agreed exact
names; integration must keep one identical copy. `OrderingOf` is owned here.

## Verification

Commands run from the collections worktree through the VM-wide serialized runner:

- `go test ./rt -run 'Test(Map|Set|Array)'`: passed.
- `go test ./rt ./types ./codegen ./prelude`: passed (`rt` 0.661s, `types` cached,
  `codegen` 0.312s, `prelude` cached). The initial run found one whitespace-sensitive
  assertion in the new shape test; fixed by testing signature/body separately.
- `go run ./cmd/gofx run examples/collections/main.gfx`: passed; exact output is in
  `examples/collections/README.md`.
- `go run ./cmd/gofx check examples/wc/main.gfx`: passed with no diagnostics.

Runtime tests cover 100,000 ascending inserts/lookups/deletes with deterministic
comparison-count bounds (not a timing threshold), mixed-order AVL balance and
cached sizes, retained snapshots, custom comparator ordering, set algebra, array
bounds/persistence/stable sorting, and shape errors. Checker tests cover builtin
kinds, opaque redeclaration errors, missing `Ord`, and unconstrained `Eq`/`Functor`
usage. The codegen integration test compiles and runs the real generated program
against `rt`, exercising all public collection APIs and instances.
