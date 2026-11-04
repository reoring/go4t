package codegen

import (
	"fmt"
	goast "go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// Export generation runs after module renaming. Only references to the erased
// implementation use renamed; the natural Go API never passes through that pass.
type mirrorGenerator struct {
	prog          *types.Program
	prefixes      map[string]string
	renamed       map[string]string
	root          string
	entries       map[string]*mirrorType
	dataTypes     map[string]*types.DataType
	declared      map[string]sourceLocation
	imports       map[string]string
	names         map[string]mirrorNameKind
	sources       *generator
	legacyHandles map[string]bool
	err           error
}

type mirrorType struct {
	typ       types.Type
	goType    string
	id        string
	debugDict string
}

type mirrorFunction struct {
	*types.FuncInfo
	goName      string
	renamedFrom string
}

type mirrorNameKind uint8

const (
	mirrorNameOther mirrorNameKind = iota + 1
	mirrorNameData
)

func addGoExports(prog *types.Program, pkg string, prefixes, renamed map[string]string, out map[string][]byte) error {
	if pkg == "main" {
		return nil
	}
	g := &mirrorGenerator{prog: prog, prefixes: make(map[string]string), renamed: renamed,
		entries: make(map[string]*mirrorType), dataTypes: make(map[string]*types.DataType),
		declared: make(map[string]sourceLocation), imports: make(map[string]string), names: make(map[string]mirrorNameKind), sources: &generator{},
		legacyHandles: make(map[string]bool)}
	g.sources.indexSources(prog)
	g.indexLegacyHandles()
	if len(prog.Modules) != 0 {
		g.root = prog.Modules[len(prog.Modules)-1].Path
	}
	usedPrefixes := make(map[string]bool)
	for _, module := range prog.Modules {
		base := exported(prefixes[module.Path])
		prefix := base
		for n := 2; usedPrefixes[prefix]; n++ {
			prefix = fmt.Sprintf("%s_%d", base, n)
		}
		usedPrefixes[prefix] = true
		g.prefixes[module.Path] = prefix
	}
	for _, name := range sortedKeys(prog.DataTypes) {
		g.declared[g.dataName(name)] = g.sources.typeSources[name]
	}
	for _, name := range sortedKeys(g.sources.aliases) {
		alias := g.sources.aliases[name]
		g.declared[g.dataName(name)] = sourceLocation{g.sources.sources[alias], alias.Pos}
	}
	g.sources.instances = make(map[string]*types.Instance)
	for _, instance := range prog.Instances {
		g.sources.instances[instanceName(instance)] = instance
	}
	// Include implementation declarations when checking the API namespace. This
	// also diagnoses programmatic Generate calls without module renaming.
	for _, filename := range sortedKeys(out) {
		file, err := parser.ParseFile(token.NewFileSet(), filename, out[filename], 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *goast.FuncDecl:
				if decl.Recv == nil {
					g.names[decl.Name.Name] = mirrorNameOther
				}
			case *goast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *goast.TypeSpec:
						g.names[spec.Name.Name] = mirrorNameOther
					case *goast.ValueSpec:
						for _, name := range spec.Names {
							g.names[name.Name] = mirrorNameOther
						}
					}
				}
			}
		}
	}
	var functions []mirrorFunction
	var body strings.Builder
	for _, key := range sortedKeys(prog.Funcs) {
		f := prog.Funcs[key]
		name := g.sourceName(f.Name)
		if !g.entryFunction(f) || name == "main" || strings.HasPrefix(name, "_") {
			continue
		}
		reason := g.functionReason(f)
		if reason != "" {
			fmt.Fprintf(&body, "// not exported: %s (%s)\n", name, reason)
			continue
		}
		functions = append(functions, mirrorFunction{FuncInfo: f, goName: exported(name)})
		for _, p := range f.Scheme.Type.Params {
			g.collect(p)
		}
		g.collect(f.Scheme.Type.Ret)
	}
	if g.err != nil {
		return g.err
	}
	// Sort by natural Go spelling, not traversal order. Allocate converter names
	// only after the closure is known, avoiding ambiguous composite spellings.
	entries := make([]*mirrorType, 0, len(g.entries))
	for _, m := range g.entries {
		entries = append(entries, m)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].goType < entries[j].goType })
	for _, m := range entries {
		if d := g.data(m.typ); d != nil {
			if err := g.emitData(&body, m, d); err != nil {
				return err
			}
		}
	}
	for i := range functions {
		f := &functions[i]
		if g.names[f.goName] == mirrorNameData {
			f.renamedFrom = f.goName
			f.goName += "Fn"
		}
		if err := g.reserve(f.goName, mirrorNameOther); err != nil {
			return g.functionError(f.FuncInfo, err)
		}
	}
	for _, m := range entries {
		base := g.converterID(m.typ)
		m.id = base
		for n := 2; g.names["toGo_"+m.id] != 0 || g.names["fromGo_"+m.id] != 0; n++ {
			m.id = fmt.Sprintf("%s_%d", base, n)
		}
		g.names["toGo_"+m.id], g.names["fromGo_"+m.id] = mirrorNameOther, mirrorNameOther
	}
	for _, m := range entries {
		g.emitConverters(&body, m)
		g.emitString(&body, m)
	}
	for i := range functions {
		g.emitWrapper(&body, &functions[i])
	}
	var source strings.Builder
	fmt.Fprintf(&source, "// Code generated by go4t. DO NOT EDIT.\npackage %s\n", pkg)
	for _, path := range sortedKeys(g.imports) {
		fmt.Fprintf(&source, "import %s %q\n", g.imports[path], path)
	}
	source.WriteString(body.String())
	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return fmt.Errorf("format Go exports: %w", err)
	}
	out["go4t_export.go"] = formatted
	return nil
}

func (g *mirrorGenerator) sourceName(name string) string {
	if source := g.prog.ModuleNames[name]; source != "" {
		return source
	}
	return name
}

func (g *mirrorGenerator) internal(name string) string {
	if renamed := g.renamed[name]; renamed != "" {
		return renamed
	}
	return name
}

func (g *mirrorGenerator) entryFunction(f *types.FuncInfo) bool {
	if strings.HasPrefix(f.Name, "ext_go_") {
		return false // required foreign-instantiation copies are compiler-private
	}
	if len(g.prog.Modules) == 0 {
		// Check([]*ast.File) has no module metadata; retain its entry file boundary.
		if len(g.prog.Files) > 1 && g.prog.Main != nil {
			return g.sources.sources[f.Decl] == g.sources.sources[g.prog.Main.Decl]
		}
		return true
	}
	return g.prog.Owners[f.Name] == g.root
}

func (g *mirrorGenerator) functionReason(f *types.FuncInfo) string {
	if f.Decl.Extern != nil {
		return "extern function"
	}
	if len(f.Scheme.TParams) != 0 {
		return "type parameters"
	}
	if len(f.Scheme.Constraints) != 0 {
		return "constraints"
	}
	for i, p := range f.Scheme.Type.Params {
		if why := g.mirrorReason(p, false, make(map[string]bool)); why != "" {
			if strings.HasPrefix(why, "foreign handle ") {
				return why
			}
			return fmt.Sprintf("parameter %d: %s", i+1, why)
		}
	}
	if why := g.mirrorReason(f.Scheme.Type.Ret, true, make(map[string]bool)); why != "" {
		if strings.HasPrefix(why, "foreign handle ") {
			return why
		}
		return "result: " + why
	}
	return ""
}

func mirrorHead(t types.Type) (*types.TCon, []types.Type) {
	t = types.Unalias(t)
	if app, ok := t.(*types.TApp); ok {
		head, _ := types.Unalias(app.Fun).(*types.TCon)
		return head, app.Args
	}
	head, _ := t.(*types.TCon)
	return head, nil
}

func mirrorScalar(t types.Type) string {
	head, args := mirrorHead(t)
	if head == nil || len(args) != 0 {
		return ""
	}
	switch head.Name {
	case "Int":
		return "int64"
	case "Float":
		return "float64"
	case "Bool":
		return "bool"
	case "String":
		return "string"
	}
	return ""
}

// Legacy inspection bindings have no Go identity metadata. Recognize their
// nullary Handle constructors only at foreign boundaries, not in ordinary enums.
func (g *mirrorGenerator) indexLegacyHandles() {
	seen := make(map[string]bool)
	var visit func(types.Type)
	visit = func(t types.Type) {
		t = types.Unalias(t)
		if f, ok := t.(*types.TFunc); ok {
			for _, p := range f.Params {
				visit(p)
			}
			visit(f.Ret)
			return
		}
		head, args := mirrorHead(t)
		for _, arg := range args {
			visit(arg)
		}
		if head == nil || seen[head.Name] {
			return
		}
		seen[head.Name] = true
		d := g.prog.DataTypes[head.Name]
		if d == nil {
			return
		}
		if len(d.TParams) == 0 && len(d.Ctors) == 1 {
			c := d.Ctors[0]
			if len(c.Fields) == 0 && c.FieldNames == nil && g.sourceName(c.Name) == g.sourceName(d.Name)+"Handle" {
				g.legacyHandles[d.Name] = true
			}
		}
		for _, c := range d.Ctors {
			for _, field := range c.Fields {
				visit(field)
			}
		}
	}
	for _, f := range g.prog.Funcs {
		if f.Decl.Extern != nil {
			visit(f.Scheme.Type)
		}
	}
	goOwners := make(map[string]bool)
	for _, module := range g.prog.Modules {
		if strings.HasPrefix(module.ImportPath, "go:") {
			goOwners[module.Path] = true
		}
	}
	for name := range g.prog.DataTypes {
		if goOwners[g.prog.Owners[name]] {
			visit(&types.TCon{Name: name})
		}
	}
}

func (g *mirrorGenerator) handle(t types.Type) (ast.ForeignHandle, bool) {
	head, args := mirrorHead(t)
	if head == nil || len(args) != 0 {
		return ast.ForeignHandle{}, false
	}
	handle, ok := g.prog.ForeignHandleKeys[head.Name]
	return handle, ok || g.legacyHandles[head.Name]
}

func (g *mirrorGenerator) qualifier(path string) string {
	if alias := g.imports[path]; alias != "" {
		return alias
	}
	var alias string
	switch path {
	case "context", "sort":
		alias = path
	case "github.com/reoring/go4t/rt":
		alias = "rt"
	default:
		for n := 0; ; n++ {
			alias = fmt.Sprintf("go%d", n)
			if g.names[alias] == 0 {
				break
			}
		}
		g.names[alias] = mirrorNameOther
	}
	g.imports[path] = alias
	return alias
}

func (g *mirrorGenerator) mirrorReason(t types.Type, result bool, visiting map[string]bool) string {
	if mirrorScalar(t) != "" {
		return ""
	}
	if handle, ok := g.handle(t); ok {
		if handle.PackagePath == "" || handle.TypeName == "" {
			head, _ := mirrorHead(t)
			return "foreign handle " + g.sourceName(head.Name) + " without Go identity"
		}
		return ""
	}
	head, args := mirrorHead(t)
	if head == nil {
		switch types.Unalias(t).(type) {
		case *types.TFunc:
			return "function type"
		case *types.TVar:
			return "type parameter"
		}
		return "unsupported type"
	}
	arity := -1
	switch head.Name {
	case "Unit":
		arity = 0
	case "List", "Option":
		arity = 1
	case "Result":
		arity = 2
	case "IO":
		if !result {
			return "IO is only supported as the outer result"
		}
		arity = 1
	case "Map", "Set":
		arity = 1
		if head.Name == "Map" {
			arity = 2
		}
		if len(args) != arity || mirrorScalar(args[0]) == "" {
			return head.Name + " requires a scalar key"
		}
	default:
		if n, ok := types.IsTuple(head); ok && n <= 8 {
			arity = n
		} else if d := g.prog.DataTypes[head.Name]; d != nil {
			if len(args) != len(d.TParams) {
				return "parameterised data type " + g.sourceName(d.Name)
			}
			for _, arg := range args {
				if why := g.mirrorReason(arg, false, visiting); why != "" {
					return why
				}
			}
			key := mirrorKey(t)
			if visiting[key] {
				return ""
			}
			visiting[key] = true
			defer delete(visiting, key)
			for _, c := range g.data(t).Ctors {
				for _, f := range c.Fields {
					if why := g.mirrorReason(f, false, visiting); why != "" {
						if strings.HasPrefix(why, "foreign handle ") {
							return why
						}
						return g.sourceName(d.Name) + ": " + why
					}
				}
			}
			return ""
		}
	}
	if arity < 0 || len(args) != arity {
		return "unsupported type " + g.sourceName(head.Name)
	}
	for _, arg := range args {
		if why := g.mirrorReason(arg, false, visiting); why != "" {
			return why
		}
	}
	return ""
}

// data returns a cached constructor view with the application's arguments
// substituted. Ctor.Data still names the shared erased constructor definition.
func (g *mirrorGenerator) data(t types.Type) *types.DataType {
	if _, ok := g.handle(t); ok {
		return nil
	}
	head, args := mirrorHead(t)
	if head == nil || mirrorScalar(t) != "" {
		return nil
	}
	switch head.Name {
	case "Unit", "List", "Option", "Result", "IO", "Map", "Set":
		return nil
	}
	if _, tuple := types.IsTuple(head); tuple {
		return nil
	}
	d := g.prog.DataTypes[head.Name]
	if d == nil || len(args) != len(d.TParams) {
		return nil
	}
	if len(args) == 0 {
		return d
	}
	key := mirrorKey(t)
	if instance := g.dataTypes[key]; instance != nil {
		return instance
	}
	sub := make(map[string]types.Type, len(args))
	for i, param := range d.TParams {
		sub[param.Name] = args[i]
	}
	instance := &types.DataType{Name: d.Name, Ctors: make([]*types.Ctor, len(d.Ctors))}
	for i, ctor := range d.Ctors {
		c := *ctor
		c.Fields = make([]types.Type, len(ctor.Fields))
		for j, field := range ctor.Fields {
			c.Fields[j] = substitute(field, sub)
		}
		instance.Ctors[i] = &c
	}
	g.dataTypes[key] = instance
	return instance
}

func (g *mirrorGenerator) dataName(name string) string {
	source := g.sourceName(name)
	if owner := g.prog.Owners[name]; len(g.prog.Modules) != 0 && owner != g.root {
		source = g.prefixes[owner] + source
	}
	return source
}

// Mirror names use the language's nominal spellings, including List and Int,
// rather than Go representation syntax such as [] and int64.
func (g *mirrorGenerator) mirrorName(t types.Type) string {
	head, args := mirrorHead(t)
	name := head.Name
	_, handle := g.handle(t)
	if handle || g.data(t) != nil {
		name = g.dataName(head.Name)
	}
	for _, arg := range args {
		name += g.mirrorName(arg)
	}
	return name
}

func (g *mirrorGenerator) goType(t types.Type) string {
	if name := mirrorScalar(t); name != "" {
		return name
	}
	if handle, ok := g.handle(t); ok {
		name := g.qualifier(handle.PackagePath) + "." + handle.TypeName
		if handle.Struct {
			return "*" + name
		}
		return name
	}
	head, args := mirrorHead(t)
	if g.data(t) != nil {
		return g.mirrorName(t)
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = g.goType(a)
	}
	switch head.Name {
	case "List":
		return "[]" + parts[0]
	case "Map":
		return "map[" + parts[0] + "]" + parts[1]
	case "Set":
		return "map[" + parts[0] + "]struct{}"
	case "IO":
		g.qualifier("context")
		return "func(context.Context) (" + parts[0] + ", error)"
	default:
		g.qualifier("github.com/reoring/go4t/rt")
		name := "rt." + head.Name
		if len(parts) != 0 {
			name += "[" + strings.Join(parts, ", ") + "]"
		}
		return name
	}
}

func (g *mirrorGenerator) converterID(t types.Type) string {
	if _, ok := g.handle(t); ok {
		return g.mirrorName(t)
	}
	if d := g.data(t); d != nil {
		return g.goType(t)
	}
	head, args := mirrorHead(t)
	parts := []string{head.Name}
	for _, a := range args {
		parts = append(parts, g.converterID(a))
	}
	return strings.Join(parts, "_")
}

func (g *mirrorGenerator) collect(t types.Type) {
	t = types.Unalias(t)
	key := mirrorKey(t)
	if g.entries[key] != nil {
		return
	}
	name := g.goType(t)
	if d := g.data(t); d != nil && len(g.prog.DataTypes[d.Name].TParams) != 0 {
		if loc, exists := g.declared[name]; exists {
			g.err = &types.Error{File: loc.file, Pos: loc.pos, Msg: "Go export name collision: " + name}
			return
		}
	}
	g.entries[key] = &mirrorType{typ: t, goType: name}
	_, args := mirrorHead(t)
	for _, a := range args {
		g.collect(a)
	}
	if d := g.data(t); d != nil {
		for _, c := range d.Ctors {
			for _, f := range c.Fields {
				g.collect(f)
			}
		}
	}
}

func mirrorKey(t types.Type) string {
	head, args := mirrorHead(t)
	key := head.Name
	for _, arg := range args {
		key += "[" + mirrorKey(arg) + "]"
	}
	return key
}

func (g *mirrorGenerator) reserve(name string, kind mirrorNameKind) error {
	if g.names[name] != 0 {
		return fmt.Errorf("Go export name collision: %s", name)
	}
	g.names[name] = kind
	return nil
}

func (g *mirrorGenerator) dataError(d *types.DataType, err error) error {
	loc := g.sources.typeSources[d.Name]
	return &types.Error{File: loc.file, Pos: loc.pos, Msg: err.Error()}
}

func (g *mirrorGenerator) functionError(f *types.FuncInfo, err error) error {
	return &types.Error{File: g.sources.sources[f.Decl], Pos: f.Decl.Pos, Msg: err.Error()}
}

func mirrorEnum(d *types.DataType) bool {
	for _, c := range d.Ctors {
		if len(c.Fields) != 0 || c.FieldNames != nil {
			return false
		}
	}
	return true
}

func mirrorField(c *types.Ctor, i int) string {
	if c.FieldNames != nil {
		return exported(c.FieldNames[i])
	}
	return fmt.Sprintf("F%d", i)
}

// Go value structs cannot contain themselves without an indirection. Sums,
// slices and maps provide that indirection; Option/Result/tuples do not.
func (g *mirrorGenerator) recursiveValue(t types.Type, visiting map[string]bool) bool {
	if d := g.data(t); d != nil {
		if len(d.Ctors) != 1 {
			return false
		}
		key := mirrorKey(t)
		if visiting[key] {
			return true
		}
		visiting[key] = true
		defer delete(visiting, key)
		for _, f := range d.Ctors[0].Fields {
			if g.recursiveValue(f, visiting) {
				return true
			}
		}
		return false
	}
	head, args := mirrorHead(t)
	if head.Name == "List" || head.Name == "Map" || head.Name == "Set" || head.Name == "IO" {
		return false
	}
	for _, a := range args {
		if g.recursiveValue(a, visiting) {
			return true
		}
	}
	return false
}

func (g *mirrorGenerator) sourceConstructor(c *types.Ctor) string {
	name := g.sourceName(c.Name)
	fields := make([]string, len(c.Fields))
	for i, field := range c.Fields {
		fields[i] = types.TypeStringFull(field)
		if c.FieldNames != nil {
			fields[i] = c.FieldNames[i] + ": " + fields[i]
		}
	}
	if c.FieldNames != nil {
		return name + " { " + strings.Join(fields, ", ") + " }"
	}
	if len(fields) != 0 {
		return name + "(" + strings.Join(fields, ", ") + ")"
	}
	return name
}

func (g *mirrorGenerator) sourceDeclaration(d *types.DataType) string {
	// Describe the original generic declaration, not its substituted Go mirror.
	d = g.prog.DataTypes[d.Name]
	name := g.sourceName(d.Name)
	if len(d.TParams) != 0 {
		params := make([]string, len(d.TParams))
		for i, param := range d.TParams {
			params[i] = param.Name
		}
		name += "[" + strings.Join(params, ", ") + "]"
	}
	ctors := make([]string, len(d.Ctors))
	for i, c := range d.Ctors {
		ctors[i] = g.sourceConstructor(c)
	}
	return "type " + name + " = " + strings.Join(ctors, " | ")
}

func mirrorLiteral(name string, c *types.Ctor) string {
	fields := make([]string, len(c.Fields))
	for i := range c.Fields {
		fields[i] = mirrorField(c, i) + ": ..."
	}
	return name + "{" + strings.Join(fields, ", ") + "}"
}

func (g *mirrorGenerator) emitStructDoc(out *strings.Builder, name string, c *types.Ctor) {
	fmt.Fprintf(out, "// Construct with %s; fields:", mirrorLiteral(name, c))
	if len(c.Fields) == 0 {
		out.WriteString(" none")
	}
	for i, field := range c.Fields {
		if i != 0 {
			out.WriteString(",")
		}
		fmt.Fprintf(out, " %s %s", mirrorField(c, i), g.goType(field))
	}
	out.WriteString(".\n")
}

func (g *mirrorGenerator) emitData(out *strings.Builder, m *mirrorType, d *types.DataType) error {
	if g.recursiveValue(m.typ, make(map[string]bool)) {
		return g.dataError(d, fmt.Errorf("recursive Go value type %s", m.goType))
	}
	if err := g.reserve(m.goType, mirrorNameData); err != nil {
		return g.dataError(d, err)
	}
	fmt.Fprintf(out, "// %s mirrors go4t `%s`.\n", m.goType, g.sourceDeclaration(d))
	if mirrorEnum(d) {
		names := make([]string, len(d.Ctors))
		for i, c := range d.Ctors {
			names[i] = m.goType + g.sourceName(c.Name)
		}
		fmt.Fprintf(out, "// All constructors are nullary, so values are the constants %s (compare with == or switch on the value).\n", strings.Join(names, ", "))
		fmt.Fprintf(out, "type %s int\nconst (\n", m.goType)
		for i, c := range d.Ctors {
			name := m.goType + g.sourceName(c.Name)
			if err := g.reserve(name, mirrorNameData); err != nil {
				return g.dataError(d, err)
			}
			if i == 0 {
				fmt.Fprintf(out, "%s %s = iota\n", name, m.goType)
			} else {
				fmt.Fprintln(out, name)
			}
		}
		fmt.Fprintf(out, ")\nfunc (v %s) String() string { switch v {\n", m.goType)
		for _, c := range d.Ctors {
			fmt.Fprintf(out, "case %s%s: return %q\n", m.goType, g.sourceName(c.Name), g.sourceName(c.Name))
		}
		out.WriteString("default: panic(\"invalid constructor\")\n} }\n")
		return nil
	}
	var err error
	m.debugDict, err = g.debugDictionary(m.typ)
	if err != nil {
		return g.dataError(d, err)
	}
	if len(d.Ctors) > 1 {
		literals := make([]string, len(d.Ctors))
		for i, c := range d.Ctors {
			literals[i] = mirrorLiteral(m.goType+g.sourceName(c.Name), c)
		}
		fmt.Fprintf(out, "// Construct with %s; switch on the concrete struct types.\n", strings.Join(literals, ", "))
		fmt.Fprintf(out, "type %s interface { is%s() }\n", m.goType, m.goType)
	}
	for _, c := range d.Ctors {
		name := m.goType
		if len(d.Ctors) > 1 {
			name += g.sourceName(c.Name)
			if err := g.reserve(name, mirrorNameData); err != nil {
				return g.dataError(d, err)
			}
			fmt.Fprintf(out, "// %s mirrors go4t constructor `%s` of %s.\n", name, g.sourceConstructor(c), g.sourceName(d.Name))
		}
		g.emitStructDoc(out, name, c)
		fmt.Fprintf(out, "type %s struct {\n", name)
		fields := make(map[string]bool)
		for i, f := range c.Fields {
			field := mirrorField(c, i)
			if fields[field] {
				return g.dataError(d, fmt.Errorf("duplicate exported field %s in %s", field, g.sourceName(d.Name)))
			}
			if field == "String" && m.debugDict != "" {
				return g.dataError(d, fmt.Errorf("exported field String conflicts with String method in %s", g.sourceName(d.Name)))
			}
			fields[field] = true
			fmt.Fprintf(out, "%s %s\n", field, g.goType(f))
		}
		out.WriteString("}\n")
		if len(d.Ctors) > 1 {
			fmt.Fprintf(out, "func (%s) is%s() {}\n", name, m.goType)
		}
	}
	return nil
}

// Resolve only concrete instance heads, following the checker's constructor-keyed
// matching and declaration-ordered context. Missing context means no String method.
func (g *mirrorGenerator) concreteDictionary(class string, t types.Type) (types.DictRef, bool) {
	head, args := mirrorHead(t)
	if head == nil {
		return types.DictRef{}, false
	}
	instance := g.sources.instances[class+"_"+head.Name]
	if instance == nil || len(instance.HeadArgs) != len(args) {
		return types.DictRef{}, false
	}
	sub := make(map[string]types.Type, len(args))
	for i, headArg := range instance.HeadArgs {
		if param, ok := headArg.(*types.TVar); ok {
			sub[param.Name] = args[i]
		} else if mirrorKey(headArg) != mirrorKey(args[i]) {
			return types.DictRef{}, false
		}
	}
	ref := types.DictRef{Kind: types.DictInstance, Class: class, Instance: instance}
	if len(instance.Context) != 0 {
		ref.Args = make([]types.DictRef, len(instance.Context))
		for i, constraint := range instance.Context {
			arg, ok := g.concreteDictionary(constraint.Class, substitute(constraint.Arg, sub))
			if !ok {
				return types.DictRef{}, false
			}
			ref.Args[i] = arg
		}
	}
	return ref, true
}

func (g *mirrorGenerator) debugDictionary(t types.Type) (string, error) {
	ref, ok := g.concreteDictionary("Debug", t)
	if !ok {
		return "", nil
	}
	// Reuse the normal/specialised-call emitter for dictionary fields and embedded
	// superclasses, then apply the same module identifier renaming as their callers.
	expr, err := parser.ParseExpr(g.sources.dict(&ref))
	if err != nil {
		return "", err
	}
	goast.Inspect(expr, func(n goast.Node) bool {
		if id, ok := n.(*goast.Ident); ok {
			id.Name = g.internal(id.Name)
		}
		return true
	})
	var out strings.Builder
	if err := format.Node(&out, token.NewFileSet(), expr); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (g *mirrorGenerator) emitString(out *strings.Builder, m *mirrorType) {
	if m.debugDict == "" {
		return
	}
	d := g.data(m.typ)
	// The module pass renames method identifiers as well as dictionary types.
	method := g.internal("Debug")
	for _, c := range d.Ctors {
		name := m.goType
		if len(d.Ctors) > 1 {
			name += g.sourceName(c.Name)
		}
		fmt.Fprintf(out, "func (v %s) String() string { return (%s).%s(%s).(string) }\n", name, m.debugDict, method, g.from(m.typ, "v"))
	}
}

func (g *mirrorGenerator) to(t types.Type, value string) string {
	return "toGo_" + g.entries[mirrorKey(t)].id + "(" + value + ")"
}

func (g *mirrorGenerator) from(t types.Type, value string) string {
	return "fromGo_" + g.entries[mirrorKey(t)].id + "(" + value + ")"
}

func (g *mirrorGenerator) builtinCtor(data, ctor string) string {
	return g.internal(data + "_" + ctor)
}

func (g *mirrorGenerator) emitConverters(out *strings.Builder, m *mirrorType) {
	for _, to := range []bool{true, false} {
		if to {
			fmt.Fprintf(out, "func toGo_%s(v any) %s {\n", m.id, m.goType)
		} else {
			fmt.Fprintf(out, "func fromGo_%s(v %s) any {\n", m.id, m.goType)
		}
		g.emitConversion(out, m, to)
		out.WriteString("}\n")
	}
}

func (g *mirrorGenerator) emitConversion(out *strings.Builder, m *mirrorType, to bool) {
	if handle, ok := g.handle(m.typ); ok {
		if !to {
			out.WriteString("return v\n")
		} else if handle.Struct {
			fmt.Fprintf(out, "if p, ok := v.(%s); ok { return p }; x := v.(%s); return &x\n", m.goType, strings.TrimPrefix(m.goType, "*"))
		} else {
			fmt.Fprintf(out, "return v.(%s)\n", m.goType)
		}
		return
	}
	if mirrorScalar(m.typ) != "" || m.goType == "rt.Unit" {
		if to {
			fmt.Fprintf(out, "return v.(%s)\n", m.goType)
		} else {
			out.WriteString("return v\n")
		}
		return
	}
	if d := g.data(m.typ); d != nil {
		g.emitDataConversion(out, m, d, to)
		return
	}
	head, args := mirrorHead(m.typ)
	switch head.Name {
	case "List":
		cons, nilCtor := g.builtinCtor("List", "Cons"), g.builtinCtor("List", "Nil")
		if to {
			fmt.Fprintf(out, "var result %s\nfor { switch x := v.(type) {\ncase *%s: result = append(result, %s); v = x.F1\ncase *%s: return result\ndefault: panic(\"invalid constructor\")\n} }\n", m.goType, cons, g.to(args[0], "x.F0"), nilCtor)
		} else {
			fmt.Fprintf(out, "var result any = &%s{}\nfor i := len(v)-1; i >= 0; i-- { result = &%s{F0: %s, F1: result} }\nreturn result\n", nilCtor, cons, g.from(args[0], "v[i]"))
		}
	case "Option", "Result":
		first, second, fieldA, fieldB, flag := "None", "Some", "", "Value", "Present"
		if head.Name == "Result" {
			first, second, fieldA, fieldB, flag = "Err", "Ok", "Err", "Ok", "IsOk"
		}
		a, b := g.builtinCtor(head.Name, first), g.builtinCtor(head.Name, second)
		if to {
			// Bind x only when at least one arm reads its payload (always true).
			fmt.Fprintf(out, "switch x := v.(type) {\ncase *%s: return %s{", a, m.goType)
			if fieldA != "" {
				fmt.Fprintf(out, "%s: %s", fieldA, g.to(args[0], "x.F0"))
			}
			fmt.Fprintf(out, "}\ncase *%s: return %s{%s: true, %s: %s}\ndefault: panic(\"invalid constructor\")\n}\n", b, m.goType, flag, fieldB, g.to(args[len(args)-1], "x.F0"))
		} else {
			fmt.Fprintf(out, "if v.%s { return &%s{F0: %s} }\nreturn &%s{", flag, b, g.from(args[len(args)-1], "v."+fieldB), a)
			if fieldA != "" {
				fmt.Fprintf(out, "F0: %s", g.from(args[0], "v."+fieldA))
			}
			out.WriteString("}\n")
		}
	case "IO":
		g.qualifier("github.com/reoring/go4t/rt")
		g.qualifier("context")
		if to {
			fmt.Fprintf(out, "action := v.(rt.IO)\nreturn func(ctx context.Context) (%s, error) { value, err := action(ctx); if err != nil { var zero %s; return zero, rt.ClassifyError(err) }; return %s, nil }\n", g.goType(args[0]), g.goType(args[0]), g.to(args[0], "value"))
		} else {
			fmt.Fprintf(out, "return rt.IO(func(ctx context.Context) (any, error) { value, err := v(ctx); if err != nil { return nil, rt.ClassifyError(err) }; return %s, nil })\n", g.from(args[0], "value"))
		}
	case "Map", "Set":
		g.emitCollectionConversion(out, m, head.Name, args, to)
	default: // Tuple2 ... Tuple8, admitted by mirrorReason.
		name, conv := head.Name, g.from
		if to {
			fmt.Fprintf(out, "x := v.(*%s)\n", name)
			name, conv = m.goType, g.to
		} else {
			out.WriteString("x := v\n")
			name = "&" + name
		}
		fmt.Fprintf(out, "return %s{", name)
		for i, arg := range args {
			fmt.Fprintf(out, "F%d: %s,", i, conv(arg, fmt.Sprintf("x.F%d", i)))
		}
		out.WriteString("}\n")
	}
}

func (g *mirrorGenerator) emitDataConversion(out *strings.Builder, m *mirrorType, d *types.DataType, to bool) {
	enum, single := mirrorEnum(d), len(d.Ctors) == 1
	payload := false
	for _, c := range d.Ctors {
		payload = payload || len(c.Fields) != 0
	}
	if to {
		if !payload {
			out.WriteString("switch v.(type) {\n")
		} else {
			out.WriteString("switch x := v.(type) {\n")
		}
	} else if enum {
		out.WriteString("switch v {\n")
	} else if !single {
		if payload {
			out.WriteString("switch x := v.(type) {\n")
		} else {
			out.WriteString("switch v.(type) {\n")
		}
	} else if payload {
		out.WriteString("x := v\n")
	}
	for _, c := range d.Ctors {
		natural := m.goType
		if enum || !single {
			natural += g.sourceName(c.Name)
		}
		erased := g.internal(ctorName(c))
		if to {
			fmt.Fprintf(out, "case *%s: return %s", erased, natural)
		} else {
			if enum || !single {
				fmt.Fprintf(out, "case %s: ", natural)
			}
			fmt.Fprintf(out, "return &%s", erased)
		}
		if !to || !enum {
			out.WriteString("{")
			for i, field := range c.Fields {
				if to {
					fmt.Fprintf(out, "%s: %s,", mirrorField(c, i), g.to(field, fmt.Sprintf("x.F%d", i)))
				} else {
					value := g.from(field, "x."+mirrorField(c, i))
					if scalarRecord(c) {
						value = convert(value, "any", scalar(field))
					}
					fmt.Fprintf(out, "F%d: %s,", i, value)
				}
			}
			out.WriteString("}")
		}
		out.WriteByte('\n')
	}
	if to || enum || !single {
		out.WriteString("default: panic(\"invalid constructor\")\n}\n")
	}
}

func mirrorLess(t types.Type, a, b string) string {
	switch mirrorScalar(t) {
	case "bool":
		return "(!" + a + " && " + b + ")"
	case "float64":
		return "(" + a + " != " + a + " && " + b + " == " + b + ") || " + a + " < " + b
	default:
		return a + " < " + b
	}
}

func (g *mirrorGenerator) emitCollectionConversion(out *strings.Builder, m *mirrorType, kind string, args []types.Type, to bool) {
	g.qualifier("github.com/reoring/go4t/rt")
	if to {
		fmt.Fprintf(out, "result := make(%s, rt.%sSize(v).(int64))\n", m.goType, kind)
		if kind == "Map" {
			fmt.Fprintf(out, "rt.MapFold(v, nil, func(_ any, key any, value any) any { result[%s] = %s; return nil })\n", g.to(args[0], "key"), g.to(args[1], "value"))
		} else {
			fmt.Fprintf(out, "for xs := rt.SetToList(v); ; { key, rest, ok := rt.Shapes.Uncons(xs); if !ok { break }; result[%s] = struct{}{}; xs = rest }\n", g.to(args[0], "key"))
		}
		out.WriteString("return result\n")
		return
	}
	g.qualifier("sort")
	if kind == "Map" {
		// Store values with their keys: indexing a Go map by a NaN key loses its value.
		fmt.Fprintf(out, "type entry struct { key %s; value %s }\nentries := make([]entry, 0, len(v))\nfor key, value := range v { entries = append(entries, entry{key: key, value: value}) }\n", g.goType(args[0]), g.goType(args[1]))
	} else {
		fmt.Fprintf(out, "type entry struct { key %s }\nentries := make([]entry, 0, len(v))\nfor key := range v { entries = append(entries, entry{key: key}) }\n", g.goType(args[0]))
	}
	fmt.Fprintf(out, "sort.Slice(entries, func(i, j int) bool { return %s })\n", mirrorLess(args[0], "entries[i].key", "entries[j].key"))
	if mirrorScalar(args[0]) == "float64" {
		fmt.Fprintf(out, "if len(entries) > 1 && entries[0].key != entries[0].key && entries[1].key != entries[1].key { panic(%q) }\n", "go4t: duplicate NaN "+strings.ToLower(kind)+" key")
	}
	fmt.Fprintf(out, "compare := func(a, b any) any { x, y := a.(%s), b.(%s); if %s { return &%s{} }; if %s { return &%s{} }; return &%s{} }\n", g.goType(args[0]), g.goType(args[0]), mirrorLess(args[0], "x", "y"), g.builtinCtor("Ordering", "LT"), mirrorLess(args[0], "y", "x"), g.builtinCtor("Ordering", "GT"), g.builtinCtor("Ordering", "EQ"))
	fmt.Fprintf(out, "result := rt.Empty%s()\nfor _, entry := range entries { result = rt.%sInsert(result, %s", kind, kind, g.from(args[0], "entry.key"))
	if kind == "Map" {
		fmt.Fprintf(out, ", %s", g.from(args[1], "entry.value"))
	}
	out.WriteString(", compare) }\nreturn result\n")
}

func (g *mirrorGenerator) emitWrapper(out *strings.Builder, f *mirrorFunction) {
	params, args := make([]string, len(f.Scheme.Type.Params)), make([]string, len(f.Scheme.Type.Params))
	for i, t := range f.Scheme.Type.Params {
		name := "p" + strconv.Itoa(i)
		params[i] = name + " " + g.goType(t)
		args[i] = name
		if scalar(t) == "any" {
			args[i] = g.from(t, name)
		}
	}
	call := g.internal(functionName(f.Name)) + "(" + strings.Join(args, ", ") + ")"
	if scalar(f.Scheme.Type.Ret) == "any" {
		call = g.to(f.Scheme.Type.Ret, call)
	}
	if f.renamedFrom != "" {
		fmt.Fprintf(out, "// %s exports go4t function %s.\n// (renamed from %s: collides with type %s)\n", f.goName, g.sourceName(f.Name), f.renamedFrom, f.renamedFrom)
	}
	fmt.Fprintf(out, "func %s(%s) %s { return %s }\n", f.goName, strings.Join(params, ", "), g.goType(f.Scheme.Type.Ret), call)
}
