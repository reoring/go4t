package rt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var handlerLog io.Writer = os.Stderr

// HTTPRequest returns (status, headers, body); std/http constructs the Response.
// A nonempty path replaces the URL path; explicit query pairs append to its query.
func HTTPRequest(method, address, path, query, headers, body, millis any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		n := millis.(int64)
		if n <= 0 {
			return nil, ClassifyError(context.DeadlineExceeded)
		}
		if n > math.MaxInt64/int64(time.Millisecond) {
			return nil, &Failure{Kind: FailureKind, Message: "requestWithTimeout: duration out of range"}
		}
		ctx, cancel := context.WithTimeout(ctx, time.Duration(n)*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, method.(string), address.(string), strings.NewReader(body.(string)))
		if err != nil {
			return nil, ClassifyError(&url.Error{Op: method.(string), URL: address.(string), Err: err})
		}
		if path := path.(string); path != "" {
			req.URL.Path, req.URL.RawPath = path, ""
		}
		values := make(url.Values)
		forEachHTTPPair(query, func(key, value string) { values.Add(key, value) })
		if encoded := values.Encode(); encoded != "" {
			if req.URL.RawQuery != "" {
				req.URL.RawQuery += "&"
			}
			req.URL.RawQuery += encoded
		}
		forEachHTTPPair(headers, func(key, value string) {
			if strings.EqualFold(key, "Host") {
				req.Host = value
			} else {
				req.Header.Add(key, value)
			}
		})
		// The context bounds connect, redirects, response headers and body reads.
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, ClassifyError(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, ClassifyError(&url.Error{Op: method.(string), URL: address.(string), Err: err})
		}
		return Tuple3Of(int64(response.StatusCode), httpPairs(response.Header), string(data)), nil
	})
}

func forEachHTTPPair(list any, f func(string, string)) {
	for {
		head, tail, ok := Shapes.Uncons(list)
		if !ok {
			return
		}
		key, value := Shapes.Untuple2(head)
		f(key.(string), value.(string))
		list = tail
	}
}

// httpPairs sorts keys while retaining the order of repeated values.
func httpPairs(values map[string][]string) any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	list := Shapes.Nil()
	for i := len(keys) - 1; i >= 0; i-- {
		key := keys[i]
		for j := len(values[key]) - 1; j >= 0; j-- {
			list = Shapes.Cons(Tuple2Of(key, values[key][j]), list)
		}
	}
	return list
}

// HTTPHeader finds the first matching header without changing its value.
func HTTPHeader(headers, name any) any {
	for {
		head, tail, ok := Shapes.Uncons(headers)
		if !ok {
			return NoneOf()
		}
		key, value := Shapes.Untuple2(head)
		if strings.EqualFold(key.(string), name.(string)) {
			return SomeOf(value)
		}
		headers = tail
	}
}

// HTTPServe passes six Request fields to the go4t adapter and receives a
// (status, headers, body) tuple. net/http supplies each request's goroutine.
func HTTPServe(port, handler any) any {
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		listener, err := net.Listen("tcp", ":"+strconv.FormatInt(port.(int64), 10))
		if err != nil {
			return nil, ClassifyError(err)
		}
		return serveHTTP(ctx, listener, handler)
	})
}

func httpHandler(handler any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := runConcurrentIO(r.Context(), IO(func(ctx context.Context) (any, error) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, ClassifyError(err)
			}
			headers := httpPairs(r.Header)
			if r.Host != "" {
				headers = Shapes.Cons(Tuple2Of("Host", r.Host), headers)
			}
			return handler.(func(any, any, any, any, any, any) any)(r.Method, r.URL.String(), r.URL.Path, httpPairs(r.URL.Query()), headers, string(body)).(IO)(ctx)
		}))
		if err != nil {
			var failure *Failure
			if r.Context().Err() == nil || !errors.As(err, &failure) || failure.Kind != Cancelled {
				fmt.Fprintln(handlerLog, "http handler:", err)
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, err.Error())
			return
		}
		status, headers, body := Shapes.Untuple3(value)
		forEachHTTPPair(headers, func(key, value string) { w.Header().Add(key, value) })
		w.WriteHeader(int(status.(int64)))
		_, _ = io.WriteString(w, body.(string))
	}
}

func serveHTTP(ctx context.Context, listener net.Listener, handler any) (any, error) {
	server := &http.Server{
		BaseContext: func(net.Listener) context.Context { return ctx },
		Handler:     httpHandler(handler),
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		_ = server.Close()
		return nil, ClassifyError(err)
	case <-ctx.Done():
		grace, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(grace); err != nil {
			_ = server.Close()
		}
		<-done
		return nil, ClassifyError(ctx.Err())
	}
}
