package main

import (
	"github.com/reoring/go4t/internal/testgo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBindCachedPackages(t *testing.T) {
	testgo.Require(t, 25, "prometheus/client_golang v1.24.1 requires go 1.25")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
	dir := toolsModule(t)
	module, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(dir, "go.mod"), string(module)+"\nrequire github.com/prometheus/client_golang v1.24.1\n")
	for _, p := range []struct{ path, dir string }{{"database/sql", "sql"}, {"github.com/prometheus/client_golang/prometheus", "prom"}} {
		destination := filepath.Join(dir, p.dir)
		if err := os.Mkdir(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		out, stderr, status := toolsCommand(t, dir, "", "bind", p.path, "--shim", p.dir, "-o", destination)
		if out != "" || stderr != "" || status != 0 {
			t.Fatalf("bind %s: stdout=%q stderr=%q status=%d", p.path, out, stderr, status)
		}
		source, err := os.ReadFile(filepath.Join(destination, "bindings.gt"))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s skipped: %d", p.path, strings.Count(string(source), "// skipped:"))
	}
	toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import sql "sql"
import prom "prom"
func main() IO[Unit] { println("bound") }
`)
	out, stderr, status := toolsCommand(t, dir, "", "run", "main.gt")
	// sql.Result binds as ResultGo, so no prelude-shadow warning is printed.
	if out != "bound\n" || stderr != "" || status != 0 {
		t.Fatalf("run cached bindings: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}
