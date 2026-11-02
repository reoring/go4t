package lexer_test

import (
	"testing"

	"go4t/lexer"
)

func TestLawKeywords(t *testing.T) {
	fixed := map[string]lexer.Kind{}
	lexer.VisitFixedTokens(func(kind lexer.Kind, text string) { fixed[text] = kind })
	for _, word := range []string{"law", "proof", "refl", "rewrite"} {
		t.Run(word, func(t *testing.T) {
			got := tokens(t, word+" "+word+"ful;")
			if got[0].Kind != lexer.Keyword || got[0].Text != word || got[1].Kind != lexer.Ident || got[1].Text != word+"ful" {
				t.Fatalf("tokens = %#v", got)
			}
			if kind, ok := fixed[word]; !ok || kind != lexer.Keyword {
				t.Fatalf("VisitFixedTokens[%q] = %v, %t", word, kind, ok)
			}
		})
	}
}

func TestLawKeywordLayout(t *testing.T) {
	for _, tc := range []struct {
		word string
		semi bool
	}{
		{"law", false}, {"proof", false}, {"refl", true}, {"rewrite", false},
	} {
		t.Run(tc.word, func(t *testing.T) {
			got := tokens(t, tc.word+"\nnext;")
			if semi := got[1].Kind == lexer.Punct && got[1].Text == ";"; semi != tc.semi {
				t.Fatalf("newline semicolon = %t, want %t; tokens = %#v", semi, tc.semi, got)
			}
			got = tokens(t, "previous\n"+tc.word+";")
			if got[1].Text != ";" {
				t.Fatalf("%q must not continue a previous statement: %#v", tc.word, got)
			}
		})
	}
}
