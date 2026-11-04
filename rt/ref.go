package rt

import (
	"context"
	"sync"
)

// Ref is an opaque mutable cell. Every observation and mutation runs inside IO;
// the stored go4t value remains immutable and is shared without copying.
// A Ref must not be copied after first use.
type Ref struct {
	mu    sync.Mutex
	value any
}

// RefNew allocates a fresh cell each time the returned IO is executed.
func RefNew(a any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		return &Ref{value: a}, nil
	})
}

// RefGet atomically reads the current value when its IO is executed.
func RefGet(r any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		ref := r.(*Ref)
		ref.mu.Lock()
		defer ref.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		return ref.value, nil
	})
}

// RefSet atomically replaces the current value when its IO is executed.
func RefSet(r, a any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		ref := r.(*Ref)
		ref.mu.Lock()
		defer ref.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		ref.value = a
		return Unit{}, nil
	})
}

// RefModify applies the pure callback once under the cell's lock. The callback
// must not execute IO or re-enter the cell; waiting for this lock does not
// interrupt a running callback. Cancellation observed before commit leaves the
// cell unchanged, as does a panic; deferred unlocking also covers panics.
func RefModify(r, f any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		ref := r.(*Ref)
		ref.mu.Lock()
		defer ref.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		value := f.(func(any) any)(ref.value)
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		ref.value = value
		return Unit{}, nil
	})
}

// RefUpdate atomically replaces the value and returns a separate result from the
// callback's (new value, result) tuple. Its pure callback runs once under the lock
// with the same cancellation and panic boundaries as RefModify.
func RefUpdate(r, f any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		ref := r.(*Ref)
		ref.mu.Lock()
		defer ref.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		if Shapes.Untuple2 == nil {
			panic("rt.Shapes is not initialized: Untuple2 is required")
		}
		value, result := Shapes.Untuple2(f.(func(any) any)(ref.value))
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		ref.value = value
		return result, nil
	})
}
