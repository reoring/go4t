package extern

import (
	"fmt"
	"go/token"
	"go/types"
	"go4t/lexer"
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
	// mutable marks a call that receives a handle (as receiver or parameter):
	// the callee can read or change shared Go state, so the call is an effect
	// even without ctx or error. Records, maps and slices are copies.
	mutable bool
}
type mirrorSet struct {
	floats                         bool
	owner                          string
	imports                        map[string]string
	used                           map[string]bool
	records                        map[string]*foreignMirror
	handles                        map[string]*types.Named
	channels                       map[string]*foreignMirror
	resolving                      map[types.Type]bool
	lockTypes                      map[types.Type]bool
	dyn                            bool
	jsonMarshaler, jsonUnmarshaler *types.Interface
	// Templates share the concrete mirror grammar, but retain type variables.
	generic, forceIO bool
	parameters       map[*types.TypeParam]string
	unknown          map[types.Type]string
	origins          map[*types.Signature]*types.Signature
}

func newMirrorSet(floats bool, pkg *Package, used map[string]bool) *mirrorSet {
	return &mirrorSet{
		floats: floats, owner: pkg.Path, used: used, imports: map[string]string{},
		records: map[string]*foreignMirror{}, handles: map[string]*types.Named{},
		channels:      map[string]*foreignMirror{},
		resolving:     map[types.Type]bool{},
		jsonMarshaler: pkg.jsonMarshaler, jsonUnmarshaler: pkg.jsonUnmarshaler,
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
	if pkg.Path() == "C" {
		return false
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

// Match vet's copylocks predicate without importing sync into the bound package.
// In particular, embedding the Locker interface is not itself a lock value.
var lockerInterface = func() *types.Interface {
	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	return types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Lock", signature),
		types.NewFunc(token.NoPos, nil, "Unlock", signature),
	}, nil).Complete()
}()

func (s *mirrorSet) containsLock(t types.Type) bool {
	t = types.Unalias(t)
	if locked, known := s.lockTypes[t]; known {
		return locked
	}
	if s.lockTypes == nil {
		s.lockTypes = make(map[types.Type]bool)
	}
	locked := false
	switch v := t.Underlying().(type) {
	case *types.Array:
		locked = s.containsLock(v.Elem())
	case *types.Struct:
		locked = types.Implements(types.NewPointer(t), lockerInterface) && !types.Implements(t, lockerInterface)
		for i := 0; !locked && i < v.NumFields(); i++ {
			locked = s.containsLock(v.Field(i).Type())
		}
	}
	s.lockTypes[t] = locked
	return locked
}

// copyLockReason follows value conversions, including container elements and
// callback signatures. A pointer to a lock-containing struct stays a handle.
func (s *mirrorSet) copyLockReason(t types.Type) string {
	if locked := s.copiedLock(t, map[types.Type]bool{}); locked != nil {
		return "copies lock value " + types.TypeString(locked, func(*types.Package) string { return "" })
	}
	return ""
}

func (s *mirrorSet) copiedLock(t types.Type, seen map[types.Type]bool) types.Type {
	t = types.Unalias(t)
	if seen[t] {
		return nil
	}
	seen[t] = true
	if s.containsLock(t) {
		return t
	}
	switch v := t.Underlying().(type) {
	case *types.Pointer:
		if _, structValue := v.Elem().Underlying().(*types.Struct); !structValue {
			return s.copiedLock(v.Elem(), seen)
		}
	case *types.Slice:
		return s.copiedLock(v.Elem(), seen)
	case *types.Array:
		return s.copiedLock(v.Elem(), seen)
	case *types.Map:
		if locked := s.copiedLock(v.Key(), seen); locked != nil {
			return locked
		}
		return s.copiedLock(v.Elem(), seen)
	case *types.Chan:
		return s.copiedLock(v.Elem(), seen)
	case *types.Signature:
		for _, tuple := range []*types.Tuple{v.Params(), v.Results()} {
			for i := range tuple.Len() {
				if locked := s.copiedLock(tuple.At(i).Type(), seen); locked != nil {
					return locked
				}
			}
		}
	}
	return nil
}

// addressableMirror preserves the storage identity of a struct handle. Other
// mirrors still copy values, and pointer-origin handles already have an address.
func addressableMirror(m *foreignMirror) *foreignMirror {
	if m != nil && m.kind == "handle" {
		if _, ok := m.typ.Underlying().(*types.Struct); ok {
			copy := *m
			copy.typ = types.NewPointer(m.typ)
			return &copy
		}
	}
	return m
}

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
	if p, ok := t.(*types.TypeParam); ok && s.generic {
		if shape := structuralConstraint(p); shape != nil {
			return s.valueMirror(shape, visiting)
		}
		return &foreignMirror{typ: t, kind: "parameter", gt: s.parameters[p]}
	}
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
		if (v.TypeParams().Len() != 0 && v.TypeArgs().Len() == 0) || !v.Obj().Exported() || !s.importable(v) {
			return nil
		}
		if _, ok := v.Underlying().(*types.Struct); ok {
			if s.containsLock(v) {
				return nil
			}
			if v.TypeParams().Len() != 0 {
				return nil // generic named records need argument-indexed shape registration
			}
			// A declared JSON wire form takes precedence over representation fields.
			if s.jsonMarshaler != nil && s.jsonUnmarshaler != nil &&
				types.Implements(v, s.jsonMarshaler) && types.Implements(types.NewPointer(v), s.jsonUnmarshaler) {
				s.dyn = true
				m.kind, m.gt = "jsonscalar", "dyn.Value"
				return m
			}
			fields := promotedFields(v)
			// A private-state object (sql.DB, url.Userinfo, kubernetes.Clientset)
			// is a handle, not a record: copying its exported fields would
			// silently discard the unexported ones.
			if len(fields) == 0 || hasUnexportedState(v) {
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
				name := recordFieldName(f.name)
				if name == "" || names[name] {
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
		if m.key == nil || m.elem == nil || (!scalarMirror(m.key) && !(s.generic && m.key.kind == "parameter")) {
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

// hasUnexportedState reports whether a struct declares an unexported
// non-embedded field, directly or through a struct it embeds. A pointer
// embedding (kubernetes.Clientset's *DiscoveryClient) is shared state in
// itself and counts as well.
func hasUnexportedState(t types.Type) bool {
	return unexportedState(t, map[types.Type]bool{})
}

func unexportedState(t types.Type, seen map[types.Type]bool) bool {
	st, ok := t.Underlying().(*types.Struct)
	if !ok || seen[t] {
		return false
	}
	seen[t] = true
	for i := range st.NumFields() {
		f := st.Field(i)
		if f.Embedded() {
			if _, ptr := f.Type().(*types.Pointer); ptr {
				return true
			}
			if unexportedState(f.Type(), seen) {
				return true
			}
			continue
		}
		if !f.Exported() {
			return true
		}
	}
	return false
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
	if p, ok := t.(*types.TypeParam); ok && s.generic {
		if shape := structuralConstraint(p); shape != nil {
			return s.resolve(shape, callback)
		}
		return &foreignMirror{typ: t, kind: "parameter", gt: s.parameters[p]}
	}
	// Container and callback plans can register converters before a caller
	// reports a skip. Reject lock-copying elements before those side effects.
	switch t.Underlying().(type) {
	case *types.Slice, *types.Map, *types.Array, *types.Chan, *types.Signature:
		if s.copyLockReason(t) != "" {
			return nil
		}
	}
	// Iterators are materialised only in result position, never callbacks.
	if named(t, "iter", "Seq") || named(t, "iter", "Seq2") {
		if s.generic {
			return s.genericUnknown(t)
		}
		return nil
	}
	if ch, ok := t.Underlying().(*types.Chan); ok {
		if n, named := t.(*types.Named); named && (!n.Obj().Exported() || n.TypeParams().Len() != 0 || !s.importable(n)) {
			return nil
		}
		if s.resolving[t] {
			return nil
		}
		s.resolving[t] = true
		defer delete(s.resolving, t)
		var elem *foreignMirror
		if st, ok := types.Unalias(ch.Elem()).(*types.Struct); ok && st.NumFields() == 0 {
			// Go's anonymous completion signal carries no data.
			elem = &foreignMirror{typ: ch.Elem(), kind: "unit", gt: "Unit"}
		} else {
			elem = s.resolve(ch.Elem(), true)
		}
		if elem == nil {
			return nil
		}
		s.channels[types.TypeString(ch.Elem(), nil)] = elem
		name := "Chan"
		if ch.Dir() == types.RecvOnly {
			name = "RecvChan"
		} else if ch.Dir() == types.SendOnly {
			name = "SendChan"
		}
		return &foreignMirror{typ: t, kind: "chan", gt: name + "[" + elem.gt + "]", elem: elem}
	}
	if p, ok := t.(*types.Pointer); ok {
		if _, channel := p.Elem().Underlying().(*types.Chan); channel {
			if elem := s.resolve(p.Elem(), true); elem != nil {
				return &foreignMirror{typ: t, kind: "pointer", gt: "Option[" + elem.gt + "]", elem: elem}
			}
			return nil
		}
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
		if c.effect() {
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
	if s.generic {
		return s.genericUnknown(t)
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
	if !s.generic && (sig.TypeParams().Len() != 0 || sig.RecvTypeParams().Len() != 0) {
		return nil, "generic function"
	}
	c := &foreignCall{variadic: sig.Variadic(), mutable: s.generic || s.forceIO, result: &foreignMirror{kind: "unit", gt: "Unit"}}
	origin := sig
	if original := s.origins[sig]; original != nil {
		origin = original
	}
	start := 0
	if sig.Params().Len() > 0 && contextType(origin.Params().At(0).Type()) {
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
		if reason := s.copyLockReason(t); reason != "" {
			return nil, reason
		}
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
		c.mutable = c.mutable || m.sharesState()
	}
	n := sig.Results().Len()
	if n > 0 && errorType(origin.Results().At(n-1).Type()) {
		c.failure = true
		n--
	}
	if n == 2 && !c.failure && types.Identical(origin.Results().At(1).Type(), types.Typ[types.Bool]) {
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
			if reason := s.copyLockReason(t); reason != "" {
				return nil, reason
			}
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
	switch name {
	case "Ref", "Chan", "RecvChan", "SendChan":
		return name + "Go"
	}
	for _, reserved := range gfxtypes.PreludeExports {
		if reserved == name {
			return name + "Go"
		}
	}
	return name
}

// effect reports whether a call must run inside IO.
func (c *foreignCall) effect() bool { return c.context || c.failure || c.mutable }

// sharesState reports whether a value of this mirror carries a handle, directly
// or inside a container or Option. Invoking an effectful callback also observes
// shared state; channel-producing callbacks need IO for checked native conversion.
func (m *foreignMirror) sharesState() bool {
	if m == nil {
		return false
	}
	if m.kind == "handle" || m.kind == "chan" {
		return true
	}
	if m.callback != nil && (m.callback.effect() || m.callback.hasChannel()) {
		return true
	}
	if m.elem != nil && m.elem.sharesState() {
		return true
	}
	for _, e := range m.elems {
		if e.sharesState() {
			return true
		}
	}
	return false
}

// recordFieldName maps a Go field to a go4t record field. A field whose name
// is a go4t keyword (`Type` in SELinuxOptions) gets a trailing underscore, the
// same rule as package names; other reserved spellings make the type opaque.
func recordFieldName(goName string) string {
	name, reason := bindingName(goName, "")
	if reason == "" {
		return name
	}
	keyword := false
	lexer.VisitFixedTokens(func(kind lexer.Kind, text string) {
		if kind == lexer.Keyword && text == name {
			keyword = true
		}
	})
	if !keyword {
		return ""
	}
	return name + "_"
}
