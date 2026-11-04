package extern

import (
	"strings"
	"testing"
)

func TestChannelStandardBindings(t *testing.T) {
	dir := bindingTestModule(t)
	loader := Loader{Dir: dir}
	for _, tc := range []struct { path, name string; want []string }{
		{"time", "shim_time", []string{
			"extern func after(p0 Int) RecvChan[Int]",
			"extern func tick(p0 Int) RecvChan[Int]",
			"extern func newTimeChan(buffer Int) IO[Chan[Int]]",
		}},
		{"context", "shim_context", []string{"extern func contextDone(recv Context) IO[RecvChan[Unit]]"}},
		{"os/signal", "shim_signal", []string{
			"extern func notify(p0 SendChan[go_os.Signal], p1 List[go_os.Signal]) IO[Unit]",
			"extern func stop(p0 SendChan[go_os.Signal]) IO[Unit]",
			"extern func newSignalChan(buffer Int) IO[Chan[go_os.Signal]]",
		}},
		{"net/http", "shim_http", []string{
			"extern func closeNotifierCloseNotify(recv CloseNotifier) IO[RecvChan[Bool]]",
			"extern func requestCancel(recv Request) IO[RecvChan[Unit]]",
			"extern func requestSetCancel(recv Request, value RecvChan[Unit]) IO[Unit]",
		}},
	} {
		pkg, err := loader.Load(tc.path)
		if err != nil { t.Fatal(err) }
		decls, shim, _, err := BindWithShim(pkg, BindOptions{Float: true}, "example.test/bindings/"+tc.name, tc.name)
		if err != nil { t.Fatal(err) }
		for _, want := range tc.want {
			if !strings.Contains(decls,want) { t.Errorf("%s missing %q:\n%s",tc.path,want,decls) }
		}
		writeBindingTestFile(t, dir, tc.name+"/shim.go", string(shim))
	}
	writeBindingTestFile(t, dir, "integration/channel_test.go", standardChannelRuntimeTest)
	runBindingTestGo(t, dir, "test", "./integration")
}

const standardChannelRuntimeTest = `package integration
import (
 "context"
 "net/http"
 "os"
 "syscall"
 "testing"
 "time"
 "go4t/rt"
 c "example.test/bindings/shim_context"
 h "example.test/bindings/shim_http"
 s "example.test/bindings/shim_signal"
 tm "example.test/bindings/shim_time"
)
type option struct { value any; present bool }
func TestStandardChannels(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  Some: func(v any) any { return option{v,true} }, None: func() any { return option{} },
  Uncons: func(v any) (any,any,bool) { xs := v.([]any); if len(xs)==0 { return nil,nil,false }; return xs[0],xs[1:],true },
 }
 ctx,stop := context.WithTimeout(context.Background(),time.Second*3)
 defer stop()
 run := func(action any) any { t.Helper(); v,err := action.(rt.IO)(ctx); if err != nil { t.Fatal(err) }; return v }
 timer := run(rt.ChanRecv(tm.After(int64(0)))).(option)
 if !timer.present || timer.value.(int64) <= 0 { t.Fatalf("time.After: %#v",timer) }
 timestamp := run(tm.Go4t_NewTimeChan(int64(1)))
 run(rt.ChanSend(timestamp,int64(123)))
 if got := (<-rt.GoChannel[chan time.Time](timestamp)).UnixNano(); got != 123 { t.Fatalf("time converter = %d",got) }
 doneCtx,cancel := context.WithCancel(ctx)
 done := run(c.Context_Done(doneCtx))
 cancel()
 if got := run(rt.ChanRecv(done)).(option); got.present { t.Fatalf("context.Done = %#v",got) }
 signals := run(s.Go4t_NewSignalChan(int64(1)))
 nativeSignals := rt.GoChannel[chan os.Signal](signals)
 run(s.Notify(rt.ChanAsSend(signals),[]any{syscall.SIGUSR1}))
 defer run(s.Stop(rt.ChanAsSend(signals)))
 nativeSignals <- syscall.SIGUSR1
 got := run(rt.ChanRecv(rt.ChanAsRecv(signals))).(option)
 if !got.present || got.value != syscall.SIGUSR1 { t.Fatalf("signal handle = %#v",got) }
 request := &http.Request{}
 cancellation := run(h.Go4t_NewUnitChan(int64(0)))
 run(h.Request_SetCancel(request,rt.ChanAsRecv(cancellation)))
 if request.Cancel != rt.GoChannel[<-chan struct{}](cancellation) { t.Fatal("request cancel channel lost identity") }
 run(rt.ChanClose(rt.ChanAsSend(cancellation)))
 if got := run(rt.ChanRecv(run(h.Request_Cancel(request)))).(option); got.present { t.Fatalf("Request.Cancel = %#v",got) }
}
`
