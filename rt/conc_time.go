package rt

import (
	"errors"
	"math"
	"time"
)

var monotonicOrigin = time.Now()

// MonotonicMillis returns elapsed milliseconds since runtime initialization.
func MonotonicMillis() any {
	return IO(func() (any, error) {
		return time.Since(monotonicOrigin).Milliseconds(), nil
	})
}

// Sleep delays execution by millis; nonpositive durations return immediately.
func Sleep(millis any) any {
	return IO(func() (any, error) {
		ms := millis.(int64)
		if ms <= 0 {
			return Unit{}, nil
		}
		if ms > math.MaxInt64/int64(time.Millisecond) {
			return nil, errors.New("sleep: duration out of range")
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return Unit{}, nil
	})
}
