// Package lsp implements go4t's dependency-free LSP 3.17 stdio server.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const maxMessageSize = 16 << 20

func readMessage(r *bufio.Reader) ([]byte, error) {
	length := -1
	total := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && total > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		total += len(line)
		if total > 8192 {
			return nil, errors.New("LSP header exceeds 8192 bytes")
		}
		if line == "\r\n" {
			break
		}
		name, value, ok := strings.Cut(strings.TrimSuffix(line, "\r\n"), ":")
		if !ok {
			return nil, errors.New("invalid LSP header")
		}
		if strings.EqualFold(name, "Content-Length") {
			if length >= 0 {
				return nil, errors.New("duplicate Content-Length")
			}
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 || n > maxMessageSize {
				return nil, errors.New("invalid Content-Length")
			}
			length = n
		}
	}
	if length < 0 {
		return nil, errors.New("missing Content-Length")
	}
	data := make([]byte, length)
	_, err := io.ReadFull(r, data)
	return data, err
}
func writeMessage(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err = io.WriteString(w, header); err != nil {
		return err
	}
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}
func (s *server) reply(id json.RawMessage, result any, failure *rpcError) error {
	if id == nil {
		id = json.RawMessage("null")
	}
	if failure != nil {
		return writeMessage(s.out, struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   *rpcError       `json:"error"`
		}{"2.0", id, failure})
	}
	return writeMessage(s.out, struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result})
}
func (s *server) notify(method string, params any) error {
	return writeMessage(s.out, struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", method, params})
}
