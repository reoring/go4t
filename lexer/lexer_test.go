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

func TestLayoutMatrix(t *testing.T) {
	type layoutCase struct {
		name, src, want string
	}
	var cases []layoutCase

	// Rule 1: every terminating token class, and all remaining keywords and
	// punctuation. An explicit final semicolon isolates the tested newline.
	previous := []struct {
		name, src, text string
		insert          bool
	}{
		{"identifier", "name", "name", true},
		{"int", "123", "123", true},
		{"float", "1.5", "1.5", true},
		{"string", `"text"`, "text", true},
		{"interpolation", `"${x}"`, `"${x}"`, true},
		{"true", "true", "true", true},
		{"false", "false", "false", true},
		{"close-paren", ")", ")", true},
		{"close-bracket", "]", "]", true},
		{"close-brace", "}", "}", true},
		{"underscore", "_", "_", true},
		{"empty", "", "", false},
	}
	for _, text := range strings.Fields("package class instance type func extern let do match if else given deriving => <- == != <= >= && || ( [ { , ; . = : | @ + - * / < > !") {
		previous = append(previous, struct {
			name, src, text string
			insert          bool
		}{text, text, text, false})
	}
	for _, prev := range previous {
		want := prev.text
		if prev.insert {
			want += " ;"
		}
		cases = append(cases, layoutCase{
			"previous/" + prev.name, prev.src + "\nnext;", strings.TrimSpace(want + " next ;"),
		})
		// EOF follows the same previous-token and nesting rules, with no
		// continuation token. Blank lines and comments never duplicate `;`.
		for _, ending := range []struct{ name, text string }{
			{"bare", ""},
			{"newline", "\n"},
			{"blank-lines", "\n\n"},
			{"comment-newline", " // comment\n"},
			{"comment-eof", " // comment"},
		} {
			cases = append(cases, layoutCase{
				"eof/" + prev.name + "/" + ending.name, prev.src + ending.text, strings.TrimSpace(want),
			})
		}
	}

	// Rule 2: only the innermost delimiter controls layout; braces restore
	// statement layout inside parens/brackets and closing them restores it.
	cases = append(cases,
		layoutCase{"nesting/parens", "f(\n a\n b\n)\nx", "f ( a b ) ; x ;"},
		layoutCase{"nesting/brackets", "[\n a\n b\n]\nx", "[ a b ] ; x ;"},
		layoutCase{"nesting/braces", "{\n a\n b\n}\nx", "{ a ; b ; } ; x ;"},
		layoutCase{"nesting/brackets-in-parens", "f(\n [1,\n 2]\n)\nx", "f ( [ 1 , 2 ] ) ; x ;"},
		layoutCase{"nesting/block-in-parens", "f(func() => {\n a\n b\n})", "f ( func ( ) => { a ; b ; } ) ;"},
		layoutCase{"nesting/restore-parens", "f({\n a\n b\n}\n c\n)\nx", "f ( { a ; b ; } c ) ; x ;"},
		layoutCase{"nesting/restore-brackets", "[{\n a\n b\n}\n c\n]\nx", "[ { a ; b ; } c ] ; x ;"},
		layoutCase{"nesting/restore-braces", "{\nf(\n a\n b\n)\nx\n}", "{ f ( a b ) ; x ; } ;"},
		layoutCase{"eof/open-paren", "(x", "( x"},
		layoutCase{"eof/open-bracket", "[x", "[ x"},
		layoutCase{"eof/open-brace", "{x", "{ x ;"},
		layoutCase{"eof/block-in-parens", "({x", "( { x ;"},
		layoutCase{"eof/restore-parens", "({x}", "( { x }"},
		layoutCase{"eof/comment-only", "// comment", ""},
		layoutCase{"eof/else-continuation", "x\nelse", "x else"},
		layoutCase{"eof/given-continuation", "x\ngiven", "x given"},
		layoutCase{"eof/bar-continuation", "x\n|", "x |"},
		layoutCase{"explicit-semicolon", "x;\nelse", "x ; else"},
		layoutCase{"if-else", "if a { x }\nelse { y }", "if a { x } else { y } ;"},
		layoutCase{"leading-bar", "type T =\n A(Int)\n | B\nx", "type T = A ( Int ) | B ; x ;"},
		layoutCase{"deriving", "type T = A\n | B\n deriving Eq, Ord, Show\nx", "type T = A | B deriving Eq , Ord , Show ; x ;"},
		layoutCase{"let-annotation", "let xs:\n List[Int] =\n []\nxs", "let xs : List [ Int ] = [ ] ; xs ;"},
		layoutCase{"do-let-annotation", "do {\n let p: (Int,\n String) = (1,\n name)\n pure(p)\n}", "do { let p : ( Int , String ) = ( 1 , name ) ; pure ( p ) ; } ;"},
		layoutCase{"match-guard", "match xs {\n Some(x) if valid(x)\n && ready()\n => x\n | _ => 0\n}", "match xs { Some ( x ) if valid ( x ) && ready ( ) => x | _ => 0 ; } ;"},
		layoutCase{"lambda-patterns", "func(\n (a, b),\n _,\n whole @\n Point{x: x, ..},\n c Int\n) => c", "func ( ( a , b ) , _ , whole @ Point { x : x , .. } , c Int ) => c ;"},
		// DESIGN deliberately treats line-leading -x as continuation, not a
		// new unary-minus statement. Use 0 - x or parentheses to start one.
		layoutCase{"leading-unary-minus", "x\n-x;", "x - x ;"},
	)

	// Rule 3: all leading continuation tokens, plus token-boundary negatives.
	// Apply the same whitespace/comment gaps to both outcomes.
	leading := []struct {
		src, text string
		continues bool
	}{
		{"else", "else", true}, {"|", "|", true}, {".", ".", true},
		{")", ")", true}, {"]", "]", true}, {"given", "given", true},
		{"deriving", "deriving", true},
		{"=>", "=>", true}, {"<-", "<-", true},
		{"+", "+", true}, {"-", "-", true}, {"*", "*", true}, {"/", "/", true},
		{"==", "==", true}, {"!=", "!=", true}, {"<", "<", true}, {"<=", "<=", true},
		{">", ">", true}, {">=", ">=", true}, {"&&", "&&", true},
		{"||", "||", true},
		{"elsewhere", "elsewhere", false}, {"else_", "else_", false},
		{"else2", "else2", false}, {"elseα", "elseα", false}, {"else٢", "else٢", false},
		{"given2", "given2", false}, {"given_", "given_", false},
		{"givenα", "givenα", false}, {"given٢", "given٢", false},
		{"deriving2", "deriving2", false}, {"deriving_", "deriving_", false},
		{"derivingα", "derivingα", false}, {"deriving٢", "deriving٢", false},
		{`"else"`, "else", false}, {`"given"`, "given", false},
		{"other else", "other else", false},
		{"=", "=", false}, {"!", "!", false}, {";", ";", false},
		{",", ",", false}, {":", ":", false}, {"@", "@", false},
		{"(", "(", false}, {"[", "[", false}, {"{", "{", false}, {"}", "}", false},
	}
	for _, lead := range leading {
		want := "x "
		if !lead.continues {
			want += "; "
		}
		want += lead.text + " next ;"
		for _, gap := range []struct{ name, text string }{
			{"newline", "\n"},
			{"whitespace", "\r\n \t\n  "},
			{"comment", "\n// between\n  "},
			{"multiple-comments", " // trailing\n // else given | + are comment text\n\n\t// another\n "},
		} {
			cases = append(cases, layoutCase{
				"leading/" + lead.src + "/" + gap.name, "x" + gap.text + lead.src + " next;", want,
			})
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tokens(t, tc.src)
			want := strings.Fields(tc.want)
			if len(got) != len(want)+1 || got[len(want)].Kind != lexer.EOF {
				t.Fatalf("source %q: tokens = %#v, want %v and EOF", tc.src, got, want)
			}
			for i, text := range want {
				if got[i].Text != text || text == ";" && got[i].Kind != lexer.Punct {
					t.Errorf("token %d = %#v, want %q", i, got[i], text)
				}
			}
		})
	}
}

func TestGivenKeyword(t *testing.T) {
	got := tokens(t, "given\ngiven_")
	want := []lexer.Token{
		{Kind: lexer.Keyword, Text: "given", Pos: ast.Pos{Line: 1, Col: 1}, End: ast.Pos{Line: 1, Col: 6}},
		{Kind: lexer.Ident, Text: "given_", Pos: ast.Pos{Line: 2, Col: 1}, End: ast.Pos{Line: 2, Col: 7}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 2, Col: 7}, End: ast.Pos{Line: 2, Col: 7}},
		{Kind: lexer.EOF, Pos: ast.Pos{Line: 2, Col: 7}, End: ast.Pos{Line: 2, Col: 7}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %#v, want %#v", got, want)
	}
}

func TestCommentsAndPositions(t *testing.T) {
	got := tokens(t, "// heading\n  α // trailing\n// between\n  beta // end")
	want := []lexer.Token{
		{Kind: lexer.Ident, Text: "α", Pos: ast.Pos{Line: 2, Col: 3}, End: ast.Pos{Line: 2, Col: 4}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 2, Col: 16}, End: ast.Pos{Line: 3, Col: 1}},
		{Kind: lexer.Ident, Text: "beta", Pos: ast.Pos{Line: 4, Col: 3}, End: ast.Pos{Line: 4, Col: 7}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 4, Col: 14}, End: ast.Pos{Line: 4, Col: 14}},
		{Kind: lexer.EOF, Pos: ast.Pos{Line: 4, Col: 14}, End: ast.Pos{Line: 4, Col: 14}},
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
	got := tokens(t, "<-< == = => != ! <= >= && || + - * / ( ) [ ] { } , ; ... : | @ _")
	want := strings.Fields("<- < == = => != ! <= >= && || + - * / ( ) [ ] { } , ; .. . : | @ _ ;")
	if len(got) != len(want)+1 {
		t.Fatalf("tokens = %#v", got)
	}
	for i, text := range want {
		if got[i].Kind != lexer.Punct || got[i].Text != text {
			t.Errorf("token %d = %#v, want punct %q", i, got[i], text)
		}
	}
}

func TestAsPatternPunctuation(t *testing.T) {
	got := tokens(t, "whole@\nMkStock")
	want := []lexer.Token{
		{Kind: lexer.Ident, Text: "whole", Pos: ast.Pos{Line: 1, Col: 1}, End: ast.Pos{Line: 1, Col: 6}},
		{Kind: lexer.Punct, Text: "@", Pos: ast.Pos{Line: 1, Col: 6}, End: ast.Pos{Line: 1, Col: 7}},
		{Kind: lexer.Ident, Text: "MkStock", Pos: ast.Pos{Line: 2, Col: 1}, End: ast.Pos{Line: 2, Col: 8}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 2, Col: 8}, End: ast.Pos{Line: 2, Col: 8}},
		{Kind: lexer.EOF, Pos: ast.Pos{Line: 2, Col: 8}, End: ast.Pos{Line: 2, Col: 8}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %#v, want %#v", got, want)
	}
}

func TestRecordPatternRestPunctuation(t *testing.T) {
	got := tokens(t, "Point{x: 0, ..\n}")
	want := []lexer.Token{
		{Kind: lexer.Ident, Text: "Point", Pos: ast.Pos{Line: 1, Col: 1}, End: ast.Pos{Line: 1, Col: 6}},
		{Kind: lexer.Punct, Text: "{", Pos: ast.Pos{Line: 1, Col: 6}, End: ast.Pos{Line: 1, Col: 7}},
		{Kind: lexer.Ident, Text: "x", Pos: ast.Pos{Line: 1, Col: 7}, End: ast.Pos{Line: 1, Col: 8}},
		{Kind: lexer.Punct, Text: ":", Pos: ast.Pos{Line: 1, Col: 8}, End: ast.Pos{Line: 1, Col: 9}},
		{Kind: lexer.Int, Text: "0", Pos: ast.Pos{Line: 1, Col: 10}, End: ast.Pos{Line: 1, Col: 11}},
		{Kind: lexer.Punct, Text: ",", Pos: ast.Pos{Line: 1, Col: 11}, End: ast.Pos{Line: 1, Col: 12}},
		{Kind: lexer.Punct, Text: "..", Pos: ast.Pos{Line: 1, Col: 13}, End: ast.Pos{Line: 1, Col: 15}},
		{Kind: lexer.Punct, Text: "}", Pos: ast.Pos{Line: 2, Col: 1}, End: ast.Pos{Line: 2, Col: 2}},
		{Kind: lexer.Punct, Text: ";", Pos: ast.Pos{Line: 2, Col: 2}, End: ast.Pos{Line: 2, Col: 2}},
		{Kind: lexer.EOF, Pos: ast.Pos{Line: 2, Col: 2}, End: ast.Pos{Line: 2, Col: 2}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %#v, want %#v", got, want)
	}
}

func TestTupleAccessTokens(t *testing.T) {
	for _, tc := range []struct {
		src   string
		texts []string
		kinds []lexer.Kind
	}{
		{"p.0", []string{"p", ".", "0"}, []lexer.Kind{lexer.Ident, lexer.Punct, lexer.Int}},
		{"p.0.1", []string{"p", ".", "0", ".", "1"}, []lexer.Kind{lexer.Ident, lexer.Punct, lexer.Int, lexer.Punct, lexer.Int}},
		{"p.0.name", []string{"p", ".", "0", ".", "name"}, []lexer.Kind{lexer.Ident, lexer.Punct, lexer.Int, lexer.Punct, lexer.Ident}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got := tokens(t, tc.src)
			if len(got) != len(tc.texts)+2 {
				t.Fatalf("tokens = %#v", got)
			}
			col := 1
			for i, text := range tc.texts {
				want := lexer.Token{Kind: tc.kinds[i], Text: text, Pos: ast.Pos{Line: 1, Col: col}, End: ast.Pos{Line: 1, Col: col + len(text)}}
				if !reflect.DeepEqual(got[i], want) {
					t.Errorf("token %d = %#v, want %#v", i, got[i], want)
				}
				col += len(text)
			}
			if got[len(tc.texts)].Kind != lexer.Punct || got[len(tc.texts)].Text != ";" || got[len(tc.texts)+1].Kind != lexer.EOF {
				t.Errorf("trailing tokens = %#v", got[len(tc.texts):])
			}
		})
	}
}

func TestIdentifierAndKeywordKinds(t *testing.T) {
	got := tokens(t, "_ _name name_2 true false if iffy deriving deriving_ 42")
	kinds := []lexer.Kind{lexer.Punct, lexer.Ident, lexer.Ident, lexer.Keyword, lexer.Keyword, lexer.Keyword, lexer.Ident, lexer.Keyword, lexer.Ident, lexer.Int, lexer.Punct, lexer.EOF}
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
		{"unexpected", "\n $", "2:2: unexpected character '$'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := lexer.New(tc.src).All()
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %v, want prefix %q", err, tc.want)
			}
		})
	}
}

func TestInterpolationParts(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []lexer.InterpPart
	}{
		{`"hello ${name}: ${1 + 2}!"`, []lexer.InterpPart{
			{Text: "hello "},
			{Expr: "name", Pos: ast.Pos{Line: 1, Col: 10}},
			{Text: ": "},
			{Expr: "1 + 2", Pos: ast.Pos{Line: 1, Col: 19}},
			{Text: "!"},
		}},
		{`"${x}${y}"`, []lexer.InterpPart{
			{}, {Expr: "x", Pos: ast.Pos{Line: 1, Col: 4}},
			{}, {Expr: "y", Pos: ast.Pos{Line: 1, Col: 8}}, {},
		}},
		{`"{${x}}"`, []lexer.InterpPart{
			{Text: "{"}, {Expr: "x", Pos: ast.Pos{Line: 1, Col: 5}}, {Text: "}"},
		}},
		{`"{{${x}}}"`, []lexer.InterpPart{
			{Text: "{{"}, {Expr: "x", Pos: ast.Pos{Line: 1, Col: 6}}, {Text: "}}"},
		}},
		{`"$$${x} $$ $"`, []lexer.InterpPart{
			{Text: "$"}, {Expr: "x", Pos: ast.Pos{Line: 1, Col: 6}}, {Text: " $ $"},
		}},
		{`"\x24${x}"`, []lexer.InterpPart{
			{Text: "$"}, {Expr: "x", Pos: ast.Pos{Line: 1, Col: 8}}, {},
		}},
		{`"a\n${x}\t"`, []lexer.InterpPart{
			{Text: "a\n"}, {Expr: "x", Pos: ast.Pos{Line: 1, Col: 7}}, {Text: "\t"},
		}},
		{`"${show({a: 1}.a)}"`, []lexer.InterpPart{
			{}, {Expr: "show({a: 1}.a)", Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
		{`"${show(Point{a: "quoted \"text\""}.a)}"`, []lexer.InterpPart{
			{}, {Expr: `show(Point{a: "quoted \"text\""}.a)`, Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
		{`"${"nested"}"`, []lexer.InterpPart{
			{}, {Expr: `"nested"`, Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
		{`"${show("{braces}")}"`, []lexer.InterpPart{
			{}, {Expr: `show("{braces}")`, Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
		{`"${show("{{braces}}")}"`, []lexer.InterpPart{
			{}, {Expr: `show("{{braces}}")`, Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
		{`"${show("${Point{a: "x"}.a}")}"`, []lexer.InterpPart{
			{}, {Expr: `show("${Point{a: "x"}.a}")`, Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
		{`"${}"`, []lexer.InterpPart{
			{}, {Expr: "", Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
		{"\"${x + // } and \" are comment text\n 1}\"", []lexer.InterpPart{
			{}, {Expr: "x + // } and \" are comment text\n 1", Pos: ast.Pos{Line: 1, Col: 4}}, {},
		}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got := tokens(t, tc.src)
			if got[0].Kind != lexer.InterpTok || got[0].Text != tc.src || got[0].Pos != (ast.Pos{Line: 1, Col: 1}) {
				t.Fatalf("token = %#v, want interpolation at 1:1", got[0])
			}
			if !reflect.DeepEqual(got[0].Parts, tc.want) {
				t.Errorf("parts = %#v, want %#v", got[0].Parts, tc.want)
			}
			for _, tok := range got[1:] {
				if tok.Parts != nil {
					t.Errorf("non-interpolation token has parts: %#v", tok)
				}
			}
		})
	}
}

func TestLiteralBracesAndDollars(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`"{not interp}"`, "{not interp}"},
		{`"{x}"`, "{x}"},
		{`"{{x}}"`, "{{x}}"},
		{`"{{{{x}}}}"`, "{{{{x}}}}"},
		{`"\x7bx\u007d"`, "{x}"},
		{`"\x7b{{x}}\x7d"`, "{{{x}}}"},
		{`"{"`, "{"},
		{`"}"`, "}"},
		{`"}}"`, "}}"},
		{`"$"`, "$"},
		{`"$value"`, "$value"},
		{`"end$"`, "end$"},
		{`"$$"`, "$"},
		{`"$$$"`, "$$"},
		{`"$$$$"`, "$$"},
		{`"$${x}"`, "${x}"},
		{`"$${"`, "${"},
		{`"\x24$"`, "$$"},
		{`"$\u0024{x}"`, "$${x}"},
		{`"\x24$${x}"`, "$${x}"},
		{`"plain"`, "plain"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got := tokens(t, tc.src)[0]
			if got.Kind != lexer.String || got.Text != tc.want || got.Parts != nil {
				t.Errorf("token = %#v, want plain string %q", got, tc.want)
			}
		})
	}
}

func TestStartingPosition(t *testing.T) {
	got, err := lexer.NewAt("α +\n  \"value ${x}\"", ast.Pos{Line: 7, Col: 11}).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[0].Pos != (ast.Pos{Line: 7, Col: 11}) ||
		got[1].Pos != (ast.Pos{Line: 7, Col: 13}) || got[2].Pos != (ast.Pos{Line: 8, Col: 3}) ||
		got[2].Parts[1].Pos != (ast.Pos{Line: 8, Col: 12}) || got[4].Pos != (ast.Pos{Line: 8, Col: 15}) {
		t.Errorf("offset tokens = %#v", got)
	}
}

func TestInterpolationErrorsWithPositions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"\n  \"bad ${", "2:8: unterminated interpolation"},
		{"\n  \"bad ${x", "2:8: unterminated interpolation"},
		{"\n  \"bad ${x\"", "2:8: unterminated interpolation"},
		{"\n  \"α ${x", "2:6: unterminated interpolation"},
		{"\n  \"bad $$ ${x", "2:11: unterminated interpolation"},
		{`"${Point{a: 1}"`, "1:2: unterminated interpolation"},
		{`"${show("x")"`, "1:2: unterminated interpolation"},
		{`"${x}`, "1:1: unterminated string"},
		{`"\q${x}"`, "1:1: bad string literal:"},
		{`"${x}\q"`, "1:1: bad string literal:"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := lexer.New(tc.src).All()
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %v, want prefix %q", err, tc.want)
			}
		})
	}
}
