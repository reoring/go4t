package stdhttp

import (
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
	got, err := GetStatus(server.URL)
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
	got, err := GetStatus(server.URL)
	if got != 0 || err == nil {
		t.Fatalf("GetStatus result = (%v, %v), want (0, network error)", got, err)
	}
}
