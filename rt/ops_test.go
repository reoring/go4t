package rt

import "testing"

func TestIntArithmetic(t *testing.T) {
	for _, tt := range []struct {
		name string
		op   func(any, any) any
		a, b int64
		want int64
	}{
		{"add", AddInt, 8, -3, 5},
		{"subtract", SubInt, 3, 8, -5},
		{"multiply", MulInt, -3, 4, -12},
		{"multiply zero", MulInt, 9, 0, 0},
		{"divide", DivInt, 9, 2, 4},
		{"divide negative", DivInt, -9, 2, -4},
		{"divide by negative", DivInt, 9, -2, -4},
		{"add overflow", AddInt, 1<<63 - 1, 1, -1 << 63},
		{"divide minimum", DivInt, -1 << 63, -1, -1 << 63},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.op(tt.a, tt.b); got != tt.want {
				t.Errorf("result = %v (%T), want %d (int64)", got, got, tt.want)
			}
		})
	}
}

func TestAddString(t *testing.T) {
	for _, tt := range []struct{ a, b, want string }{
		{"hello", " world", "hello world"},
		{"", "right", "right"},
		{"left", "", "left"},
		{"", "", ""},
		{"こんにちは", "世界", "こんにちは世界"},
	} {
		if got := AddString(tt.a, tt.b); got != tt.want {
			t.Errorf("AddString(%q, %q) = %v (%T), want %q (string)", tt.a, tt.b, got, got, tt.want)
		}
	}
}

func TestConcat(t *testing.T) {
	for _, tt := range []struct {
		name  string
		parts []any
		want  string
	}{
		{"empty", nil, ""},
		{"single", []any{"hello"}, "hello"},
		{"empty parts", []any{"", "", ""}, ""},
		{"mixed", []any{"hello", "", " ", "world"}, "hello world"},
		{"unicode", []any{"こんにちは", "・", "世界"}, "こんにちは・世界"},
		{"shown values", []any{ShowInt(int64(42)), " ", ShowBool(true)}, "42 true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Concat(tt.parts...); got != tt.want {
				t.Errorf("Concat(%v) = %v (%T), want %q (string)", tt.parts, got, got, tt.want)
			}
		})
	}
}

func TestDivIntByZeroPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("DivInt by zero did not panic")
		}
	}()
	DivInt(int64(1), int64(0))
}

func TestNegInt(t *testing.T) {
	for _, tt := range []struct{ value, want int64 }{
		{5, -5}, {-5, 5}, {0, 0}, {-1 << 63, -1 << 63},
	} {
		if got := NegInt(tt.value); got != tt.want {
			t.Errorf("NegInt(%d) = %v (%T), want %d (int64)", tt.value, got, got, tt.want)
		}
	}
}

func TestComparisons(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		op                   func(any, any) any
		less, equal, greater bool
		low, high            any
	}{
		{"EqInt", EqInt, false, true, false, int64(-1 << 63), int64(1<<63 - 1)},
		{"LtInt", LtInt, true, false, false, int64(-1 << 63), int64(1<<63 - 1)},
		{"EqString", EqString, false, true, false, "", "a"},
		{"LtString", LtString, true, false, false, "a", "aa"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, pair := range []struct {
				a, b any
				want bool
			}{
				{tt.low, tt.high, tt.less},
				{tt.low, tt.low, tt.equal},
				{tt.high, tt.high, tt.equal},
				{tt.high, tt.low, tt.greater},
			} {
				if got := tt.op(pair.a, pair.b); got != pair.want {
					t.Errorf("(%v, %v) = %v (%T), want %t (bool)", pair.a, pair.b, got, got, pair.want)
				}
			}
		})
	}
}

func TestBoolOperators(t *testing.T) {
	for _, a := range []bool{false, true} {
		for _, b := range []bool{false, true} {
			if got := EqBool(a, b); got != (a == b) {
				t.Errorf("EqBool(%t, %t) = %v (%T), want %t (bool)", a, b, got, got, a == b)
			}
		}
		if got := Not(a); got != !a {
			t.Errorf("Not(%t) = %v (%T), want %t (bool)", a, got, got, !a)
		}
	}
}

func TestIntBounds(t *testing.T) {
	for _, tt := range []struct {
		name string
		call func() any
		want int64
	}{
		{"MinInt", MinInt, -1 << 63},
		{"MaxInt", MaxInt, 1<<63 - 1},
	} {
		if got := tt.call(); got != tt.want {
			t.Errorf("%s() = %v (%T), want %d (int64)", tt.name, got, got, tt.want)
		}
	}
}
