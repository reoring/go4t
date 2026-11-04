package testrt

import (
	"context"
	"math"
	"strings"
	"testing"

	"go4t/rt"
)

func TestSampleSeedAndFirstCounterexample(t *testing.T) {
	t.Setenv("GO4T_SAMPLE_SEED", "42")
	calls := int64(0)
	action := Sample(int64(100000), func(seed, index any) any {
		if seed != int64(42) || index != calls {
			t.Fatalf("seed/index = %v/%v", seed, index)
		}
		calls++
		if index == int64(2) {
			return "x = 0"
		}
		return ""
	}).(rt.IO)
	_, err := action(context.Background())
	if calls != 3 || err == nil || err.Error() != "counterexample (seed 42, sample 3): x = 0" {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestSampleBoundariesAndPurity(t *testing.T) {
	seed, offset := int64(42), int64(0)
	ints := []int64{math.MinInt64, math.MaxInt64, 0, 1, -1}
	for i, want := range ints {
		if got := SampleInt(seed, int64(i), offset); got != want {
			t.Fatalf("integer edge %d = %v, want %d", i, got, want)
		}
	}
	floats := []float64{math.NaN(), math.Copysign(0, -1), math.Inf(1), math.Inf(-1), 0, 1, -1, 1e308, 5e-324}
	for i, want := range floats {
		got := SampleFloat(seed, int64(i), offset).(float64)
		if math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("float edge %d = %x, want %x", i, math.Float64bits(got), math.Float64bits(want))
		}
	}
	if SampleString(seed, int64(0), offset) != "" {
		t.Fatal("missing empty string edge")
	}
	for i := int64(9); i < 100; i++ {
		first := SampleFloat(seed, i, offset).(float64)
		_ = SampleFloat(int64(99), i, int64(2))
		second := SampleFloat(seed, i, offset).(float64)
		if math.Float64bits(first) != math.Float64bits(second) {
			t.Fatal("generator has shared state")
		}
	}
	if SampleFloatEqual(math.NaN(), math.NaN()) != true || SampleFloatEqual(0.0, math.Copysign(0, -1)) != false || SampleFloatEqual(math.Float64frombits(0x7ff8000000000001), math.Float64frombits(0x7ff8000000000002)) != false {
		t.Fatal("Float equality must compare all bits")
	}
}

func TestSampleExecutionFailuresIncludeSeed(t *testing.T) {
	t.Setenv("GO4T_SAMPLE_SEED", "-7")
	_, err := Sample(int64(1), func(_, _ any) any { panic("broken") }).(rt.IO)(context.Background())
	if err == nil || !strings.Contains(err.Error(), "seed -7, sample 1") || !strings.Contains(err.Error(), "broken") {
		t.Fatal(err)
	}
	t.Setenv("GO4T_SAMPLE_SEED", "bad")
	_, err = Sample(int64(1), func(_, _ any) any { return "" }).(rt.IO)(context.Background())
	if err == nil || !strings.Contains(err.Error(), "GO4T_SAMPLE_SEED must be an Int") {
		t.Fatal(err)
	}
}
