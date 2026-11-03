package extern

import (
	"fmt"
	"go/types"
	"path"
	"sort"
	"strings"
	"unicode"
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

func (g *shimGenerator) upcasts(decl *strings.Builder, set *mirrorSet, pkg *Package, options BindOptions, candidates *bindingCandidates) {
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
		if m == nil || (m.kind != "record" && m.kind != "handle" && m.kind != "jsonscalar") {
			continue
		}
		type upcast struct {
			target *types.Named
			recv   foreignMirror
			result *foreignMirror
			method string
		}
		var casts []upcast
		counts := map[string]int{}
		for _, target := range interfaces {
			if types.Identical(source, target) {
				continue
			}
			iface := target.Underlying().(*types.Interface)
			recv := *m
			pointer := types.NewPointer(source)
			if types.Implements(pointer, iface) {
				recv.typ = pointer
				if m.kind != "handle" {
					recv.kind, recv.elem = "receiver", m
				}
			} else if !types.Implements(source, iface) {
				continue
			}
			result := set.resolve(target, false)
			if result == nil {
				continue
			}
			counts[target.Obj().Name()]++
			casts = append(casts, upcast{target: target, recv: recv, result: result})
		}
		// Qualify both sides of an ambiguous interface name before ranking
		// bindings, so package scan order never drops an otherwise valid cast.
		methods := map[string]int{}
		for i := range casts {
			cast := &casts[i]
			target := cast.target.Obj()
			suffix := ""
			if counts[target.Name()] > 1 {
				suffix = upcastOwnerSuffix(target.Pkg().Path(), false)
			}
			cast.method = "As" + suffix + target.Name()
			methods[cast.method]++
		}
		for _, cast := range casts {
			method := cast.method
			if methods[method] > 1 {
				alias := set.imports[cast.target.Obj().Pkg().Path()]
				if alias == "" {
					alias = cast.target.Obj().Pkg().Path()
				}
				method = "As" + upcastOwnerSuffix(strings.TrimPrefix(alias, "go_"), true) + cast.target.Obj().Name()
			}
			name, reason := bindingName(methodBindingName(source.Obj().Name(), method, options.Prefix), options.Prefix)
			display := source.Obj().Name() + "." + method
			if reason != "" {
				fmt.Fprintf(decl, "// skipped: %s (%s)\n", display, reason)
				continue
			}
			candidates.add(name, display, bindingUpcast, func() {
				shimName := source.Obj().Name() + "_" + method
				fmt.Fprintf(decl, "extern func %s(recv %s) %s = %q.%s\n", name, m.gt, cast.result.gt, g.path, shimName)
				converter := g.conversion(&cast.recv)
				fmt.Fprintf(&g.body, "func %s(recv any) any { return %s(%sTo(recv,context.Background())) }\n", shimName, g.goType(cast.target), converter)
			})
		}
	}
}

func upcastOwnerSuffix(owner string, full bool) string {
	if !full {
		base := path.Base(owner)
		if len(base) > 1 && base[0] == 'v' && base[1] >= '0' && base[1] <= '9' {
			base = path.Base(path.Dir(owner)) + "_" + base
		}
		owner = base
	}
	parts := strings.FieldsFunc(owner, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for i, part := range parts {
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, "")
}
