package rt

import (
	"math"
	"testing"
)

func TestDebugString(t *testing.T) {
	for _, tt := range []struct{ value, want string }{
		{"", `""`},
		{"plain", `"plain"`},
		{"\"\\\n\r\t\x00", `"\"\\\n\r\t\x00"`},
		{"世界", `"世界"`},
		{string([]byte{0xff}), `"\xff"`},
	} {
		if got := DebugString(tt.value); got != tt.want {
			t.Errorf("DebugString(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestDebugFloat(t *testing.T) {
	for _, tt := range []struct {
		value float64
		want  string
	}{
		{0.0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{1.0, "1.0"},
		{-2.0, "-2.0"},
		{1.5, "1.5"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{1e21, "1e+21"},
		{1e-7, "1e-07"},
	} {
		if got := DebugFloat(tt.value); got != tt.want {
			t.Errorf("DebugFloat(%g) = %q, want %q", tt.value, got, tt.want)
		}
	}
}
