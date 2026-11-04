package extern

import (
	"fmt"
	"go/format"
	"go/types"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"go4t/ast"
)

type shimGenerator struct {
	path       string
	imports    map[string]string
	body       strings.Builder
	converters map[string]string
	next       int
}

const (
	bindingCall = iota + 1
	bindingAccessor
	bindingValue
	bindingUpcast
)

type bindingCandidate struct {
	name, display string
	rank          int
	emit          func()
}

type bindingCandidates []bindingCandidate

func (c *bindingCandidates) add(name, display string, rank int, emit func()) {
	*c = append(*c, bindingCandidate{name: name, display: display, rank: rank, emit: emit})
}

func (c bindingCandidates) emit(decl *strings.Builder) {
	winners := make(map[string]int, len(c))
	for i, candidate := range c {
		winner, exists := winners[candidate.name]
		if !exists || candidate.rank < c[winner].rank {
			winners[candidate.name] = i
		}
	}
	for i, candidate := range c {
		if winner := winners[candidate.name]; winner != i {
			fmt.Fprintf(decl, "// skipped: %s (name %s taken by %s)\n", candidate.display, candidate.name, c[winner].display)
		} else {
			candidate.emit()
		}
	}
}

func (g *shimGenerator) qualifier(p *types.Package) string {
	if p.Path() == g.path {
		return ""
	}
	if a := g.imports[p.Path()]; a != "" {
		return a
	}
	a := fmt.Sprintf("go%d", len(g.imports))
	g.imports[p.Path()] = a
	return a
}
func (g *shimGenerator) goType(t types.Type) string {
	return types.TypeString(types.Unalias(t), g.qualifier)
}
func (g *shimGenerator) conversion(m *foreignMirror) string {
	key := m.kind + ":" + types.TypeString(m.typ, nil) + ":" + m.gt
	if n := g.converters[key]; n != "" {
		return n
	}
	n := fmt.Sprintf("go4tMirror%d", g.next)
	g.next++
	g.converters[key] = n
	var to, from strings.Builder
	typ := g.goType(m.typ)
	switch m.kind {
	case "int", "float", "bool", "string":
		erased := map[string]string{"int": "int64", "float": "float64", "bool": "bool", "string": "string"}[m.kind]
		fmt.Fprintf(&to, "return %s(v.(%s))\n", typ, erased)
		fmt.Fprintf(&from, "return %s(v)\n", erased)
	case "unit":
		fmt.Fprintf(&to, "return %s{}\n", typ)
		from.WriteString("return rt.Unit{}\n")
	case "chan":
		inner := g.conversion(m.elem)
		channel := m.typ.Underlying().(*types.Chan)
		fmt.Fprintf(&to, "return rt.GoChannel[%s](v)\n", typ)
		switch channel.Dir() {
		case types.RecvOnly:
			fmt.Fprintf(&from, "return rt.WrapRecvChan((<-chan %s)(v),%sFrom)\n", g.goType(m.elem.typ), inner)
		case types.SendOnly:
			fmt.Fprintf(&from, "return rt.WrapSendChan((chan<- %s)(v),%sTo)\n", g.goType(m.elem.typ), inner)
		default:
			fmt.Fprintf(&from, "return rt.WrapChan((chan %s)(v),%sFrom,%sTo)\n", g.goType(m.elem.typ), inner, inner)
		}
	case "handle":
		if _, ok := m.typ.Underlying().(*types.Struct); ok {
			// Value-origin opaque structs get one mutable box at the boundary.
			fmt.Fprintf(&to, "if p, ok := v.(*%s); ok { return *p }; return v.(%s)\n", typ, typ)
			from.WriteString("return &v\n")
		} else {
			fmt.Fprintf(&to, "return v.(%s)\n", typ)
			from.WriteString("return v\n")
		}
	case "bytes":
		fmt.Fprintf(&to, "return %s(v.(string))\n", typ)
		from.WriteString("return string(v)\n")
	case "time":
		timeAlias := g.qualifier(types.NewPackage("time", "time"))
		fmt.Fprintf(&to, "ns := v.(int64); if ns == 0 { return %s.Time{} }; return %s.Unix(0, ns)\n", timeAlias, timeAlias)
		from.WriteString("if v.IsZero() { return int64(0) }; return v.UnixNano()\n")
	case "error":
		errorsAlias := g.qualifier(types.NewPackage("errors", "errors"))
		fmt.Fprintf(&to, "x, ok := rt.OptionValue(v); if !ok { return nil }; return %s.New(x.(string))\n", errorsAlias)
		from.WriteString("if v == nil { return rt.NoneOf() }; return rt.SomeOf(v.Error())\n")
	case "pointer":
		inner := g.conversion(m.elem)
		fmt.Fprintf(&to, "x, ok := rt.OptionValue(v); if !ok { return nil }; result := %sTo(x,ctx); return &result\n", inner)
		fmt.Fprintf(&from, "if v == nil { return rt.NoneOf() }; return rt.SomeOf(%sFrom(*v))\n", inner)
	case "ref":
		inner := g.conversion(m.elem)
		// IO entry checks cancellation. Reads and writeback use Background so a
		// completed foreign mutation is not discarded if the callee cancels ctx.
		// Ref's public helpers own the cell lock; it is not held across Go code.
		fmt.Fprintf(&to, "x, _ := rt.RefGet(v).(rt.IO)(context.Background()); result := %sTo(x,ctx); return &result\n", inner)
		fmt.Fprintf(&from, "if v == nil { panic(\"go4t ref: nil pointer parameter\") }; result, _ := rt.RefNew(%sFrom(*v)).(rt.IO)(context.Background()); return result\n", inner)
	case "slice":
		inner := g.conversion(m.elem)
		fmt.Fprintf(&to, "var result %s; for { x, rest, ok := rt.Shapes.Uncons(v); if !ok { return result }; result = append(result, %sTo(x,ctx)); v = rest }\n", typ, inner)
		fmt.Fprintf(&from, "result := rt.Shapes.Nil(); for i := len(v)-1; i >= 0; i-- { result = rt.Shapes.Cons(%sFrom(v[i]),result) }; return result\n", inner)
	case "seq":
		params := make([]string, len(m.elems))
		values := make([]string, len(m.elems))
		converted := make([]string, len(m.elems))
		for i, elem := range m.elems {
			values[i] = fmt.Sprintf("x%d", i)
			params[i] = values[i] + " " + g.goType(elem.typ)
			converted[i] = fmt.Sprintf("%sTo(%s,ctx)", g.conversion(elem), values[i])
		}
		fmt.Fprintf(&from, "var values []any; v(func(%s) bool { values = append(values,%s); return true }); result := rt.Shapes.Nil(); for i := len(values)-1; i >= 0; i-- { result = rt.Shapes.Cons(values[i],result) }; return result\n", strings.Join(params, ","), g.fromValues(m.elem, values))
		for i, elem := range m.elems {
			params[i] = g.goType(elem.typ)
		}
		fmt.Fprintf(&to, "return func(yield func(%s) bool) { rest := v; for { x, tail, ok := rt.Shapes.Uncons(rest); if !ok { return }; rest = tail\n", strings.Join(params, ","))
		if len(m.elems) == 1 {
			to.WriteString("x0 := x\n")
		} else {
			to.WriteString("x0,x1 := rt.Shapes.Untuple2(x)\n")
		}
		fmt.Fprintf(&to, "if !yield(%s) { return } } }\n", strings.Join(converted, ","))
	case "receiver":
		inner := g.conversion(m.elem)
		fmt.Fprintf(&to, "result := %sTo(v,ctx); return &result\n", inner)
		fmt.Fprintf(&from, "return %sFrom(*v)\n", inner)
	case "dyn":
		to.WriteString("return rt.DynToGo(v)\n")
		from.WriteString("return rt.DynFromGo(v)\n")
	case "jsonscalar":
		jsonAlias := g.qualifier(types.NewPackage("encoding/json", "json"))
		prefix := "go4t json scalar: " + types.TypeString(m.typ, nil) + ": "
		fmt.Fprintf(&to, "data, err := %s.Marshal(rt.DynToGo(v)); if err != nil { panic(%q + err.Error()) }; var result %s; if err := result.UnmarshalJSON(data); err != nil { panic(%q + err.Error()) }; return result\n", jsonAlias, prefix, typ, prefix)
		fmt.Fprintf(&from, "data, err := %s.Marshal(v); if err != nil { panic(%q + err.Error()) }; var value any; if err := %s.Unmarshal(data,&value); err != nil { panic(%q + err.Error()) }; return rt.DynFromGo(value)\n", jsonAlias, prefix, jsonAlias, prefix)
	case "dynlist":
		to.WriteString("return rt.DynListToGo(v)\n")
		from.WriteString("return rt.DynListFromGo(v)\n")
	case "map":
		k, e := g.conversion(m.key), g.conversion(m.elem)
		fmt.Fprintf(&to, "result := make(%s); rt.ForeignMapEach(v,func(k,x any) { result[%sTo(k,ctx)] = %sTo(x,ctx) }); return result\n", typ, k, e)
		fmt.Fprintf(&from, "entries := make([][2]any,0,len(v)); for k,x := range v { entries = append(entries,[2]any{%sFrom(k),%sFrom(x)}) }; return rt.ForeignMap(entries)\n", k, e)
	case "record":
		obj := m.typ.(*types.Named).Obj()
		recordKey := obj.Pkg().Path() + "." + obj.Name()
		fmt.Fprintf(&to, "var result %s\n", typ)
		from.WriteString("fields := make([]any, " + strconv.Itoa(len(m.fields)) + ")\n")
		initialized := map[string]bool{}
		for i, f := range m.fields {
			inner := g.conversion(f.mirror)
			target, source := "result", "v"
			cur := m.typ
			var guards []string
			var leafName string
			for j, index := range f.path {
				st := cur.Underlying().(*types.Struct)
				field := st.Field(index)
				leafName = field.Name()
				target += "." + field.Name()
				source += "." + field.Name()
				cur = field.Type()
				if ptr, ok := cur.(*types.Pointer); ok && j < len(f.path)-1 {
					if !initialized[target] {
						fmt.Fprintf(&to, "%s = new(%s)\n", target, g.goType(ptr.Elem()))
						initialized[target] = true
					}
					guards = append(guards, source+" != nil")
					cur = ptr.Elem()
				}
			}
			// Use the promoted selector for the leaf; naming an unexported embedded
			// value itself would be illegal outside its package.
			target, source = "result."+leafName, "v."+leafName
			fmt.Fprintf(&to, "%s = %sTo(rt.ForeignRecordField(%q,v,%d),ctx)\n", target, inner, recordKey, i)
			if len(guards) > 0 {
				fmt.Fprintf(&from, "var field%d %s; if %s { field%d = %s }; fields[%d] = %sFrom(field%d)\n", i, g.goType(f.mirror.typ), strings.Join(guards, " && "), i, source, i, inner, i)
			} else {
				fmt.Fprintf(&from, "fields[%d] = %sFrom(%s)\n", i, inner, source)
			}
		}
		to.WriteString("return result\n")
		fmt.Fprintf(&from, "return rt.ForeignRecord(%q,fields)\n", recordKey)
	case "callback":
		c := m.callback
		sig := m.typ.Underlying().(*types.Signature)
		args := make([]string, len(c.params))
		for i := range args {
			args[i] = "any"
		}
		fmt.Fprintf(&to, "f := v.(func(%s) any)\nreturn func(", strings.Join(args, ","))
		for i := range sig.Params().Len() {
			if i > 0 {
				to.WriteString(",")
			}
			t := sig.Params().At(i).Type()
			spelling := g.goType(t)
			if sig.Variadic() && i == sig.Params().Len()-1 {
				spelling = "..." + g.goType(t.(*types.Slice).Elem())
			}
			fmt.Fprintf(&to, "a%d %s", i, spelling)
		}
		to.WriteString(")")
		g.results(&to, sig)
		to.WriteString(" {\n")
		ctx := "ctx"
		start := 0
		if c.context {
			ctx = "a0"
			start = 1
		}
		var writeback strings.Builder
		for i, p := range c.params {
			native := "a" + strconv.Itoa(i+start)
			args[i] = g.conversion(p) + "From(" + native + ")"
			if p.refParameter() {
				cell := "ref" + strconv.Itoa(i)
				fmt.Fprintf(&to, "%s := %s\n", cell, args[i])
				g.refWriteback(&writeback, p, cell, native, ctx, true)
				args[i] = cell
			}
		}
		fmt.Fprintf(&to, "value := f(%s)\n", strings.Join(args, ","))
		if c.effect() {
			fmt.Fprintf(&to, "value, err := value.(rt.IO)(%s)\n", ctx)
			to.WriteString(writeback.String())
			if c.failure {
				var zero []string
				to.WriteString("if err != nil {\n")
				for i := range sig.Results().Len() - 1 {
					name := fmt.Sprintf("zero%d", i)
					fmt.Fprintf(&to, "var %s %s\n", name, g.goType(sig.Results().At(i).Type()))
					zero = append(zero, name)
				}
				zero = append(zero, "err")
				fmt.Fprintf(&to, "return %s\n}\n", strings.Join(zero, ","))
			} else {
				to.WriteString("if err != nil { panic(err) }\n")
			}
		}
		if c.result.kind == "unit" && sig.Results().Len() == 0 {
			to.WriteString("_ = value; return\n")
		} else if c.result.kind == "unit" && sig.Results().Len() == 1 && c.failure {
			to.WriteString("_ = value; return nil\n")
		} else {
			if c.result.kind == "tuple" {
				values := make([]string, len(c.result.elems))
				converted := make([]string, len(values))
				for i, elem := range c.result.elems {
					values[i] = fmt.Sprintf("value%d", i)
					converted[i] = fmt.Sprintf("%sTo(%s,%s)", g.conversion(elem), values[i], ctx)
				}
				fmt.Fprintf(&to, "%s := rt.Shapes.Untuple%d(value)\nreturn %s", strings.Join(values, ","), len(values), strings.Join(converted, ","))
			} else {
				fmt.Fprintf(&to, "return %sTo(value,%s)", g.conversion(c.result), ctx)
			}
			if c.failure {
				to.WriteString(",nil")
			}
			to.WriteByte('\n')
		}
		to.WriteString("}\n")
		from.WriteString("return func(")
		for i := range c.params {
			if i > 0 {
				from.WriteByte(',')
			}
			fmt.Fprintf(&from, "p%d any", i)
		}
		from.WriteString(") any {\n")
		if c.effect() {
			beginShimIO(&from, c.hasChannel())
		} else {
			from.WriteString("ctx := context.Background(); _ = ctx\n")
		}
		g.callBody(&from, "v", sig, c)
		if c.effect() {
			from.WriteString("})\n")
		}
		from.WriteString("}\n")
	}
	fmt.Fprintf(&g.body, "func %sTo(v any, ctx context.Context) %s {\n%s}\nfunc %sFrom(v %s) any {\n%s}\n", n, typ, to.String(), n, typ, from.String())
	return n
}
func (g *shimGenerator) results(out *strings.Builder, sig *types.Signature) {
	if sig.Results().Len() == 0 {
		return
	}
	out.WriteString(" (")
	for i := range sig.Results().Len() {
		if i > 0 {
			out.WriteString(",")
		}
		out.WriteString(g.goType(sig.Results().At(i).Type()))
	}
	out.WriteString(")")
}
func (g *shimGenerator) emit(name, callee string, sig *types.Signature, c *foreignCall, recv *foreignMirror) {
	var out strings.Builder
	fmt.Fprintf(&out, "func %s(", name)
	if recv != nil {
		out.WriteString("recv any")
		if len(c.params) > 0 {
			out.WriteByte(',')
		}
	}
	for i := range c.params {
		if i > 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, "p%d any", i)
	}
	out.WriteString(") any {\n")
	effect := c.effect()
	if effect {
		beginShimIO(&out, c.hasChannel() || recv.hasChannel())
	} else {
		out.WriteString("ctx := context.Background(); _ = ctx\n")
	}
	if recv != nil {
		fmt.Fprintf(&out, "receiver := %sTo(recv,ctx)\n", g.conversion(recv))
	}
	g.callBody(&out, callee, sig, c)
	if effect {
		out.WriteString("})\n")
	}
	out.WriteString("}\n")
	g.body.WriteString(out.String())
}

func (g *shimGenerator) fromValues(m *foreignMirror, values []string) string {
	if m.kind != "tuple" {
		return g.conversion(m) + "From(" + values[0] + ")"
	}
	converted := make([]string, len(m.elems))
	for i, elem := range m.elems {
		converted[i] = g.conversion(elem) + "From(" + values[i] + ")"
	}
	return fmt.Sprintf("rt.Tuple%dOf(%s)", len(converted), strings.Join(converted, ","))
}

func (m *foreignMirror) refParameter() bool {
	return m.kind == "ref" || m.kind == "slice" && m.elem.kind == "ref"
}

// refWriteback synchronizes parameter cells after invocation, before handling
// either payloads or errors. Reverse adapters copy go4t callback writes back to
// the caller's original Go pointers.
func (g *shimGenerator) refWriteback(out *strings.Builder, m *foreignMirror, cell, native, ctx string, reverse bool) {
	if m.kind == "slice" {
		fmt.Fprintf(out, "{ rest := %s; for _, value := range %s { cell, tail, _ := rt.Shapes.Uncons(rest); rest = tail\n", cell, native)
		g.refWriteback(out, m.elem, "cell", "value", ctx, reverse)
		out.WriteString("} }\n")
	} else if reverse {
		fmt.Fprintf(out, "*%s = *%sTo(%s,%s)\n", native, g.conversion(m), cell, ctx)
	} else {
		fmt.Fprintf(out, "_, _ = rt.RefSet(%s,%sFrom(*%s)).(rt.IO)(context.Background())\n", cell, g.conversion(m.elem), native)
	}
}

func (g *shimGenerator) callBody(out *strings.Builder, callee string, sig *types.Signature, c *foreignCall) {
	effect := c.effect()
	var args []string
	var writeback strings.Builder
	if c.context {
		args = append(args, "ctx")
	}
	if c.scan {
		out.WriteString("count := p0.(int64); if count < 0 { return nil,rt.ClassifyError(fmt.Errorf(\"go4t scan: negative count %d\",count)) }; values := make([]any,count); dest := make([]any,count); for i := range dest { dest[i] = &values[i] }\n")
		g.imports["fmt"] = "fmt"
		args = append(args, "dest...")
	} else {
		for i, p := range c.params {
			arg := fmt.Sprintf("%sTo(p%d,ctx)", g.conversion(p), i)
			if p.refParameter() {
				native := fmt.Sprintf("arg%d", i)
				fmt.Fprintf(out, "%s := %s\n", native, arg)
				g.refWriteback(&writeback, p, fmt.Sprintf("p%d", i), native, "ctx", false)
				arg = native
			}
			if c.variadic && i == len(c.params)-1 {
				arg += "..."
			}
			args = append(args, arg)
		}
	}
	call := callee + "(" + strings.Join(args, ",") + ")"
	if c.scan {
		fmt.Fprintf(out, "if err := %s; err != nil { return nil,rt.ClassifyError(err) }; return rt.DynListFromGo(values),nil\n", call)
		return
	}
	count := sig.Results().Len()
	result := "rt.Unit{}"
	if count == 0 {
		out.WriteString(call + "\n")
		out.WriteString(writeback.String())
	} else {
		n := count
		if c.failure || c.optional {
			n--
		}
		values := make([]string, n)
		for i := range values {
			values[i] = fmt.Sprintf("value%d", i)
		}
		targets := append([]string(nil), values...)
		if c.failure {
			targets = append(targets, "err")
		} else if c.optional {
			targets = append(targets, "present")
		}
		fmt.Fprintf(out, "%s := %s\n", strings.Join(targets, ","), call)
		out.WriteString(writeback.String())
		if c.failure {
			out.WriteString("if err != nil { return nil,rt.ClassifyError(err) }\n")
		}
		if n > 0 {
			result = g.fromValues(c.result, values)
		}
		if c.optional {
			suffix := ""
			if effect {
				suffix = ",nil"
			}
			fmt.Fprintf(out, "if !present { return rt.NoneOf()%s }; return rt.SomeOf(%s)%s\n", suffix, result, suffix)
			return
		}
	}
	out.WriteString("return " + result)
	if effect {
		out.WriteString(",nil")
	}
	out.WriteByte('\n')
}

func (g *shimGenerator) accessors(decl *strings.Builder, set *mirrorSet, options BindOptions, candidates *bindingCandidates) {
	seen := map[string]bool{}
	for {
		var names []string
		for name := range set.handles {
			if !seen[name] {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			return
		}
		sort.Strings(names)
		for _, name := range names {
			seen[name] = true
			n := set.handles[name]
			if _, ok := n.Underlying().(*types.Struct); !ok {
				continue
			}
			for _, field := range promotedFields(n) {
				display := n.Obj().Name() + "." + field.name
				getter, reason := bindingName(methodBindingName(n.Obj().Name(), field.name, options.Prefix), options.Prefix)
				setter, setterReason := bindingName(methodBindingName(n.Obj().Name(), "Set"+field.name, options.Prefix), options.Prefix)
				if reason == "" {
					reason = setterReason
				}
				m := set.resolve(field.typ, true)
				getMirror := addressableMirror(m)
				copyReason := ""
				if getMirror == m {
					copyReason = set.copyLockReason(field.typ)
				}
				if reason == "" {
					reason = copyReason
				}
				if reason == "" && m == nil {
					reason = "unsupported field type " + types.TypeString(field.typ, nil)
				}
				if reason != "" {
					fmt.Fprintf(decl, "// skipped: %s (%s)\n", display, reason)
					continue
				}
				getGo := n.Obj().Name() + "_" + field.name
				setGo := n.Obj().Name() + "_Set" + field.name
				candidates.add(getter, display, bindingAccessor, func() {
					fmt.Fprintf(decl, "extern func %s(recv %s) IO[%s] = %q.%s\n", getter, name, m.gt, g.path, getGo)
					conv := g.conversion(getMirror)
					typ := g.goType(n)
					value := "receiver." + field.name
					if getMirror != m {
						value = "&" + value
					}
					fmt.Fprintf(&g.body, "func %s(recv any) any { return rt.IO(func(ctx context.Context) (any,error) { if err := ctx.Err(); err != nil { return nil,rt.ClassifyError(err) }; receiver := recv.(*%s); return %sFrom(%s),nil }) }\n", getGo, typ, conv, value)
				})
				if reason := set.copyLockReason(field.typ); reason != "" {
					fmt.Fprintf(decl, "// skipped: %s.Set%s (%s)\n", n.Obj().Name(), field.name, reason)
					continue
				}
				candidates.add(setter, display, bindingAccessor, func() {
					fmt.Fprintf(decl, "extern func %s(recv %s, value %s) IO[Unit] = %q.%s\n", setter, name, m.gt, g.path, setGo)
					conv := g.conversion(m)
					typ := g.goType(n)
					fmt.Fprintf(&g.body, "func %s(recv,value any) any { ", setGo)
					beginShimIO(&g.body, m.hasChannel())
					fmt.Fprintf(&g.body, "receiver := recv.(*%s); receiver.%s = %sTo(value,ctx); return rt.Unit{},nil }) }\n", typ, field.name, conv)
				})
			}
		}
	}
}

// BindingMetadata preserves Go identities independently of binding spellings.
type BindingMetadata struct {
	RecordKeys map[string]string
	HandleKeys map[string]ast.ForeignHandle
}

// BindWithShim emits the complete binding file and its erased Go companion.
// Natural-ABI functions continue to target the original package; other calls
// use copy-boundary converters, with context captured only at IO execution.
// Exported variables/constants are pure zero-argument getters, evaluated at
// each call. Owned opaque structs also get fresh zero-value handle constructors
// unless their generated name is already reserved by a package binding.
// The third result retains Go record and handle identities, including names
// whose go4t spelling changes to avoid a prelude collision.
func BindWithShim(pkg *Package, options BindOptions, shimPath, shimPackage string) (string, []byte, *BindingMetadata, error) {
	g := &shimGenerator{path: shimPath, imports: map[string]string{"context": "context", "go4t/rt": "rt"}, converters: map[string]string{}}
	var decl strings.Builder
	// These reservations only keep generated import aliases out of the way;
	// all binding-name competition is decided by candidates below.
	importNames := map[string]bool{}
	for _, f := range pkg.Functions {
		name, _ := bindingName(f.Name, options.Prefix)
		importNames[name] = true
	}
	for _, m := range pkg.Methods {
		name, _ := bindingName(methodBindingName(m.TypeName, m.Name, options.Prefix), options.Prefix)
		importNames[name] = true
	}
	for _, value := range exportedValues(pkg.goPackage) {
		name, _ := bindingName(value.Name(), options.Prefix)
		importNames[name] = true
	}
	set := newMirrorSet(options.Float, pkg, importNames)
	// A referenced type may have no constructors or methods in its own package.
	// Still declare it there, so every importer shares its owning module's type.
	for _, n := range exportedNamedTypes(pkg.goPackage) {
		set.resolve(n, false)
	}
	var candidates bindingCandidates
	collect := func(goName, display, shimName, callee string, sig *types.Signature, recvType types.Type) {
		c, reason := set.call(sig, recvType != nil)
		var recv *foreignMirror
		if recvType != nil {
			base := recvType
			if p, ok := base.(*types.Pointer); ok {
				base = p.Elem()
			}
			if n, ok := base.(*types.Named); !ok || n.TypeParams().Len() != 0 {
				reason = "generic function"
			} else if iface, ok := base.Underlying().(*types.Interface); ok && !iface.IsMethodSet() {
				reason = "generic function"
			}
			if reason == "" {
				// Methods use the value record, not Option[Record], for pointer receivers.
				recv = set.resolve(base, false)
				if recv == nil {
					reason = "unsupported receiver"
				} else {
					copy := *recv
					copy.typ = recvType
					recv = &copy
					recv = handleMethodReceiver(recv, strings.TrimPrefix(callee, "receiver."))
					if p, ok := recvType.(*types.Pointer); ok && recv.kind != "handle" {
						recv = &foreignMirror{typ: p, kind: "receiver", gt: copy.gt, elem: set.resolve(base, false)}
					}
				}
			}
		}
		if c != nil {
			c.mutable = recv.sharesState()
			for _, p := range c.params {
				c.mutable = c.mutable || p.sharesState()
			}
		}
		name, nreason := bindingName(goName, options.Prefix)
		if reason == "" {
			reason = nreason
		}
		if reason != "" {
			fmt.Fprintf(&decl, "// skipped: %s (%s)\n", display, reason)
			return
		}
		candidates.add(name, display, bindingCall, func() {
			if options.Prefix == "" && options.Prelude[name] {
				fmt.Fprintf(&decl, "// collides with prelude: %s\n", name)
			}
			params := []string{}
			if recv != nil {
				params = append(params, "recv "+recv.gt)
			}
			for i, m := range c.params {
				p := fmt.Sprintf("p%d", i)
				if c.scan {
					p = "count"
				}
				params = append(params, p+" "+m.gt)
			}
			result := c.result.gt
			if c.optional {
				result = "Option[" + result + "]"
			}
			if c.effect() {
				result = "IO[" + result + "]"
			}
			target := shimPath
			targetName := shimName
			if recv == nil {
				if _, _, r := bindingTypes(sig, options.Float); r == "" {
					target = pkg.Path
					targetName = goName
				}
			}
			fmt.Fprintf(&decl, "extern func %s(%s) %s = %q.%s\n", name, strings.Join(params, ", "), result, target, targetName)
			if target == shimPath {
				g.emit(shimName, callee, sig, c, recv)
			}
		})
	}
	// A shim living in the bound package itself cannot reuse the function's name.
	samePackage := g.qualifier(types.NewPackage(pkg.Path, pkg.Name)) == ""
	for _, f := range pkg.Functions {
		callee, shimName := g.qualifier(types.NewPackage(pkg.Path, pkg.Name))+"."+f.Name, f.Name
		if samePackage {
			if strings.HasPrefix(f.Name, "Go4t_") {
				continue // output of a previous run of this command
			}
			callee, shimName = f.Name, "Go4t_"+f.Name
		}
		if f.Signature.TypeParams().Len() != 0 {
			name, reason := bindingName(f.Name, options.Prefix)
			if reason != "" {
				fmt.Fprintf(&decl, "// skipped: %s (%s)\n", f.Name, reason)
			} else {
				source := genericTemplate(set, f, name)
				candidates.add(name, f.Name, bindingCall, func() { decl.WriteString(source) })
			}
			continue
		}
		collect(f.Name, f.Name, shimName, callee, f.Signature, nil)
	}
	for _, m := range pkg.Methods {
		collect(methodBindingName(m.TypeName, m.Name, options.Prefix), m.TypeName+"."+m.Name, m.TypeName+"_"+m.Name, "receiver."+m.Name, m.Signature, m.Receiver)
	}
	g.values(&decl, set, pkg, options, &candidates)
	g.constructors(&decl, set, options, &candidates)
	g.accessors(&decl, set, options, &candidates)
	g.upcasts(&decl, set, pkg, options, &candidates)
	g.channelConstructors(&decl, set, options, &candidates)
	candidates.emit(&decl)
	g.errorClassifier(pkg)
	name := options.Package
	if name == "" {
		name = "main"
	}
	command := pkg.Path
	if options.Prefix != "" {
		command += " --prefix " + options.Prefix
	}
	source := fmt.Sprintf("// Code generated by go4t bind %s --shim. DO NOT EDIT.\npackage %s\n\n", command, name) + set.declarations() + decl.String()
	var goSource strings.Builder
	fmt.Fprintf(&goSource, "// Code generated by go4t bind %s --shim. DO NOT EDIT.\npackage %s\n", pkg.Path, shimPackage)
	// Qualification may inspect a natural function without emitting any code.
	if g.body.Len() > 0 {
		goSource.WriteString("import (\n")
		paths := []string{}
		for p, a := range g.imports {
			if strings.Contains(g.body.String(), a+".") {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		for _, p := range paths {
			fmt.Fprintf(&goSource, "%s %q\n", g.imports[p], p)
		}
		goSource.WriteString(")\n")
		goSource.WriteString(g.body.String())
	}
	formatted, err := format.Source([]byte(goSource.String()))
	if err != nil {
		return "", nil, nil, fmt.Errorf("format binding shim: %w", err)
	}
	metadata := &BindingMetadata{
		RecordKeys: make(map[string]string, len(set.records)),
		HandleKeys: make(map[string]ast.ForeignHandle, len(set.handles)),
	}
	for name, record := range set.records {
		obj := record.typ.(*types.Named).Obj()
		metadata.RecordKeys[name] = obj.Pkg().Path() + "." + obj.Name()
	}
	for name, handle := range set.handles {
		if set.records[name] != nil {
			continue // declarations prefer a record if both plans were discovered
		}
		_, boxed := handle.Underlying().(*types.Struct)
		obj := handle.Obj()
		metadata.HandleKeys[name] = ast.ForeignHandle{PackagePath: obj.Pkg().Path(), TypeName: obj.Name(), Struct: boxed}
	}
	return source, formatted, metadata, nil
}

// methodBindingName keeps the Go spelling after an explicit prefix; without one the
// receiver type is lower-cased so DB.ExecContext binds as dbExecContext.
func methodBindingName(typeName, method, prefix string) string {
	if prefix != "" {
		return typeName + method
	}
	return receiverPrefix(typeName) + method
}

// receiverPrefix lower-cases a receiver type name for binding names: DB.ExecContext
// becomes dbExecContext, URL.Hostname urlHostname, Rows.Scan rowsScan.
func receiverPrefix(typeName string) string {
	runes := []rune(typeName)
	i := 0
	for i < len(runes) && unicode.IsUpper(runes[i]) {
		i++
	}
	if i > 1 && i < len(runes) {
		i-- // keep the last capital for the next word: HTTPClient -> httpClient
	}
	return strings.ToLower(string(runes[:i])) + string(runes[i:])
}
