package rt

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func httpTestPairs(pairs ...[2]string) any {
	list := Shapes.Nil()
	for i := len(pairs) - 1; i >= 0; i-- {
		list = Shapes.Cons(Tuple2Of(pairs[i][0], pairs[i][1]), list)
	}
	return list
}

func httpTestRequest(url string, millis int64) IO {
	return HTTPRequest("GET", url, "", Shapes.Nil(), Shapes.Nil(), "", millis).(IO)
}

func TestHTTPRequest(t *testing.T) {
	installTestShapes(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/echo" || r.URL.Query().Get("a") != "日" || r.URL.Query().Get("old") != "1" || string(body) != "hello" || !reflect.DeepEqual(r.Header.Values("X-Test"), []string{"one", "two"}) {
			t.Errorf("request = %s %s %v %q", r.Method, r.URL, r.Header, body)
		}
		w.Header().Add("X-Reply", "first")
		w.Header().Add("X-Reply", "second")
		w.WriteHeader(418)
		_, _ = io.WriteString(w, "teapot")
	}))
	defer server.Close()
	action := HTTPRequest("POST", server.URL+"/old?old=1", "/echo", httpTestPairs([2]string{"a", "日"}), httpTestPairs([2]string{"X-Test", "one"}, [2]string{"x-test", "two"}), "hello", int64(30000)).(IO)
	if requests.Load() != 0 {
		t.Fatal("HTTP effect executed before IO")
	}
	for range 2 {
		value, err := action(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		status, headers, body := Shapes.Untuple3(value)
		if status != int64(418) || body != "teapot" || HTTPHeader(headers, "x-REPLY") != (testSome{"first"}) || HTTPHeader(headers, "absent") != (testNone{}) {
			t.Fatalf("response = %v %v %v", status, headers, body)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestHTTPRequestFailures(t *testing.T) {
	installTestShapes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	for _, millis := range []int64{-1, 0, 20} {
		_, err := httpTestRequest(server.URL, millis)(context.Background())
		if !errors.Is(err, context.DeadlineExceeded) || err.(*Failure).Kind != TimeoutKind || err.Error() != "context deadline exceeded" {
			t.Fatalf("timeout %d = %#v", millis, err)
		}
	}
	_, err := httpTestRequest(server.URL, math.MaxInt64)(context.Background())
	if !reflect.DeepEqual(err, &Failure{Kind: FailureKind, Message: "requestWithTimeout: duration out of range"}) {
		t.Fatalf("overflow = %#v", err)
	}
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closed.Close()
	for _, url := range []string{closed.URL, ":bad", "ftp://localhost/"} {
		_, err := httpTestRequest(url, 30000)(context.Background())
		failure, ok := err.(*Failure)
		if !ok || failure.Kind != Network || failure.Message == "" {
			t.Fatalf("request %q = %#v", url, err)
		}
	}
}

func TestHTTPRequestCancellation(t *testing.T) {
	installTestShapes(t)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := httpTestRequest(server.URL, 30000)(ctx); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || err.(*Failure).Kind != Cancelled {
			t.Fatalf("cancel = %#v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not stop")
	}
}

func startHTTPTestServer(t *testing.T, handler any) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := serveHTTP(ctx, listener, handler); done <- err; close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("serve did not stop within 1 s")
		}
	})
	return "http://" + listener.Addr().String(), cancel, done
}

func TestHTTPServe(t *testing.T) {
	installTestShapes(t)
	handler := func(method, url, path, query, headers, body any) any {
		if method != "POST" || url != "/sum?a=1&a=2" || path != "/sum" || !reflect.DeepEqual(query, httpTestPairs([2]string{"a", "1"}, [2]string{"a", "2"})) || HTTPHeader(headers, "x-test") != (testSome{"yes"}) || body != "input" {
			t.Errorf("handler fields = %v %v %v %v %v %v", method, url, path, query, headers, body)
		}
		return Pure(Tuple3Of(int64(201), httpTestPairs([2]string{"X-Reply", "one"}, [2]string{"X-Reply", "two"}), "created"))
	}
	url, cancel, done := startHTTPTestServer(t, handler)
	request, _ := http.NewRequest("POST", url+"/sum?a=1&a=2", strings.NewReader("input"))
	request.Header.Set("X-Test", "yes")
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 201 || string(body) != "created" || !reflect.DeepEqual(response.Header.Values("X-Reply"), []string{"one", "two"}) {
		t.Fatalf("response = %d %v %q", response.StatusCode, response.Header, body)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("serve cancellation = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve cancellation exceeded 1 s")
	}
}

func TestHTTPServeHandlerFailures(t *testing.T) {
	installTestShapes(t)
	for _, mode := range []string{"failure", "construction panic", "IO panic"} {
		t.Run(mode, func(t *testing.T) {
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			previous := os.Stderr
			os.Stderr = stderr
			defer func() { os.Stderr = previous }()
			handler := func(_, _, _, _, _, _ any) any {
				if mode == "construction panic" {
					panic("broken handler")
				}
				if mode == "IO panic" {
					return IO(func(context.Context) (any, error) { panic("broken handler") })
				}
				return Fail("broken handler")
			}
			url, _, _ := startHTTPTestServer(t, handler)
			client := &http.Client{Timeout: time.Second}
			response, err := client.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 500 || string(body) != "broken handler" {
				t.Fatalf("failure response = %d %q", response.StatusCode, body)
			}
			logged, err := os.ReadFile(stderr.Name())
			if err != nil || string(logged) != "http handler: broken handler\n" {
				t.Fatalf("stderr = %q, %v", logged, err)
			}
		})
	}
}

func TestHTTPRequestBodyDeadline(t *testing.T) {
	installTestShapes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	_, err := httpTestRequest(server.URL, 20)(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) || err.(*Failure).Kind != TimeoutKind {
		t.Fatalf("body deadline = %#v", err)
	}
}

func TestHTTPServeRequestContexts(t *testing.T) {
	installTestShapes(t)
	started := make(chan struct{}, 1)
	stopped := make(chan error, 1)
	handler := func(_, _, path, _, _, _ any) any {
		return IO(func(ctx context.Context) (any, error) {
			if path == "/hold" {
				started <- struct{}{}
				<-ctx.Done()
				stopped <- ctx.Err()
				return nil, ctx.Err()
			}
			return Tuple3Of(int64(200), Shapes.Nil(), "fast"), nil
		})
	}
	url, stopServer, done := startHTTPTestServer(t, handler)
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for _, cancelServer := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		request, _ := http.NewRequestWithContext(ctx, "GET", url+"/hold", nil)
		returned := make(chan struct{})
		go func() {
			defer close(returned)
			response, err := client.Do(request)
			if err == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				response.Body.Close()
			}
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("handler did not start")
		}
		// A blocked handler does not serialize another request.
		response, err := client.Get(url + "/fast")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != "fast" {
			t.Fatalf("concurrent response = %q", body)
		}
		if cancelServer {
			stopServer()
		} else {
			cancel()
		}
		select {
		case err := <-stopped:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("handler context = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("handler context was not cancelled")
		}
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Fatal("client did not stop")
		}
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("serve = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active serve did not stop within 1 s")
	}
}

func TestHTTPServeBindAndPrecancel(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := int64(listener.Addr().(*net.TCPAddr).Port)
	action := HTTPServe(port, nil).(IO)
	_, err = action(context.Background())
	if failure, ok := err.(*Failure); !ok || failure.Kind != Network {
		t.Fatalf("bind conflict = %#v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = action(ctx)
	if !errors.Is(err, context.Canceled) || err.Error() != "context canceled" {
		t.Fatalf("cancel before bind = %#v", err)
	}
}
