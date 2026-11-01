package types

import (
	"fmt"
	"sort"
	"strings"

	"go4t/ast"
)

type defaultOrigin struct {
	pos      ast.Pos
	end      ast.Pos
	what     string
	priority int
	named    bool // what is a canonical constructor; qualify only when warning
}

func expressionKind(e ast.Expr) string { return strings.TrimPrefix(fmt.Sprintf("%T", e), "*ast.") }

func before(a, b ast.Pos) bool { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }

func (b *bodyChecker) prepareDefaults() {
	b.defaults = make(map[*TMeta]defaultOrigin)
	for _, expr := range b.exprs {
		ann := AnnOf(expr)
		origin := defaultOrigin{pos: expr.GetPos(), what: expressionKind(expr), priority: 1}
		if binding, ok := b.bindings[expr]; ok {
			origin = binding
		}
		switch e := expr.(type) {
		case *ast.ListLit:
			if len(e.Elems) == 0 {
				origin = defaultOrigin{pos: e.GetPos(), what: "[]", priority: 3}
			}
		case *ast.Ident:
			if ann.Ctor != nil && len(ann.Ctor.Fields) == 0 {
				origin = defaultOrigin{pos: e.GetPos(), what: e.Name, priority: 3, named: true}
			}
		}
		origin.end = expr.GetEnd()
		record := func(m *TMeta) {
			old, ok := b.defaults[m]
			if !ok || origin.priority > old.priority || origin.priority == old.priority && before(origin.pos, old.pos) {
				b.defaults[m] = origin
			}
		}
		visitMetas(ann.Type, record)
		for _, inst := range ann.Inst {
			visitMetas(inst, record)
		}
	}
}

func (b *bodyChecker) defaultMeta(meta *TMeta) {
	origin := b.defaults[meta]
	if origin.named {
		origin.what = b.c.program.namesFor(b.c.file).name(origin.what, "")
	}
	b.c.program.Warnings = append(b.c.program.Warnings, &Error{
		Pos: origin.pos, End: origin.end, File: b.c.file,
		Msg: "type of " + origin.what + " defaulted to Unit; add an annotation or use the value",
	})
}

func (b *bodyChecker) sortWarnings(start int) {
	warnings := b.c.program.Warnings[start:]
	sort.SliceStable(warnings, func(i, j int) bool { return before(warnings[i].Pos, warnings[j].Pos) })
}
