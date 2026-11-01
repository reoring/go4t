package lexer_test

import (
	"reflect"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/lexer"
)

func tokens(t *testing.T, src string) []lexer.Token {
	t.Helper()
	got, err := lexer.New(src).All()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestSemicolonInsertion(t *testing.T) {
	for _, src := range []string{"name", "123", `"text"`, ")", "]", "}", "_", "true", "false"} {
		t.Run(src, func(t *testing.T) {
			for _, ending := range []string{"", "\n", "\n\n", " // comment\n"} {
				got := tokens(t, src+ending)
				if len(got) != 3 || got[1].Kind != lexer.Punct || got[1].Text != ";" || got[2].Kind != lexer.EOF {
					t.Errorf("source %q: tokens = %#v, want token, semicolon, EOF", src+ending, got)
				}
			}
		})
	}
}

func TestNoSemicolonInsertion(t *testing.T) {
	for _, src := range []string{"=>", "<-", "{", "+", "-", "*", "/", "==", "!=", "<", "<=", ">", ">=", "&&", "||", "!", "=", "(", "[", ",", ":", "|", "func", "let"} {
		t.Run(src, func(t *testing.T) {
			got := tokens(t, src+"\nnext")
			if len(got) != 4 || got[1].Text != "next" {
				t.Errorf("tokens = %#v, want no semicolon before next", got)
			}
		})
	}
	got := tokens(t, "name;\n")
	if len(got) != 3 {
		t.Errorf("explicit semicolon duplicated: %#v", got)
	}
}

func TestElseLookahead(t *testing.T) {
	for _, tc := range []struct {
		name string
		gap  string
		pos  ast.Pos
	}{
		{"newline", "\n", ast.Pos{Line: 2, Col: 1}},
		{"whitespace", "\r\n \t\n  ", ast.Pos{Line: 3, Col: 3}},
		{"comment line", "\n// between\n  ", ast.Pos{Line: 3, Col: 3}},
		{"multiple comments", " // trailing\n // else is in a comment\n\n\t// another\n ", ast.Pos{Line: 5, Col: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tokens(t, "if a { x }"+tc.gap+"else { y }")
			want := strings.Fields("if a { x } else { y } ;")
			if len(got) != len(want)+1 || got[len(want)].Kind != lexer.EOF {
				t.Fatalf("tokens = %#v, want %v and EOF", got, want)
			}
			for i, text := range want {
				if got[i].Text != text {
					t.Errorf("token %d = %#v, want %q", i, got[i], text)
				}
			}
			if got[5].Kind != lexer.Keyword || got[5].Pos != tc.pos {
				t.Errorf("else = %#v, want keyword at %s", got[5], tc.pos)
			}
		})
	}
}

func TestElseLookaheadIdentifierBoundary(t *testing.T) {
	for _, name := range []string{"else_", "elsewhere", "else2", "elseα", "else٢"} {
		t.Run(name, func(t *testing.T) {
			got := tokens(t, "x\n"+name+" = 1")
			if len(got) != 7 || got[1].Kind != lexer.Punct || got[1].Text != ";" ||
				got[1].Pos != (ast.Pos{Line: 1, Col: 2}) || got[2].Kind != lexer.Ident || got[2].Text != name {
				t.Fatalf("tokens = %#v, want semicolon before identifier %q", got, name)
			}
		})
	}
}

func TestElseLookaheadDoesNotSkipOtherTokens(t *testing.T) {
	for _, next := range []string{`"else"`, "/ else", "other else", "; else"} {
		t.Run(next, func(t *testing.T) {
			got := tokens(t, "x\n"+next)
			if got[1].Kind != lexer.Punct || got[1].Text != ";" || got[1].Pos != (ast.Pos{Line: 1, Col: 2}) {
				t.Errorf("tokens = %#v, want semicolon at first newline", got)
			}
		})
	}
	got := tokens(t, "x;\nelse")
	if len(got) != 4 || got[1].Text != ";" || got[2].Text != "else" {
		t.Errorf("explicit semicolon removed: %#v", got)
	}
}

func TestGivenKeyword(t *testing.T) {
	got := tokens(t, "given\ngiven_")
	want := []lexer.Token{
		{Kind: lexer.Keyword, Text: "given", Pos: ast.Pos{Line: 1, Col: 1}},
		{Kind: lexer.Ident, Text: "given_", Pos: ast.Pos{Line: 2, Col: 1}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 2, Col: 7}},
		{Kind: lexer.EOF, Pos: ast.Pos{Line: 2, Col: 7}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %#v, want %#v", got, want)
	}
}

func TestCommentsAndPositions(t *testing.T) {
	got := tokens(t, "// heading\n  α // trailing\n// between\n  beta // end")
	want := []lexer.Token{
		{Kind: lexer.Ident, Text: "α", Pos: ast.Pos{Line: 2, Col: 3}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 2, Col: 16}},
		{Kind: lexer.Ident, Text: "beta", Pos: ast.Pos{Line: 4, Col: 3}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 4, Col: 14}},
		{Kind: lexer.EOF, Pos: ast.Pos{Line: 4, Col: 14}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %#v, want %#v", got, want)
	}
	if got := tokens(t, "// only comment"); len(got) != 1 || got[0].Kind != lexer.EOF {
		t.Errorf("comment-only tokens = %#v", got)
	}
}

func TestStringEscapes(t *testing.T) {
	src := `"line\n\t\r\"\\\x41\u03b1\U0001F600"`
	got := tokens(t, src)
	if got[0].Kind != lexer.String || got[0].Text != "line\n\t\r\"\\Aα😀" {
		t.Errorf("decoded token = %#v", got[0])
	}
	got = tokens(t, `"// not a comment"`)
	if got[0].Text != "// not a comment" {
		t.Errorf("string parsed as comment: %#v", got)
	}
}

func TestMaximalMunchPunctuation(t *testing.T) {
	got := tokens(t, "<-< == = => != ! <= >= && || + - * / ( ) [ ] { } , ; . : | _")
	want := strings.Fields("<- < == = => != ! <= >= && || + - * / ( ) [ ] { } , ; . : | _ ;")
	if len(got) != len(want)+1 {
		t.Fatalf("tokens = %#v", got)
	}
	for i, text := range want {
		if got[i].Kind != lexer.Punct || got[i].Text != text {
			t.Errorf("token %d = %#v, want punct %q", i, got[i], text)
		}
	}
}

func TestIdentifierAndKeywordKinds(t *testing.T) {
	got := tokens(t, "_ _name name_2 true false if iffy 42")
	kinds := []lexer.Kind{lexer.Punct, lexer.Ident, lexer.Ident, lexer.Keyword, lexer.Keyword, lexer.Keyword, lexer.Ident, lexer.Int, lexer.Punct, lexer.EOF}
	if len(got) != len(kinds) {
		t.Fatalf("tokens = %#v", got)
	}
	for i, kind := range kinds {
		if got[i].Kind != kind {
			t.Errorf("token %d kind = %v, want %v", i, got[i].Kind, kind)
		}
	}
}

func TestLexerErrorsWithPositions(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"unterminated", "\n  \"abc", "2:3: unterminated string"},
		{"newline", "\n  \"abc\n", "2:3: unterminated string"},
		{"trailing escape", "\n  \"abc\\", "2:3: unterminated string"},
		{"escaped newline", "\n  \"abc\\\n\"", "2:3: unterminated string"},
		{"bad escape", `"\q"`, "1:1: bad string literal:"},
		{"unexpected", "\n @", "2:2: unexpected character '@'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := lexer.New(tc.src).All()
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %v, want prefix %q", err, tc.want)
			}
		})
	}
}
