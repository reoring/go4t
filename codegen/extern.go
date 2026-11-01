package codegen

import (
	"fmt"
	"strings"

	goextern "gofx/extern"
	"gofx/types"
)

// emitExtern keeps the generated function's any ABI and adapts only its foreign
// call. Dictionary parameters remain on the gofx wrapper, not the Go function.
func (g *generator) emitExtern(f *types.FuncInfo) string {
	ref := f.Decl.Extern
	pkg, err := g.externs.Load(ref.ImportPath)
	if err != nil {
		g.fail(f.Decl.Pos, "extern %s: %v", ref.Name, err)
	}
	foreign, ok := pkg.Lookup(ref.Name)
	if !ok {
		g.fail(f.Decl.Pos, "extern %s: exported Go function not found in %s", ref.Name, ref.ImportPath)
	}
	adapter, err := goextern.Match(foreign, f.Scheme.Type)
	if err != nil {
		g.fail(f.Decl.Pos, "%s", err)
	}
	args := make([]string, len(f.Decl.Params))
	for i, p := range f.Decl.Params {
		args[i] = mangle(p.Name)
	}
	callee := g.imports[ref.ImportPath] + "." + ref.Name
	if adapter.Uniform {
		return fmt.Sprintf("return %s(%s)\n", callee, strings.Join(args, ", "))
	}
	var out strings.Builder
	if adapter.Error {
		// No Go call or mutable slice conversion runs until the IO is executed.
		// Re-running an IO therefore receives a fresh list-to-slice conversion.
		out.WriteString("return rt.IO(func() (any, error) {\n")
	}
	var natural []string
	for i, value := range adapter.Params {
		if value.Kind != goextern.Optional {
			natural = append(natural, unboxExtern(value, args[i]))
			continue
		}
		some := g.ctors["Some"]
		if some == nil || some.Data.Name != "Option" || len(some.Fields) != 1 {
			g.fail(f.Decl.Pos, "extern %s: Option requires the Some constructor", ref.Name)
		}
		payload, present, option := g.fresh(), g.fresh(), g.fresh()
		fmt.Fprintf(&out, "var %s %s\n%s, %s := %s.(*%s)\nif %s { %s = %s }\n", payload, value.Go, option, present, args[i], ctorName(some), present, payload, unboxExtern(value, option+".F0"))
		natural = append(natural, payload, present)
	}
	call := callee + "(" + strings.Join(natural, ", ") + ")"
	result := adapter.Result
	if adapter.Error {
		value, failure := g.fresh(), g.fresh()
		fmt.Fprintf(&out, "%s, %s := %s\nif %s != nil { return nil, rt.ClassifyError(%s) }\nreturn %s, nil\n})\n", value, failure, call, failure, failure, boxExtern(result, value))
	} else if result.Kind == goextern.Optional {
		value, present := g.fresh(), g.fresh()
		fmt.Fprintf(&out, "%s, %s := %s\nif %s { return rt.SomeOf(%s) }\nreturn rt.NoneOf()\n", value, present, call, present, boxExtern(result, value))
	} else {
		fmt.Fprintf(&out, "return %s\n", boxExtern(result, call))
	}
	return out.String()
}

func unboxExtern(value goextern.Value, expr string) string {
	switch value.Kind {
	case goextern.Erased:
		return expr
	case goextern.Strings:
		return "rt.ListToStrings(" + expr + ")"
	case goextern.Ints:
		return "rt.ListToInts(" + expr + ")"
	default:
		if value.Go == "int" {
			return "int(" + expr + ".(int64))"
		}
		return expr + ".(" + value.Go + ")"
	}
}

func boxExtern(value goextern.Value, expr string) string {
	switch value.Kind {
	case goextern.Strings:
		return "rt.ListFromStrings(" + expr + ")"
	case goextern.Ints:
		return "rt.ListFromInts(" + expr + ")"
	default:
		if value.Go == "int" {
			return "int64(" + expr + ")"
		}
		return expr
	}
}
