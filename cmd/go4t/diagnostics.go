package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go4t/ast"
	"go4t/modules"
	"go4t/std"
	"go4t/types"
)

var positionedError = regexp.MustCompile(`^(\d+):(\d+):`)

type sourcePosition interface {
	GetPos() ast.Pos
	GetEnd() ast.Pos
}

func diagnostic(file string, err error) error {
	var checked *types.Error
	if errors.As(err, &checked) && checked.File != "" {
		file = checked.File
	}
	var loaded *modules.Error
	if errors.As(err, &loaded) {
		return fmt.Errorf("%s:%s: %s%s", loaded.File, loaded.Pos, loaded.Msg, sourceExcerpt(loaded.File, loaded.Pos, loaded.End))
	}
	pos, end := ast.Pos{Line: 1, Col: 1}, ast.Pos{}
	var positioned sourcePosition
	if errors.As(err, &positioned) && positioned.GetPos() != (ast.Pos{}) {
		pos, end = positioned.GetPos(), positioned.GetEnd()
	} else if match := positionedError.FindStringSubmatch(err.Error()); match != nil {
		pos.Line, _ = strconv.Atoi(match[1])
		pos.Col, _ = strconv.Atoi(match[2])
	}
	excerpt := sourceExcerpt(file, pos, end)
	if positionedError.MatchString(err.Error()) {
		return fmt.Errorf("%s:%w%s", file, err, excerpt)
	}
	return fmt.Errorf("%s:1:1: %w%s", file, err, excerpt)
}

func printWarning(file string, warning *types.Error) {
	if warning.File != "" {
		file = warning.File
	}
	fmt.Fprintf(os.Stderr, "%s:%s: warning: %s%s\n", file, warning.Pos, warning.Msg, sourceExcerpt(file, warning.Pos, warning.End))
}

func sourceExcerpt(file string, pos, end ast.Pos) string {
	source, err := os.ReadFile(file)
	if err != nil && strings.HasPrefix(file, "std/") {
		source, err = std.FS.ReadFile(strings.TrimPrefix(file, "std/"))
	}
	if err != nil {
		return ""
	}
	return renderExcerpt(string(source), pos, end)
}

// Positions use rune columns, not byte offsets. Preserve tabs in the underline
// prefix so it aligns with the source, and never underline beyond the first line.
func renderExcerpt(source string, pos, end ast.Pos) string {
	if pos.Line < 1 {
		return ""
	}
	lines := strings.Split(source, "\n")
	if pos.Line > len(lines) {
		return ""
	}
	line := strings.TrimSuffix(lines[pos.Line-1], "\r")
	runes := []rune(line)
	start := max(0, min(pos.Col-1, len(runes)))
	stop := start + 1
	if end.Line > pos.Line {
		stop = len(runes)
	} else if end.Line == pos.Line {
		stop = min(end.Col-1, len(runes))
	}
	stop = max(start+1, stop)
	var prefix strings.Builder
	for _, r := range runes[:start] {
		if r == '\t' {
			prefix.WriteRune('\t')
		} else {
			prefix.WriteByte(' ')
		}
	}
	return "\n" + line + "\n" + prefix.String() + "^" + strings.Repeat("~", stop-start-1)
}
