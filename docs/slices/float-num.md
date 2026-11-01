# float-num SPEC delta

## Grammar and representation

- Add builtin `Float : *`, exposed by `types.Float`, represented by Go `float64` inside the existing uniform `any` ABI.
- Add token `lexer.Float` and expression node `ast.FloatLit` with a `float64` value.
- Decimal Float literals have a fractional part and/or an exponent:

  ```text
  Digits   := Digit { Digit }
  Exponent := ('e' | 'E') ['+' | '-'] Digits
  Float    := Digits '.' Digits [Exponent] | Digits Exponent
  ```

  Examples: `1.5`, `2e3`, `1.0e-3`, `2E+3`. Neither `1.` nor `.5` is a Float literal. A number after a field-access dot remains an Int index, including `t.0`, `t.1`, and nested `t.0.1`.
- Float tokens end statements under the existing layout rule. Float literals work in expressions, interpolation, and literal patterns (patterns still require a trailing catch-all).
- Source literals use `strconv.ParseFloat(..., 64)` after decimal lexing; overflow is a positioned parse error. Malformed exponents report `bad float literal: exponent requires digits`; an out-of-range literal reports `bad float literal <text>`.
- Int literals retain type Int. There is no implicit Int-to-Float coercion or numeric-literal defaulting. `1 + 1.5` is rejected with `type mismatch: expected Int, got Float`.

## Operators and instances

- Binary `+ - * /` accept Int/Int or Float/Float and return that same type; `+` also retains String concatenation. `%` remains Int-only. Unary `-` accepts Int or Float.
- Arithmetic selects its concrete runtime implementation after inference, including contextual lambdas and later uses of a local lambda. These operands remain monomorphic, not type-class-polymorphic. Unconstrained arithmetic operands retain the previous Int default.
- Float operators call `rt.AddFloat`, `SubFloat`, `MulFloat`, `DivFloat`, and `NegFloat`. Division uses IEEE 754 semantics: nonzero/zero gives signed infinity and zero/zero gives NaN, without a division-by-zero panic. Unary negation preserves signed-zero semantics.
- `Show[Float]` uses `strconv.FormatFloat(f, 'g', -1, 64)`, including `NaN`, `+Inf`, and `-Inf`. Consequently `${1.5}` renders `1.5`.
- `Eq[Float]` uses IEEE equality: NaN never equals any value, including another NaN; positive and negative zero are equal.
- `Ord[Float]` places NaN below every non-NaN, including negative infinity. Two NaNs compare as `EQ` even though their `Eq` result is false. Other values use numeric order, with signed zeros equivalent.
- `Debug[Float]` delegates to `show`, coordinated with the deriving slice.
- Invalid arithmetic reports `addition requires Int, Float or String, got T` for `+`, and `arithmetic requires Int or Float, got T` for other overloaded arithmetic. Ordinary unification mismatches retain their existing wording and source positions. CLI diagnostic formatting is unchanged.

## Prelude signatures and semantics

All functions are pure. Runtime-backed functions use the existing erased extern ABI.

| Signature | Semantics |
|---|---|
| `toFloat(n Int) Float` | Convert signed int64 to float64; large integers may round. |
| `truncate(f Float) Int` | Convert toward zero using Go's float64-to-int64 conversion. |
| `floor(f Float) Int` | Round toward negative infinity, then convert to Int. |
| `ceil(f Float) Int` | Round toward positive infinity, then convert to Int. |
| `round(f Float) Int` | Round to nearest integer, ties away from zero, then convert to Int. |
| `sqrt(f Float) Float` | Go `math.Sqrt`; negative finite inputs produce NaN. |
| `pow(x Float, y Float) Float` | Go `math.Pow`, including its IEEE special-value behavior. |
| `powInt(x Int, y Int) Int` | Integer exponentiation by squaring; overflow wraps like Int multiplication; `0^0 = 1`. Negative exponents truncate reciprocal powers toward zero: base 1 returns 1, base -1 returns ±1 by exponent parity, every other base (including 0) returns 0. No negative-exponent panic. |
| `parseFloat(s String) Option[Float]` | Go `strconv.ParseFloat(s, 64)`; syntax/range errors return None; valid NaN/infinities return Some. |
| `showFloatFixed(f Float, decimals Int) String` | Go `strconv.FormatFloat(f, 'f', decimals, 64)`; ties-to-even decimal formatting, trailing zeros preserved; negative precision selects the shortest fixed form. |
| `min[A Ord](a A, b A) A` | Written in gofx: return a when `a <= b`, otherwise b; ties choose a. |
| `max[A Ord](a A, b A) A` | Written in gofx: return a when `a >= b`, otherwise b; ties choose a. |
| `absFloat(f Float) Float` | Go `math.Abs`, including converting negative zero to positive zero. |
| `nan() Float` | Produce an IEEE NaN. |
| `isNaN(f Float) Bool` | True exactly for IEEE NaN values. |

Backing prelude extern names follow existing Int/String conventions: `showFloat(f Float) String`, `eqFloat(a Float, b Float) Bool`, and `ltFloat(a Float, b Float) Bool`; `ltFloat` implements the NaN-low strict order used by `Ord`.

`parseFloat` accepts Go's string grammar (including `.5`, `1.`, signed values, and hexadecimal floats), which is intentionally broader than source literal grammar. It does not trim whitespace. Conversion of NaN, infinity, or a rounded value outside int64 range follows Go's implementation-dependent integer conversion result, not a portable mathematical integer. No extra validation or saturation is introduced.

## Removed limitations and cutover

Float arithmetic, conversions, math, parsing, formatting, and numeric ordering are now available; remove the no-floats limitation from SPEC. Retain other limitations. Replaced the addition-only deferred-check path with one arithmetic path, and removed obsolete tests that expected `0.1` to be tuple access or arithmetic to be Int/String-only.

## Deviations and clarified boundaries

No deviation from Pass 12's fixed names, signatures, ABI, or required behavior. Pass 12 does not specify negative integer powers or formatting precision boundaries; the behavior above records the primary's approved policies (negative powers specifically clarified by Main). Non-finite/out-of-range integer conversion retains Go semantics rather than adding a new error policy.

## Duplicated names

`class Debug[A] { debug(a A) String }` is declared exactly as owned by **deriving**, solely so `Debug[Float]` can compile independently. At integration keep deriving's class and this slice's Float instance; remove the duplicate class. No other cross-slice declarations are duplicated.

## Verification

- Focused packages: `go test ./lexer ./parser ./types ./codegen ./rt ./prelude`.
- End-to-end program: `go run ./cmd/gofx run examples/float-num/main.gfx`; README records observed output.
- Existing-program compatibility: `go run ./cmd/gofx check examples/wc/main.gfx`.

Commands are run through the VM-wide serialization wrapper; no full-suite tests, vet, or unrelated builds are requested by this slice.
