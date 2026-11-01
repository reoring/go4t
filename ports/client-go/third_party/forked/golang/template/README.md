# Native forked template helpers

This is the complete function set shipped in client-go v0.37.2's `third_party/forked/golang/template`, not the whole Go template package. Source revision: `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Original Go copyright, BSD license and patent grant are retained in the parent directory.

| Upstream | Native |
| --- | --- |
| `Equal(a, bs...)` | `equal(a Scalar, bs List[Scalar]) Result[String,Bool]` |
| `NotEqual(a,b)` | `notEqual(a,b)` |
| `Less(a,b)` | `less(a,b)` |
| `LessEqual(a,b)` | `lessEqual(a,b)` |
| `Greater(a,b)` | `greater(a,b)` |
| `GreaterEqual(a,b)` | `greaterEqual(a,b)` |
| `Indirect(reflect.Value)` | `indirect[A](reflection Reflection[A], value A) (A,Bool)` |
| `PrintableValue(reflect.Value)` | `printableValue[A](reflection Reflection[A], value A) Option[String]` |

All six comparisons are implemented in go4t. `Scalar` retains signed Int64, unsigned high/low uint32 limbs, Float64, complex real/imaginary components, Bool and String separately; `Invalid` models non-comparable values. Signed/unsigned comparison never converts through Float. Mixed numeric kinds other than signed/unsigned are incompatible, including Int versus Float. Complex supports equality but not ordering. Equal accepts multiple right operands and stops on the first match; errors encountered before a match are retained.

Stable errors: `invalid type for comparison`, `incompatible types for comparison`, `missing argument for comparison`. Failed comparisons return `Err`, rather than exposing Go's otherwise unusable accompanying zero/inverted Boolean.

`Reflection[A]` is an explicit **immutable** inspection capability. `link` reports direct values, pointers, or interfaces with their method-bearing flag and optional target. `indirect` unwraps pointers/empty interfaces, stops on a method-bearing interface, and returns true for nil. The `printable` callback exposes the adapter's already resolved textual value or None. Adapters must preserve invalid-value `<no value>`, nil printing, error/Stringer precedence (including addressable pointer methods), and channel/function rejection unless a print method applies. This native contract does not claim arbitrary host identities implement Go reflection; mutable host data must first be snapshotted in IO.

Focused fixtures cover exact integers above 2^53, max unsigned values, mixed-sign ordering, multiple equality arguments, all scalar kind/error branches, NaN, indirection, method-bearing interfaces, nil, invalid, unprintable, and explicit Stringer rendering. There are no pinned upstream test files in this fork. The sibling JSONPath module exercises these helpers through real selectors and records the complete upstream symbol inventory in `util/jsonpath/upstream-coverage.json`.

Supervisor-authorized focused verification through `serialize-vm-tests` passed **3 tests, 0 failures**. The initial check failed on an unqualified `fmap`; qualifying imported monadic operations fixed the source, and the same command passed afterward. Child command, from the worktree root:

```
go run ./cmd/go4t test ports/client-go/third_party/forked/golang/template
```
