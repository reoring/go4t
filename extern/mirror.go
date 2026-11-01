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
	floats  bool
	records map[string]*foreignMirror
	handles map[string]bool
	dyn     bool
}

func newMirrorSet(floats bool) *mirrorSet {
	return &mirrorSet{floats: floats, records: map[string]*foreignMirror{}, handles: map[string]bool{}}
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
		if v.TypeParams().Len() != 0 || !v.Obj().Exported() {
			return nil
		}
		if _, ok := v.Underlying().(*types.Struct); ok {
			fields := promotedFields(v)
			// A private-state object (sql.DB, url.Userinfo, ...) is a handle, not
			// an empty record which would silently discard its state.
			if len(fields) == 0 {
				return nil
			}
			m.kind, m.gt = "record", foreignTypeName(v.Obj().Name())
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
	if isAny(t) {
		s.dyn = true
		return &foreignMirror{typ: t, kind: "dyn", gt: "dyn.Value"}
	}
	if callback {
		if sig, ok := t.Underlying().(*types.Signature); ok {
			c, reason := s.call(sig, false)
			if reason == "" && !c.optional {
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
		}
	}
	if m := s.valueMirror(t, map[types.Type]bool{}); m != nil {
		s.register(m)
		return m
	}
	t = types.Unalias(t)
	base := t
	if p, ok := base.(*types.Pointer); ok {
		base = types.Unalias(p.Elem())
	}
	if n, ok := base.(*types.Named); ok && n.Obj().Exported() && n.TypeParams().Len() == 0 {
		if i, ok := n.Underlying().(*types.Interface); ok && !i.IsMethodSet() {
			return nil
		}
		s.handles[foreignTypeName(n.Obj().Name())] = true
		return &foreignMirror{typ: t, kind: "handle", gt: foreignTypeName(n.Obj().Name())}
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
func (s *mirrorSet) register(m *foreignMirror) {
	if m.kind == "record" {
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
	if n > 1 {
		return nil, "unsupported multiple results"
	}
	if n == 1 {
		t := sig.Results().At(0).Type()
		c.result = s.resolve(t, false)
		if c.result == nil {
			return nil, fmt.Sprintf("unsupported result type %s", types.TypeString(t, nil))
		}
	}
	return c, ""
}
func (s *mirrorSet) declarations() string {
	var out strings.Builder
	if s.dyn {
		out.WriteString("import dyn \"std/dyn\"\n\n")
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
