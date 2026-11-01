package rt

import (
	"math"
	"testing"
)

func TestFloatOperations(t *testing.T) {
	for _, tt := range []struct {
		name string
		got  any
		want float64
	}{
		{"add", AddFloat(1.5, 2.0), 3.5},
		{"subtract", SubFloat(1.5, 2.0), -0.5},
		{"multiply", MulFloat(1.5, 2.0), 3},
		{"divide", DivFloat(3.0, 2.0), 1.5},
		{"negate", NegFloat(1.5), -1.5},
		{"convert", ToFloat(int64(42)), 42},
		{"sqrt", Sqrt(9.0), 3},
		{"power", Pow(9.0, 0.5), 3},
		{"negative power", Pow(2.0, -3.0), 0.125},
		{"absolute", AbsFloat(-1.5), 1.5},
	} {
		if got, ok := tt.got.(float64); !ok || got != tt.want {
			t.Errorf("%s = %T(%v), want float64(%v)", tt.name, tt.got, tt.got, tt.want)
		}
	}
	for _, tt := range []struct {
		a, b float64
		sign int
	}{
		{1, 0, 1}, {-1, 0, -1}, {1, math.Copysign(0, -1), -1},
	} {
		if got := DivFloat(tt.a, tt.b).(float64); !math.IsInf(got, tt.sign) {
			t.Errorf("%v / %v = %v", tt.a, tt.b, got)
		}
	}
	for _, value := range []any{DivFloat(0.0, 0.0), DivFloat(math.Inf(1), math.Inf(1)), Sqrt(-1.0), Nan()} {
		if !IsNaN(value).(bool) {
			t.Errorf("expected NaN, got %v", value)
		}
	}
	if IsNaN(1.5).(bool) || !math.Signbit(NegFloat(0.0).(float64)) || math.Signbit(AbsFloat(math.Copysign(0, -1)).(float64)) {
		t.Fatal("NaN predicate or signed-zero arithmetic changed")
	}
}

func TestFloatOrdering(t *testing.T) {
	for _, tt := range []struct {
		a, b        float64
		equal, less bool
	}{
		{1.5, 2.0, false, true}, {2.0, 1.5, false, false}, {1.5, 1.5, true, false},
		{0, math.Copysign(0, -1), true, false},
		{math.NaN(), math.NaN(), false, false},
		{math.NaN(), math.Inf(-1), false, true},
		{math.Inf(1), math.NaN(), false, false},
		{math.Inf(-1), 0, false, true},
	} {
		if EqFloat(tt.a, tt.b) != tt.equal || LtFloat(tt.a, tt.b) != tt.less {
			t.Errorf("(%v, %v): eq=%v lt=%v, want %v %v", tt.a, tt.b, EqFloat(tt.a, tt.b), LtFloat(tt.a, tt.b), tt.equal, tt.less)
		}
	}
}

func TestFloatRounding(t *testing.T) {
	for _, tt := range []struct {
		value                        float64
		truncate, floor, ceil, round int64
	}{
		{2.75, 2, 2, 3, 3}, {-2.75, -2, -3, -2, -3},
		{2.5, 2, 2, 3, 3}, {-2.5, -2, -3, -2, -3},
		{0.25, 0, 0, 1, 0}, {-0.25, 0, -1, 0, 0}, {0, 0, 0, 0, 0},
	} {
		if Truncate(tt.value) != tt.truncate || Floor(tt.value) != tt.floor || Ceil(tt.value) != tt.ceil || Round(tt.value) != tt.round {
			t.Errorf("rounding %v: truncate=%v floor=%v ceil=%v round=%v", tt.value, Truncate(tt.value), Floor(tt.value), Ceil(tt.value), Round(tt.value))
		}
	}
}

func TestFloatFormatting(t *testing.T) {
	for _, tt := range []struct {
		value float64
		want  string
	}{
		{1.5, "1.5"}, {2000, "2000"}, {0.001, "0.001"}, {1e20, "1e+20"},
		{math.Copysign(0, -1), "-0"}, {math.NaN(), "NaN"}, {math.Inf(1), "+Inf"}, {math.Inf(-1), "-Inf"},
	} {
		if got := ShowFloat(tt.value); got != tt.want {
			t.Errorf("ShowFloat(%v) = %v, want %q", tt.value, got, tt.want)
		}
	}
	for _, tt := range []struct {
		value    float64
		decimals int64
		want     string
	}{
		{1.5, 2, "1.50"}, {1.25, 1, "1.2"}, {1.75, 1, "1.8"}, {2.5, 0, "2"},
		{-2.75, 3, "-2.750"}, {1.25, -1, "1.25"}, {math.NaN(), 2, "NaN"}, {math.Inf(1), 2, "+Inf"},
	} {
		if got := ShowFloatFixed(tt.value, tt.decimals); got != tt.want {
			t.Errorf("ShowFloatFixed(%v, %d) = %v, want %q", tt.value, tt.decimals, got, tt.want)
		}
	}
}

func TestParseFloat(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		source string
		want   float64
	}{
		{"1.5", 1.5}, {"2e3", 2000}, {"1.0e-3", 0.001}, {"-2.5", -2.5},
		{".5", 0.5}, {"1.", 1}, {"0x1p2", 4}, {"5e-324", math.SmallestNonzeroFloat64},
		{"+Inf", math.Inf(1)}, {"-Inf", math.Inf(-1)}, {"NaN", math.NaN()},
	} {
		some, ok := ParseFloat(tt.source).(testSome)
		if !ok {
			t.Errorf("ParseFloat(%q) returned None", tt.source)
			continue
		}
		got := some.value.(float64)
		if got != tt.want && !(math.IsNaN(got) && math.IsNaN(tt.want)) {
			t.Errorf("ParseFloat(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
	for _, source := range []string{"", "oops", "1e", "1e309", " 1.5", "1.5 "} {
		if got := ParseFloat(source); got != (testNone{}) {
			t.Errorf("ParseFloat(%q) = %#v, want None", source, got)
		}
	}
}

func TestPowInt(t *testing.T) {
	for _, tt := range []struct{ base, exponent, want int64 }{
		{3, 5, 243}, {0, 0, 1}, {0, 3, 0}, {-2, 3, -8}, {-2, 4, 16},
		{2, 62, 1 << 62}, {2, 63, -1 << 63}, {2, 64, 0}, {1, 1<<63 - 1, 1},
		{2, -1, 0}, {-2, -3, 0}, {0, -1, 0}, {1, -1, 1},
		{-1, -1, -1}, {-1, -2, 1}, {-1, -1 << 63, 1},
		{-1, -1<<63 + 1, -1}, {2, -1 << 63, 0}, {-1 << 63, -1, 0},
	} {
		if got := PowInt(tt.base, tt.exponent); got != tt.want {
			t.Errorf("PowInt(%d, %d) = %v, want %d", tt.base, tt.exponent, got, tt.want)
		}
	}
}
