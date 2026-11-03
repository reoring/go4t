package extern

import (
	"fmt"
	"go/types"
	"sort"
	"strings"
)

// HasErrorClassifier reports whether the package exports a predicate that its
// shim registers for error classification.
func HasErrorClassifier(pkg *Package) bool {
	for _, f := range pkg.Functions {
		if isErrorClassifier(f) {
			return true
		}
	}
	return false
}

func isErrorClassifier(f Function) bool {
	sig := f.Signature
	return strings.HasPrefix(f.Name, "Is") && len(f.Name) > 2 &&
		sig.TypeParams().Len() == 0 && !sig.Variadic() &&
		sig.Params().Len() == 1 && sig.Results().Len() == 1 &&
		errorType(sig.Params().At(0).Type()) && types.Identical(sig.Results().At(0).Type(), types.Typ[types.Bool])
}

// errorClassifier is the sole name-based semantic inference in bindings. It
// calls the original predicates on the original error, not their erased wrappers
// (which mirror error arguments as Option[String] and lose the error's type).
func (g *shimGenerator) errorClassifier(pkg *Package) {
	var names []string
	for _, f := range pkg.Functions {
		if isErrorClassifier(f) {
			names = append(names, f.Name)
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Slice(names, func(i, j int) bool {
		_, a := errorClassifierKind(names[i])
		_, b := errorClassifierKind(names[j])
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	qualifier := g.qualifier(types.NewPackage(pkg.Path, pkg.Name))
	if qualifier != "" {
		qualifier += "."
	}
	g.body.WriteString("func init() { rt.RegisterErrorClassifier(func(err error) (rt.Kind,string,bool) {\n")
	for _, name := range names {
		kind, _ := errorClassifierKind(name)
		message := "err.Error()"
		if kind == "FailureKind" {
			message = fmt.Sprintf("%q + err.Error()", strings.TrimPrefix(name, "Is")+": ")
		}
		fmt.Fprintf(&g.body, "if %s%s(err) { return rt.%s,%s,true }\n", qualifier, name, kind, message)
	}
	g.body.WriteString("return rt.FailureKind,\"\",false\n}) }\n")
}

func errorClassifierKind(name string) (kind string, priority int) {
	switch name {
	case "IsNotFound":
		return "NotFound", 0
	case "IsAlreadyExists":
		return "AlreadyExists", 1
	case "IsForbidden":
		return "PermissionDenied", 2
	case "IsUnauthorized":
		return "PermissionDenied", 3
	case "IsTimeout":
		return "TimeoutKind", 4
	case "IsServerTimeout":
		return "TimeoutKind", 5
	default:
		return "FailureKind", 6
	}
}
