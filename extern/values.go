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

func (g *shimGenerator) values(decl *strings.Builder, set *mirrorSet, pkg *Package, options BindOptions, candidates *bindingCandidates) {
	qualifier := g.qualifier(types.NewPackage(pkg.Path, pkg.Name))
	for _, obj := range exportedValues(pkg.goPackage) {
		name, reason := bindingName(obj.Name(), options.Prefix)
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
			addressed := addressableMirror(m)
			if addressed == m && reason == "" {
				reason = set.copyLockReason(obj.Type())
			}
			m = addressed
		}
		if reason == "" && m == nil {
			reason = "unsupported value type " + types.TypeString(obj.Type(), nil)
		}
		if reason != "" {
			fmt.Fprintf(decl, "// skipped: %s (%s)\n", obj.Name(), reason)
			continue
		}
		candidates.add(name, obj.Name(), bindingValue, func() {
			if options.Prefix == "" && options.Prelude[name] {
				fmt.Fprintf(decl, "// collides with prelude: %s\n", name)
			}
			shimName, value := obj.Name(), obj.Name()
			if qualifier == "" {
				shimName = "Go4t_" + shimName
			} else {
				value = qualifier + "." + value
			}
			if !types.Identical(m.typ, obj.Type()) {
				if _, variable := obj.(*types.Var); variable {
					value = "&" + value
				}
			}
			fmt.Fprintf(decl, "extern func %s() %s = %q.%s\n", name, m.gt, g.path, shimName)
			converter := g.conversion(m)
			fmt.Fprintf(&g.body, "func %s() any { return %sFrom(%s) }\n", shimName, converter, value)
		})
	}
}

// constructors emits new<Type>() for every struct the owning package declares:
// a fresh pointer box for handles, the Go zero value converted to a record
// otherwise. Records are built field-complete in go4t, so the zero value is the
// base for `newListOptions(){fieldSelector: …}` updates.
func (g *shimGenerator) constructors(decl *strings.Builder, set *mirrorSet, options BindOptions, candidates *bindingCandidates) {
	type target struct {
		named  *types.Named
		mirror *foreignMirror
	}
	targets := map[string]target{}
	for name, n := range set.handles {
		if _, ok := n.Underlying().(*types.Struct); ok {
			targets[name] = target{named: n}
		}
	}
	for name, m := range set.records {
		if n, ok := m.typ.(*types.Named); ok {
			targets[name] = target{named: n, mirror: m}
		}
	}
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := targets[name]
		goName := "New" + t.named.Obj().Name()
		binding, reason := bindingName(goName, options.Prefix)
		if reason != "" {
			fmt.Fprintf(decl, "// skipped: %s (%s)\n", goName, reason)
			continue
		}
		candidates.add(binding, goName, bindingValue, func() {
			shimName := "New_" + t.named.Obj().Name()
			fmt.Fprintf(decl, "extern func %s() %s = %q.%s\n", binding, name, g.path, shimName)
			if t.mirror == nil {
				fmt.Fprintf(&g.body, "func %s() any { return new(%s) }\n", shimName, g.goType(t.named))
				return
			}
			fmt.Fprintf(&g.body, "func %s() any { var zero %s; return %sFrom(zero) }\n", shimName, g.goType(t.named), g.conversion(t.mirror))
		})
	}
}
