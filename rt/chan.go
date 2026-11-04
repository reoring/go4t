package rt

import (
	"context"
	"fmt"
	"reflect"
)

// channel retains the original Go channel and the shim's element converters.
// The language's Chan/RecvChan/SendChan types control which methods are exposed.
// No forwarding goroutine or replacement channel is introduced at the boundary.
type channel[T any] struct {
	native any
	input <-chan T
	output chan<- T
	from func(T) any
	to func(any, context.Context) T
	local bool
}

func WrapChan[T any](value chan T, from func(T) any, to func(any, context.Context) T) any {
	return &channel[T]{native: value, input: value, output: value, from: from, to: to}
}

func WrapRecvChan[T any](value <-chan T, from func(T) any) any {
	return &channel[T]{native: value, input: value, from: from}
}

func WrapSendChan[T any](value chan<- T, to func(any, context.Context) T) any {
	return &channel[T]{native: value, output: value, to: to}
}

func (c *channel[T]) goChannel() (any, bool) { return c.native, c.local }

func (c *channel[T]) recv(ctx context.Context) (any, error) {
	select {
	case value, open := <-c.input:
		if !open {
			return NoneOf(), nil
		}
		return SomeOf(c.from(value)), nil
	case <-ctx.Done():
		return nil, ClassifyError(ctx.Err())
	}
}

func (c *channel[T]) send(ctx context.Context, value any) (err error) {
	// Only the actual Go send is recovered. Element conversion failures keep
	// the existing shim contract rather than disguising unrelated panics.
	converted := c.to(value, ctx)
	defer channelOperationFailure("send", &err)
	select {
	case c.output <- converted:
		return nil
	case <-ctx.Done():
		return ClassifyError(ctx.Err())
	}
}

func (c *channel[T]) close() (err error) {
	defer channelOperationFailure("close", &err)
	close(c.output)
	return nil
}

func channelOperationFailure(operation string, err *error) {
	if p := recover(); p != nil {
		*err = &Failure{Kind: FailureKind, Message: "chan." + operation + ": " + fmt.Sprint(p)}
	}
}

// A mismatched native element cannot be repaired without losing channel
// identity. Generated IO boundaries turn this private signal into a Failure.
type channelConversionFailure struct{ failure *Failure }

func GoChannel[C any](value any) C {
	native, local := value.(interface{ goChannel() (any, bool) }).goChannel()
	if local {
		panic(channelConversionFailure{&Failure{Kind: FailureKind, Message: "chan: go4t-made channel cannot be passed to Go; use the binding's typed channel constructor"}})
	}
	if result, ok := native.(C); ok {
		return result
	}
	v, target := reflect.ValueOf(native), reflect.TypeFor[C]()
	if v.Type().ConvertibleTo(target) {
		return v.Convert(target).Interface().(C)
	}
	panic(channelConversionFailure{&Failure{Kind: FailureKind, Message: fmt.Sprintf("chan: cannot pass %s as %s; use the binding's typed channel constructor", v.Type(), target)}})
}

// RecoverChanFailure is deferred only by generated IO actions containing channel
// conversions. It does not intercept ordinary foreign panics.
func RecoverChanFailure(value *any, err *error) {
	if p := recover(); p != nil {
		if failure, ok := p.(channelConversionFailure); ok {
			*value, *err = nil, failure.failure
			return
		}
		panic(p)
	}
}

// NewGoChan allocates on every execution, using the native element selected by
// a generated binding constructor. Its conversions are shared with the shim.
func NewGoChan[T any](buffer any, from func(T) any, to func(any, context.Context) T) any {
	return newChannel(buffer, from, to, false)
}

func newChannel[T any](buffer any, from func(T) any, to func(any, context.Context) T, local bool) any {
	return IO(func(ctx context.Context) (value any, err error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		size := buffer.(int64)
		if size < 0 || int64(int(size)) != size {
			return nil, &Failure{Kind: FailureKind, Message: "chan.make: buffer out of range"}
		}
		defer channelOperationFailure("make", &err)
		native := make(chan T, int(size))
		return &channel[T]{native: native, input: native, output: native, from: from, to: to, local: local}, nil
	})
}

// ChanMake is erased: its channels are for communication between go4t fibers.
// Foreign functions require a channel from a typed binding constructor instead.
func ChanMake(buffer any) any {
	return newChannel(buffer, func(v any) any { return v }, func(v any, _ context.Context) any { return v }, true)
}

// Capability upcasts preserve the channel's identity and do not observe it.
func ChanAsRecv(value any) any { return value }
func ChanAsSend(value any) any { return value }

func ChanRecv(value any) any {
	return IO(func(ctx context.Context) (result any, err error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		defer RecoverChanFailure(&result, &err)
		return value.(interface{ recv(context.Context) (any, error) }).recv(ctx)
	})
}

func ChanSend(value, item any) any {
	return IO(func(ctx context.Context) (result any, err error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		defer RecoverChanFailure(&result, &err)
		if err := value.(interface{ send(context.Context, any) error }).send(ctx, item); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}

func ChanClose(value any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		if err := value.(interface{ close() error }).close(); err != nil {
			return nil, err
		}
		return Unit{}, nil
	})
}
