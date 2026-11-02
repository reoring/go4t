package extern

import (
	"fmt"
	"go/types"
	gfxtypes "go4t/types"
	"sort"
	"strings"
	"unicode"
)

// A foreignMirror is a copy-boundary plan. Handles deliberately do not occur
// inside records: one opaque field makes the enclosing named struct opaque.
type foreignMirror struct {
	typ       types.Type
	kind, gt  string
	elem, key *foreignMirror
	fields    []mirrorField
	elems     []*foreignMirror
	callback  *foreignCall
}
type mirrorField struct {
	name   string
	path   []int
	mirror *foreignMirror
}
type foreignCall struct {
	params                                     []*foreignMirror
	result                                     *foreignMirror
	context, failure, optional, variadic, scan bool
}
type mirrorSet struct {
	floats    bool
	owner     string
	imports   map[string]string
	used      map[string]bool
	records   map[string]*foreignMirror
	handles   map[string]*types.Named
	resolving map[types.Type]bool
	dyn       bool
}

func newMirrorSet(floats bool, owner string, used map[string]bool) *mirrorSet {
	return &mirrorSet{
		floats: floats, owner: owner, used: used, imports: map[string]string{},
		records: map[string]*foreignMirror{}, handles: map[string]*types.Named{},
		resolving: map[types.Type]bool{},
	}
}

func (s *mirrorSet) typeName(n *types.Named) string {
	name := foreignTypeName(n.Obj().Name())
	pkg := n.Obj().Pkg()
	if pkg == nil || pkg.Path() == s.owner {
		return name
	}
	alias := s.imports[pkg.Path()]
	if alias == "" {
		base := "go_" + strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return '_'
		}, pkg.Path())
		alias = base
		for i := 2; s.used[alias]; i++ {
			alias = fmt.Sprintf("%s_%d", base, i)
		}
		s.imports[pkg.Path()] = alias
		s.used[alias] = true
	}
	return alias + "." + name
}

// importable reports whether a foreign type's owning package can be imported by
// the binding module: types from internal packages are referenced by exported
// APIs (protobuf's pragma.DoNotImplement) but cannot be bound from outside.
func (s *mirrorSet) importable(n *types.Named) bool {
	pkg := n.Obj().Pkg()
	if pkg == nil || pkg.Path() == s.owner {
		return true
	}
	for _, part := range strings.Split(pkg.Path(), "/") {
		if part == "internal" || part == "vendor" {
			return false
		}
	}
	return true
}
func lowerName(s string) string     { r := []rune(s); r[0] = unicode.ToLower(r[0]); return string(r) }
func errorType(t types.Type) bool   { return types.Identical(t, types.Universe.Lookup("error").Type()) }
func contextType(t types.Type) bool { return named(t, "context", "Context") }

// valueMirror implements the foreign-mirrorable predicate, without fallback to
// handles. The visiting set rejects cycles even through slices and pointers.
func (s *mirrorSet) valueMirror(t types.Type, visiting map[types.Type]bool) *foreignMirror {
	t = types.Unalias(t)
	if visiting[t] {
		return nil
	}
	visiting[t] = true
	defer delete(visiting, t)
	m := &foreignMirror{typ: t}
	if named(t, "time", "Time") {
		m.kind, m.gt = "time", "Int"
		return m
	}
	if named(t, "go4t/rt", "Unit") {
		m.kind, m.gt = "unit", "Unit"
		return m
	}
	if errorType(t) {
		m.kind, m.gt = "error", "Option[String]"
		return m
	}
	switch v := t.(type) {
	case *types.Basic:
		switch {
		case v.Info()&types.IsInteger != 0:
			m.kind, m.gt = "int", "Int"
		case v.Info()&types.IsFloat != 0 && s.floats:
			m.kind, m.gt = "float", "Float"
		case v.Kind() == types.Bool:
			m.kind, m.gt = "bool", "Bool"
		case v.Kind() == types.String:
			m.kind, m.gt = "string", "String"
		default:
			return nil
		}
	case *types.Named:
		if v.TypeParams().Len() != 0 || !v.Obj().Exported() || !s.importable(v) {
			return nil
		}
		if _, ok := v.Underlying().(*types.Struct); ok {
			fields := promotedFields(v)
			// A private-state object (sql.DB, url.Userinfo, ...) is a handle, not
			// an empty record which would silently discard its state.
			if len(fields) == 0 {
				return nil
			}
			m.kind, m.gt = "record", s.typeName(v)
			names := map[string]bool{}
			for _, f := range fields {
				current := types.Type(v)
				for _, index := range f.path {
					if p, ok := current.(*types.Pointer); ok {
						current = p.Elem()
					}
					field := current.Underlying().(*types.Struct).Field(index)
					if _, pointer := field.Type().(*types.Pointer); pointer && field.Embedded() && !field.Exported() {
						return nil
					}
					current = field.Type()
				}
				fm := s.valueMirror(f.typ, visiting)
				if fm == nil {
					return nil
				}
				name := lowerName(f.name)
				if _, reason := bindingName(f.name, ""); reason != "" || names[name] {
					return nil
				}
				names[name] = true
				m.fields = append(m.fields, mirrorField{name: name, path: f.path, mirror: fm})
			}
		} else {
			inner := s.valueMirror(v.Underlying(), visiting)
			if inner == nil {
				return nil
			}
			*m = *inner
			m.typ = t
		}
	case *types.Pointer:
		m.elem = s.valueMirror(v.Elem(), visiting)
		if m.elem == nil {
			return nil
		}
		m.kind, m.gt = "pointer", "Option["+m.elem.gt+"]"
	case *types.Slice:
		if types.Identical(v.Elem(), types.Typ[types.Byte]) {
			m.kind, m.gt = "bytes", "String"
			break
		}
		m.elem = s.valueMirror(v.Elem(), visiting)
		if m.elem == nil {
			return nil
		}
		m.kind, m.gt = "slice", "List["+m.elem.gt+"]"
	case *types.Map:
		m.key = s.valueMirror(v.Key(), visiting)
		m.elem = s.valueMirror(v.Elem(), visiting)
		if m.key == nil || m.elem == nil || !scalarMirror(m.key) {
			return nil
		}
		m.kind, m.gt = "map", "Map["+m.key.gt+", "+m.elem.gt+"]"
	default:
		return nil
	}
	return m
}
func scalarMirror(m *foreignMirror) bool {
	return m.kind == "int" || m.kind == "float" || m.kind == "string" || m.kind == "bool"
}

type promotedField struct {
	name string
	typ  types.Type
	path []int
}

func promotedFields(t types.Type) []promotedField {
	st := t.Underlying().(*types.Struct)
	var candidates []promotedField
	var walk func(*types.Struct, []int, map[*types.Struct]bool)
	walk = func(cur *types.Struct, path []int, seen map[*types.Struct]bool) {
		if seen[cur] {
			return
		}
		seen[cur] = true
		defer delete(seen, cur)
		for i := range cur.NumFields() {
			f := cur.Field(i)
			p := append(append([]int(nil), path...), i)
			ft := types.Unalias(f.Type())
			if ptr, ok := ft.(*types.Pointer); ok {
				ft = ptr.Elem()
			}
			if f.Embedded() {
				if inner, ok := ft.Underlying().(*types.Struct); ok {
					walk(inner, p, seen)
					continue
				}
			}
			if f.Exported() {
				candidates = append(candidates, promotedField{f.Name(), f.Type(), p})
			}
		}
	}
	walk(st, nil, map[*types.Struct]bool{})
	var result []promotedField
	for _, f := range candidates {
		obj, index, _ := types.LookupFieldOrMethod(t, true, nil, f.name)
		if obj != nil && fmt.Sprint(index) == fmt.Sprint(f.path) {
			result = append(result, f)
		}
	}
	return result
}
func (s *mirrorSet) resolve(t types.Type, callback bool) *foreignMirror {
	t = types.Unalias(t)
	// Iterators are materialised only in result position, never callbacks.
	if named(t, "iter", "Seq") || named(t, "iter", "Seq2") {
		return nil
	}
	if isAny(t) {
		s.dyn = true
		return &foreignMirror{typ: t, kind: "dyn", gt: "dyn.Value"}
	}
	if sig, ok := t.Underlying().(*types.Signature); ok && callback {
		if s.resolving[t] {
			return nil
		}
		s.resolving[t] = true
		defer delete(s.resolving, t)
		c, reason := s.call(sig, false)
		if reason != "" || c.optional {
			return nil
		}
		var p []string
		for _, m := range c.params {
			p = append(p, m.gt)
		}
		r := c.result.gt
		if c.context || c.failure {
			r = "IO[" + r + "]"
		}
		return &foreignMirror{typ: t, kind: "callback", gt: "func(" + strings.Join(p, ", ") + ") " + r, callback: c}
	}
	if m := s.valueMirror(t, map[types.Type]bool{}); m != nil {
		s.register(m)
		return m
	}
	base := t
	if p, ok := base.(*types.Pointer); ok {
		base = types.Unalias(p.Elem())
	}
	if n, ok := base.(*types.Named); ok && n.Obj().Exported() && n.TypeParams().Len() == 0 && s.importable(n) {
		if i, ok := n.Underlying().(*types.Interface); ok && !i.IsMethodSet() {
			return nil
		}
		if n.Obj().Pkg().Path() == s.owner {
			s.handles[foreignTypeName(n.Obj().Name())] = n
		}
		return &foreignMirror{typ: t, kind: "handle", gt: s.typeName(n)}
	}
	// Containers of handles preserve the opaque elements, rather than exposing
	// their fields (for example []prometheus.Collector).
	switch v := t.(type) {
	case *types.Slice:
		if e := s.resolve(v.Elem(), false); e != nil {
			return &foreignMirror{typ: t, kind: "slice", gt: "List[" + e.gt + "]", elem: e}
		}
	case *types.Map:
		k := s.valueMirror(v.Key(), map[types.Type]bool{})
		e := s.resolve(v.Elem(), false)
		if k != nil && scalarMirror(k) && e != nil {
			return &foreignMirror{typ: t, kind: "map", gt: "Map[" + k.gt + ", " + e.gt + "]", key: k, elem: e}
		}
	}
	return nil
}
func tupleMirror(elems []*foreignMirror) *foreignMirror {
	names := make([]string, len(elems))
	for i, elem := range elems {
		names[i] = elem.gt
	}
	return &foreignMirror{kind: "tuple", gt: "(" + strings.Join(names, ", ") + ")", elems: elems}
}

func (s *mirrorSet) resultMirror(t types.Type) *foreignMirror {
	t = types.Unalias(t)
	if named(t, "iter", "Seq") || named(t, "iter", "Seq2") {
		args := t.(*types.Named).TypeArgs()
		elems := make([]*foreignMirror, args.Len())
		for i := range elems {
			elems[i] = s.resolve(args.At(i), true)
			if elems[i] == nil {
				return nil
			}
		}
		elem := elems[0]
		if len(elems) == 2 {
			elem = tupleMirror(elems)
		}
		return &foreignMirror{typ: t, kind: "seq", gt: "List[" + elem.gt + "]", elem: elem, elems: elems}
	}
	return s.resolve(t, true)
}

func (s *mirrorSet) register(m *foreignMirror) {
	if m.kind == "record" && m.typ.(*types.Named).Obj().Pkg().Path() == s.owner {
		s.records[m.gt] = m
	}
	if m.elem != nil {
		s.register(m.elem)
	}
	if m.key != nil {
		s.register(m.key)
	}
	for _, f := range m.fields {
		s.register(f.mirror)
	}
}
func (s *mirrorSet) call(sig *types.Signature, method bool) (*foreignCall, string) {
	if sig.TypeParams().Len() != 0 || sig.RecvTypeParams().Len() != 0 {
		return nil, "generic function"
	}
	c := &foreignCall{variadic: sig.Variadic(), result: &foreignMirror{kind: "unit", gt: "Unit"}}
	start := 0
	if sig.Params().Len() > 0 && contextType(sig.Params().At(0).Type()) {
		c.context = true
		start = 1
	}
	c.scan = method && sig.Params().Len()-start == 1 && sig.Variadic() && isAny(sig.Params().At(start).Type().(*types.Slice).Elem()) && sig.Results().Len() == 1 && errorType(sig.Results().At(0).Type())
	if c.scan {
		s.dyn = true
		c.params = []*foreignMirror{{typ: types.Typ[types.Int64], kind: "int", gt: "Int"}}
		c.result = &foreignMirror{kind: "dynlist", gt: "List[dyn.Value]"}
		c.failure = true
		return c, ""
	}
	for i := start; i < sig.Params().Len(); i++ {
		t := sig.Params().At(i).Type()
		var m *foreignMirror
		if sig.Variadic() && i == sig.Params().Len()-1 && isAny(t.(*types.Slice).Elem()) {
			s.dyn = true
			m = &foreignMirror{typ: t, kind: "dynlist", gt: "List[dyn.Value]"}
		} else {
			m = s.resolve(t, true)
		}
		if m == nil {
			return nil, fmt.Sprintf("unsupported parameter type %s", types.TypeString(t, nil))
		}
		c.params = append(c.params, m)
	}
	n := sig.Results().Len()
	if n > 0 && errorType(sig.Results().At(n-1).Type()) {
		c.failure = true
		n--
	}
	if n == 2 && !c.failure && types.Identical(sig.Results().At(1).Type(), types.Typ[types.Bool]) {
		c.optional = true
		n--
	}
	if n > 3 {
		return nil, fmt.Sprintf("unsupported multiple results: arity %d", n)
	}
	if n > 0 {
		elems := make([]*foreignMirror, n)
		for i := range elems {
			t := sig.Results().At(i).Type()
			elems[i] = s.resultMirror(t)
			if elems[i] == nil {
				return nil, fmt.Sprintf("unsupported result type %s", types.TypeString(t, nil))
			}
		}
		c.result = elems[0]
		if n > 1 {
			c.result = tupleMirror(elems)
		}
	}
	return c, ""
}
func (s *mirrorSet) declarations() string {
	var out strings.Builder
	if s.dyn {
		out.WriteString("import dyn \"std/dyn\"\n\n")
	}
	paths := make([]string, 0, len(s.imports))
	for path := range s.imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Fprintf(&out, "import %s %q\n", s.imports[path], "go:"+path)
	}
	if len(paths) > 0 {
		out.WriteByte('\n')
	}
	names := make([]string, 0, len(s.records)+len(s.handles))
	for n := range s.records {
		names = append(names, n)
	}
	for n := range s.handles {
		if s.records[n] == nil {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if m := s.records[n]; m != nil {
			fmt.Fprintf(&out, "type %s = %s{", n, n)
			for i, f := range m.fields {
				if i > 0 {
					out.WriteString(", ")
				}
				fmt.Fprintf(&out, "%s: %s", f.name, f.mirror.gt)
			}
			out.WriteString("} deriving Eq, Debug\n")
		} else {
			fmt.Fprintf(&out, "type %s = %sHandle\n", n, n)
		}
	}
	if len(names) > 0 {
		out.WriteByte('\n')
	}
	return out.String()
}

// foreignTypeName keeps Go type names, except that a name owned by the implicit
// prelude (Result, Option, List, ...) gets a Go suffix so generated bindings never
// shadow it; users write the qualified binding name either way.
func foreignTypeName(name string) string {
	for _, reserved := range gfxtypes.PreludeExports {
		if reserved == name {
			return name + "Go"
		}
	}
	return name
}
