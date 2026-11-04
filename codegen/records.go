package codegen

import (
	"fmt"
	"strings"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

func (g *generator) commonField(e *ast.Field) string {
	var out strings.Builder
	base := g.fresh()
	fmt.Fprintf(&out, "func() any {\nswitch %s := any(%s).(type) {\n", base, g.expr(e.X))
	for _, record := range g.ann(e).RecordCases {
		fmt.Fprintf(&out, "case *%s:\nreturn %s.F%d\n", ctorName(record.Ctor), base, record.Indexes[0])
	}
	out.WriteString("}\npanic(\"invalid record constructor\")\n}()")
	return out.String()
}

func (g *generator) commonRecordUpdate(e *ast.RecordUpdate) string {
	var out strings.Builder
	base := g.fresh()
	fmt.Fprintf(&out, "func() any {\nvar %s any = %s\n", base, g.expr(e.X))
	// Evaluate the receiver once, then the replacements once in source order.
	// The switch only selects the constructor layout; it cannot reorder work.
	values := make([]string, len(e.Values))
	for i, value := range e.Values {
		values[i] = g.fresh()
		fmt.Fprintf(&out, "var %s any = %s\n", values[i], g.expr(value))
	}
	matched := g.fresh()
	fmt.Fprintf(&out, "switch %s := %s.(type) {\n", matched, base)
	for _, record := range g.ann(e).RecordCases {
		fmt.Fprintf(&out, "case *%s:\n_ = %s\nreturn %s\n", ctorName(record.Ctor), matched,
			updatedRecord(record.Ctor, record.Indexes, matched, values))
	}
	out.WriteString("}\npanic(\"invalid record constructor\")\n}()")
	return out.String()
}

func updatedRecord(ctor *types.Ctor, indexes []int, base string, values []string) string {
	replaced := make([]bool, len(ctor.Fields))
	for _, index := range indexes {
		replaced[index] = true
	}
	fields := make([]string, 0, len(ctor.Fields))
	for index, changed := range replaced {
		if !changed {
			fields = append(fields, fmt.Sprintf("F%d: %s.F%d", index, base, index))
		}
	}
	// Replacements follow source order rather than declaration order, including
	// the single-constructor fast path where values are still expressions.
	for i, value := range values {
		fields = append(fields, fmt.Sprintf("F%d: %s", indexes[i], value))
	}
	return "&" + ctorName(ctor) + "{" + strings.Join(fields, ", ") + "}"
}
