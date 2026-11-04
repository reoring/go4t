package extern

import (
	"go/types"
	"sort"
)

// Method describes a callable exported method. Receiver is the exported type
// (or its pointer), including when the method is promoted from an embedded type.
// The signature excludes the receiver from its ordinary parameters.
type Method struct {
	TypeName  string
	Name      string
	Receiver  types.Type
	Signature *types.Signature
}

func exportedMethods(pkg *types.Package) []Method {
	var methods []Method
	for _, name := range pkg.Scope().Names() {
		obj, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || !obj.Exported() || obj.IsAlias() {
			continue
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}
		seen := make(map[string]bool)
		// Retain value receivers for records. Binding collection
		// switches struct handles to their pointer method set after resolving.
		for _, receiver := range []types.Type{named, types.NewPointer(named)} {
			set := types.NewMethodSet(receiver)
			for i := range set.Len() {
				method := set.At(i).Obj().(*types.Func)
				if !method.Exported() || seen[method.Name()] {
					continue
				}
				seen[method.Name()] = true
				methods = append(methods, Method{
					TypeName: name, Name: method.Name(), Receiver: receiver,
					Signature: method.Type().(*types.Signature),
				})
			}
		}
	}
	sort.Slice(methods, func(i, j int) bool {
		if methods[i].TypeName != methods[j].TypeName {
			return methods[i].TypeName < methods[j].TypeName
		}
		return methods[i].Name < methods[j].Name
	})
	return methods
}

func handleMethodReceiver(m *foreignMirror, method string) *foreignMirror {
	addressed := addressableMirror(m)
	if addressed != m && types.NewMethodSet(addressed.typ).Lookup(nil, method) != nil {
		return addressed
	}
	return m
}
