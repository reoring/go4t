package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"go4t/parser"
)

func TestSqlServiceExample(t *testing.T) {
	offlineDependencies(t)
	for _, target := range []string{"modernc.org/sqlite@v1.59.0", "github.com/prometheus/client_golang@v1.24.1"} {
		command := exec.Command("go", "list", "-m", "-json", target)
		data, err := command.Output()
		if err != nil {
			t.Skipf("SQL service needs cached %s (GOPROXY=off): %v", target, err)
		}
		var module struct{ Dir, Version string }
		if err := json.Unmarshal(data, &module); err != nil {
			t.Fatal(err)
		}
		_, version, _ := strings.Cut(target, "@")
		if module.Dir == "" || module.Version != version {
			t.Skipf("SQL service needs extracted cached %s", target)
		}
		if _, err := os.Stat(module.Dir); err != nil {
			t.Skipf("SQL service needs extracted cached %s: %v", target, err)
		}
		t.Logf("cached %s: %s", target, module.Dir)
	}
	root := t.TempDir()
	checkout, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go.mod", "go.sum", "go4t.mod", "deps.go", "main.gt", "glue/glue.go"} {
		data, err := os.ReadFile(filepath.Join(checkout, "examples/sql-service", name))
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		if name == "go.mod" {
			source = strings.Replace(source, "replace go4t => ../..", "replace go4t => "+strconv.Quote(checkout), 1)
		}
		if name == "main.gt" {
			if _, err := parser.Parse(source); err != nil {
				t.Fatalf("SQL service source: %v", err)
			}
		}
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		toolsWrite(t, file, source)
	}
	// A killed go4t run may not execute its deferred temporary-binary cleanup.
	// Keep all child scratch space inside the test's owned cleanup directory.
	t.Setenv("TMPDIR", t.TempDir())
	probe := toolsWrite(t, filepath.Join(root, "probe.gt"), "package main\nimport \"go:example.com/sql-service/glue\"\n")
	out, stderr, status := toolsCommand(t, root, "", "eval", "glue.renderMetrics()", probe)
	if status != 0 || stderr != "" || !strings.Contains(out, "go_goroutines") || !strings.Contains(out, "process_cpu_seconds_total") {
		t.Fatalf("metrics bridge: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Fatal("SQL service smoke requires curl:", err)
	}
	log, err := os.Create(filepath.Join(root, "service.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	database := filepath.Join(root, "service.db")
	service := exec.Command(os.Args[0], "-test.run=^TestCLIToolsProcess$", "--", "run", "main.gt", "--", strconv.Itoa(port), database)
	service.Dir = root
	service.Env = append(os.Environ(), "GO4T_CLI_TOOLS_PROCESS=1")
	service.Stdout, service.Stderr = log, log
	service.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var serviceErr error
	go func() {
		serviceErr = service.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = syscall.Kill(-service.Process.Pid, syscall.SIGKILL)
			<-done
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	ready := make(chan struct{})
	var countBody []byte
	var curlErr error
	go func() {
		command := exec.CommandContext(ctx, curl, "--fail", "--silent", "--show-error", "--max-time", "2", "--retry", "60", "--retry-connrefused", "--retry-delay", "1", "--retry-max-time", "60", url+"/count")
		countBody, curlErr = command.Output()
		close(ready)
	}()
	select {
	case <-done:
		cancel()
		<-ready
		data, _ := os.ReadFile(log.Name())
		t.Fatalf("service exited before readiness: %v\n%s", serviceErr, data)
	case <-ready:
		if curlErr != nil {
			data, _ := os.ReadFile(log.Name())
			t.Fatalf("curl /count: %v\n%s", curlErr, data)
		}
	}
	var count struct{ Count int }
	if err := json.Unmarshal(countBody, &count); err != nil || count.Count != 3 {
		t.Fatalf("/count=%q, decode error=%v; want {\"count\":3}", countBody, err)
	}
	metrics, err := exec.Command(curl, "--fail", "--silent", "--show-error", "--max-time", "5", url+"/metrics").CombinedOutput()
	if err != nil || !strings.Contains(string(metrics), "# HELP go_goroutines") || !strings.Contains(string(metrics), "process_cpu_seconds_total") {
		t.Fatalf("curl /metrics: %v\n%s", err, metrics)
	}
	if info, err := os.Stat(database); err != nil || info.Size() == 0 {
		t.Fatalf("SQLite file missing or empty: %v", err)
	}
}
