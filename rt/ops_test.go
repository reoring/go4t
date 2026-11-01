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
		{"NeInt", NeInt, true, false, true, int64(-1 << 63), int64(1<<63 - 1)},
		{"LtInt", LtInt, true, false, false, int64(-1 << 63), int64(1<<63 - 1)},
		{"LeInt", LeInt, true, true, false, int64(-1 << 63), int64(1<<63 - 1)},
		{"GtInt", GtInt, false, false, true, int64(-1 << 63), int64(1<<63 - 1)},
		{"GeInt", GeInt, false, true, true, int64(-1 << 63), int64(1<<63 - 1)},
		{"EqString", EqString, false, true, false, "", "a"},
		{"NeString", NeString, true, false, true, "", "a"},
		{"LtString", LtString, true, false, false, "a", "aa"},
		{"LeString", LeString, true, true, false, "a", "b"},
		{"GtString", GtString, false, false, true, "a", "b"},
		{"GeString", GeString, false, true, true, "a", "aa"},
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
	for _, tt := range []struct {
		a, b                     bool
		equal, notEqual, and, or bool
	}{
		{false, false, true, false, false, false},
		{false, true, false, true, false, true},
		{true, false, false, true, false, true},
		{true, true, true, false, true, true},
	} {
		for _, op := range []struct {
			name string
			call func(any, any) any
			want bool
		}{
			{"EqBool", EqBool, tt.equal},
			{"NeBool", NeBool, tt.notEqual},
			{"And", And, tt.and},
			{"Or", Or, tt.or},
		} {
			if got := op.call(tt.a, tt.b); got != op.want {
				t.Errorf("%s(%t, %t) = %v (%T), want %t (bool)", op.name, tt.a, tt.b, got, got, op.want)
			}
		}
	}
	for _, value := range []bool{false, true} {
		if got := Not(value); got != !value {
			t.Errorf("Not(%t) = %v (%T), want %t (bool)", value, got, got, !value)
		}
	}
}
