package stdhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
	if got != nil || err == nil {
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
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("GetStatus error = %v, want context.Canceled", err)
	}
}
