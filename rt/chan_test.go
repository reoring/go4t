package rt

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func runChanTest(t *testing.T, action any) any {
	t.Helper()
	value, err := action.(IO)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestChanRoundTrip(t *testing.T) {
	installTestShapes(t)
	for _, buffer := range []int64{0, 2} {
		t.Run(string(rune('0'+buffer)), func(t *testing.T) {
			c := runChanTest(t, ChanMake(buffer))
			if ChanAsRecv(c) != c || ChanAsSend(c) != c {
				t.Fatal("capability upcast changed identity")
			}
			sent := runConcTest(ChanSend(ChanAsSend(c), int64(42)))
			if got := runChanTest(t, ChanRecv(ChanAsRecv(c))); got != (testSome{int64(42)}) {
				t.Fatalf("recv = %#v", got)
			}
			if result := receiveConcTest(t, sent); result.value != (Unit{}) || result.err != nil {
				t.Fatalf("send = %#v", result)
			}
			runChanTest(t, ChanClose(ChanAsSend(c)))
			for range 2 {
				if got := runChanTest(t, ChanRecv(ChanAsRecv(c))); got != (testNone{}) {
					t.Fatalf("closed recv = %#v", got)
				}
			}
		})
	}
}

func TestChanBufferAndFreshAllocation(t *testing.T) {
	installTestShapes(t)
	makeAction := ChanMake(int64(2))
	first, second := runChanTest(t, makeAction), runChanTest(t, makeAction)
	if first == second {
		t.Fatal("make reused shared state")
	}
	runChanTest(t, ChanSend(first, int64(1)))
	runChanTest(t, ChanSend(first, int64(2)))
	runChanTest(t, ChanClose(first))
	for _, want := range []any{testSome{int64(1)}, testSome{int64(2)}, testNone{}} {
		if got := runChanTest(t, ChanRecv(first)); got != want {
			t.Fatalf("buffer drain = %#v, want %#v", got, want)
		}
	}
	if _, err := ChanMake(int64(-1)).(IO)(context.Background()); !reflect.DeepEqual(err, &Failure{Kind: FailureKind, Message: "chan.make: buffer out of range"}) {
		t.Fatalf("negative buffer = %#v", err)
	}
}

// Observe Done evaluation, not scheduler timing, before cancelling a blocked
// select. Neither action has a peer, so it cannot complete before cancellation.
type chanSelectContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *chanSelectContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func TestChanCancellationUnblocks(t *testing.T) {
	installTestShapes(t)
	for _, send := range []bool{false, true} {
		base, cancel := context.WithCancel(context.Background())
		ctx := &chanSelectContext{Context: base, entered: make(chan struct{})}
		c := runChanTest(t, ChanMake(int64(0)))
		action := ChanRecv(c)
		if send {
			action = ChanSend(c, int64(1))
		}
		done := make(chan concTestResult, 1)
		go func() {
			value, err := action.(IO)(ctx)
			done <- concTestResult{value, err}
		}()
		receiveConcTest(t, ctx.entered)
		cancel()
		result := receiveConcTest(t, done)
		if result.value != nil || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("blocked send=%v: %#v", send, result)
		}
	}
}

func TestChanCancelledOperationsDoNotObserveState(t *testing.T) {
	installTestShapes(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := runChanTest(t, ChanMake(int64(1)))
	runChanTest(t, ChanSend(c, int64(7)))
	for _, action := range []any{ChanMake(int64(0)), ChanRecv(c), ChanSend(c, int64(8)), ChanClose(c)} {
		if _, err := action.(IO)(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-cancelled operation = %v", err)
		}
	}
	if got := runChanTest(t, ChanRecv(c)); got != (testSome{int64(7)}) {
		t.Fatalf("cancelled receive consumed a value: %#v", got)
	}
	runChanTest(t, ChanClose(c))
}

func TestChanClosedOperationsAreFailures(t *testing.T) {
	installTestShapes(t)
	c := runChanTest(t, ChanMake(int64(0)))
	runChanTest(t, ChanClose(c))
	for _, tc := range []struct {
		action  any
		message string
	}{
		{ChanClose(c), "chan.close: close of closed channel"},
		{ChanSend(c, int64(1)), "chan.send: send on closed channel"},
		{ChanClose(WrapSendChan((chan<- int)(nil), func(v any, _ context.Context) int { return int(v.(int64)) })), "chan.close: close of nil channel"},
	} {
		got := runChanTest(t, TryIO(tc.action))
		if want := (testErr{testIoError{"Failure", tc.message}}); got != want {
			t.Fatalf("failure = %#v, want %#v", got, want)
		}
	}
}

func TestChanNativeIdentityAndConversions(t *testing.T) {
	installTestShapes(t)
	native := make(chan int16, 1)
	from := func(v int16) any { return int64(v) }
	to := func(v any, _ context.Context) int16 { return int16(v.(int64)) }
	c := WrapChan(native, from, to)
	if GoChannel[chan int16](c) != native || GoChannel[<-chan int16](c) != native || GoChannel[chan<- int16](c) != native {
		t.Fatal("native channel identity changed")
	}
	runChanTest(t, ChanSend(c, int64(65537)))
	if got := <-native; got != 1 {
		t.Fatalf("send narrowing = %d", got)
	}
	native <- 42
	if got := runChanTest(t, ChanRecv(WrapRecvChan((<-chan int16)(native), from))); got != (testSome{int64(42)}) {
		t.Fatalf("receive widening = %#v", got)
	}
	wrong := runChanTest(t, ChanMake(int64(0)))
	var value any
	var err error
	func() {
		defer RecoverChanFailure(&value, &err)
		value = GoChannel[chan int16](wrong)
	}()
	want := &Failure{Kind: FailureKind, Message: "chan: go4t-made channel cannot be passed to Go; use the binding's typed channel constructor"}
	if value != nil || !reflect.DeepEqual(err, want) {
		t.Fatalf("native mismatch = (%v, %#v)", value, err)
	}
	func() {
		defer RecoverChanFailure(&value, &err)
		value = GoChannel[chan int](c)
	}()
	want = &Failure{Kind: FailureKind, Message: "chan: cannot pass chan int16 as chan int; use the binding's typed channel constructor"}
	if value != nil || !reflect.DeepEqual(err, want) {
		t.Fatalf("foreign element mismatch = (%v, %#v)", value, err)
	}
	func() {
		defer RecoverChanFailure(&value, &err)
		value = GoChannel[chan any](wrong)
	}()
	if failure, ok := err.(*Failure); value != nil || !ok || failure.Message != "chan: go4t-made channel cannot be passed to Go; use the binding's typed channel constructor" {
		t.Fatalf("erased and native any channels were confused: (%v, %#v)", value, err)
	}
}
