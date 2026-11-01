package rt

import (
	"context"
	"math"
	"reflect"
	"testing"
)

func TestTuple2Shape(t *testing.T) {
	installTestShapes(t)
	Shapes.Tuple2 = func(a, b any) any { return [2]any{a, b} }
	for _, pair := range [][2]any{{int64(7), "right"}, {nil, false}} {
		if got := Tuple2Of(pair[0], pair[1]); !reflect.DeepEqual(got, pair) {
			t.Errorf("Tuple2Of(%v, %v) = %#v", pair[0], pair[1], got)
		}
	}
	Shapes.Tuple2 = nil
	defer func() {
		if got := recover(); got != "rt.Shapes is not initialized: Tuple2 is required" {
			t.Errorf("panic = %v, want missing Tuple2 shape diagnostic", got)
		}
	}()
	Tuple2Of(nil, nil)
}

func TestConcurrencyTimingDependencies(t *testing.T) {
	for _, ms := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
		value, err := Sleep(ms).(IO)(context.Background())
		if ms == math.MaxInt64 {
			if value != nil || err == nil || err.Error() != "sleep: duration out of range" {
				t.Fatalf("Sleep(%d) = (%v, %v), want duration failure", ms, value, err)
			}
		} else if value != (Unit{}) || err != nil {
			t.Fatalf("Sleep(%d) = (%v, %v), want (Unit, nil)", ms, value, err)
		}
	}
	start, err := MonotonicMillis().(IO)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	end, err := MonotonicMillis().(IO)(context.Background())
	if err != nil || start.(int64) < 0 || end.(int64) < start.(int64) {
		t.Fatalf("monotonic interval = %v .. %v, error = %v", start, end, err)
	}
}
