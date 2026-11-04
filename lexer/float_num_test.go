package lexer_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
)

func TestFloatLiteralTokens(t *testing.T) {
	for _, source := range []string{"1.5", "2e3", "1.0e-3", "2E+3", "0.0", "01.25", "1e309"} {
		t.Run(source, func(t *testing.T) {
			got := tokens(t, source)
			if len(got) != 3 || got[0].Kind != lexer.Float || got[0].Text != source || got[0].Pos != (ast.Pos{Line: 1, Col: 1}) || got[1].Text != ";" {
				t.Fatalf("tokens = %#v", got)
			}
		})
	}
}

func TestFloatTupleAccessBoundaries(t *testing.T) {
	for _, tt := range []struct {
		source string
		kinds  []lexer.Kind
		texts  []string
	}{
		{"t.0", []lexer.Kind{lexer.Ident, lexer.Punct, lexer.Int}, []string{"t", ".", "0"}},
		{"t.1", []lexer.Kind{lexer.Ident, lexer.Punct, lexer.Int}, []string{"t", ".", "1"}},
		{"t.0.1", []lexer.Kind{lexer.Ident, lexer.Punct, lexer.Int, lexer.Punct, lexer.Int}, []string{"t", ".", "0", ".", "1"}},
		{"1.", []lexer.Kind{lexer.Int, lexer.Punct}, []string{"1", "."}},
		{".5", []lexer.Kind{lexer.Punct, lexer.Int}, []string{".", "5"}},
		{"1..2", []lexer.Kind{lexer.Int, lexer.Punct, lexer.Int}, []string{"1", "..", "2"}},
		{"1.5 + t.0", []lexer.Kind{lexer.Float, lexer.Punct, lexer.Ident, lexer.Punct, lexer.Int}, []string{"1.5", "+", "t", ".", "0"}},
	} {
		t.Run(tt.source, func(t *testing.T) {
			var kinds []lexer.Kind
			var texts []string
			for _, token := range tokens(t, tt.source) {
				if token.Kind != lexer.EOF && token.Text != ";" {
					kinds = append(kinds, token.Kind)
					texts = append(texts, token.Text)
				}
			}
			if !reflect.DeepEqual(kinds, tt.kinds) || !reflect.DeepEqual(texts, tt.texts) {
				t.Fatalf("tokens = %v / %v; want %v / %v", kinds, texts, tt.kinds, tt.texts)
			}
		})
	}
}

func TestMalformedFloatExponent(t *testing.T) {
	for _, source := range []string{"1e", "1e+", "1.0e-", "1Efoo"} {
		_, err := lexer.New("\n  " + source).All()
		if err == nil || !strings.HasPrefix(err.Error(), "2:3: bad float literal: exponent requires digits") {
			t.Errorf("%q: error = %v", source, err)
		}
	}
}
