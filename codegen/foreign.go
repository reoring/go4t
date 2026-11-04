package codegen

import (
	"fmt"
	"strings"

	"github.com/reoring/go4t/types"
)

// The generated program owns all erased record constructors. Register them in
// its companion init (never in the Go shim, which cannot import the program).
// Records use their owning Go package and Go type name, never the calling shim.
func (g *generator) foreignShapes(out *strings.Builder, prog *types.Program) {
	seen := map[string]bool{}
	goOwners := make(map[string]string)
	for _, module := range prog.Modules {
		if owner, ok := strings.CutPrefix(module.ImportPath, "go:"); ok {
			goOwners[module.Path] = owner
		}
	}
	var visit func(string, types.Type)
	visit = func(path string, t types.Type) {
		t = types.Unalias(t)
		switch t := t.(type) {
		case *types.TFunc:
			for _, p := range t.Params {
				visit(path, p)
			}
			visit(path, t.Ret)
		case *types.TApp:
			visit(path, t.Fun)
			for _, a := range t.Args {
				visit(path, a)
			}
		case *types.TCon:
			d := prog.DataTypes[t.Name]
			if d == nil {
				return
			}
			name := prog.ModuleNames[d.Name]
			if name == "" {
				name = d.Name
			}
			recordKey := prog.ForeignRecordKeys[d.Name]
			if recordKey == "" {
				owner := goOwners[prog.Owners[d.Name]]
				if owner == "" {
					owner = path // legacy inspection bindings have no owner metadata
				}
				recordKey = owner + "." + name
			}
			key := recordKey + ":" + d.Name
			if seen[key] {
				return
			}
			seen[key] = true
			for _, c := range d.Ctors {
				for _, f := range c.Fields {
					visit(path, f)
				}
			}
			if len(d.Ctors) != 1 || len(d.TParams) != 0 {
				return
			}
			c := d.Ctors[0]
			if c.FieldNames == nil {
				return
			}
			fmt.Fprintf(out, "%sfunc init() { rt.RegisterForeignRecord(%q,\n%sfunc(fields []any) any { return &%s{", g.typeLine(d.Name), recordKey, g.typeLine(d.Name), ctorName(c))
			for i := range c.Fields {
				value := fmt.Sprintf("fields[%d]", i)
				if scalarRecord(c) {
					value = convert(value, "any", scalar(c.Fields[i]))
				}
				fmt.Fprintf(out, "F%d:%s,", i, value)
			}
			fmt.Fprintf(out, "} },\n%sfunc(value any,index int) any { v := value.(*%s); _ = v; switch index {", g.typeLine(d.Name), ctorName(c))
			for i := range c.Fields {
				fmt.Fprintf(out, "case %d: return v.F%d;", i, i)
			}
			out.WriteString("default: panic(\"go4t foreign record: invalid field index\") } }) }\n")
		}
	}
	for _, name := range sortedKeys(prog.Funcs) {
		f := prog.Funcs[name]
		if f.Decl.Extern != nil {
			visit(f.Decl.Extern.ImportPath, f.Scheme.Type)
		}
	}
}
func (g *generator) dynShapes(out *strings.Builder, prog *types.Program) {
	for _, name := range sortedKeys(prog.DataTypes) {
		d := prog.DataTypes[name]
		if prog.Owners[name] != "std/dyn" {
			continue
		}
		source := prog.ModuleNames[name]
		if source != "Value" {
			continue
		}
		kinds := []string{"null", "bool", "int", "float", "str", "bytes", "time", "list", "map"}
		fields := []string{"DynNull", "DynBool", "DynInt", "DynFloat", "DynStr", "DynBytes", "DynTime", "DynList", "DynMap"}
		if len(d.Ctors) != len(kinds) {
			continue
		}
		fmt.Fprintf(out, "%sfunc init() {\n", g.typeLine(d.Name))
		for i, c := range d.Ctors {
			if i == 0 {
				fmt.Fprintf(out, "rt.Shapes.%s = func() any { return &%s{} }\n", fields[i], ctorName(c))
			} else {
				fmt.Fprintf(out, "rt.Shapes.%s = func(v any) any { return &%s{F0:v} }\n", fields[i], ctorName(c))
			}
		}
		out.WriteString("rt.Shapes.DynView = func(value any) (string,any) { switch v := value.(type) {\n")
		for i, c := range d.Ctors {
			v := "v.F0"
			if i == 0 {
				v = "nil"
			}
			fmt.Fprintf(out, "case *%s: return %q,%s\n", ctorName(c), kinds[i], v)
		}
		out.WriteString("default: panic(\"rt.Shapes.DynView: invalid Value\") } }\n}\n")
	}
}
