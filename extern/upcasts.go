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
	type upcast struct {
		target *types.Named
		recv   foreignMirror
		result *foreignMirror
		method string
	}
	type sourceCasts struct {
		source *types.Named
		mirror *foreignMirror
		casts  []upcast
		name   string
	}
	var interfaces, sources []*types.Named
	for _, p := range packages {
		for _, n := range exportedNamedTypes(p) {
			if !set.importable(n) {
				continue
			}
			if v, ok := n.Underlying().(*types.Interface); ok {
				if !v.IsMethodSet() {
					continue
				}
				interfaces = append(interfaces, n)
			}
			sources = append(sources, n)
		}
	}
	var groups []sourceCasts
	sourceCounts := map[string]int{}
	for _, source := range sources {
		var m *foreignMirror
		pointer := types.NewPointer(source)
		var casts []upcast
		counts := map[string]int{}
		for _, target := range interfaces {
			if types.Identical(source, target) {
				continue
			}
			if source.Obj().Pkg() != pkg.goPackage && target.Obj().Pkg() != pkg.goPackage {
				continue // At least one side must be declared in the bound package.
			}
			iface := target.Underlying().(*types.Interface)
			valueImplements := types.Implements(source, iface)
			pointerImplements := types.Implements(pointer, iface)
			if !valueImplements && !pointerImplements {
				continue
			}
			// Resolving a foreign source registers its module import. Do it only
			// after Implements succeeds, never for every direct-import type.
			if m == nil {
				m = set.resolve(source, false)
			}
			if m == nil {
				continue
			}
			valueSource := scalarMirror(m) || m.kind == "bytes" || m.kind == "slice" || m.kind == "map"
			if !valueSource && m.kind != "record" && m.kind != "handle" {
				continue
			}
			recv := *m
			if pointerImplements && (!valueSource || !valueImplements) {
				recv.typ = pointer
				if m.kind != "handle" {
					recv.kind, recv.elem = "receiver", m
				}
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
		for i := range casts {
			cast := &casts[i]
			target := cast.target.Obj()
			suffix := ""
			if counts[target.Name()] > 1 {
				suffix = upcastOwnerSuffix(target.Pkg().Path(), false)
			}
			cast.method = "As" + suffix + target.Name()
		}
		if len(casts) > 0 {
			sourceCounts[source.Obj().Name()]++
			groups = append(groups, sourceCasts{source: source, mirror: m, casts: casts})
		}
	}
	// P's sources retain their names and precede foreign sources in ties.
	// Qualify foreign homonyms, falling back to the full owner for equal bases.
	names := map[string]int{}
	for i := range groups {
		group := &groups[i]
		obj := group.source.Obj()
		group.name = obj.Name()
		if obj.Pkg() != pkg.goPackage && sourceCounts[obj.Name()] > 1 {
			group.name = upcastOwnerSuffix(obj.Pkg().Path(), false) + obj.Name()
		}
		names[group.name]++
	}
	for _, group := range groups {
		source, m, casts := group.source, group.mirror, group.casts
		valueSource := scalarMirror(m) || m.kind == "bytes" || m.kind == "slice" || m.kind == "map"
		sourceName := group.name
		if source.Obj().Pkg() != pkg.goPackage && names[sourceName] > 1 {
			alias := set.imports[source.Obj().Pkg().Path()]
			sourceName = upcastOwnerSuffix(strings.TrimPrefix(alias, "go_"), true) + source.Obj().Name()
		}
		methods := map[string]int{}
		for _, cast := range casts {
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
			name, reason := bindingName(methodBindingName(sourceName, method, options.Prefix), options.Prefix)
			display := sourceName + "." + method
			if reason != "" {
				fmt.Fprintf(decl, "// skipped: %s (%s)\n", display, reason)
				continue
			}
			candidates.add(name, display, bindingUpcast, func() {
				shimName := sourceName + "_" + method
				fmt.Fprintf(decl, "extern func %s(recv %s) %s = %q.%s\n", name, m.gt, cast.result.gt, g.path, shimName)
				if valueSource {
					underlying := *m
					underlying.typ = source.Underlying()
					converter := g.conversion(&underlying)
					value := "value"
					if cast.recv.kind == "receiver" {
						value = "&value"
					}
					fmt.Fprintf(&g.body, "func %s(recv any) any { value := %s(%sTo(recv,context.Background())); return %s(%s) }\n", shimName, g.goType(source), converter, g.goType(cast.target), value)
					return
				}
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
