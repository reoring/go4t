package codegen

import (
	"fmt"
	"strings"

	"github.com/reoring/go4t/types"
)

// Like Result and JSON, IoError registers after the shared List/Option shapes.
// Resolve its module identity so user constructors named Failure cannot collide.
func (g *generator) ioErrorShapes(out *strings.Builder, prog *types.Program) {
	var data *types.DataType
	for _, module := range prog.Modules {
		if module.ImportPath != "std/io" {
			continue
		}
		for name, candidate := range prog.DataTypes {
			if prog.Owners[name] == module.Path && prog.ModuleNames[name] == "IoError" {
				data = candidate
				break
			}
		}
	}
	if data == nil {
		return
	}
	names := []string{"NotFound", "PermissionDenied", "AlreadyExists", "Timeout", "Cancelled", "Panic", "Network", "Failure"}
	ctors := make(map[string]*types.Ctor, len(names))
	for _, ctor := range data.Ctors {
		ctors[prog.ModuleNames[ctor.Name]] = ctor
	}
	for _, name := range names {
		ctor := ctors[name]
		arity := 1
		if name == "Timeout" || name == "Cancelled" {
			arity = 0
		}
		if ctor == nil || len(ctor.Fields) != arity {
			return
		}
	}
	fmt.Fprintf(out, "%sfunc rt_ioError(kind string, msg string) any {\nswitch kind {\n", g.typeLine(data.Name))
	for _, name := range names {
		field := "F0: msg"
		if len(ctors[name].Fields) == 0 {
			field = ""
		}
		fmt.Fprintf(out, "case %q: return &%s{%s}\n", name, ctorName(ctors[name]), field)
	}
	out.WriteString("default: panic(\"rt.Shapes.IoError: invalid kind\")\n}\n}\n")
	fmt.Fprintf(out, "%sfunc rt_ioErrorOf(v any) (string, string) {\nswitch v := v.(type) {\n", g.typeLine(data.Name))
	for _, name := range names {
		message := "v.F0.(string)"
		switch name {
		case "Timeout":
			message = `"context deadline exceeded"`
		case "Cancelled":
			message = `"context canceled"`
		}
		fmt.Fprintf(out, "case *%s: return %q, %s\n", ctorName(ctors[name]), name, message)
	}
	out.WriteString("default: panic(\"rt.Shapes.IoErrorOf: invalid IoError\")\n}\n}\n")
	fmt.Fprintf(out, "%sfunc init() {\nrt.Shapes.IoError = rt_ioError\nrt.Shapes.IoErrorOf = rt_ioErrorOf\n}\n", g.typeLine(data.Name))
}
