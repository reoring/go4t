package types

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gofx/ast"
)

// Only the failing check traverses source spans. Successful inference continues
// to pass compact positions through unification and dictionary resolution.
func (c *checker) completeSpan(e *Error) {
	if e.End != (ast.Pos{}) {
		return
	}
	if c.decl != nil {
		e.End = ast.EndAt(c.decl, e.Pos)
		if e.End != (ast.Pos{}) {
			return
		}
	}
	for _, file := range c.program.Files {
		if file.Name != e.File {
			continue
		}
		for _, decl := range file.Decls {
			if end := ast.EndAt(decl, e.Pos); end != (ast.Pos{}) {
				e.End = end
			}
		}
	}
}

func (c *checker) instanceHint(class string, arg Type) string {
	head := arg
	if app, ok := head.(*TApp); ok {
		head = prune(app.Fun)
	}
	con, ok := head.(*TCon)
	if !ok || c.program.DataTypes[con.Name] == nil {
		return ""
	}
	switch class {
	case "Show":
		return fmt.Sprintf("; hint: write 'instance Show[%s]' (or 'deriving Debug' for a structural form)", TypeString(arg))
	case "Debug", "Eq", "Ord":
		return fmt.Sprintf("; hint: add 'deriving %s' or write 'instance %s[%s]'", class, class, TypeString(arg))
	}
	return ""
}

type localUse struct {
	name     string
	pos, end ast.Pos
	used     bool
}

// Each binding owns one usage record, even when later bindings shadow it.
// Names with no source span are compiler-generated and cannot be user warnings.
func (b *bodyChecker) trackLocal(env *locals, name string, pos, end ast.Pos) {
	delete(env.uses, name)
	if strings.HasPrefix(name, "_") || pos == (ast.Pos{}) || end == (ast.Pos{}) {
		return
	}
	use := &localUse{name: name, pos: pos, end: ast.Pos{Line: pos.Line, Col: pos.Col + utf8.RuneCountInString(name)}}
	if env.uses == nil {
		env.uses = make(map[string]*localUse)
	}
	env.uses[name] = use
	b.localUses = append(b.localUses, use)
}

func (b *bodyChecker) warnUnused() {
	for _, use := range b.localUses {
		if !use.used {
			b.c.program.Warnings = append(b.c.program.Warnings, &Error{
				File: b.c.file, Pos: use.pos, End: use.end,
				Msg: use.name + " is unused; prefix with _ to silence",
			})
		}
	}
}
