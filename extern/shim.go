package extern

import (
	"fmt"
	"go/format"
	"go/types"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type shimGenerator struct {
	path       string
	imports    map[string]string
	body       strings.Builder
	converters map[string]string
	next       int
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
	key := m.kind + ":" + types.TypeString(m.typ, nil)
	if n := g.converters[key]; n != "" {
		return n
	}
	n := fmt.Sprintf("gofxMirror%d", g.next)
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
		to.WriteString("return rt.Unit{}\n")
		from.WriteString("return rt.Unit{}\n")
	case "handle":
		fmt.Fprintf(&to, "return v.(%s)\n", typ)
		from.WriteString("return v\n")
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
	case "slice":
		inner := g.conversion(m.elem)
		fmt.Fprintf(&to, "var result %s; for { x, rest, ok := rt.Shapes.Uncons(v); if !ok { return result }; result = append(result, %sTo(x,ctx)); v = rest }\n", typ, inner)
		fmt.Fprintf(&from, "result := rt.Shapes.Nil(); for i := len(v)-1; i >= 0; i-- { result = rt.Shapes.Cons(%sFrom(v[i]),result) }; return result\n", inner)
	case "receiver":
		inner := g.conversion(m.elem)
		fmt.Fprintf(&to, "result := %sTo(v,ctx); return &result\n", inner)
		fmt.Fprintf(&from, "return %sFrom(*v)\n", inner)
	case "dyn":
		to.WriteString("return rt.DynToGo(v)\n")
		from.WriteString("return rt.DynFromGo(v)\n")
	case "dynlist":
		to.WriteString("return rt.DynListToGo(v)\n")
		from.WriteString("return rt.DynListFromGo(v)\n")
	case "map":
		k, e := g.conversion(m.key), g.conversion(m.elem)
		fmt.Fprintf(&to, "result := make(%s); rt.ForeignMapEach(v,func(k,x any) { result[%sTo(k,ctx)] = %sTo(x,ctx) }); return result\n", typ, k, e)
		fmt.Fprintf(&from, "entries := make([][2]any,0,len(v)); for k,x := range v { entries = append(entries,[2]any{%sFrom(k),%sFrom(x)}) }; return rt.ForeignMap(entries)\n", k, e)
	case "record":
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
			fmt.Fprintf(&to, "%s = %sTo(rt.ForeignRecordField(%q,v,%d),ctx)\n", target, inner, g.path+"."+m.gfx, i)
			if len(guards) > 0 {
				fmt.Fprintf(&from, "var field%d %s; if %s { field%d = %s }; fields[%d] = %sFrom(field%d)\n", i, g.goType(f.mirror.typ), strings.Join(guards, " && "), i, source, i, inner, i)
			} else {
				fmt.Fprintf(&from, "fields[%d] = %sFrom(%s)\n", i, inner, source)
			}
		}
		to.WriteString("return result\n")
		fmt.Fprintf(&from, "return rt.ForeignRecord(%q,fields)\n", g.path+"."+m.gfx)
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
		for i, p := range c.params {
			args[i] = g.conversion(p) + "From(a" + strconv.Itoa(i+start) + ")"
		}
		fmt.Fprintf(&to, "value := f(%s)\n", strings.Join(args, ","))
		if c.failure || c.context {
			fmt.Fprintf(&to, "value, err := value.(rt.IO)(%s)\n", ctx)
			if c.failure {
				if sig.Results().Len() == 1 {
					to.WriteString("if err != nil { return err }\n")
				} else {
					fmt.Fprintf(&to, "if err != nil { var zero %s; return zero, err }\n", g.goType(sig.Results().At(0).Type()))
				}
			} else {
				to.WriteString("if err != nil { panic(err) }\n")
			}
		}
		if c.result.kind == "unit" && sig.Results().Len() == 0 {
			to.WriteString("_ = value; return\n")
		} else if c.result.kind == "unit" && sig.Results().Len() == 1 && c.failure {
			to.WriteString("_ = value; return nil\n")
		} else {
			fmt.Fprintf(&to, "return %sTo(value,%s)", g.conversion(c.result), ctx)
			if c.failure {
				to.WriteString(",nil")
			}
			to.WriteByte('\n')
		}
		to.WriteString("}\n")
		// Callback values are only converted into Go, never reflected back.
		from.WriteString("return v\n")
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
	effect := c.context || c.failure
	if effect {
		out.WriteString("return rt.IO(func(ctx context.Context) (any,error) {\nif err := ctx.Err(); err != nil { return nil,rt.ClassifyError(err) }\n")
	} else {
		out.WriteString("ctx := context.Background(); _ = ctx\n")
	}
	if recv != nil {
		if recv.kind == "handle" {
			fmt.Fprintf(&out, "receiver, ok := recv.(%s); if !ok { panic(fmt.Sprintf(%q,recv)) }\n", g.goType(recv.typ), "gofx shim: "+name+": receiver is <%T>")
			g.imports["fmt"] = "fmt"
		} else {
			fmt.Fprintf(&out, "receiver := %sTo(recv,ctx)\n", g.conversion(recv))
		}
	}
	var args []string
	if c.context {
		args = append(args, "ctx")
	}
	if c.scan {
		out.WriteString("count := p0.(int64); if count < 0 { return nil,rt.ClassifyError(fmt.Errorf(\"gofx scan: negative count %d\",count)) }; values := make([]any,count); dest := make([]any,count); for i := range dest { dest[i] = &values[i] }\n")
		g.imports["fmt"] = "fmt"
		args = append(args, "dest...")
	} else {
		for i, p := range c.params {
			arg := fmt.Sprintf("%sTo(p%d,ctx)", g.conversion(p), i)
			if c.variadic && i == len(c.params)-1 {
				arg += "..."
			}
			args = append(args, arg)
		}
	}
	call := callee + "(" + strings.Join(args, ",") + ")"
	count := sig.Results().Len()
	if c.scan {
		fmt.Fprintf(&out, "if err := %s; err != nil { return nil,rt.ClassifyError(err) }; return rt.DynListFromGo(values),nil\n", call)
	} else {
		result := "rt.Unit{}"
		if count == 0 {
			out.WriteString(call + "\n")
		} else if c.failure && count == 1 {
			fmt.Fprintf(&out, "if err := %s; err != nil { return nil,rt.ClassifyError(err) }\n", call)
		} else {
			if c.failure {
				fmt.Fprintf(&out, "value,err := %s; if err != nil { return nil,rt.ClassifyError(err) }\n", call)
			} else if c.optional {
				fmt.Fprintf(&out, "value,present := %s\n", call)
			} else {
				fmt.Fprintf(&out, "value := %s\n", call)
			}
			result = g.conversion(c.result) + "From(value)"
			if c.optional {
				suffix := ""
				if effect {
					suffix = ",nil"
				}
				fmt.Fprintf(&out, "if !present { return rt.NoneOf()%s }; return rt.SomeOf(%s)%s\n", suffix, result, suffix)
				result = ""
			}
		}
		if result != "" {
			out.WriteString("return " + result)
			if effect {
				out.WriteString(",nil")
			}
			out.WriteByte('\n')
		}
	}
	if effect {
		out.WriteString("})\n")
	}
	out.WriteString("}\n")
	g.body.WriteString(out.String())
}

// BindWithShim emits the complete binding file and its erased Go companion.
// Natural-ABI functions continue to target the original package; other calls
// use copy-boundary converters, with context captured only at IO execution.
func BindWithShim(pkg *Package, options BindOptions, shimPath, shimPackage string) (string, []byte, error) {
	set := newMirrorSet(options.Float)
	g := &shimGenerator{path: shimPath, imports: map[string]string{"context": "context", "gofx/rt": "rt"}, converters: map[string]string{}}
	var decl strings.Builder
	emit := func(goName, display, shimName, callee string, sig *types.Signature, recvType types.Type) {
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
					if p, ok := recvType.(*types.Pointer); ok && recv.kind != "handle" {
						recv = &foreignMirror{typ: p, kind: "receiver", gfx: copy.gfx, elem: set.resolve(base, false)}
					}
				}
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
		if options.Prefix == "" && options.Prelude[name] {
			fmt.Fprintf(&decl, "// collides with prelude: %s\n", name)
		}
		params := []string{}
		if recv != nil {
			params = append(params, "recv "+recv.gfx)
		}
		for i, m := range c.params {
			p := fmt.Sprintf("p%d", i)
			if c.scan {
				p = "count"
			}
			params = append(params, p+" "+m.gfx)
		}
		result := c.result.gfx
		if c.optional {
			result = "Option[" + result + "]"
		}
		if c.context || c.failure {
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
	}
	// A shim living in the bound package itself cannot reuse the function's name.
	samePackage := g.qualifier(types.NewPackage(pkg.Path, pkg.Name)) == ""
	for _, f := range pkg.Functions {
		callee, shimName := g.qualifier(types.NewPackage(pkg.Path, pkg.Name))+"."+f.Name, f.Name
		if samePackage {
			if strings.HasPrefix(f.Name, "Gofx_") {
				continue // output of a previous run of this command
			}
			callee, shimName = f.Name, "Gofx_"+f.Name
		}
		emit(f.Name, f.Name, shimName, callee, f.Signature, nil)
	}
	for _, m := range pkg.Methods {
		emit(receiverPrefix(m.TypeName)+m.Name, m.TypeName+"."+m.Name, m.TypeName+"_"+m.Name, "receiver."+m.Name, m.Signature, m.Receiver)
	}
	name := options.Package
	if name == "" {
		name = "main"
	}
	command := pkg.Path
	if options.Prefix != "" {
		command += " --prefix " + options.Prefix
	}
	source := fmt.Sprintf("// Code generated by gofx bind %s --shim. DO NOT EDIT.\npackage %s\n\n", command, name) + set.declarations() + decl.String()
	var goSource strings.Builder
	fmt.Fprintf(&goSource, "// Code generated by gofx bind %s --shim. DO NOT EDIT.\npackage %s\n", pkg.Path, shimPackage)
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
		return "", nil, fmt.Errorf("format binding shim: %w", err)
	}
	return source, formatted, nil
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
