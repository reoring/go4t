package rt

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Fiber holds one goroutine's outcome. Closing done publishes the immutable
// result to every awaiter; awaiting a fiber never reruns its action.
type Fiber struct {
	done  chan struct{}
	value any
	err   error
}

// runConcurrentIO converts a task's panic into an ordinary IO failure, so a
// detached or abandoned goroutine cannot crash the process with a panic.
func runConcurrentIO(action IO) (value any, err error) {
	defer func() {
		if p := recover(); p != nil {
			value, err = nil, &Failure{Kind: Panic, Message: fmt.Sprint(p)}
		}
	}()
	value, err = action()
	return value, ClassifyError(err)
}

func forkIO(action IO) *Fiber {
	fiber := &Fiber{done: make(chan struct{})}
	go func() {
		fiber.value, fiber.err = runConcurrentIO(action)
		close(fiber.done)
	}()
	return fiber
}

// Fork starts a fresh fiber each time the returned IO is executed.
func Fork(action any) any {
	return IO(func() (any, error) {
		return forkIO(action.(IO)), nil
	})
}

// Await waits for the cached outcome, returning the original failure unchanged.
// Multiple sequential or concurrent awaiters share the same result.
func Await(f any) any {
	return IO(func() (any, error) {
		fiber := f.(*Fiber)
		<-fiber.done
		return fiber.value, fiber.err
	})
}

// ParMapIO runs every mapper/action concurrently, waits for all of them, and
// returns a generated List in input order. The first observed failure wins.
func ParMapIO(xs, f any) any {
	return IO(func() (any, error) {
		if Shapes.Uncons == nil {
			panic("rt.Shapes is not initialized: Uncons is required")
		}
		if Shapes.Cons == nil || Shapes.Nil == nil {
			panic("rt.Shapes is not initialized: Cons and Nil are required")
		}
		var values []any
		for list := xs; ; {
			head, tail, ok := Shapes.Uncons(list)
			if !ok {
				break
			}
			values = append(values, head)
			list = tail
		}
		var tasks sync.WaitGroup
		var failure sync.Once
		var firstErr error
		tasks.Add(len(values))
		for i := range values {
			go func() {
				defer tasks.Done()
				value, err := runConcurrentIO(IO(func() (any, error) {
					return f.(func(any) any)(values[i]).(IO)()
				}))
				values[i] = value
				if err != nil {
					failure.Do(func() { firstErr = err })
				}
			}()
		}
		tasks.Wait()
		if firstErr != nil {
			return nil, firstErr
		}
		list := Shapes.Nil()
		for i := len(values) - 1; i >= 0; i-- {
			list = Shapes.Cons(values[i], list)
		}
		return list, nil
	})
}

// Race returns the first completed outcome, including failure. The loser is not
// cancelled or joined; its eventual outcome is buffered and abandoned.
func Race(a, b any) any {
	return IO(func() (any, error) {
		type result struct {
			value any
			err   error
		}
		completed := make(chan result, 2)
		for _, action := range [2]any{a, b} {
			go func() {
				value, err := runConcurrentIO(action.(IO))
				completed <- result{value, err}
			}()
		}
		first := <-completed
		return first.value, first.err
	})
}

// ParBoth runs both actions concurrently and joins both, returning their values
// in argument order as a generated tuple, or the first observed failure.
func ParBoth(a, b any) any {
	return IO(func() (any, error) {
		var values [2]any
		var tasks sync.WaitGroup
		var failure sync.Once
		var firstErr error
		tasks.Add(2)
		for i, action := range [2]any{a, b} {
			go func() {
				defer tasks.Done()
				value, err := runConcurrentIO(action.(IO))
				values[i] = value
				if err != nil {
					failure.Do(func() { firstErr = err })
				}
			}()
		}
		tasks.Wait()
		if firstErr != nil {
			return nil, firstErr
		}
		return Tuple2Of(values[0], values[1]), nil
	})
}

// Timeout returns Some on success within the budget and None on expiry; an
// early failure remains an IO failure. Expired actions are not cancelled or
// joined. A nonpositive budget returns None without starting the action.
func Timeout(millis, action any) any {
	return IO(func() (any, error) {
		ms := millis.(int64)
		if ms <= 0 {
			return NoneOf(), nil
		}
		if ms > math.MaxInt64/int64(time.Millisecond) {
			return nil, &Failure{Kind: FailureKind, Message: "timeout: duration out of range"}
		}
		timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
		defer timer.Stop()
		fiber := forkIO(action.(IO))
		select {
		case <-fiber.done:
			if fiber.err != nil {
				return nil, fiber.err
			}
			return SomeOf(fiber.value), nil
		case <-timer.C:
			return NoneOf(), nil
		}
	})
}
