package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLSPStdioCommand(t *testing.T) {
	if os.Getenv("GO4T_LSP_HELPER") == "1" {
		if err := run([]string{"lsp"}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	for _, message := range []string{`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{}}`, `{"jsonrpc":"2.0","id":2,"method":"shutdown"}`, `{"jsonrpc":"2.0","method":"exit"}`} {
		fmt.Fprintf(&input, "Content-Length: %d\r\n\r\n%s", len(message), message)
	}
	command := exec.Command(executable, "-test.run=^TestLSPStdioCommand$")
	command.Env = append(os.Environ(), "GO4T_LSP_HELPER=1")
	command.Stdin = &input
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("command: %v stderr=%s", err, stderr.String())
	}
	if stderr.Len() != 0 || strings.Count(string(output), "Content-Length:") != 2 || !strings.Contains(string(output), `"id":"init"`) || !strings.Contains(string(output), `"hoverProvider":true`) || !strings.Contains(string(output), `"id":2,"result":null`) {
		t.Fatalf("stdout=%s stderr=%s", output, stderr.String())
	}
	if err := run([]string{"lsp", "extra"}); err == nil || err.Error() != usage {
		t.Fatalf("lsp extra: %v", err)
	}
}
