package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	runtimefs "go4t"
	"go4t/modules"
)

func offlineDependencies(t *testing.T) {
	t.Helper()
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GO4T_PATH", "")
}

func TestInitUsesResolvedCheckout(t *testing.T) {
	offlineDependencies(t)
	parent := toolsModule(t)
	dir := filepath.Join(parent, "nested")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, stderr, status := toolsCommand(t, dir, "", "init", "example.com/app")
	if out != "" || stderr != "" || status != 0 {
		t.Fatalf("init: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	checkout, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	want := "module example.com/app\n\ngo 1.22\n\nrequire go4t v0.0.0\n\nreplace go4t => " + strconv.Quote(checkout) + "\n"
	assertFile(t, filepath.Join(dir, "go.mod"), want)
	assertFile(t, filepath.Join(dir, "go4t.mod"), "module example.com/app\n")
}

func TestInitExtractsAndReusesEmbeddedRuntime(t *testing.T) {
	offlineDependencies(t)
	// Changing XDG_CACHE_HOME must not discard Go's existing build cache.
	if os.Getenv("GOCACHE") == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOCACHE", filepath.Join(cache, "go-build"))
	}
	cache := filepath.Join(t.TempDir(), "cache space")
	t.Setenv("XDG_CACHE_HOME", cache)
	digest, err := runtimefs.Digest()
	if err != nil || len(digest) != 64 {
		t.Fatalf("runtime digest=%q error=%v", digest, err)
	}
	runtimeDir := filepath.Join(cache, "go4t", "runtime", digest)
	for range 2 {
		dir := t.TempDir()
		out, stderr, status := toolsCommand(t, dir, "", "init", "example.com/app")
		if out != "" || stderr != "" || status != 0 {
			t.Fatalf("init: stdout=%q stderr=%q status=%d", out, stderr, status)
		}
		assertFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.22\n\nrequire go4t v0.0.0\n\nreplace go4t => " + strconv.Quote(runtimeDir) + "\n")
		assertFile(t, filepath.Join(dir, "go4t.mod"), "module example.com/app\n")
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 2 {
			t.Fatalf("init must only create manifests: %v, %v", entries, err)
		}
		toolsWrite(t, filepath.Join(dir, "main.gt"), "package main\nfunc main() IO[Unit] { println(\"cached runtime\") }\n")
		out, stderr, status = toolsCommand(t, dir, "", "run", ".")
		if out != "cached runtime\n" || stderr != "" || status != 0 {
			t.Fatalf("run with cached runtime: stdout=%q stderr=%q status=%d", out, stderr, status)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(runtimeDir))
	if err != nil || len(entries) != 1 || entries[0].Name() != digest {
		t.Fatalf("cache should contain only one complete runtime, no extraction leftovers: %v, %v", entries, err)
	}
	assertFile(t, filepath.Join(runtimeDir, "go.mod"), "module go4t\n\ngo "+runtimefs.GoVersion()+"\n")
}

func TestInitRejectsExistingManifests(t *testing.T) {
	for _, name := range []string{"go.mod", "go4t.mod"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			toolsWrite(t, filepath.Join(dir, name), "do not overwrite\n")
			out, stderr, status := toolsCommand(t, dir, "", "init", "example.com/app")
			if out != "" || stderr != "go4t init: "+name+" already exists\n" || status != 1 {
				t.Fatalf("init: stdout=%q stderr=%q status=%d", out, stderr, status)
			}
			assertFile(t, filepath.Join(dir, name), "do not overwrite\n")
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("failed init changed directory: %v, %v", entries, err)
			}
		})
	}
}

func TestDependencyCommandArgumentErrors(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"init"}, "usage: go4t init <module-path>"},
		{[]string{"init", "one", "two"}, "usage: go4t init <module-path>"},
		{[]string{"init", "../escape"}, "go4t init: invalid module path \"../escape\""},
		{[]string{"init", "bad\nmodule name"}, "go4t init: invalid module path \"bad\\nmodule name\""},
		{[]string{"get"}, getUsage},
		{[]string{"get", "example.com/lib"}, getUsage},
		{[]string{"get", "example.com/lib@"}, getUsage},
		{[]string{"get", "example.com/lib@none"}, getUsage},
		{[]string{"get", "example.com/lib@v1.0.0", "extra"}, getUsage},
		{[]string{"get", "-u@v1.0.0"}, getUsage},
	} {
		if err := run(tt.args); err == nil || err.Error() != tt.want {
			t.Fatalf("run(%q)=%v; want %q", tt.args, err, tt.want)
		}
	}
}

func TestGetAddsUpdatesAndRunsLocalDependencyOffline(t *testing.T) {
	offlineDependencies(t)
	root := toolsModule(t)
	library, err := filepath.Abs(filepath.Join("..", "..", "modules", "testdata", "deplib"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, file, string(data)+"replace example.com/lib => "+strconv.Quote(library)+"\n")
	toolsWrite(t, filepath.Join(root, "go4t.mod"), "// application\nmodule example.com/app\n")
	nested := filepath.Join(root, "app")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"v1.0.0", "v1.1.0", "v1.1.0"} {
		out, stderr, status := toolsCommand(t, nested, "", "get", "example.com/lib@"+version)
		if out != "" || status != 0 {
			t.Fatalf("get %s: stdout=%q stderr=%q status=%d", version, out, stderr, status)
		}
		assertFile(t, filepath.Join(root, "go4t.mod"), "// application\nmodule example.com/app\nrequire example.com/lib "+version+"\n")
		selected, err := modules.ResolveGoModule(root, "example.com/lib")
		if err != nil || selected.Version != version || selected.Dir != library {
			t.Fatalf("Go module graph=%+v, error=%v", selected, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Name() != "app" && entry.Name() != "go.mod" && entry.Name() != "go.sum" && entry.Name() != "go4t.mod" {
				t.Fatalf("get created unexpected project artifact %s", entry.Name())
			}
		}
	}
	out, stderr, status := toolsCommand(t, nested, "", "get", "example.com/lib@latest")
	if out != "" || status != 0 {
		t.Fatalf("get latest: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	selected, err := modules.ResolveGoModule(root, "example.com/lib")
	if err != nil || selected.Version == "" || selected.Version == "latest" {
		t.Fatalf("selected version query=%+v, error=%v", selected, err)
	}
	assertFile(t, filepath.Join(root, "go4t.mod"), "// application\nmodule example.com/app\nrequire example.com/lib "+selected.Version+"\n")
	toolsWrite(t, filepath.Join(nested, "main.gt"), `package main
import "example.com/lib/shapes"
func main() IO[Unit] { println(show(shapes.area(6, 7))) }
`)
	out, stderr, status = toolsCommand(t, root, "", "run", nested)
	if out != "42\n" || stderr != "" || status != 0 {
		t.Fatalf("run dependency: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestGetFailurePreservesLanguageManifest(t *testing.T) {
	offlineDependencies(t)
	root := toolsModule(t)
	const source = "module example.com/app\nrequire example.com/existing v1.0.0\n"
	toolsWrite(t, filepath.Join(root, "go4t.mod"), source)
	out, stderr, status := toolsCommand(t, root, "", "get", "example.invalid/missing@v1.0.0")
	if out != "" || status != 1 || !strings.Contains(stderr, "module lookup disabled by GOPROXY=off") || !strings.Contains(stderr, "go4t get: go get example.invalid/missing@v1.0.0:") {
		t.Fatalf("failed get: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	assertFile(t, filepath.Join(root, "go4t.mod"), source)
}

func TestGetRequiresBothProjectManifests(t *testing.T) {
	root := t.TempDir()
	_, stderr, status := toolsCommand(t, root, "", "get", "example.com/lib@v1.0.0")
	if want := "go4t get: no go4t.mod found above current directory; run go4t init <module-path>\n"; stderr != want || status != 1 {
		t.Fatalf("get without project: stderr=%q status=%d; want %q", stderr, status, want)
	}
	toolsWrite(t, filepath.Join(root, "go4t.mod"), "module example.com/app\n")
	_, stderr, status = toolsCommand(t, root, "", "get", "example.com/lib@v1.0.0")
	if want := "go4t get: no go.mod in project root " + root + "\n"; stderr != want || status != 1 {
		t.Fatalf("get without Go module: stderr=%q status=%d; want %q", stderr, status, want)
	}
}

func assertFile(t *testing.T, name, want string) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil || string(data) != want {
		t.Fatalf("%s=%q, error=%v; want %q", name, data, err, want)
	}
}
