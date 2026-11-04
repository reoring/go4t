package extern

import (
	"crypto/sha256"
	"fmt"
	"go/format"
	"go/types"
	"sort"
	"strings"

	"go4t/ast"
)

// GenericPrefix marks templates, never executable Go import paths.
const GenericPrefix = "go4t:generic/"

// genericTemplate exposes the mirror's shape before Go type arguments exist.
// Unsupported shapes remain templates: their use, not their import, diagnoses
// the unsupported boundary. Structural parameters (S ~[]E) are not independent
// go4t parameters; their concrete Go arguments follow from E.
func genericTemplate(set *mirrorSet, f Function, name string) string {
	channels := set.channels
	set.channels = make(map[string]*foreignMirror)
	defer func() { set.channels = channels }()
	set.generic = true
	defer func() { set.generic = false; set.parameters = nil; set.unknown = nil }()
	set.parameters = make(map[*types.TypeParam]string)
	set.unknown = make(map[types.Type]string)
	var params []string
	for i := range f.Signature.TypeParams().Len() {
		p := f.Signature.TypeParams().At(i)
		if structuralConstraint(p) == nil {
			n := fmt.Sprintf("G%d", len(params))
			set.parameters[p] = n
			params = append(params, n)
		}
	}
	c, reason := set.call(f.Signature, false)
	if reason != "" {
		// Result arity is a call-site error too; keep a typeable template.
		c = &foreignCall{mutable: true, result: set.genericUnknown(f.Signature.Results())}
		for i := range f.Signature.Params().Len() {
			c.params = append(c.params, set.genericUnknown(f.Signature.Params().At(i).Type()))
		}
	}
	unknown := make([]string, 0, len(set.unknown))
	for _, n := range set.unknown { unknown = append(unknown, n) }
	sort.Strings(unknown)
	params = append(params, unknown...)
	args := make([]string, len(c.params))
	for i, p := range c.params { args[i] = fmt.Sprintf("p%d %s", i, p.gt) }
	ret := c.result.gt
	if c.optional { ret = "Option[" + ret + "]" }
	tp := ""
	if len(params) > 0 { tp = "[" + strings.Join(params, ", ") + "]" }
	return fmt.Sprintf("extern func %s%s(%s) IO[%s] = %q.%s\n", name, tp, strings.Join(args, ", "), ret, GenericPrefix+set.owner, f.Name)
}

func (s *mirrorSet) genericUnknown(t types.Type) *foreignMirror {
	n := s.unknown[t]
	if n == "" { n = fmt.Sprintf("U%d", len(s.unknown)); s.unknown[t] = n }
	return &foreignMirror{typ: t, kind: "parameter", gt: n}
}

// A single ~[]E or ~map[K]V term supplies the structural type parameter.
func structuralConstraint(p *types.TypeParam) types.Type {
	var find func(types.Type) types.Type
	find = func(t types.Type) types.Type {
		switch t := t.Underlying().(type) {
		case *types.Interface:
			for i := range t.NumEmbeddeds() { if r := find(t.EmbeddedType(i)); r != nil { return r } }
		case *types.Union:
			if t.Len() == 1 {
				switch t.Term(0).Type().Underlying().(type) {
				case *types.Slice, *types.Map, *types.Chan: return t.Term(0).Type()
				}
			}
		}
		return nil
	}
	return find(p.Constraint())
}

// GenericInstance is a validated concrete erased wrapper, independent of other
// calls in the program. Key includes the exact Go tuple and the effect policy.
type GenericInstance struct {
	Key string
	Signature *types.Signature
	Arguments []types.Type
	function Function
	pkg *Package
	call *foreignCall
}

// InstantiateGeneric infers Go arguments from the checked boundary signature,
// validates Go constraints with go/types, then applies ordinary concrete mirrors.
func InstantiateGeneric(loader *Loader, pkg *Package, symbol string, signature *ast.FuncType) (*GenericInstance, error) {
	f, ok := pkg.Lookup(symbol)
	if !ok || f.Signature.TypeParams().Len() == 0 { return nil, fmt.Errorf("not a generic Go function: %s.%s", pkg.Path, symbol) }
	args := make(map[*types.TypeParam]types.Type)
	infer := genericInference{loader: loader, args: args}
	if err := infer.call(f.Signature, signature); err != nil { return nil, err }
	tuple := make([]types.Type, f.Signature.TypeParams().Len())
	for i := range tuple {
		p := f.Signature.TypeParams().At(i)
		tuple[i] = args[p]
		if tuple[i] == nil { return nil, fmt.Errorf("cannot determine Go type parameter %s", p.Obj().Name()) }
	}
	concrete, err := types.Instantiate(nil, f.Signature, tuple, true)
	if err != nil { return nil, fmt.Errorf("Go constraint not satisfied: %w", err) }
	sig := concrete.(*types.Signature)
	set := newMirrorSet(true, pkg, map[string]bool{})
	set.forceIO = true
	set.origins = make(map[*types.Signature]*types.Signature)
	genericOrigins(f.Signature, sig, set.origins)
	call, reason := set.call(sig, false)
	if reason != "" { return nil, fmt.Errorf("%s", reason) }
	parts := []string{"generic-io-v1", pkg.Path, symbol}
	for _, t := range tuple { parts = append(parts, types.TypeString(t, func(p *types.Package) string { return p.Path() })) }
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
	return &GenericInstance{Key: key, Signature: sig, Arguments: tuple, function: f, pkg: pkg, call: call}, nil
}

// Shim emits a standalone companion so base package cache entries never depend
// on callers and no converter names collide with the package's ordinary shim.
func (i *GenericInstance) Shim(shimPath string) ([]byte, error) {
	g := &shimGenerator{path: shimPath, imports: map[string]string{"context": "context", "go4t/rt": "rt"}, converters: map[string]string{}}
	args := make([]string, len(i.Arguments))
	for n, t := range i.Arguments { args[n] = g.goType(t) }
	callee := g.qualifier(i.pkg.goPackage) + "." + i.function.Name + "[" + strings.Join(args, ",") + "]"
	g.emit(i.function.Name, callee, i.Signature, i.call, nil)
	var out strings.Builder
	out.WriteString("// Code generated by go4t generic instantiation. DO NOT EDIT.\npackage instance\nimport (\n")
	paths := make([]string, 0, len(g.imports))
	for p, a := range g.imports { if strings.Contains(g.body.String(), a+".") { paths = append(paths, p) } }
	sort.Strings(paths)
	for _, p := range paths { fmt.Fprintf(&out, "%s %q\n", g.imports[p], p) }
	out.WriteString(")\n")
	out.WriteString(g.body.String())
	return format.Source([]byte(out.String()))
}

type genericInference struct {
	loader *Loader
	args map[*types.TypeParam]types.Type
}

func (i *genericInference) call(sig *types.Signature, actual *ast.FuncType) error {
	start := 0
	if sig.Params().Len() > 0 && contextType(sig.Params().At(0).Type()) { start = 1 }
	if len(actual.Params) != sig.Params().Len()-start { return fmt.Errorf("generic function parameter arity mismatch") }
	for n, a := range actual.Params {
		if err := i.match(sig.Params().At(n+start).Type(), a, false); err != nil { return fmt.Errorf("parameter %d: %w", n+1, err) }
	}
	ret, ok := actual.Ret.(*ast.TypeName)
	if !ok || ret.Name != "IO" || len(ret.Args) != 1 { return fmt.Errorf("generic Go calls and callbacks require IO") }
	result := ret.Args[0]
	n := sig.Results().Len()
	failure := n > 0 && errorType(sig.Results().At(n-1).Type())
	if failure { n-- }
	if n == 2 && !failure && types.Identical(sig.Results().At(1).Type(), types.Typ[types.Bool]) {
		opt, ok := result.(*ast.TypeName)
		if !ok || opt.Name != "Option" || len(opt.Args) != 1 { return fmt.Errorf("expected Option result") }
		result = opt.Args[0]; n--
	}
	if n > 3 { return fmt.Errorf("unsupported multiple results: arity %d", n) }
	if n == 0 { return nil }
	if n == 1 { return i.match(sig.Results().At(0).Type(), result, true) }
	tuple, ok := result.(*ast.TupleType)
	if !ok || len(tuple.Elems) != n { return fmt.Errorf("expected tuple result") }
	for j := range n { if err := i.match(sig.Results().At(j).Type(), tuple.Elems[j], true); err != nil { return err } }
	return nil
}

func (i *genericInference) match(pattern types.Type, actual ast.Type, result bool) error {
	pattern = types.Unalias(pattern)
	if !hasGoTypeParameter(pattern) {
		return nil
	}
	if p, ok := pattern.(*types.TypeParam); ok {
		if shape := structuralConstraint(p); shape != nil {
			if err := i.match(shape, actual, result); err != nil { return err }
		}
		candidate, err := i.native(actual)
		if err != nil { return err }
		if old := i.args[p]; old != nil && !types.Identical(old, candidate) { return fmt.Errorf("conflicting Go types for %s: %s and %s", p.Obj().Name(), old, candidate) }
		i.args[p] = candidate
		return nil
	}
	if named(pattern, "iter", "Seq") || named(pattern, "iter", "Seq2") {
		if !result { return fmt.Errorf("iterator parameters are unsupported") }
		a, ok := actual.(*ast.TypeName)
		if !ok || a.Name != "List" || len(a.Args) != 1 { return fmt.Errorf("expected List for iterator result") }
		params := pattern.(*types.Named).TypeArgs()
		if params.Len() == 1 { return i.match(params.At(0), a.Args[0], false) }
		pair, ok := a.Args[0].(*ast.TupleType)
		if !ok || len(pair.Elems) != 2 { return fmt.Errorf("expected pair iterator result") }
		for n := range 2 { if err := i.match(params.At(n), pair.Elems[n], false); err != nil { return err } }
		return nil
	}
	if n, ok := pattern.(*types.Named); ok {
		if n.TypeParams().Len() != 0 {
			switch n.Underlying().(type) {
			case *types.Slice, *types.Map: return i.match(n.Underlying(), actual, result)
			default: return fmt.Errorf("unsupported instantiated named type %s", n)
			}
		}
		return nil // exact nongeneric shapes were already checked against the template
	}
	a, _ := actual.(*ast.TypeName)
	switch p := pattern.(type) {
	case *types.Slice:
		if a == nil || a.Name != "List" || len(a.Args) != 1 { return fmt.Errorf("expected List for %s", pattern) }
		return i.match(p.Elem(), a.Args[0], false)
	case *types.Map:
		if a == nil || a.Name != "Map" || len(a.Args) != 2 { return fmt.Errorf("expected Map for %s", pattern) }
		if err := i.match(p.Key(), a.Args[0], false); err != nil { return err }
		return i.match(p.Elem(), a.Args[1], false)
	case *types.Pointer:
		if a != nil && a.Name == "Option" && len(a.Args) == 1 { return i.match(p.Elem(), a.Args[0], false) }
		return nil
	case *types.Signature:
		f, ok := actual.(*ast.FuncType)
		if !ok { return fmt.Errorf("expected callback") }
		return i.call(p, f)
	case *types.Chan:
		if a == nil || len(a.Args) != 1 { return fmt.Errorf("expected channel") }
		return i.match(p.Elem(), a.Args[0], false)
	}
	return nil
}

func (i *genericInference) native(t ast.Type) (types.Type, error) {
	if f, ok := t.(*ast.FuncType); ok {
		ret, ok := f.Ret.(*ast.TypeName)
		if !ok || ret.Name != "IO" || len(ret.Args) != 1 {
			return nil, fmt.Errorf("generic callbacks require IO results")
		}
		params := make([]*types.Var, len(f.Params))
		for n, p := range f.Params {
			native, err := i.native(p)
			if err != nil { return nil, err }
			params[n] = types.NewVar(0, nil, "", native)
		}
		native, err := i.native(ret.Args[0])
		if err != nil { return nil, err }
		return types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), types.NewTuple(types.NewVar(0, nil, "", native)), false), nil
	}
	n, ok := t.(*ast.TypeName)
	if !ok { return nil, fmt.Errorf("cannot mirror Go type argument from function or tuple") }
	switch n.Name {
	case "Int": return types.Typ[types.Int64], nil
	case "Float": return types.Typ[types.Float64], nil
	case "Bool": return types.Typ[types.Bool], nil
	case "String": return types.Typ[types.String], nil
	case "Unit":
		pkg, err := i.loader.Load("go4t/rt")
		if err != nil { return nil, err }
		return pkg.goPackage.Scope().Lookup("Unit").Type(), nil
	case "List", "Map", "Option":
		args := make([]types.Type, len(n.Args))
		for j, a := range n.Args { var err error; args[j], err = i.native(a); if err != nil { return nil, err } }
		switch n.Name {
		case "List": return types.NewSlice(args[0]), nil
		case "Map": return types.NewMap(args[0], args[1]), nil
		case "Option": return types.NewPointer(args[0]), nil
		}
	}
	if strings.HasPrefix(n.Name, "go:") {
		owner := strings.TrimPrefix(n.Name, "go:")
		at := strings.LastIndexByte(owner, '.')
		if at > 0 {
			pkg, err := i.loader.Load(owner[:at]); if err != nil { return nil, err }
			obj := pkg.goPackage.Scope().Lookup(owner[at+1:])
			if obj == nil {
				for _, n := range exportedNamedTypes(pkg.goPackage) {
					if foreignTypeName(n.Obj().Name()) == owner[at+1:] { obj = n.Obj(); break }
				}
			}
			if obj != nil {
				set := newMirrorSet(true, pkg, map[string]bool{})
				typ := obj.Type()
				if m := set.resolve(typ, false); m != nil && (m.kind == "handle" || m.kind == "record") {
					// Opaque struct bindings represent pointer identity at the boundary.
					if m.kind == "handle" { if _, ok := typ.Underlying().(*types.Struct); ok { typ = types.NewPointer(typ) } }
					return typ, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("cannot mirror Go type argument from %s", n.Name)
}

func hasGoTypeParameter(t types.Type) bool {
	switch t := types.Unalias(t).(type) {
	case *types.TypeParam:
		return true
	case *types.Named:
		for n := range t.TypeArgs().Len() { if hasGoTypeParameter(t.TypeArgs().At(n)) { return true } }
	case *types.Pointer:
		return hasGoTypeParameter(t.Elem())
	case *types.Slice:
		return hasGoTypeParameter(t.Elem())
	case *types.Map:
		return hasGoTypeParameter(t.Key()) || hasGoTypeParameter(t.Elem())
	case *types.Chan:
		return hasGoTypeParameter(t.Elem())
	case *types.Signature:
		for n := range t.Params().Len() { if hasGoTypeParameter(t.Params().At(n).Type()) { return true } }
		for n := range t.Results().Len() { if hasGoTypeParameter(t.Results().At(n).Type()) { return true } }
	}
	return false
}

// Context/error/optional conventions belong to the original signature. A
// substituted T=Bool must not turn a tuple result into Option after inference.
func genericOrigins(original, concrete types.Type, out map[*types.Signature]*types.Signature) {
	switch original := types.Unalias(original).(type) {
	case *types.Signature:
		sig := concrete.Underlying().(*types.Signature)
		out[sig] = original
		for n := range original.Params().Len() { genericOrigins(original.Params().At(n).Type(), sig.Params().At(n).Type(), out) }
		for n := range original.Results().Len() { genericOrigins(original.Results().At(n).Type(), sig.Results().At(n).Type(), out) }
	case *types.Named:
		if _, ok := original.Underlying().(*types.Signature); ok { genericOrigins(original.Underlying(), concrete.Underlying(), out) }
	case *types.Pointer:
		genericOrigins(original.Elem(), concrete.Underlying().(*types.Pointer).Elem(), out)
	case *types.Slice:
		genericOrigins(original.Elem(), concrete.Underlying().(*types.Slice).Elem(), out)
	case *types.Map:
		m := concrete.Underlying().(*types.Map)
		genericOrigins(original.Key(), m.Key(), out)
		genericOrigins(original.Elem(), m.Elem(), out)
	case *types.Chan:
		genericOrigins(original.Elem(), concrete.Underlying().(*types.Chan).Elem(), out)
	}
}
