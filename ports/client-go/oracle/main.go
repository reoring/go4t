// Copyright 2026 The go4t Authors.
// SPDX-License-Identifier: Apache-2.0

// This executable is an isolated oracle, not a production client-go adapter.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime/debug"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
)

const upstreamVersion = "v0.37.2"
const upstreamRevision = "72c52dd0cc3b9099b36af11290b0304045dd1ada"

type event struct {
	Kind string `json:"kind"`
	Fields map[string]any `json:"fields"`
}

type scenario struct {
	ID string `json:"id"`
	Events []event `json:"events"`
}

type trace struct {
	SchemaVersion int `json:"schema_version"`
	UpstreamVersion string `json:"upstream_version"`
	UpstreamRevision string `json:"upstream_revision"`
	Scenarios []scenario `json:"scenarios"`
}

func require(ok bool, format string, args ...any) error {
	if !ok { return fmt.Errorf(format, args...) }
	return nil
}

// snapshot freezes observed values before later upstream calls can mutate them.
// JSON numbers remain json.Number, not binary64 approximations.
func snapshot(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil { return nil, err }
	var value any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil { return nil, err }
	return value, nil
}


func observe(s *scenario, kind string, fields map[string]any) error {
	frozen, err := snapshot(fields)
	if err != nil { return fmt.Errorf("snapshot %s: %w", kind, err) }
	s.Events = append(s.Events, event{Kind: kind, Fields: frozen.(map[string]any)})
	return nil
}

func errorFields(err error) map[string]any {
	if err == nil { return map[string]any{"error": nil} }
	fields := map[string]any{"message": err.Error(), "go_type": reflect.TypeOf(err).String()}
	var status interface { Status() metav1.Status }
	if errors.As(err, &status) { fields["status"] = status.Status() }
	fields["context_canceled"] = errors.Is(err, context.Canceled)
	fields["deadline_exceeded"] = errors.Is(err, context.DeadlineExceeded)
	return fields
}

type exchange struct {
	method string
	path string
	response runtime.Object
	status int
}

type recorder struct {
	mu sync.Mutex
	s *scenario
	exchanges []exchange
	next int
	err error
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fail := func(err error) {
		if r.err == nil { r.err = err }
		http.Error(w, "oracle fixture failure", http.StatusInternalServerError)
	}
	if r.next >= len(r.exchanges) { fail(fmt.Errorf("unexpected request %s %s", req.Method, req.URL.RequestURI())); return }
	x := r.exchanges[r.next]
	r.next++
	if req.Method != x.method || req.URL.Path != x.path {
		fail(fmt.Errorf("exchange %d: got %s %s, want %s %s", r.next, req.Method, req.URL.Path, x.method, x.path)); return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil { fail(err); return }
	if err := observe(r.s, "http_request", map[string]any{
		"method": req.Method, "path": req.URL.EscapedPath(), "raw_query": req.URL.RawQuery,
		"query": req.URL.Query(), "headers": req.Header, "body": string(body),
	}); err != nil { fail(err); return }
	payload, err := json.Marshal(x.response)
	if err != nil { fail(err); return }
	w.Header().Set("Content-Type", "application/json")
	status := x.status
	if status == 0 { status = http.StatusOK }
	w.WriteHeader(status)
	if _, err := w.Write(payload); err != nil { r.err = err }
}

func withHTTP(s *scenario, exchanges []exchange, run func(context.Context, *kubernetes.Clientset) error) error {
	r := &recorder{s: s, exchanges: exchanges}
	server := httptest.NewServer(r)
	defer server.Close()
	config := &rest.Config{Host: server.URL, UserAgent: "gofx-client-go-oracle/1", Timeout: 10*time.Second}
	config.ContentType = "application/json"
	config.AcceptContentTypes = "application/json"
	client, err := kubernetes.NewForConfig(config)
	if err != nil { return err }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := run(ctx, client); err != nil { return err }
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil { return r.err }
	return require(r.next == len(exchanges), "consumed %d of %d fixture exchanges", r.next, len(exchanges))
}

func observeAction(s *scenario, phase string, action clienttesting.Action) error {
	fields := map[string]any{
		"phase": phase, "go_type": reflect.TypeOf(action).String(), "verb": action.GetVerb(),
		"resource": action.GetResource(), "namespace": action.GetNamespace(), "subresource": action.GetSubresource(),
	}
	switch a := action.(type) {
	case clienttesting.CreateActionImpl:
		fields["object"] = a.GetObject(); fields["options"] = a.GetCreateOptions()
	case clienttesting.UpdateActionImpl:
		fields["object"] = a.GetObject(); fields["options"] = a.GetUpdateOptions()
	case clienttesting.ListActionImpl:
		fields["options"] = a.GetListOptions(); fields["kind"] = a.GetKind()
		fields["labels"] = a.GetListRestrictions().Labels.String()
		fields["fields"] = a.GetListRestrictions().Fields.String()
	default:
		return fmt.Errorf("unhandled fixture action %T", action)
	}
	return observe(s, "fake_action", fields)
}

func run() error {
	output := flag.String("output", "", "also write the successful JSON trace to this path")
	flag.Parse()
	if flag.NArg() != 0 { return fmt.Errorf("unexpected arguments: %v", flag.Args()) }
	info, ok := debug.ReadBuildInfo()
	if !ok { return fmt.Errorf("build information unavailable") }
	found := false
	for _, dependency := range info.Deps {
		if dependency.Path == "k8s.io/client-go" {
			if dependency.Version != upstreamVersion || dependency.Replace != nil {
				return fmt.Errorf("oracle requires un-replaced client-go %s, got %+v", upstreamVersion, dependency)
			}
			found = true
		}
	}
	if !found { return fmt.Errorf("client-go dependency absent from build information") }
	result := trace{SchemaVersion: 1, UpstreamVersion: upstreamVersion, UpstreamRevision: upstreamRevision}
	for _, fixture := range fixtures {
		s := scenario{ID: fixture.id, Events: []event{}}
		if err := fixture.run(&s); err != nil { return fmt.Errorf("scenario %s: %w", fixture.id, err) }
		if len(s.Events) == 0 { return fmt.Errorf("scenario %s emitted no observations", fixture.id) }
		result.Scenarios = append(result.Scenarios, s)
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil { return err }
	b = append(b, '\n')
	if *output != "" {
		if err := os.WriteFile(*output, b, 0644); err != nil { return err }
	}
	_, err = os.Stdout.Write(b)
	return err
}

func main() {
	if err := run(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
