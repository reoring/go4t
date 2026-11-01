// Copyright 2026 The gofx Authors.
// SPDX-License-Identifier: Apache-2.0

// families executes every selected public typed-resource method on original
// client-go. It is part of the isolated oracle module, never the native port.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const version = "v0.37.2"
const revision = "72c52dd0cc3b9099b36af11290b0304045dd1ada"

type caseFile struct {
	SchemaVersion int `json:"schema_version"`
	CaseCount int `json:"case_count"`
	GroupCount int `json:"group_count"`
	Selection string `json:"selection"`
	Source struct {
		Version string `json:"version"`
		Revision string `json:"revision"`
	} `json:"source"`
	Config struct {
		Host string `json:"host"`
		ContentType string `json:"content_type"`
		AcceptContentTypes string `json:"accept_content_types"`
		QPS float32 `json:"qps"`
	} `json:"config"`
	Cases []familyCase `json:"cases"`
}

type familyCase struct {
	ClientSource string `json:"client_source"`
	Function string `json:"function"`
	GroupAccessor string `json:"group_accessor"`
	Input json.RawMessage `json:"input"`
	Namespace string `json:"namespace"`
	Operation string `json:"operation"`
	Options json.RawMessage `json:"options"`
	RequestName *string `json:"request_name"`
	Resource string `json:"resource"`
	ResourceGetter string `json:"resource_getter"`
	Response json.RawMessage `json:"response"`
	Scope string `json:"scope"`
	Source string `json:"source"`
}

type requestTrace struct {
	Method string `json:"method"`
	Path string `json:"path"`
	Query url.Values `json:"query"`
	Body any `json:"body"`
	Accept string `json:"accept"`
	ContentType string `json:"content_type"`
}

type resultTrace struct {
	GoType string `json:"go_type"`
	APIVersion string `json:"api_version"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	Namespace string `json:"namespace"`
	ResourceVersion string `json:"resource_version"`
}

type row struct {
	SchemaVersion int `json:"schema_version"`
	Resource string `json:"resource"`
	Operation string `json:"operation"`
	Requests []requestTrace `json:"requests"`
	Result *resultTrace `json:"result"`
	CastOK bool `json:"cast_ok"`
	Error *string `json:"error"`
}

func exported(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func qualified(t reflect.Type) string {
	if t.Kind() != reflect.Pointer { return "" }
	t = t.Elem()
	return t.PkgPath() + "." + t.Name()
}

func decodeJSON(b []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil { return err }
	var extra any
	if err := d.Decode(&extra); err != io.EOF { return fmt.Errorf("expected one JSON value, trailing decode: %v", err) }
	return nil
}

func isNull(b json.RawMessage) bool {
	return len(b) == 0 || bytes.Equal(bytes.TrimSpace(b), []byte("null"))
}

// inventory reflects the public upstream interfaces, not the case manifest.
// Thus missing, duplicated, or extra resource cases fail before any execution.
func inventory() (map[string]reflect.Type, map[string]bool, error) {
	root := reflect.TypeOf((*kubernetes.Interface)(nil)).Elem()
	resources := make(map[string]reflect.Type)
	groups := make(map[string]bool)
	for i := range root.NumMethod() {
		group := root.Method(i)
		if group.Name == "Discovery" { continue }
		if group.Type.NumIn() != 0 || group.Type.NumOut() != 1 || group.Type.Out(0).Kind() != reflect.Interface {
			return nil, nil, fmt.Errorf("unsupported upstream group accessor %s: %s", group.Name, group.Type)
		}
		groups[group.Name] = true
		gt := group.Type.Out(0)
		for j := range gt.NumMethod() {
			getter := gt.Method(j)
			if getter.Name == "RESTClient" { continue }
			if getter.Type.NumOut() != 1 || getter.Type.Out(0).Kind() != reflect.Interface || getter.Type.NumIn() > 1 || (getter.Type.NumIn() == 1 && getter.Type.In(0).Kind() != reflect.String) {
				return nil, nil, fmt.Errorf("unsupported upstream resource accessor %s.%s: %s", group.Name, getter.Name, getter.Type)
			}
			resources[group.Name+"."+getter.Name] = getter.Type.Out(0)
		}
	}
	return resources, groups, nil
}

func selectedMethod(t reflect.Type) (reflect.Method, error) {
	for _, name := range []string{"Get", "List", "Create", "Evict"} {
		if method, found := t.MethodByName(name); found { return method, nil }
	}
	return reflect.Method{}, fmt.Errorf("resource interface %s has no selected operation", t)
}

func validate(file *caseFile) (map[string]reflect.Type, error) {
	if file.SchemaVersion != 1 || file.Source.Version != version || file.Source.Revision != revision {
		return nil, fmt.Errorf("unexpected case schema or upstream provenance")
	}
	if file.Config.Host != "http://family.invalid" || file.Config.ContentType != "application/json" || file.Config.AcceptContentTypes != "application/json" || file.Config.QPS != -1 {
		return nil, fmt.Errorf("case file does not match fixed family HTTP configuration")
	}
	resources, groups, err := inventory()
	if err != nil { return nil, err }
	if len(resources) != 162 || len(groups) != 55 || len(file.Cases) != len(resources) || file.CaseCount != len(resources) || file.GroupCount != len(groups) {
		return nil, fmt.Errorf("case coverage mismatch: actual upstream %d resources/%d groups, manifest %d/%d declared %d/%d", len(resources), len(groups), len(file.Cases), file.GroupCount, file.CaseCount, file.GroupCount)
	}
	seenKeys := make(map[string]bool)
	seenObjects := make(map[string]bool)
	expectedTypes := make(map[string]reflect.Type)
	counts := map[string]int{}
	for _, c := range file.Cases {
		key := exported(c.GroupAccessor)+"."+exported(c.ResourceGetter)
		t, found := resources[key]
		if !found || seenKeys[key] || seenObjects[c.Resource] { return nil, fmt.Errorf("extra or duplicate case %s (%s)", key, c.Resource) }
		seenKeys[key] = true; seenObjects[c.Resource] = true
		method, err := selectedMethod(t)
		if err != nil { return nil, err }
		if method.Name != c.Operation { return nil, fmt.Errorf("%s selects %s in manifest, upstream selects %s", key, c.Operation, method.Name) }
		counts[c.Operation]++
		var objectType reflect.Type
		switch c.Operation {
		case "Get", "List":
			if method.Type.NumOut() != 2 { return nil, fmt.Errorf("unsupported return shape %s", method.Type) }
			objectType = method.Type.Out(0)
		case "Create", "Evict":
			if method.Type.NumIn() < 2 { return nil, fmt.Errorf("unsupported input shape %s", method.Type) }
			objectType = method.Type.In(1)
		default:
			return nil, fmt.Errorf("unsupported operation %s", c.Operation)
		}
		if qualified(objectType) != c.Resource { return nil, fmt.Errorf("%s: actual concrete object is %s, manifest says %s", key, objectType, c.Resource) }
		if c.Scope != "Cluster" && c.Scope != "Namespaced" { return nil, fmt.Errorf("invalid scope %q", c.Scope) }
		if c.Scope == "Cluster" && c.Namespace != "" || c.Scope == "Namespaced" && c.Namespace != "client-namespace" { return nil, fmt.Errorf("incorrect getter namespace for %s", c.Resource) }
		group, _ := reflect.TypeOf((*kubernetes.Interface)(nil)).Elem().MethodByName(exported(c.GroupAccessor))
		getter, _ := group.Type.Out(0).MethodByName(exported(c.ResourceGetter))
		if (getter.Type.NumIn() == 1) != (c.Scope == "Namespaced") { return nil, fmt.Errorf("scope does not match upstream getter %s", key) }
		if err := validateFixtures(c); err != nil { return nil, fmt.Errorf("%s: %w", c.Resource, err) }
		expectedTypes[c.Resource] = objectType
	}
	if len(seenKeys) != len(resources) || counts["Get"] != 147 || counts["Create"] != 13 || counts["Evict"] != 2 || counts["List"] != 0 {
		return nil, fmt.Errorf("incomplete selected operation coverage: %v", counts)
	}
	return expectedTypes, nil
}

func validateFixtures(c familyCase) error {
	var response map[string]any
	if err := decodeJSON(c.Response, &response); err != nil { return err }
	if c.Operation == "Evict" {
		if response["apiVersion"] != "v1" || response["kind"] != "Status" || response["status"] != "Success" || response["code"] != json.Number("200") { return fmt.Errorf("invalid eviction response fixture") }
	} else {
		metadata, ok := response["metadata"].(map[string]any)
		if !ok || metadata["name"] != "response-name" || metadata["namespace"] != "response-namespace" || metadata["resourceVersion"] != "response-rv" { return fmt.Errorf("invalid object response metadata fixture") }
	}
	if c.Operation == "Get" {
		if c.RequestName == nil || *c.RequestName != "request-name" || !isNull(c.Input) { return fmt.Errorf("invalid Get fixture") }
		var opts metav1.GetOptions
		if err := decodeJSON(c.Options, &opts); err != nil { return err }
		if opts.ResourceVersion != "fixture-rv" { return fmt.Errorf("Get resourceVersion fixture changed") }
		return nil
	}
	if c.RequestName != nil { return fmt.Errorf("unexpected request name") }
	var input map[string]any
	if err := decodeJSON(c.Input, &input); err != nil { return err }
	metadata, ok := input["metadata"].(map[string]any)
	if !ok || metadata["name"] != "input-name" || metadata["namespace"] != "input-namespace" { return fmt.Errorf("invalid input identity fixture") }
	if c.Operation != "Evict" && (input["kind"] != response["kind"] || input["apiVersion"] != response["apiVersion"]) { return fmt.Errorf("input/response GVK fixture mismatch") }
	if c.Operation == "Create" {
		var opts metav1.CreateOptions
		if err := decodeJSON(c.Options, &opts); err != nil { return err }
		if opts.FieldManager != "family-fixture" || opts.FieldValidation != "Strict" || !reflect.DeepEqual(opts.DryRun, []string{"All"}) { return fmt.Errorf("invalid create option fixture") }
	} else if c.Operation == "Evict" {
		var opts map[string]any
		if err := decodeJSON(c.Options, &opts); err != nil { return err }
		if len(opts) != 0 { return fmt.Errorf("Evict must not have create options") }
	}
	return nil
}

type captureTransport struct {
	mu sync.Mutex
	active *familyCase
	requests []requestTrace
	err error
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fail := func(err error) (*http.Response, error) { c.err = err; return nil, err }
	if c.active == nil || len(c.requests) != 0 { return fail(fmt.Errorf("request outside active case or duplicate request")) }
	if req.URL.Scheme != "http" || req.URL.Host != "family.invalid" { return fail(fmt.Errorf("request escaped controlled fixture host: %s", req.URL)) }
	var body any
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil { return fail(err) }
		if err := req.Body.Close(); err != nil { return fail(err) }
		if len(b) > 0 {
			if err := decodeJSON(b, &body); err != nil { return fail(fmt.Errorf("request body is not one JSON value: %w", err)) }
		}
	}
	c.requests = append(c.requests, requestTrace{Method: req.Method, Path: req.URL.Path, Query: req.URL.Query(), Body: body, Accept: req.Header.Get("Accept"), ContentType: req.Header.Get("Content-Type")})
	payload := bytes.Clone(c.active.Response)
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)), Request: req}, nil
}

func execute(client *kubernetes.Clientset, capture *captureTransport, c familyCase, expectedType reflect.Type) (row, error) {
	result := row{SchemaVersion: 1, Resource: c.Resource, Operation: c.Operation}
	capture.mu.Lock()
	capture.active = &c; capture.requests = nil; capture.err = nil
	capture.mu.Unlock()
	group := reflect.ValueOf(client).MethodByName(exported(c.GroupAccessor)).Call(nil)[0]
	getter := group.MethodByName(exported(c.ResourceGetter))
	var getterArgs []reflect.Value
	if getter.Type().NumIn() == 1 { getterArgs = []reflect.Value{reflect.ValueOf(c.Namespace)} }
	resource := getter.Call(getterArgs)[0]
	method := resource.MethodByName(c.Operation)
	args := []reflect.Value{reflect.ValueOf(context.Background())}
	switch c.Operation {
	case "Get":
		var opts metav1.GetOptions
		if err := decodeJSON(c.Options, &opts); err != nil { return result, err }
		args = append(args, reflect.ValueOf(*c.RequestName), reflect.ValueOf(opts))
	case "Create", "Evict":
		object := reflect.New(expectedType.Elem())
		if err := json.Unmarshal(c.Input, object.Interface()); err != nil { return result, fmt.Errorf("construct typed input: %w", err) }
		args = append(args, object)
		if c.Operation == "Create" {
			var opts metav1.CreateOptions
			if err := decodeJSON(c.Options, &opts); err != nil { return result, err }
			args = append(args, reflect.ValueOf(opts))
		}
	default:
		return result, fmt.Errorf("no invocation for operation %s", c.Operation)
	}
	if method.Type().NumIn() != len(args) { return result, fmt.Errorf("unsupported operation argument shape %s", method.Type()) }
	values := method.Call(args)
	capture.mu.Lock()
	result.Requests = capture.requests
	transportError := capture.err
	capture.active = nil
	capture.mu.Unlock()
	if transportError != nil { return result, transportError }
	if len(result.Requests) != 1 { return result, fmt.Errorf("expected one observed request, got %d", len(result.Requests)) }
	errorValue := values[len(values)-1]
	if !errorValue.IsNil() {
		err := errorValue.Interface().(error)
		message := err.Error(); result.Error = &message
		return result, fmt.Errorf("controlled operation unexpectedly failed: %w", err)
	}
	if c.Operation == "Evict" { result.CastOK = true; return result, nil }
	if values[0].IsNil() { return result, fmt.Errorf("nil object returned from successful operation") }
	object, ok := values[0].Interface().(runtime.Object)
	if !ok { return result, fmt.Errorf("result %T is not runtime.Object", values[0].Interface()) }
	actualType := reflect.TypeOf(object)
	result.CastOK = actualType == expectedType && actualType.AssignableTo(expectedType)
	if !result.CastOK { return result, fmt.Errorf("concrete result %s cannot cast to %s", actualType, expectedType) }
	accessor, err := meta.Accessor(object)
	if err != nil { return result, err }
	gvk := object.GetObjectKind().GroupVersionKind()
	apiVersion, kind := gvk.ToAPIVersionAndKind()
	result.Result = &resultTrace{GoType: qualified(actualType), APIVersion: apiVersion, Kind: kind, Name: accessor.GetName(), Namespace: accessor.GetNamespace(), ResourceVersion: accessor.GetResourceVersion()}
	if accessor.GetName() != "response-name" || accessor.GetNamespace() != "response-namespace" || accessor.GetResourceVersion() != "response-rv" { return result, fmt.Errorf("result metadata does not come from response fixture: %+v", result.Result) }
	return result, nil
}

func run() error {
	casePath := flag.String("cases", "../kubernetes/regression/family-cases.json", "family case manifest path (relative to oracle module by default)")
	outputPath := flag.String("output", "", "also write successful NDJSON to this path")
	flag.Parse()
	if flag.NArg() != 0 { return fmt.Errorf("unexpected arguments: %v", flag.Args()) }
	build, ok := debug.ReadBuildInfo()
	if !ok { return fmt.Errorf("build information unavailable") }
	pinned := false
	for _, dependency := range build.Deps {
		if dependency.Path == "k8s.io/client-go" {
			pinned = dependency.Version == version && dependency.Replace == nil
		}
	}
	if !pinned { return fmt.Errorf("oracle requires un-replaced client-go %s", version) }
	b, err := os.ReadFile(*casePath)
	if err != nil { return err }
	var cases caseFile
	if err := decodeJSON(b, &cases); err != nil { return err }
	expectedTypes, err := validate(&cases)
	if err != nil { return err }
	sort.Slice(cases.Cases, func(i,j int) bool { return cases.Cases[i].Resource < cases.Cases[j].Resource })
	capture := &captureTransport{}
	config := &rest.Config{Host: cases.Config.Host, QPS: cases.Config.QPS}
	config.ContentType = cases.Config.ContentType
	config.AcceptContentTypes = cases.Config.AcceptContentTypes
	client, err := kubernetes.NewForConfigAndClient(config, &http.Client{Transport: capture})
	if err != nil { return err }
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for _, c := range cases.Cases {
		observed, err := execute(client, capture, c, expectedTypes[c.Resource])
		if err != nil { return fmt.Errorf("%s: %w", c.Resource, err) }
		if err := encoder.Encode(observed); err != nil { return err }
	}
	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, output.Bytes(), 0644); err != nil { return err }
	}
	if _, err := io.Copy(os.Stdout, &output); err != nil { return err }
	fmt.Fprintf(os.Stderr, "family oracle: %d actual operations, %d groups, 147 Get / 13 Create / 2 Evict; schema_version=1\n", len(cases.Cases), cases.GroupCount)
	return nil
}

func main() {
	if err := run(); err != nil { fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error())); os.Exit(1) }
}
