package std_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/codegen"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

func TestHTTPModule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get":
			w.Header().Set("X-Reply", "yes")
			w.WriteHeader(404)
			_, _ = io.WriteString(w, "missing")
		case "/post":
			body, _ := io.ReadAll(r.Body)
			if r.Method != "POST" || r.Header.Get("Content-Type") != "text/plain" || string(body) != "hello" {
				t.Errorf("post = %s %v %q", r.Method, r.Header, body)
			}
			_, _ = io.WriteString(w, string(body))
		case "/custom":
			if r.Method != "PATCH" || r.URL.Query().Get("a") != "日" || r.Header.Get("X-Send") != "yes" {
				t.Errorf("custom = %s %s %v", r.Method, r.URL, r.Header)
			}
			w.WriteHeader(202)
		case "/slow":
			<-r.Context().Done()
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	defer server.Close()
	source := fmt.Sprintf(`package main
import "std/http"
import "std/io"
import "std/json"
type Sum = Sum{sum: Int} deriving json.Encode, json.Decode
func main() IO[Unit] {
  let base = %q
  let custom = http.Request("PATCH", base, "/custom", [("a", "日")], [("X-Send", "yes"), ("x-send", "second")], "")
  let reply = http.jsonResponse(201, Sum(3))
  let decoded: Result[String, Sum] = do { value <- json.parseJson(reply.body); json.decode(value) }
  let simple = http.response(404, "not found")
  do {
    r <- http.get(base + "/get")
    println(show(r.status == 404 && r.body == "missing" && http.responseHeader(r, "x-REPLY") == Some("yes")))
    posted <- http.post(base + "/post", "text/plain", "hello")
    println(show(posted.status == 200 && posted.body == "hello"))
    changed <- http.request(custom)
    println(show(changed.status == 202 && http.requestHeader(custom, "x-send") == Some("yes") && http.requestHeader(custom, "absent") == None))
    timed <- io.tryIO(http.requestWithTimeout(http.Request("GET", base + "/slow", "", [], [], ""), 20))
    println(show(match timed { Err(io.Timeout) => true; _ => false }))
    println(show(reply.status == 201 && http.responseHeader(reply, "CONTENT-TYPE") == Some("application/json") && (match decoded { Ok(value) => value.sum == 3; _ => false })))
    println(show(simple.status == 404 && simple.body == "not found" && http.responseHeader(simple, "Content-Type") == None))
  }
}
`, server.URL)
	file, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	file.Name = "http_test.gt"
	loaded, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	generated["zz_shapes_test_support.go"] = []byte(`package main
import "github.com/reoring/go4t/rt"
func init() {
  triple := rt.Shapes.Tuple3(int64(7), "two", true)
  a, b, c := rt.Shapes.Untuple3(triple)
  if a != int64(7) || b != "two" || c != true { panic("tuple3 shape round trip") }
}
`)
	dir := t.TempDir()
	args := []string{"run"}
	for name, source := range generated {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, source, 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}
	// Explicit go run file arguments must follow normal directory init order.
	sort.Strings(args[1:])
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", args...)
	output, err := command.CombinedOutput()
	if err != nil || string(output) != strings.Repeat("true\n", 6) {
		t.Fatalf("HTTP module: %v\n%s", err, output)
	}
}

func TestHTTPJSONResponseRequiresEncode(t *testing.T) {
	file, err := parser.Parse(`package main
import "std/http"
type Hidden = Hidden
func main() IO[Unit] { let response = http.jsonResponse(200, Hidden); println(response.body) }
`)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	_, err = types.Check(loaded)
	if err == nil || !strings.Contains(err.Error(), "no instance") || !strings.Contains(err.Error(), "Encode[Hidden]") {
		t.Fatalf("missing Encode error = %v", err)
	}
}
