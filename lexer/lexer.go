// Package lexer tokenizes gofx source. Semicolons are inserted at newlines
// after the same token classes as Go, except before the keyword else.
package lexer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"gofx/ast"
)

type Kind int

const (
	EOF Kind = iota
	Ident
	Int
	String
	Keyword
	Punct
)

type Token struct {
	Kind Kind
	Text string // identifier, keyword, punct text, raw int; decoded string for String
	Pos  ast.Pos
}

func (t Token) String() string {
	if t.Kind == EOF {
		return "end of file"
	}
	return fmt.Sprintf("%q", t.Text)
}

var keywords = map[string]bool{
	"package": true, "class": true, "instance": true, "type": true, "func": true,
	"extern": true, "let": true, "do": true, "match": true, "if": true, "else": true,
	"true": true, "false": true, "given": true,
}

// puncts ordered longest-first for maximal munch.
var puncts = []string{
	"=>", "<-", "==", "!=", "<=", ">=", "&&", "||",
	"(", ")", "[", "]", "{", "}", ",", ";", ".", "=", ":", "|", "_",
	"+", "-", "*", "/", "<", ">", "!",
}

type Lexer struct {
	src       []rune
	i         int
	line, col int
	lastKind  Kind
	lastText  string
}

func New(src string) *Lexer {
	return &Lexer{src: []rune(src), line: 1, col: 1}
}

func (l *Lexer) peek(off int) rune {
	if l.i+off >= len(l.src) {
		return 0
	}
	return l.src[l.i+off]
}

func (l *Lexer) advance() rune {
	r := l.src[l.i]
	l.i++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *Lexer) needSemi() bool {
	switch l.lastKind {
	case Ident, Int, String:
		return true
	case Keyword:
		return l.lastText == "true" || l.lastText == "false"
	case Punct:
		switch l.lastText {
		case ")", "]", "}", "_":
			return true
		}
	}
	return false
}

// elseAhead peeks past whitespace and line comments without changing positions.
func (l *Lexer) elseAhead() bool {
	i := l.i
	for i < len(l.src) {
		if unicode.IsSpace(l.src[i]) {
			i++
			continue
		}
		if l.src[i] == '/' && i+1 < len(l.src) && l.src[i+1] == '/' {
			i += 2
			for i < len(l.src) && l.src[i] != '\n' {
				i++
			}
			continue
		}
		break
	}
	for _, r := range "else" {
		if i >= len(l.src) || l.src[i] != r {
			return false
		}
		i++
	}
	return i == len(l.src) || !isIdentRune(l.src[i])
}

// All tokenizes the whole input.
func (l *Lexer) All() ([]Token, error) {
	var out []Token
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if t.Kind == EOF {
			return out, nil
		}
	}
}

func (l *Lexer) emit(t Token) Token {
	l.lastKind, l.lastText = t.Kind, t.Text
	return t
}

func (l *Lexer) next() (Token, error) {
	insertSemi := l.needSemi()
	for {
		if l.i >= len(l.src) {
			if insertSemi {
				return l.emit(Token{Punct, ";", ast.Pos{Line: l.line, Col: l.col}}), nil
			}
			return l.emit(Token{Kind: EOF, Pos: ast.Pos{Line: l.line, Col: l.col}}), nil
		}
		r := l.peek(0)
		if r == '\n' {
			pos := ast.Pos{Line: l.line, Col: l.col}
			l.advance()
			if insertSemi {
				if !l.elseAhead() {
					return l.emit(Token{Punct, ";", pos}), nil
				}
				// The next token is else; do not repeat lookahead at blank lines.
				insertSemi = false
			}
			continue
		}
		if unicode.IsSpace(r) {
			l.advance()
			continue
		}
		if r == '/' && l.peek(1) == '/' {
			for l.i < len(l.src) && l.peek(0) != '\n' {
				l.advance()
			}
			continue
		}
		pos := ast.Pos{Line: l.line, Col: l.col}
		switch {
		case unicode.IsLetter(r) || r == '_' && isIdentRune(l.peek(1)):
			var sb strings.Builder
			for l.i < len(l.src) && isIdentRune(l.peek(0)) {
				sb.WriteRune(l.advance())
			}
			s := sb.String()
			if keywords[s] {
				return l.emit(Token{Keyword, s, pos}), nil
			}
			return l.emit(Token{Ident, s, pos}), nil
		case unicode.IsDigit(r):
			var sb strings.Builder
			for l.i < len(l.src) && unicode.IsDigit(l.peek(0)) {
				sb.WriteRune(l.advance())
			}
			return l.emit(Token{Int, sb.String(), pos}), nil
		case r == '"':
			start := l.i
			l.advance()
			for {
				if l.i >= len(l.src) || l.peek(0) == '\n' {
					return Token{}, fmt.Errorf("%s: unterminated string", pos)
				}
				c := l.advance()
				if c == '\\' {
					// An escape still needs a character on the same line.
					if l.i >= len(l.src) || l.peek(0) == '\n' {
						return Token{}, fmt.Errorf("%s: unterminated string", pos)
					}
					l.advance()
					continue
				}
				if c == '"' {
					break
				}
			}
			s, err := strconv.Unquote(string(l.src[start:l.i]))
			if err != nil {
				return Token{}, fmt.Errorf("%s: bad string literal: %v", pos, err)
			}
			return l.emit(Token{String, s, pos}), nil
		}
		for _, p := range puncts {
			if l.hasPrefix(p) {
				for range p {
					l.advance()
				}
				return l.emit(Token{Punct, p, pos}), nil
			}
		}
		return Token{}, fmt.Errorf("%s: unexpected character %q", pos, r)
	}
}

func (l *Lexer) hasPrefix(p string) bool {
	for i, r := range []rune(p) {
		if l.peek(i) != r {
			return false
		}
	}
	return true
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
