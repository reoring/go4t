package lexer_test

import (
	"strings"
	"testing"

	"go4t/ast"
	"go4t/lexer"
)

func TestPipeToken(t *testing.T) {
	got := tokens(t, "x|>f || y | Z")
	want := []string{"x", "|>", "f", "||", "y", "|", "Z", ";", ""}
	if len(got) != len(want) {
		t.Fatalf("tokens = %#v", got)
	}
	for i, text := range want {
		if got[i].Text != text {
			t.Errorf("token %d = %q, want %q", i, got[i].Text, text)
		}
	}
	if got[1].Kind != lexer.Punct || got[1].Pos != (ast.Pos{Line: 1, Col: 2}) {
		t.Fatalf("pipe token = %#v", got[1])
	}
}

func TestPipeLayout(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"x\n|> f", "x |> f ;"},
		{"x // comment\n\n// another\n |> f", "x |> f ;"},
		{"x |>\n f", "x |> f ;"},
		{"{\n x\n |> f(_)\n |> g\n}", "{ x |> f ( _ ) |> g ; } ;"},
		{"x;\n|> f", "x ; |> f ;"},
		{"x\n|> f\ny", "x |> f ; y ;"},
		{"match x {\n A => y\n |> f\n | _ => z\n}", "match x { A => y |> f | _ => z ; } ;"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			var texts []string
			for _, tok := range tokens(t, tc.src) {
				if tok.Kind != lexer.EOF {
					texts = append(texts, tok.Text)
				}
			}
			if got := strings.Join(texts, " "); got != tc.want {
				t.Fatalf("tokens = %s, want %s", got, tc.want)
			}
		})
	}
}
