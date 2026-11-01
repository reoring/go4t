package codegen

import (
	"fmt"
	"strings"
)

// Register after the shared List/Option initializer, just like Result. Runtime
// JSON parsing constructs the program's actual ADTs, never a parallel JSON ABI.
func (g *generator) jsonShapes(out *strings.Builder) {
	names := []string{"JNull", "JBool", "JNum", "JStr", "JArr", "JObj"}
	kinds := []string{"null", "bool", "number", "string", "array", "object"}
	for i, name := range names {
		ctor := g.ctors[name]
		arity := 1
		if i == 0 {
			arity = 0
		}
		if ctor == nil || ctor.Data.Name != "Json" || len(ctor.Fields) != arity {
			return
		}
	}
	fmt.Fprintf(out, "%sfunc rt_jsonView(j any) (string, any) {\nswitch j := j.(type) {\n", g.typeLine("Json"))
	for i, name := range names {
		field := "j.F0"
		if i == 0 {
			field = "nil"
		}
		fmt.Fprintf(out, "case *%s: return %q, %s\n", ctorName(g.ctors[name]), kinds[i], field)
	}
	out.WriteString("default: panic(\"rt.Shapes.JsonView: invalid Json\")\n}\n}\n")
	fmt.Fprintf(out, "%sfunc init() {\n", g.typeLine("Json"))
	for i, name := range names {
		if i == 0 {
			fmt.Fprintf(out, "rt.Shapes.%s = %sfunc() any { return &%s{} }\n", name, g.typeExprLine(ctorName(g.ctors[name])), ctorName(g.ctors[name]))
		} else {
			fmt.Fprintf(out, "rt.Shapes.%s = %sfunc(v any) any { return &%s{F0: v} }\n", name, g.typeExprLine(ctorName(g.ctors[name])), ctorName(g.ctors[name]))
		}
	}
	out.WriteString("rt.Shapes.JsonView = rt_jsonView\n}\n")
}
