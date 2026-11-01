package rt

import "testing"

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
