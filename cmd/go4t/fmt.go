package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go4t/format"
)

const fmtUsage = "go4t:1:1: usage: go4t fmt [-w|-d|-l] FILE...|DIR"

func runFmt(args []string, out io.Writer) error {
	mode := ""
	var inputs []string
	flags := true
	for _, arg := range args {
		if flags && arg == "--" {
			flags = false
			continue
		}
		if flags && strings.HasPrefix(arg, "-") {
			if arg != "-w" && arg != "-d" && arg != "-l" || mode != "" {
				return errors.New(fmtUsage)
			}
			mode = arg
		} else {
			inputs = append(inputs, arg)
		}
	}
	if len(inputs) == 0 {
		return errors.New(fmtUsage)
	}
	files, err := formatFiles(inputs)
	if err != nil {
		return err
	}
	changed := false
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return diagnostic(file, err)
		}
		formatted, err := format.Source(src)
		if err != nil {
			return diagnostic(file, err)
		}
		different := !bytes.Equal(src, formatted)
		changed = changed || different
		switch mode {
		case "-w":
			if different {
				// Writing an existing file retains its permission bits.
				if err := os.WriteFile(file, formatted, 0o644); err != nil {
					return diagnostic(file, err)
				}
			}
		case "-l":
			if different {
				_, err = fmt.Fprintln(out, file)
			}
		case "-d":
			if different {
				err = formatDiff(out, file, src, formatted)
			}
		default:
			_, err = out.Write(formatted)
		}
		if err != nil {
			return diagnostic(file, err)
		}
	}
	if mode == "-l" && changed {
		return exitStatus(1)
	}
	return nil
}

func formatFiles(inputs []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	add := func(path string) error {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if !seen[absolute] {
			seen[absolute] = true
			files = append(files, filepath.Clean(path))
		}
		return nil
	}
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, diagnostic(input, err)
		}
		if !info.IsDir() {
			if err := add(input); err != nil {
				return nil, diagnostic(input, err)
			}
			continue
		}
		err = filepath.WalkDir(input, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type().IsRegular() && filepath.Ext(path) == ".gt" {
				return add(path)
			}
			return nil
		})
		if err != nil {
			return nil, diagnostic(input, err)
		}
	}
	sort.Strings(files)
	return files, nil
}

// A full-file unified hunk is deterministic, dependency-free and can be applied
// with ordinary patch tools, including when the input lacked a final newline.
func formatDiff(out io.Writer, path string, before, after []byte) error {
	lines := func(b []byte) []string {
		if len(b) == 0 {
			return nil
		}
		return strings.SplitAfter(string(b), "\n")
	}
	old, new := lines(before), lines(after)
	if len(old) > 0 && old[len(old)-1] == "" {
		old = old[:len(old)-1]
	}
	if len(new) > 0 && new[len(new)-1] == "" {
		new = new[:len(new)-1]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n@@ -1,%d +1,%d @@\n", path, path, len(old), len(new))
	for _, part := range []struct {
		prefix string
		lines  []string
	}{{"-", old}, {"+", new}} {
		for _, line := range part.lines {
			b.WriteString(part.prefix)
			b.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				b.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}
