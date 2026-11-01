package rt

import "testing"

func TestMirrorValues(t *testing.T) {
	if got := Some(0); !got.Present || got.Value != 0 {
		t.Fatalf("Some(0) = %#v", got)
	}
	if got := None[string](); got.Present || got.Value != "" {
		t.Fatalf("None[string]() = %#v", got)
	}
	if got := (Result[string, int]{Ok: 3, IsOk: true}); !got.IsOk || got.Ok != 3 || got.Err != "" {
		t.Fatalf("Result = %#v", got)
	}
	if got := (Tuple8[int, int, int, int, int, int, int, string]{F0: 1, F7: "last"}); got.F0 != 1 || got.F7 != "last" {
		t.Fatalf("Tuple8 = %#v", got)
	}
}
