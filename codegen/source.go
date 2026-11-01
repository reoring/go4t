package codegen

import (
	"fmt"
	"strings"

	"gofx/ast"
	"gofx/types"
)

type sourceLocation struct {
	file string
	pos  ast.Pos
}

func (g *generator) indexSources(prog *types.Program) {
	g.sources = make(map[ast.Decl]string)
	g.typeSources = make(map[string]sourceLocation)
	g.aliases = make(map[string]*ast.AliasDecl)
	for _, file := range prog.Files {
		for _, decl := range file.Decls {
			g.sources[decl] = file.Name
			if alias, ok := decl.(*ast.AliasDecl); ok {
				g.aliases[alias.Name] = alias
			}
			if data, ok := decl.(*ast.TypeDecl); ok {
				g.typeSources[data.Name] = sourceLocation{file.Name, data.Pos}
				for _, ctor := range data.Ctors {
					g.typeSources[data.Name+"_"+ctor.Name] = sourceLocation{file.Name, ctor.Pos}
				}
			}
		}
	}
}

func (g *generator) line(pos ast.Pos) string {
	if g.fileName == "" || pos.Line < 1 {
		return ""
	}
	if strings.ContainsAny(g.fileName, "\r\n") {
		g.fail(pos, "source filename contains a newline")
	}
	return fmt.Sprintf("//line %s:%d\n", g.fileName, pos.Line)
}

// Parentheses keep a leading directive from ending a return statement (Go's
// semicolon insertion) when a generated function is used in expression position.
func (g *generator) sourceExpr(pos ast.Pos, code string) string {
	if line := g.line(pos); line != "" {
		return "(\n" + line + code + ")"
	}
	return code
}

func (g *generator) typeLine(name string) string {
	location := g.typeSources[name]
	previous := g.fileName
	g.fileName = location.file
	line := g.line(location.pos)
	g.fileName = previous
	return line
}

func (g *generator) typeExprLine(name string) string {
	if line := g.typeLine(name); line != "" {
		return "\n" + line
	}
	return ""
}
