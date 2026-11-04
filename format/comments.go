package format

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/reoring/go4t/ast"
)

func compare(a, b ast.Pos) int {
	if a.Line < b.Line || a.Line == b.Line && a.Col < b.Col {
		return -1
	}
	if a == b {
		return 0
	}
	return 1
}
func (p *printer) hasComments(pos, end ast.Pos) bool {
	for _, c := range p.comments {
		if compare(c.Pos, pos) >= 0 && compare(c.Pos, end) < 0 {
			return true
		}
	}
	return false
}

// Law/proof ASTs retain tactics rather than Block nodes. Recover missing brace
// positions only when comments need anchors, without lexing the file again.
// Callers bound the search to a gap with no expression before the delimiter.
func (p *printer) delimiterBetween(delimiter rune, start, end ast.Pos) ast.Pos {
	if len(p.comments) == 0 || p.source == "" || start.Line < 1 || start.Col < 1 {
		return ast.Pos{}
	}
	if p.lineOffsets == nil {
		p.lineOffsets = []int{0}
		for i, r := range p.source {
			if r == '\n' {
				p.lineOffsets = append(p.lineOffsets, i+1)
			}
		}
	}
	if start.Line > len(p.lineOffsets) {
		return ast.Pos{}
	}
	offset := p.lineOffsets[start.Line-1]
	pos := ast.Pos{Line: start.Line, Col: 1}
	for pos.Col < start.Col && offset < len(p.source) {
		r, size := utf8.DecodeRuneInString(p.source[offset:])
		if r == '\n' {
			return ast.Pos{}
		}
		offset += size
		pos.Col++
	}
	comment := false
	for offset < len(p.source) && compare(pos, end) < 0 {
		r, size := utf8.DecodeRuneInString(p.source[offset:])
		if r == '/' && offset+1 < len(p.source) && p.source[offset+1] == '/' {
			comment = true
		}
		if !comment && r == delimiter {
			return pos
		}
		offset += size
		if r == '\n' {
			pos.Line++
			pos.Col = 1
			comment = false
		} else {
			pos.Col++
		}
	}
	return ast.Pos{}
}

// Attach comments to the closest source boundary, preferring the outermost
// node at a shared boundary (an arm rather than its pattern, for example).
// Output offsets are collected by the AST printer, never inferred by reparsing
// formatted text. This also works for ASTs constructed without source text.
func (p *printer) withComments() string {
	source := p.b.String()
	if len(p.comments) == 0 {
		return source
	}
	type insertion struct {
		offset, order int
		text          string
	}
	inserts := make([]insertion, 0, len(p.comments))
	for order, c := range p.comments {
		var best *span
		if c.Trailing {
			for i := range p.spans {
				s := &p.spans[i]
				if s.end.Line != c.Pos.Line || compare(s.end, c.Pos) > 0 {
					continue
				}
				if best == nil || compare(s.end, best.end) > 0 || s.end == best.end && s.stop > best.stop {
					best = s
				}
			}
		} else {
			for i := range p.spans {
				s := &p.spans[i]
				if compare(s.pos, c.End) < 0 {
					continue
				}
				if best == nil || compare(s.pos, best.pos) < 0 || s.pos == best.pos && s.start < best.start {
					best = s
				}
			}
		}
		offset, indent := len(source), 0
		if best != nil {
			indent = best.indent
			if c.Trailing {
				offset = best.stop
			} else {
				offset = best.start
			}
		}
		text := strings.TrimRight(c.Text, " \t\r")
		if c.Trailing && best != nil {
			// Separators and opening braces belong before the comment, not on the
			// continuation line; in particular a record's comma cannot be swallowed.
			for offset < len(source) {
				switch source[offset] {
				case ',', ';', '{', '(', '[':
					offset++
				case ' ':
					j := offset
					for j < len(source) && source[j] == ' ' {
						j++
					}
					if j < len(source) && strings.ContainsRune(",{([", rune(source[j])) {
						offset = j
					} else {
						goto trailing
					}
				default:
					goto trailing
				}
			}
		trailing:
			text = " " + text
			if offset < len(source) && source[offset] != '\n' {
				text += "\n" + strings.Repeat("  ", indent)
			}
		} else {
			line := strings.LastIndexByte(source[:offset], '\n') + 1
			prefix := source[line:offset]
			if strings.TrimSpace(prefix) == "" {
				offset = line
				text = strings.Repeat("  ", indent) + text + "\n"
			} else {
				text = "\n" + strings.Repeat("  ", indent) + text + "\n" + strings.Repeat("  ", indent)
			}
			if best != nil {
				next := best.pos
				if order+1 < len(p.comments) && compare(p.comments[order+1].Pos, next) < 0 {
					next = p.comments[order+1].Pos
				}
				// Preserve detached comment groups, but keep adjacent comment
				// lines together rather than separating each from the anchor.
				if next.Line > c.End.Line+1 {
					text += "\n"
				}
			}
		}
		inserts = append(inserts, insertion{offset, order, text})
	}
	sort.SliceStable(inserts, func(i, j int) bool {
		if inserts[i].offset != inserts[j].offset {
			return inserts[i].offset < inserts[j].offset
		}
		return inserts[i].order < inserts[j].order
	})
	var out strings.Builder
	at := 0
	for _, in := range inserts {
		out.WriteString(source[at:in.offset])
		out.WriteString(in.text)
		at = in.offset
	}
	out.WriteString(source[at:])
	return out.String()
}
