package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
)

var kindNames = [...]string{
	lexer.EOF: "TkEOF", lexer.Ident: "TkIdent", lexer.Int: "TkInt",
	lexer.Float: "TkFloat", lexer.String: "TkString", lexer.InterpTok: "TkInterp",
	lexer.Keyword: "TkKeyword", lexer.Punct: "TkPunct",
}

func printSource(src string, base ast.Pos) {
	tokens, err := lexer.NewAt(src, base).All()
	if err != nil {
		fmt.Printf("ERROR %s\n", err)
		return
	}
	for _, t := range tokens {
		kind := kindNames[t.Kind]
		if t.Kind == lexer.Punct && t.Text == ";" {
			kind = "TkSemi"
		}
		fmt.Printf("%d:%d %s %q\n", t.Pos.Line, t.Pos.Col, kind, t.Text)
		for i, part := range t.Parts {
			if i%2 == 0 {
				fmt.Printf("  text %q\n", part.Text)
			} else {
				fmt.Printf("  expr %d:%d %q\n", part.Pos.Line, part.Pos.Col, part.Expr)
			}
		}
	}
}

func main() {
	paths := os.Args[1:]
	base := ast.Pos{Line: 1, Col: 1}
	if len(paths) > 0 && (paths[0] == "--stdin" || paths[0] == "--base") {
		if paths[0] == "--base" {
			base.Line, _ = strconv.Atoi(paths[1])
			base.Col, _ = strconv.Atoi(paths[2])
		}
		src, err := io.ReadAll(os.Stdin)
		if err != nil { panic(err) }
		printSource(string(src), base)
		return
	}
	if len(paths) == 0 { paths = []string{"examples/parser/main.gt"} }
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil { panic(err) }
		fmt.Println("== " + path)
		printSource(string(src), base)
	}
}
