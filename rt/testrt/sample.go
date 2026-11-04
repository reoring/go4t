package testrt

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strconv"

	"go4t/rt"
)

const sampleSeed int64 = 0x674f3474

// Sample reads configuration only when its IO executes. Generators below are
// pure: their randomness depends solely on explicit seed/sample/offset inputs.
func Sample(count, check any) any {
	return rt.IO(func(ctx context.Context) (result any, err error) {
		seed := sampleSeed
		sample := int64(0)
		defer func() {
			if failure := recover(); failure != nil {
				result = nil
				err = fmt.Errorf("sample failed (seed %d, sample %d): %v", seed, sample+1, failure)
			}
		}()
		if value, ok := os.LookupEnv("GO4T_SAMPLE_SEED"); ok {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("GO4T_SAMPLE_SEED must be an Int: %q (default seed %d)", value, seed)
			}
			seed = parsed
		}
		for i := range count.(int64) {
			sample = i
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("sample failed (seed %d, sample %d): %w", seed, i+1, rt.ClassifyError(err))
			}
			message := check.(func(any, any) any)(seed, i).(string)
			if message != "" {
				return nil, fmt.Errorf("counterexample (seed %d, sample %d): %s", seed, i+1, message)
			}
		}
		return true, nil
	})
}

func sampleRandom(seed, sample, offset any) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(seed.(int64))^uint64(offset.(int64))*0x9e3779b97f4a7c15, uint64(sample.(int64))))
}

func SampleChoice(seed, sample, offset, count any) any {
	n, i, o := count.(int64), sample.(int64), offset.(int64)
	if i < n {
		return (i + o%n) % n
	}
	return sampleRandom(seed, sample, offset).Int64N(n)
}

func SampleInt(seed, sample, offset any) any {
	edges := [...]int64{math.MinInt64, math.MaxInt64, 0, 1, -1}
	i, o := sample.(int64), offset.(int64)
	if i < int64(len(edges)) {
		return edges[(i+o%int64(len(edges)))%int64(len(edges))]
	}
	rng := sampleRandom(seed, sample, offset)
	if i%2 == 0 {
		return rng.Int64N(3) - 1
	}
	if rng.IntN(3) == 0 {
		return edges[rng.IntN(len(edges))]
	}
	return int64(rng.Uint64())
}

func SampleFloat(seed, sample, offset any) any {
	edges := [...]float64{math.NaN(), math.Copysign(0, -1), math.Inf(1), math.Inf(-1), 0, 1, -1, 1e308, 5e-324}
	i, o := sample.(int64), offset.(int64)
	if i < int64(len(edges)) {
		return edges[(i+o%int64(len(edges)))%int64(len(edges))]
	}
	rng := sampleRandom(seed, sample, offset)
	if rng.IntN(3) == 0 {
		return edges[rng.IntN(len(edges))]
	}
	return math.Float64frombits(rng.Uint64())
}

func SampleString(seed, sample, offset any) any {
	edges := [...]string{"", "a", "\"\\\n\t", "日本語𝄞", "\x00\r"}
	i, o := sample.(int64), offset.(int64)
	if i < int64(len(edges)) {
		return edges[(i+o%int64(len(edges)))%int64(len(edges))]
	}
	rng := sampleRandom(seed, sample, offset)
	if i%2 == 0 {
		return [...]string{"", "a", "b"}[rng.IntN(3)]
	}
	alphabet := []rune("ab \"\\\n\t日本語𝄞")
	chars := make([]rune, rng.IntN(12))
	for j := range chars {
		chars[j] = alphabet[rng.IntN(len(alphabet))]
	}
	return string(chars)
}

func SampleFloatEqual(a, b any) any {
	return math.Float64bits(a.(float64)) == math.Float64bits(b.(float64))
}

// SampleImpossible guards the shared generator/observer depth invariant. The
// observer never invents a display for a constructor the generator cannot draw.
func SampleImpossible() any {
	panic("internal sample generator/observer depth mismatch")
}
