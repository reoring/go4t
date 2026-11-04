package extern

import (
	"fmt"
	"go/types"
	"strings"
)

// JSON is an explicit view of the ordinary mirror, never its representation.
// Keep views behind existing binding candidates so user declarations win.
func (g *shimGenerator) jsonViews(decl *strings.Builder, set *mirrorSet, pkg *Package, options BindOptions, candidates *bindingCandidates) {
	if pkg.jsonMarshaler == nil || pkg.jsonUnmarshaler == nil {
		return
	}
	for _, n := range exportedNamedTypes(pkg.goPackage) {
		if !types.Implements(n, pkg.jsonMarshaler) || !types.Implements(types.NewPointer(n), pkg.jsonUnmarshaler) {
			continue
		}
		m := set.resolve(n, false)
		for _, method := range []string{"ToJson", "FromJson"} {
			display := n.Obj().Name() + "." + method
			name, reason := bindingName(methodBindingName(n.Obj().Name(), method, options.Prefix), options.Prefix)
			if reason == "" {
				reason = set.copyLockReason(n)
			}
			if reason == "" && m == nil {
				reason = "unsupported JSON view type " + types.TypeString(n, nil)
			}
			if reason != "" {
				fmt.Fprintf(decl, "// skipped: %s (%s)\n", display, reason)
				continue
			}
			candidates.add(name, display, bindingJSONView, func() {
				set.dyn = true
				shimName := n.Obj().Name() + "_" + method
				jsonAlias := g.qualifier(types.NewPackage("encoding/json", "json"))
				if method == "ToJson" {
					fmt.Fprintf(decl, "extern func %s(recv %s) dyn.Value = %q.%s\n", name, m.gt, g.path, shimName)
					prefix := "go4t json view: " + types.TypeString(n, nil) + ": "
					converter := g.conversion(addressableMirror(m))
					fmt.Fprintf(&g.body, "func %s(recv any) any { data, err := %s.Marshal(%sTo(recv,context.Background())); if err != nil { panic(%q + err.Error()) }; var value any; if err := %s.Unmarshal(data,&value); err != nil { panic(%q + err.Error()) }; return rt.DynFromGo(value) }\n", shimName, jsonAlias, converter, prefix, jsonAlias, prefix)
				} else {
					fmt.Fprintf(decl, "extern func %s(v dyn.Value) IO[%s] = %q.%s\n", name, m.gt, g.path, shimName)
					converter := g.conversion(m)
					fmt.Fprintf(&g.body, "func %s(v any) any { ", shimName)
					beginShimIO(&g.body, false)
					fmt.Fprintf(&g.body, "data, err := %s.Marshal(rt.DynToGo(v)); if err != nil { return nil,rt.ClassifyError(err) }; var value %s; if err := %s.Unmarshal(data,&value); err != nil { return nil,rt.ClassifyError(err) }; return %sFrom(value),nil }) }\n", jsonAlias, g.goType(n), jsonAlias, converter)
				}
			})
		}
	}
}
