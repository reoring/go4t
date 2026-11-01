// Package lexer tokenizes gofx source. Semicolon insertion follows the single
// Layout rule in DESIGN.md, Pass 7.
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
	Float
	String
	InterpTok
	Keyword
	Punct
)

// InterpPart is decoded literal Text, or expression source Expr with Pos set to
// its first character (immediately after '${'). Literal parts have a zero Pos.
// Parts alternate text and expressions, including empty leading/trailing text.
type InterpPart struct {
	Text string
	Expr string
	Pos  ast.Pos
}

type Token struct {
	Kind  Kind
	Text  string // decoded for String; original source, including quotes, for InterpTok
	Pos   ast.Pos
	End   ast.Pos      // exclusive source position, before following whitespace
	Parts []InterpPart // non-nil only for InterpTok
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
	"true": true, "false": true, "given": true, "deriving": true, "alias": true,
}

// puncts ordered longest-first for maximal munch.
var puncts = []string{
	"=>", "<-", "==", "!=", "<=", ">=", "&&", "||", "|>", "..",
	"(", ")", "[", "]", "{", "}", ",", ";", ".", "=", ":", "|", "_", "@",
	"+", "-", "*", "/", "%", "<", ">", "!",
}

type Lexer struct {
	src       []rune
	i         int
	line, col int
	lastKind  Kind
	lastText  string
	lastEnd   ast.Pos // preceding real token, excluding inserted semicolons
	Comments  []ast.Comment
	// nest tracks unclosed delimiters. Newlines inside `(` or `[` never
	// insert `;` (gofx has no trailing commas); `{` resets to statement
	// context so blocks nested in calls still get their statements separated.
	nest []rune
}

func New(src string) *Lexer {
	return NewAt(src, ast.Pos{Line: 1, Col: 1})
}

// NewAt tokenizes src starting at the one-based source position base.
// Columns count runes; a newline advances the line and resets the column to 1.
func NewAt(src string, base ast.Pos) *Lexer {
	return &Lexer{src: []rune(src), line: base.Line, col: base.Col}
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

// needSemi applies all three DESIGN layout conditions at a newline or EOF.
func (l *Lexer) needSemi() bool {
	// Rule 1: only these token classes can end a statement.
	switch l.lastKind {
	case Ident, Int, Float, String, InterpTok:
	case Keyword:
		if l.lastText != "true" && l.lastText != "false" {
			return false
		}
	case Punct:
		switch l.lastText {
		case ")", "]", "}", "_":
		default:
			return false
		}
	default:
		return false
	}
	// Rule 2: the innermost delimiter must be a brace, or absent.
	if n := len(l.nest); n > 0 && l.nest[n-1] != '{' {
		return false
	}
	// Rule 3: a leading continuation token joins the preceding line.
	return !l.continuationAhead()
}

// continuationAhead recognizes DESIGN's leading continuation tokens after
// whitespace and line comments without changing source positions.
func (l *Lexer) continuationAhead() bool {
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
	if i == len(l.src) {
		return false
	}
	switch l.src[i] {
	case '.', ')', ']', '|', '+', '-', '*', '/', '%', '<', '>':
		// Covers single |, binary || and |>, <=, >= and the <- bind arrow.
		return true
	case '=':
		return i+1 < len(l.src) && (l.src[i+1] == '=' || l.src[i+1] == '>')
	case '!':
		return i+1 < len(l.src) && l.src[i+1] == '='
	case '&':
		return i+1 < len(l.src) && l.src[i+1] == '&'
	case 'e', 'g', 'd':
		for _, word := range [...]string{"else", "given", "deriving"} {
			j := 0
			for j < len(word) && i+j < len(l.src) && l.src[i+j] == rune(word[j]) {
				j++
			}
			if j == len(word) {
				return i+j == len(l.src) || !isIdentRune(l.src[i+j])
			}
		}
	}
	return false
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
	t.End = ast.Pos{Line: l.line, Col: l.col}
	if t.Text != ";" && t.Kind != EOF {
		l.lastEnd = t.End
	}
	l.lastKind, l.lastText = t.Kind, t.Text
	if t.Kind == Punct {
		switch t.Text {
		case "(", "[", "{":
			l.nest = append(l.nest, rune(t.Text[0]))
		case ")", "]", "}":
			if len(l.nest) > 0 {
				l.nest = l.nest[:len(l.nest)-1]
			}
		}
	}
	return t
}

func (l *Lexer) next() (Token, error) {
	layoutChecked := false
	for {
		if l.i >= len(l.src) {
			if !layoutChecked && l.needSemi() {
				return l.emit(Token{Kind: Punct, Text: ";", Pos: ast.Pos{Line: l.line, Col: l.col}}), nil
			}
			return l.emit(Token{Kind: EOF, Pos: ast.Pos{Line: l.line, Col: l.col}}), nil
		}
		r := l.peek(0)
		if r == '\n' {
			pos := ast.Pos{Line: l.line, Col: l.col}
			l.advance()
			if !layoutChecked {
				// The previous token and next non-comment token stay the same
				// across blank lines, so check this boundary only once.
				layoutChecked = true
				if l.needSemi() {
					return l.emit(Token{Kind: Punct, Text: ";", Pos: pos}), nil
				}
			}
			continue
		}
		if unicode.IsSpace(r) {
			l.advance()
			continue
		}
		if r == '/' && l.peek(1) == '/' {
			start := l.i
			pos := ast.Pos{Line: l.line, Col: l.col}
			for l.i < len(l.src) && l.peek(0) != '\n' {
				l.advance()
			}
			l.Comments = append(l.Comments, ast.Comment{
				Pos: pos, End: ast.Pos{Line: l.line, Col: l.col},
				Text: string(l.src[start:l.i]), Trailing: l.lastEnd.Line == pos.Line,
			})
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
				return l.emit(Token{Kind: Keyword, Text: s, Pos: pos}), nil
			}
			return l.emit(Token{Kind: Ident, Text: s, Pos: pos}), nil
		case unicode.IsDigit(r):
			start := l.i
			for unicode.IsDigit(l.peek(0)) {
				l.advance()
			}
			kind := Int
			// A number after '.' is a tuple index, including chained t.0.1.
			if l.lastKind != Punct || l.lastText != "." {
				if l.peek(0) == '.' && unicode.IsDigit(l.peek(1)) {
					kind = Float
					l.advance()
					for unicode.IsDigit(l.peek(0)) {
						l.advance()
					}
				}
				if l.peek(0) == 'e' || l.peek(0) == 'E' {
					kind = Float
					l.advance()
					if l.peek(0) == '+' || l.peek(0) == '-' {
						l.advance()
					}
					if !unicode.IsDigit(l.peek(0)) {
						return Token{}, fmt.Errorf("%s: bad float literal: exponent requires digits", pos)
					}
					for unicode.IsDigit(l.peek(0)) {
						l.advance()
					}
				}
			}
			return l.emit(Token{Kind: kind, Text: string(l.src[start:l.i]), Pos: pos}), nil
		case r == '"':
			t, err := l.scanString(pos, true)
			if err != nil {
				return Token{}, err
			}
			return l.emit(t), nil
		}
		for _, p := range puncts {
			if l.hasPrefix(p) {
				for range p {
					l.advance()
				}
				return l.emit(Token{Kind: Punct, Text: p, Pos: pos}), nil
			}
		}
		return Token{}, fmt.Errorf("%s: unexpected character %q", pos, r)
	}
}

// scanString also skips nested strings without allocating their payloads when
// collect is false. Their text is decoded later by the expression's own lexer.
func (l *Lexer) scanString(pos ast.Pos, collect bool) (Token, error) {
	start := l.i
	l.advance() // opening quote
	textStart := l.i
	var parts []InterpPart
	for l.i < len(l.src) && l.peek(0) != '\n' {
		switch l.peek(0) {
		case '\\':
			l.advance()
			if l.i >= len(l.src) || l.peek(0) == '\n' {
				return Token{}, fmt.Errorf("%s: unterminated string", pos)
			}
			l.advance()
		case '"':
			textEnd := l.i
			l.advance()
			if !collect {
				return Token{}, nil
			}
			text, err := l.stringText(textStart, textEnd, pos)
			if err != nil {
				return Token{}, err
			}
			if parts == nil {
				return Token{Kind: String, Text: text, Pos: pos}, nil
			}
			parts = append(parts, InterpPart{Text: text})
			return Token{Kind: InterpTok, Text: string(l.src[start:l.i]), Pos: pos, Parts: parts}, nil
		case '$':
			if l.peek(1) == '$' {
				l.advance()
				l.advance()
				continue
			}
			if l.peek(1) != '{' {
				l.advance()
				continue
			}
			if collect {
				text, err := l.stringText(textStart, l.i, pos)
				if err != nil {
					return Token{}, err
				}
				parts = append(parts, InterpPart{Text: text})
			}
			open := ast.Pos{Line: l.line, Col: l.col}
			l.advance()
			l.advance()
			exprStart := l.i
			exprPos := ast.Pos{Line: l.line, Col: l.col}
			if err := l.scanInterpolation(open); err != nil {
				return Token{}, err
			}
			if collect {
				parts = append(parts, InterpPart{Expr: string(l.src[exprStart : l.i-1]), Pos: exprPos})
			}
			textStart = l.i
		default:
			l.advance()
		}
	}
	return Token{}, fmt.Errorf("%s: unterminated string", pos)
}

func (l *Lexer) stringText(start, end int, pos ast.Pos) (string, error) {
	// Collapse source dollars before decoding escapes so an escaped dollar
	// cannot consume an adjacent source dollar (for example, "\x24$").
	raw := strings.ReplaceAll(string(l.src[start:end]), "$$", "$")
	text, err := strconv.Unquote(`"` + raw + `"`)
	if err != nil {
		return "", fmt.Errorf("%s: bad string literal: %v", pos, err)
	}
	return text, nil
}

func (l *Lexer) scanInterpolation(pos ast.Pos) error {
	depth := 1
	for l.i < len(l.src) {
		switch l.peek(0) {
		case '"':
			if _, err := l.scanString(ast.Pos{Line: l.line, Col: l.col}, false); err != nil {
				return fmt.Errorf("%s: unterminated interpolation", pos)
			}
		case '{':
			l.advance()
			depth++
		case '}':
			l.advance()
			depth--
			if depth == 0 {
				return nil
			}
		case '/':
			if l.peek(1) == '/' {
				for l.i < len(l.src) && l.peek(0) != '\n' {
					l.advance()
				}
			} else {
				l.advance()
			}
		default:
			l.advance()
		}
	}
	return fmt.Errorf("%s: unterminated interpolation", pos)
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
