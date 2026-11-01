# stdlib-extras: specification delta

## Scope and representation

No grammar, builtin types, or diagnostics change. All list additions and `Lazy` are
ordinary gofx definitions in `prelude/prelude.gfx`; native string and regex functions
use the existing erased `any` ABI in `rt/str.go` and `rt/regex.go`. Regexes use Go
`regexp` syntax and semantics (including replacement expansion); invalid patterns
are values, not panics. No third-party dependencies are introduced.

`type Lazy[A] = Lazy(func(Unit) A)` is an ordinary algebraic data type:

```gofx
func delay[A](f func(Unit) A) Lazy[A] // stores the thunk without evaluating its body
func force[A](l Lazy[A]) A           // calls the stored thunk with unit()
```

**Lazy is non-memoising.** Every `force` re-evaluates the pure thunk, including repeated
forces of the same `Lazy` value. Ordinary argument/closure construction remains strict;
write `delay(func(_) => expensive())`, not a precomputed value. There is no mutable
cache or new effect boundary.

List construction helpers use tail-recursive accumulators/folds where natural, with
one reversal where needed to preserve order. `sortOn` reuses stable `sortBy`,
decorates each element with its key once, compares keys with their `Ord.compare`
dictionary, then removes the keys. It does not require `Eq` or `Ord` on the elements.
Wave B perf replaced insertion sorting with stable array-backed sorting; see
`perf.md` for the updated complexity and removal of the internal `insertBy` helper.
`nub` retains first occurrences using `Eq` and is quadratic in the worst case.

## New list signatures

```gofx
zipWith[A, B, C](xs List[A], ys List[B], f func(A, B) C) List[C] // pairwise map to the shorter length
unzip[A, B](xs List[(A, B)]) (List[A], List[B])                 // separate the pair components in order
any[A](xs List[A], p func(A) Bool) Bool                        // short-circuit at the first true predicate
all[A](xs List[A], p func(A) Bool) Bool                        // short-circuit at the first false predicate
find[A](xs List[A], p func(A) Bool) Option[A]                  // first satisfying element
findIndex[A](xs List[A], p func(A) Bool) Option[Int]           // zero-based index of the first match
partition[A](xs List[A], p func(A) Bool) (List[A], List[A])    // stable (satisfying, not satisfying) lists
nub[A Eq](xs List[A]) List[A]                                 // stable removal of duplicate elements
range(lo Int, hi Int) List[Int]                              // ascending half-open interval [lo, hi)
replicate[A](n Int, x A) List[A]                              // n copies of x
intercalate[A](sep List[A], xss List[List[A]]) List[A]         // flatten with sep between adjacent lists
flatten[A](xss List[List[A]]) List[A]                         // concatenate in outer and inner order
last[A](xs List[A]) Option[A]                                // final element if present
init[A](xs List[A]) List[A]                                  // all but the final element
splitAt[A](n Int, xs List[A]) (List[A], List[A])              // prefix of at most n elements and remaining suffix
span[A](xs List[A], p func(A) Bool) (List[A], List[A])         // longest satisfying prefix and remaining suffix
takeWhile[A](xs List[A], p func(A) Bool) List[A]               // longest satisfying prefix
dropWhile[A](xs List[A], p func(A) Bool) List[A]               // suffix starting at the first predicate failure
lookup[K Eq, V](k K, xs List[(K, V)]) Option[V]               // value of the first equal key
indexed[A](xs List[A]) List[(Int, A)]                         // pair each element with its zero-based index
iterate[A](n Int, x A, f func(A) A) List[A]                   // n values x, f(x), f(f(x)), ...
product(xs List[Int]) Int                                    // left product with identity 1
count[A](xs List[A], p func(A) Bool) Int                      // number of satisfying elements
unfold[S, A](seed S, step func(S) Option[(A, S)]) List[A]     // emit (value, next seed) until step returns None
chunksOf[A](n Int, xs List[A]) List[List[A]]                  // consecutive chunks of at most n elements
sortOn[A, K Ord](xs List[A], key func(A) K) List[A]           // stable ascending order by compare(key(a), key(b))
```

No plain `foldr` is declared: this example and implementation do not need it. The
`Foldable` method remains owned by the deriving slice.

## New string/regex signatures

All are externs to the correspondingly capitalised name in `gofx/rt`.

```gofx
codePoint(s String) Option[Int]                  // first Unicode code point, if present
fromCodePoint(n Int) String                      // one Unicode scalar value, or U+FFFD if invalid
isDigit(s String) Bool                           // non-empty, every rune satisfies unicode.IsDigit
isAlpha(s String) Bool                           // non-empty, every rune satisfies unicode.IsLetter
isSpace(s String) Bool                           // non-empty, every rune satisfies unicode.IsSpace
isUpper(s String) Bool                           // non-empty, every rune satisfies unicode.IsUpper
isLower(s String) Bool                           // non-empty, every rune satisfies unicode.IsLower
indexOf(s String, sub String) Option[Int]        // rune index of the first substring occurrence
padRight(s String, width Int) String             // right-pad with ASCII spaces to a rune width
padLeft(s String, width Int) String              // left-pad with ASCII spaces to a rune width
reverseString(s String) String                  // reverse Unicode code points, not grapheme clusters
splitOnce(sep String, s String) Option[(String, String)] // split before/after the first separator
charAt(s String, i Int) Option[String]           // one-rune string at zero-based index i
showQuoted(s String) String                     // strconv.Quote: surrounding quotes and Go escapes
regexMatch(pattern String, s String) Bool        // whether any match exists; use anchors for whole-input matching
regexFind(pattern String, s String) Option[String] // first complete match, including an empty match
regexFindAll(pattern String, s String) List[String] // all non-overlapping complete matches
regexReplace(pattern String, s String, repl String) String // replace all matches with Go replacement expansion
regexSplit(pattern String, s String) List[String] // Go regexp.Split(s, -1)
regexCaptures(pattern String, s String) Option[List[String]] // first match's groups, excluding the whole match
```

## Boundary behaviour: every prelude list operation

`Nil` and `[]` are the same value. Results preserve order unless sorting/reversal is
specified. Predicates and callbacks are ordinary strict pure functions; short-circuiting
means not calling them on later elements. The table covers existing operations as well
as the additions; Option/State-only helpers are not list operations.

| Function(s) | Empty/singleton and other boundary behaviour |
|---|---|
| `Show[List[A]].show` | Empty renders `"[]"`; singleton renders `"[" + show(x) + "]"`; strings use their unquoted `Show` representation. |
| `Eq[List[A]].eq` | Two empty lists equal; empty/nonempty unequal; corresponding elements compared via `Eq`, with length mismatch unequal. |
| `Ord[List[A]].compare` | Empty precedes nonempty; lexicographic element comparison; equal shared prefixes compare by length. |
| `map` / `fmap` on List | `Nil → Nil`, callback not called; `[x] → [f(x)]`. |
| `flatMap` / `bind` / `concatMap` on List | `Nil → Nil`, callback not called; singleton returns `f(x)`; empty produced lists contribute no elements. `pure(x)` for List is `[x]`. |
| `forEach` | `Nil → pure(unit())`, callback not called; singleton sequences one action; short-circuits according to the chosen Monad. |
| `mapM` | `Nil → pure(Nil)`, callback not called; singleton maps one action and wraps its value in a singleton list; preserves order. |
| `fold` | `fold(Nil, init, f) = init`; singleton is `f(init, x)`; left-to-right fold. |
| `length` | `length(Nil) = 0`; singleton length is 1. |
| `append` | `append(Nil, ys) = ys`; `append(xs, Nil) = xs` by value. |
| `filter` | `filter(Nil, p) = Nil`; singleton kept iff `p(x)`; order preserved. |
| `reverse` | Empty and singleton unchanged by value. |
| `zip` | Either input empty gives `Nil`; truncates to the shorter input, including singleton inputs. |
| `head` | `head(Nil) = None`; `head([x]) = Some(x)`; later elements ignored. |
| `tail` | `tail(Nil) = Nil`; `tail([x]) = Nil`, never an empty-list panic. |
| `take` | `take(n <= 0, xs) = Nil`, including `take(-1, xs)` and `minInt()`; empty input stays empty; `n > length(xs)` returns all elements. |
| `drop` | `drop(n <= 0, xs) = xs`; empty input stays empty; `drop(n >= length(xs), xs) = Nil`. |
| `sum` | `sum(Nil) = 0`; singleton is its element; signed Int overflow wraps. |
| `elem` | Empty gives false; singleton compares with `Eq`; stops at the first match. |
| `sortBy` | Empty/singleton unchanged; comparator not called for these inputs; equal elements retain input order. |
| `sort` | Same boundaries as `sortBy`; uses the element's `Ord.compare`. |
| `maximum` / `minimum` | Empty gives `None`; singleton gives `Some(x)`; ties retain the earliest element. |
| `zipWith` | Either list empty gives `Nil` with no callback; singleton pair gives `[f(x, y)]`; truncates to the shorter length. |
| `unzip` | Empty gives `(Nil, Nil)`; singleton `(a,b)` gives `([a], [b])`; order preserved. |
| `any` | Empty is false; singleton is `p(x)`; first true result short-circuits. |
| `all` | Empty is true; singleton is `p(x)`; first false result short-circuits. |
| `find` | Empty/no match gives `None`; singleton match gives `Some(x)`; first match wins. |
| `findIndex` | Empty/no match gives `None`; singleton match gives `Some(0)`; indices are zero-based. |
| `partition` | Empty gives `(Nil, Nil)`; singleton goes into exactly one side; each side retains input order. |
| `nub` | Empty/singleton unchanged; first representative of each `Eq` equivalence class retained. |
| `range` | `range(5, 1)` and `range(n, n)` give `Nil`; `range(n, n+1) = [n]`; `range(-2, 2) = [-2,-1,0,1]`. Half-open endpoints do not increment past `maxInt()` or decrement below `minInt()`. |
| `replicate` | `n <= 0` gives `Nil`, including `replicate(-1, x)` and `minInt()`; `n = 1` gives `[x]`. |
| `intercalate` | Empty outer list gives `Nil`; singleton outer list gives its inner list; empty separator is ordinary flattening. Empty inner lists still have separators between them: `intercalate([0], [[], []]) = [0]`. |
| `flatten` | Empty outer list or all-empty inner lists give `Nil`; singleton outer list gives its inner list. |
| `last` | Empty gives `None`; singleton gives `Some(x)`. |
| `init` | Empty and singleton give `Nil`; longer lists lose exactly the last element. |
| `splitAt` | `n <= 0` gives `(Nil, xs)`; `n >= length(xs)` gives `(xs, Nil)`; empty input gives `(Nil, Nil)` for every n. |
| `span` | Empty gives `(Nil, Nil)`; immediate predicate failure gives `(Nil, xs)`; all true gives `(xs, Nil)`; stops at first false. |
| `takeWhile` | Empty or immediate predicate failure gives `Nil`; all true returns all elements; stops at first false. |
| `dropWhile` | Empty or all true gives `Nil`; immediate predicate failure returns original list; stops at first false. |
| `lookup` | Empty or absent key gives `None`; duplicate keys return the first value; key equality uses `Eq`. |
| `indexed` | Empty gives `Nil`; singleton gives `[(0, x)]`. |
| `iterate` | `n <= 0` gives `Nil` without calling `f`; `n = 1` gives `[x]` without calling `f`; positive n invokes `f` exactly n-1 times. |
| `product` | Empty gives 1; singleton is its element; zero produces zero; Int multiplication overflow wraps. |
| `count` | Empty gives 0; singleton gives 1 or 0 according to its predicate. |
| `unfold` | Immediate `None` gives `Nil`; one `Some((x, seed))` followed by `None` gives `[x]`; a step that never returns `None` does not terminate. |
| `chunksOf` | Empty gives `Nil`; `n <= 0` gives `Nil` (including `chunksOf(0, xs)` and negative/minimum counts); singleton positive input gives `[[x]]`; positive n beyond the length gives `[xs]` for nonempty xs; incomplete final chunks are retained. |
| `sortOn` | Empty gives `Nil` without calling key; singleton returns its value and computes its key once; keys compare through `Ord` and equal keys preserve input order. |

### Accumulator helpers

Without modules/private declarations these implementation helpers are also in the shared
prelude value namespace. They do not introduce alternative APIs: public entrypoints above
supply the initial accumulator/index. `rev(acc)` below means `reverse(acc)`.

| Helper | Boundary/result rule |
|---|---|
| `zipWithAcc(xs, ys, f, acc)` | Either input empty returns `rev(acc)`; otherwise emits pairs as `zipWith`, following that prefix. |
| `findIndexFrom(xs, p, i)` | Empty/no match gives `None`; first match at offset j gives `Some(i+j)`. |
| `rangeAcc(lo, hi, acc)` | `lo >= hi` returns acc; otherwise ascending `[lo, hi)` prepended to acc. |
| `replicateAcc(n, x, acc)` | `n <= 0` returns acc; positive n prepends n copies. |
| `initAcc(xs, acc)` | Empty/singleton returns `rev(acc)`; otherwise that prefix followed by `init(xs)`. |
| `splitAtAcc(n, xs, acc)` | `n <= 0` gives `(rev(acc), xs)`; exhaustion gives `(rev(acc) ++ xs, Nil)` by value; otherwise accumulates the taken prefix before reversing. |
| `spanAcc(xs, p, acc)` | Empty gives `(rev(acc), Nil)`; immediate failure gives `(rev(acc), xs)`; otherwise extends prefix up to first failure. |
| `indexedFrom(xs, i, acc)` | Empty returns `rev(acc)`; otherwise that prefix followed by elements indexed starting at i. |
| `iterateAcc(n, x, f, acc)` | `n <= 0` returns `rev(acc)`; otherwise that prefix followed by `iterate(n,x,f)`, still no extra final callback. |
| `unfoldAcc(seed, step, acc)` | Immediate `None` returns `rev(acc)`; otherwise that prefix followed by the unfolding. |
| `chunksOfAcc(n, xs, acc)` | Empty or `n <= 0` returns `rev(acc)`; otherwise that prefix followed by the chunks. |

## Boundary behaviour: every prelude pure string operation

Positions and widths are Unicode code points (runes), **not** bytes, grapheme clusters,
or terminal display columns. Combining marks count independently. Literal substring
matching (`contains`, `indexOf`, `split`, etc.) uses Go string matching; no Unicode
normalisation is performed.

| Function(s) | Boundary behaviour |
|---|---|
| `showInt` / `showBool` | Int64 signed decimal (`0 → "0"`, minimum Int includes its minus sign); Bool renders `"true"` or `"false"`. |
| `showString` / `Show[String].show` | Identity, including empty strings; does not add quotes. |
| `eqString` / `Eq[String].eq` | Exact byte equality; empty equals empty. |
| `ltString` / `Ord[String].compare` | Go lexicographic byte order; empty precedes every nonempty string; equal strings compare `EQ`. |
| String `+` / interpolation | Concatenate; empty text is identity; interpolation uses `Show`, not quoted display. |
| `strLength` | Empty is 0; one astral rune is 1; `e` plus combining accent is 2. |
| `strConcat` | Empty list is `""`; singleton is its element; empty elements contribute nothing. |
| `join` | Empty list is `""`; singleton is its element without separator; empty elements still delimit fields. |
| `split` | Absent separator gives `[s]`; nonempty separator on empty input gives `[""]`; adjacent/trailing separators retain empty fields. `split("", s)` yields one-rune strings; `split("", "") = Nil`. |
| `lines` | Splits only on `\n` and drops exactly one trailing empty field. Empty input gives `Nil`; `"\n" → [""]`; `"a\n\n" → ["a", ""]`; CR is retained in CRLF input. |
| `words` | Empty/Unicode-whitespace-only input gives `Nil`; runs of Unicode whitespace delimit nonempty fields. |
| `trim` | Removes Unicode whitespace only at the ends; empty or all-whitespace becomes `""`. |
| `toUpper` / `toLower` | Empty unchanged; Unicode simple case mapping, not locale-specific or full case folding. |
| `startsWith` / `endsWith` / `contains` | Empty prefix/suffix/substring matches every string, including empty; a nonempty needle does not match empty input. |
| `replace` | All non-overlapping literal occurrences; absent old value leaves s unchanged. Empty old inserts new at the start, between runes and at the end; `replace("", "", "-") = "-"`. |
| `substring` | Half-open rune interval with bounds clamped to `[0, length]`; reversed/equal ranges empty. `substring("a日本", -1, 2) = "a日"`; end past length clamps; start past length or negative end yields `""`; even Int extrema are safe. |
| `chars` | Same as `split("", s)`; empty gives `Nil`. |
| `repeat` | `n <= 0` gives `""` (negative counts fixed to stop panicking); empty s gives `""`; positive n repeats exactly n times. |
| `parseInt` | Signed base-10 Int64. `"+5" → Some(5)`; leading zeros allowed; both signed bounds accepted. Overflow, empty, surrounding whitespace, underscores, decimal fractions, non-ASCII digits and `0x` notation give `None`. |
| `showPadLeft` | Decimal number including sign, right-aligned with spaces; width <= decimal length (including any negative width) leaves the decimal unchanged, never truncates. |
| `codePoint` | Empty gives `None`; otherwise first rune only, e.g. `"日a" → Some(26085)`; malformed UTF-8 decodes as U+FFFD. |
| `fromCodePoint` | Valid Unicode scalar gives its one-rune string, including NUL at 0. Negative, > U+10FFFF, or surrogate U+D800..U+DFFF gives U+FFFD; large Ints are checked before conversion, never wrap into another character. |
| `isDigit` | Empty false; every rune must be Unicode decimal digit, e.g. `"12٣９"` true, `"1a"` false. |
| `isAlpha` | Empty false; every rune must be Unicode letter, e.g. `"é日本Z"` true, `"a1"` false. |
| `isSpace` | Empty false; every rune must be Unicode whitespace, e.g. ASCII whitespace and U+2003 true, `" a"` false. |
| `isUpper` / `isLower` | Empty false; every rune must have the respective Unicode case property; digits, uncased letters, punctuation or the opposite case make the result false. |
| `indexOf` | First match in rune indices; no match gives `None`; empty sub gives `Some(0)` even on empty input; `indexOf("日🙂本", "🙂") = Some(1)`. |
| `padRight` / `padLeft` | Pad with spaces only when width exceeds rune count; nonpositive/minimum Int widths leave s unchanged; never truncate; empty string pads to positive width. |
| `reverseString` | Empty/singleton unchanged; reverses code points, so combining accents move independently of their base characters. |
| `splitOnce` | First occurrence only; no match gives `None`; separators at an end produce an empty side. Empty separator gives `Some(("", s))`, even for empty s. |
| `charAt` | Empty, negative/minimum Int index, or index >= rune count gives `None`; valid index gives exactly one rune; malformed UTF-8 decodes as U+FFFD. |
| `showQuoted` | Empty gives `"\"\""`; escapes quotes, backslashes, control characters using Go string syntax; printable Unicode remains readable. |
| `regexMatch` | Searches anywhere; empty pattern matches even empty input; no match or invalid pattern gives false. |
| `regexFind` | No match or invalid pattern gives `None`; an actual empty match gives `Some("")`, not `None`. |
| `regexFindAll` | No match or invalid pattern gives `Nil`; returns non-overlapping matches in input order. Empty pattern on `"日🙂"` gives `["", "", ""]`; on empty input gives `[""]`. Adjacent empty-match handling follows Go regexp. |
| `regexReplace` | Invalid pattern/no match leaves input unchanged; replacement uses `$1`, `${name}`, `$$` expansion, with absent groups expanded to empty. Empty pattern inserts at rune boundaries, including both ends. |
| `regexSplit` | Invalid pattern gives `Nil`; no delimiter match gives `[s]`; matched leading/trailing nonempty delimiters retain empty fields. Empty pattern splits nonempty input into runes. `regexSplit("", "") = Nil`, while a nonempty valid pattern on empty input gives `[""]` (Go regexp's explicit empty-input rule). |
| `regexCaptures` | Invalid pattern/no match gives `None`; first match's capture groups only, no whole-match element; groups ordered by opening parentheses. Unmatched optional group gives `""`; a matching pattern with no groups gives `Some(Nil)`, including empty pattern/input. |

### Existing effectful text/list entrypoints

These are unchanged; their boundaries are read from `rt/io.go` and covered by the
existing runtime IO tests, not replaced with pure fallbacks.

| Function | Boundary behaviour when the IO is run |
|---|---|
| `println` | Empty text still prints a newline; output errors fail the IO. |
| `readLine` | Empty EOF gives `None`; a blank line gives `Some("")`; LF/CRLF terminators are stripped; a final unterminated nonempty line is `Some(line)`; other read errors fail the IO. |
| `readAll` | Empty remaining input gives `""`; includes bytes already buffered by prior `readLine` calls; read errors fail the IO. |
| `args` | No program arguments gives `Nil`; executable name excluded; empty argument strings retained. |
| `getEnv` | Absent name gives `None`; present empty value gives `Some("")`; values are not trimmed. |
| `ioFail` | Every message, including empty text, constructs a failing deferred IO; no output or failure occurs merely by constructing it. |

### Integer and resource boundaries relevant to these helpers

| Operation | Actual runtime behaviour (preserved unless explicitly stated) |
|---|---|
| Int `/` | Truncates toward zero; division by zero intentionally raises Go's `runtime error: integer divide by zero`, per the existing language contract. `minInt() / -1 = minInt()` by wrapping. |
| Int `%` | Remainder has dividend's sign; zero divisor intentionally raises the same divide-by-zero panic; `minInt() % -1 = 0`. |
| Int `+`, `-`, `*`, negation, `abs` | Int64 wraparound; `abs(minInt()) = minInt()`. No arbitrary-precision arithmetic. |
| Huge positive counts/widths | Requests requiring unrepresentable string sizes can panic in Go strings allocation/repetition; otherwise allocations and recursion are bounded by host resources. No invented fallback or silent truncation is added for resource exhaustion. Ordinary empty/negative count and index inputs are total. |

## Removed limitations and deviations

- Added the specified pure list utilities, Unicode inspection/padding/indexing, regex
  operations, and explicit delayed pure values without new language machinery.
- Fixed the existing `repeat` negative-count panic; nonpositive counts now return empty
  text, consistently with list count-based constructors.
- Preserved intentional Int division/remainder-by-zero errors rather than inventing a
  numeric sentinel. They are not ordinary empty/count input handling.
- Pass 12 did not specify the full parameter types/order of all list utilities or capture
  content. The signatures above make them explicit; the primary approved `unfold`'s
  seed-first protocol, `iterate`'s length/initial-value semantics, generic list
  `intercalate`, capture groups without the full match, invalid regex lists as `Nil`,
  and invalid replacement as unchanged input. These are contract clarifications, not
  narrowed functionality.
- No `SPEC.md` or `DESIGN.md` changes and no extra numeric, effect, or Foldable APIs.

## Duplicated names / shared integration hunks

No cross-slice **prelude** names are duplicated; in particular no `foldr`.
For standalone `splitOnce` tuple construction, duplicated the primary-approved shared
shape bridge also needed by concurrency and collections:

- `rt.ShapeFuncs.Tuple2 func(a, b any) any`
- `rt.ShapeFuncs.Untuple2 func(p any) (a, b any)`
- `rt.Tuple2Of(a, b any) any`; unset constructor panics with
  `rt.Shapes is not initialized: Tuple2 is required` (programming/configuration error).
- Generated `rt_tuple2` / `rt_untuple2`, registered in the existing valid List/Option
  shapes init block; the same gate ensures the generated `Tuple2` struct is present.
- Corresponding fake-importer shape fields in codegen tests and tuple test shapes.

Keep one copy on integration; this is one shared value-shape protocol, not a new ABI.

## Verification

- Regression red: `go test ./rt -run 'TestStringFunctions/repeat_negative$' -count=1`
  failed with `panic: strings: negative Repeat count` before the native guard.
- `go test ./rt ./prelude ./codegen`: **passed** (`rt` and `codegen` cached on the final
  run; `prelude` 0.663s). Covers runtime boundaries, generated list behaviour, Lazy
  non-evaluation/re-evaluation, tuple round-trip, key class/type errors and shape tests.
- `go run ./cmd/gofx run examples/stdlib-extras/main.gfx`: **passed**, exit 0; observed
  output matches the example README, ending with `deferred checksum: 50005000`.
- `go run ./cmd/gofx check examples/wc/main.gfx`: **passed**, exit 0, no diagnostics;
  run exactly once.

Go checks ran through
`/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --`.
Only owned Go changes were formatted with `gofmt`; no repo-wide tests/lint/vet were run.
The first combined test run exposed an incorrect new expectation for
`regexSplit("", "")`; Go regexp's source confirms its empty list. A subsequent run
exposed a test-only diagnostic-format assumption: `types.Error.Error()` prints the
position/message, while the filename is carried separately in `types.Error.File`.
The corrected test checks File/Pos/Msg without changing the existing diagnostic ABI.
Both fixtures now pass. Generated smoke output was removed; no throwaway scripts remain.
