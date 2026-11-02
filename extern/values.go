package extern

import (
	"fmt"
	"go/constant"
	"go/types"
	"math"
	"sort"
	"strings"
)

func exportedValues(pkg *types.Package) []types.Object {
	if pkg == nil {
		return nil
	}
	var values []types.Object
	for _, name := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch obj.(type) {
		case *types.Var, *types.Const:
			values = append(values, obj)
		}
	}
	return values
}

func (g *shimGenerator) values(decl *strings.Builder, set *mirrorSet, pkg *Package, options BindOptions, used, emitted map[string]bool) {
	qualifier := g.qualifier(types.NewPackage(pkg.Path, pkg.Name))
	for _, obj := range exportedValues(pkg.goPackage) {
		name, reason := bindingName(obj.Name(), options.Prefix)
		if reason == "" && emitted[name] {
			reason = "binding name collides with existing binding"
		}
		var m *foreignMirror
		if c, ok := obj.(*types.Const); ok {
			typ := types.Default(c.Type())
			m = set.valueMirror(typ, map[types.Type]bool{})
			if basic, ok := c.Type().(*types.Basic); ok && basic.Info()&types.IsUntyped != 0 && m != nil {
				switch m.kind {
				case "int":
					value, ok := constant.Int64Val(c.Val())
					if !ok || (typ == types.Typ[types.Rune] && (value < math.MinInt32 || value > math.MaxInt32)) {
						reason = "constant overflows " + types.TypeString(typ, nil)
					}
				case "float":
					value, _ := constant.Float64Val(c.Val())
					if math.IsInf(value, 0) {
						reason = "constant overflows float64"
					}
				}
			}
		} else {
			m = set.resultMirror(obj.Type())
		}
		if reason == "" && m == nil {
			reason = "unsupported value type " + types.TypeString(obj.Type(), nil)
		}
		if reason != "" {
			fmt.Fprintf(decl, "// skipped: %s (%s)\n", obj.Name(), reason)
			continue
		}
		used[name], emitted[name] = true, true
		if options.Prefix == "" && options.Prelude[name] {
			fmt.Fprintf(decl, "// collides with prelude: %s\n", name)
		}
		shimName, value := obj.Name(), obj.Name()
		if qualifier == "" {
			shimName = "Go4t_" + shimName
		} else {
			value = qualifier + "." + value
		}
		fmt.Fprintf(decl, "extern func %s() %s = %q.%s\n", name, m.gt, g.path, shimName)
		converter := g.conversion(m)
		fmt.Fprintf(&g.body, "func %s() any { return %sFrom(%s) }\n", shimName, converter, value)
	}
}

func (g *shimGenerator) constructors(decl *strings.Builder, set *mirrorSet, options BindOptions, used, emitted map[string]bool) {
	names := make([]string, 0, len(set.handles))
	for name := range set.handles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		n := set.handles[name]
		if _, ok := n.Underlying().(*types.Struct); !ok {
			continue
		}
		goName := "New" + n.Obj().Name()
		binding, reason := bindingName(goName, options.Prefix)
		if used[binding] || emitted[binding] {
			reason = "constructor name collides with existing binding"
		}
		if reason != "" {
			fmt.Fprintf(decl, "// skipped: %s (%s)\n", goName, reason)
			continue
		}
		used[binding], emitted[binding] = true, true
		shimName := "New_" + n.Obj().Name()
		fmt.Fprintf(decl, "extern func %s() %s = %q.%s\n", binding, name, g.path, shimName)
		fmt.Fprintf(&g.body, "func %s() any { return new(%s) }\n", shimName, g.goType(n))
	}
}
