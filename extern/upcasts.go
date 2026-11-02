package extern

import (
	"fmt"
	"go/types"
	"sort"
	"strings"
)

func exportedNamedTypes(pkg *types.Package) []*types.Named {
	if pkg == nil {
		return nil
	}
	var result []*types.Named
	for _, name := range pkg.Scope().Names() {
		obj, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || !obj.Exported() || obj.IsAlias() {
			continue
		}
		n, ok := obj.Type().(*types.Named)
		if ok && n.TypeParams().Len() == 0 {
			result = append(result, n)
		}
	}
	return result
}

func (g *shimGenerator) upcasts(decl *strings.Builder, set *mirrorSet, pkg *Package, options BindOptions, used map[string]bool) {
	if pkg.goPackage == nil {
		return
	}
	packages := append([]*types.Package(nil), pkg.imports...)
	sort.Slice(packages, func(i, j int) bool { return packages[i].Path() < packages[j].Path() })
	packages = append([]*types.Package{pkg.goPackage}, packages...)
	var interfaces []*types.Named
	for _, p := range packages {
		for _, n := range exportedNamedTypes(p) {
			if iface, ok := n.Underlying().(*types.Interface); ok && iface.IsMethodSet() {
				interfaces = append(interfaces, n)
			}
		}
	}
	for _, source := range exportedNamedTypes(pkg.goPackage) {
		m := set.resolve(source, false)
		if m == nil || (m.kind != "record" && m.kind != "handle") {
			continue
		}
		for _, target := range interfaces {
			if types.Identical(source, target) {
				continue
			}
			iface := target.Underlying().(*types.Interface)
			recv := *m
			pointer := types.NewPointer(source)
			if types.Implements(pointer, iface) {
				recv.typ = pointer
				if m.kind == "record" {
					recv.kind, recv.elem = "receiver", m
				}
			} else if !types.Implements(source, iface) {
				continue
			}
			method := "As" + target.Obj().Name()
			name, reason := bindingName(methodBindingName(source.Obj().Name(), method, options.Prefix), options.Prefix)
			if used[name] {
				reason = "upcast name collides with existing binding"
			}
			if reason != "" {
				fmt.Fprintf(decl, "// skipped: %s.%s (%s)\n", source.Obj().Name(), method, reason)
				continue
			}
			result := set.resolve(target, false)
			if result == nil {
				continue
			}
			used[name] = true
			shimName := source.Obj().Name() + "_" + method
			fmt.Fprintf(decl, "extern func %s(recv %s) %s = %q.%s\n", name, m.gt, result.gt, g.path, shimName)
			converter := g.conversion(&recv)
			fmt.Fprintf(&g.body, "func %s(recv any) any { return %s(%sTo(recv,context.Background())) }\n", shimName, g.goType(target), converter)
		}
	}
}
