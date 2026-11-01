package stdhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gofx/rt"
)

func TestGetStatus(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(server.Close)
	action := GetStatus(server.URL)
	if requests.Load() != 0 {
		t.Fatal("GetStatus executed before its IO ran")
	}
	got, err := action(context.Background())
	if err != nil || got != int64(418) {
		t.Fatalf("GetStatus result = (%v, %v), want (int64(418), nil)", got, err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestGetStatusClosedServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	got, err := GetStatus(server.URL)(context.Background())
	failure, ok := err.(*rt.Failure)
	if got != nil || !ok || failure.Kind != rt.Network {
		t.Fatalf("GetStatus result = (%v, %v), want (nil, network error)", got, err)
	}
}

func TestGetStatusCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := GetStatus(server.URL)(ctx)
		done <- err
	}()
	<-started
	cancel()
	err := <-done
	failure, ok := err.(*rt.Failure)
	if !errors.Is(err, context.Canceled) || !ok || failure.Kind != rt.Cancelled {
		t.Fatalf("GetStatus error = %#v, want classified Cancelled", err)
	}
}
