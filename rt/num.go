package rt

import (
	"math"
	"strconv"
)

func AddFloat(a, b any) any { return a.(float64) + b.(float64) }
func SubFloat(a, b any) any { return a.(float64) - b.(float64) }
func MulFloat(a, b any) any { return a.(float64) * b.(float64) }
func DivFloat(a, b any) any { return a.(float64) / b.(float64) }
func NegFloat(a any) any    { return -a.(float64) }
func EqFloat(a, b any) any  { return a.(float64) == b.(float64) }

// LtFloat places NaN below every non-NaN; two NaNs compare as equivalent.
// EqFloat deliberately retains IEEE equality, so NaN is not equal to itself.
func LtFloat(a, b any) any {
	x, y := a.(float64), b.(float64)
	return x < y || math.IsNaN(x) && !math.IsNaN(y)
}

func ShowFloat(f any) any { return strconv.FormatFloat(f.(float64), 'g', -1, 64) }

// A negative precision requests the shortest fixed-point representation, as in Go.
func ShowFloatFixed(f, decimals any) any {
	return strconv.FormatFloat(f.(float64), 'f', int(decimals.(int64)), 64)
}

func ToFloat(n any) any  { return float64(n.(int64)) }
func Truncate(f any) any { return int64(f.(float64)) }
func Floor(f any) any    { return int64(math.Floor(f.(float64))) }
func Ceil(f any) any     { return int64(math.Ceil(f.(float64))) }
func Round(f any) any    { return int64(math.Round(f.(float64))) }
func Sqrt(f any) any     { return math.Sqrt(f.(float64)) }
func Pow(x, y any) any   { return math.Pow(x.(float64), y.(float64)) }
func AbsFloat(f any) any { return math.Abs(f.(float64)) }
func Nan() any           { return math.NaN() }
func IsNaN(f any) any    { return math.IsNaN(f.(float64)) }

// PowInt uses integer exponentiation by squaring. Overflow wraps like Int
// multiplication; zero to the zeroth power is one. Negative powers truncate
// toward zero, with zero to a negative power defined as zero.
func PowInt(x, y any) any {
	base, exponent := x.(int64), y.(int64)
	if exponent < 0 {
		if base == 1 {
			return int64(1)
		}
		if base == -1 {
			if exponent&1 != 0 {
				return int64(-1)
			}
			return int64(1)
		}
		return int64(0)
	}
	result := int64(1)
	for exponent > 0 {
		if exponent&1 != 0 {
			result *= base
		}
		exponent >>= 1
		if exponent > 0 {
			base *= base
		}
	}
	return result
}

// ParseFloat follows strconv.ParseFloat at 64-bit precision; syntax and range
// errors become None, while explicit NaN and infinities remain valid values.
func ParseFloat(s any) any {
	value, err := strconv.ParseFloat(s.(string), 64)
	if err != nil {
		return NoneOf()
	}
	return SomeOf(value)
}
