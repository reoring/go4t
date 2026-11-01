package rt

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// Fiber holds a child execution context and one immutable, cached outcome.
// Closing done publishes the outcome to every awaiter.
type Fiber struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	value  any
	err    error
}

// runConcurrentIO contains panics even in abandoned, noncooperative externs.
func runConcurrentIO(ctx context.Context, action IO) (value any, err error) {
	defer func() {
		if p := recover(); p != nil {
			value, err = nil, fmt.Errorf("panic: %v", p)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, err = action(ctx)
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}
	return value, err
}

func forkIO(parent context.Context, action IO) *Fiber {
	ctx, cancel := context.WithCancel(parent)
	fiber := &Fiber{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer cancel()
		fiber.value, fiber.err = runConcurrentIO(ctx, action)
		close(fiber.done)
	}()
	return fiber
}

const cancellationGrace = 100 * time.Millisecond

// waitUntil bounds cleanup for foreign code that does not honor its context.
func waitUntil(done <-chan struct{}, until time.Time) {
	select {
	case <-done:
		return
	default:
	}
	timer := time.NewTimer(time.Until(until))
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

// Fork starts a fresh child fiber each time the returned IO is executed.
func Fork(action any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return forkIO(ctx, action.(IO)), nil
	})
}

// Cancel requests cancellation without waiting for the action to finish.
func Cancel(f any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f.(*Fiber).cancel()
		return Unit{}, nil
	})
}

// Await is cancellable by either its caller or an unfinished fiber's context.
// A completed fiber keeps its original outcome, including across repeated awaits.
func Await(f any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fiber := f.(*Fiber)
		select {
		case <-fiber.done:
			return fiber.value, fiber.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-fiber.ctx.Done():
			// Successful completion releases the child context too. Prefer the
			// published outcome in that case, not the cleanup cancellation.
			select {
			case <-fiber.done:
				return fiber.value, fiber.err
			default:
				return nil, fiber.ctx.Err()
			}
		}
	})
}

// ParMapIO cancels siblings on the first failure, joins them, and preserves input
// order. A foreign mapper that ignores cancellation can delay failure cleanup.
func ParMapIO(xs, f any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		if len(values) == 0 {
			return Shapes.Nil(), nil
		}
		children, cancel := context.WithCancel(ctx)
		defer cancel()
		var tasks sync.WaitGroup
		var failure sync.Once
		var firstErr error
		tasks.Add(len(values))
		for i := range values {
			go func() {
				defer tasks.Done()
				value, err := runConcurrentIO(children, IO(func(ctx context.Context) (any, error) {
					return f.(func(any) any)(values[i]).(IO)(ctx)
				}))
				values[i] = value
				if err != nil {
					failure.Do(func() {
						firstErr = err
						cancel()
					})
				}
			}()
		}
		done := make(chan struct{})
		go func() { tasks.Wait(); close(done) }()
		select {
		case <-done:
		case <-ctx.Done():
			cancel()
			waitUntil(done, time.Now().Add(cancellationGrace))
			return nil, ctx.Err()
		}
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

// Race returns the first completion, including failure, after cancelling and
// joining the loser. An uncooperative loser is abandoned after 100 ms.
func Race(a, b any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		left, right := forkIO(ctx, a.(IO)), forkIO(ctx, b.(IO))
		defer left.cancel()
		defer right.cancel()
		var winner, loser *Fiber
		select {
		case <-left.done:
			winner, loser = left, right
		case <-right.done:
			winner, loser = right, left
		case <-ctx.Done():
			left.cancel()
			right.cancel()
			until := time.Now().Add(cancellationGrace)
			waitUntil(left.done, until)
			waitUntil(right.done, until)
			return nil, ctx.Err()
		}
		loser.cancel()
		waitUntil(loser.done, time.Now().Add(cancellationGrace))
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return winner.value, winner.err
	})
}

// ParBoth joins both actions, returning values in argument order. Unlike
// ParMapIO, an action failure does not cancel its sibling.
func ParBoth(a, b any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		children, cancel := context.WithCancel(ctx)
		defer cancel()
		var values [2]any
		var tasks sync.WaitGroup
		var failure sync.Once
		var firstErr error
		tasks.Add(2)
		for i, action := range [2]any{a, b} {
			go func() {
				defer tasks.Done()
				value, err := runConcurrentIO(children, action.(IO))
				values[i] = value
				if err != nil {
					failure.Do(func() { firstErr = err })
				}
			}()
		}
		done := make(chan struct{})
		go func() { tasks.Wait(); close(done) }()
		select {
		case <-done:
		case <-ctx.Done():
			cancel()
			waitUntil(done, time.Now().Add(cancellationGrace))
			return nil, ctx.Err()
		}
		if firstErr != nil {
			return nil, firstErr
		}
		return Tuple2Of(values[0], values[1]), nil
	})
}

// withTimeout returns an expiry bit separately so Timeout never swallows an
// action's own deadline error or the enclosing context's deadline.
func withTimeout(ctx context.Context, millis int64, action IO, name string) (any, error, bool) {
	if err := ctx.Err(); err != nil {
		return nil, err, false
	}
	if millis <= 0 {
		return nil, fmt.Errorf("%s: %w", name, context.DeadlineExceeded), true
	}
	if millis > math.MaxInt64/int64(time.Millisecond) {
		return nil, fmt.Errorf("%s: duration out of range", name), false
	}
	deadline, cancel := context.WithTimeout(ctx, time.Duration(millis)*time.Millisecond)
	defer cancel()
	fiber := forkIO(deadline, action)
	select {
	case <-fiber.done:
	case <-deadline.Done():
		fiber.cancel()
		until := time.Now().Add(cancellationGrace)
		if end, ok := deadline.Deadline(); ok && end.Before(time.Now()) {
			until = end.Add(cancellationGrace)
		}
		waitUntil(fiber.done, until)
	}
	if err := ctx.Err(); err != nil {
		return nil, err, false
	}
	if deadline.Err() != nil {
		return nil, fmt.Errorf("%s: %w", name, context.DeadlineExceeded), true
	}
	return fiber.value, fiber.err, false
}

// WithTimeout fails with a wrapped context.DeadlineExceeded on expiry. A
// nonpositive budget does not start the action. Cleanup has a 100 ms grace.
func WithTimeout(millis, action any) any {
	return IO(func(ctx context.Context) (any, error) {
		value, err, _ := withTimeout(ctx, millis.(int64), action.(IO), "withTimeout")
		return value, err
	})
}

// Timeout is the Option form of WithTimeout; timely failures still fail IO.
func Timeout(millis, action any) any {
	return IO(func(ctx context.Context) (any, error) {
		value, err, expired := withTimeout(ctx, millis.(int64), action.(IO), "timeout")
		if expired {
			return NoneOf(), nil
		}
		if err != nil {
			return nil, err
		}
		return SomeOf(value), nil
	})
}
