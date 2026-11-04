package laws

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

// AddSampleTests adds ordinary checked functions to the unmodified source
// modules. The caller rebuilds its test harness and type-checks once more, so
// generated equality/debug calls use the same dictionaries as user code.
func AddSampleTests(modules []*ast.Module, p *types.Program) error {
	entry := modules[len(modules)-1]
	names := make(map[string]bool)
	for _, module := range modules {
		for _, file := range module.Files {
			for _, imp := range file.Imports {
				alias := imp.Alias
				if alias == "" {
					alias = path.Base(imp.Path)
				}
				names[alias] = true
			}
			for _, decl := range file.Decls {
				ast.Walk(decl, func(node ast.Node) {
					switch n := node.(type) {
					case *ast.Ident:
						names[n.Name] = true
					case *ast.FuncDecl:
						names[n.Name] = true
					case *ast.ClassDecl:
						for _, method := range n.Methods {
							names[method.Name] = true
						}
					case *ast.LawDecl:
						for _, param := range n.Params {
							names[param.Name] = true
						}
					}
				})
			}
		}
	}
	fresh := func(base string) string {
		for {
			collision := false
			for name := range names {
				collision = collision || strings.HasPrefix(name, base)
			}
			if !collision {
				names[base] = true
				return base
			}
			base += "_"
		}
	}
	ordinal := 0
	for _, module := range modules {
		// Appending generated files must not revisit them.
		for _, file := range append([]*ast.File(nil), module.Files...) {
			for _, decl := range file.Decls {
				law, ok := decl.(*ast.LawDecl)
				if !ok {
					continue
				}
				var info *types.LawInfo
				for name, candidate := range p.Laws {
					if p.Owners[name] == module.Path && sourceName(p, name) == law.Name {
						info = candidate
						break
					}
				}
				if info == nil {
					continue
				}
				sample, ok := info.Proof.Decl.Body.(*ast.TSample)
				if !ok {
					continue
				}
				if err := checkSample(p, info); err != nil {
					return &types.Error{File: file.Name, Pos: law.Pos, End: law.End, Msg: "law " + law.Name + ": " + err.Error()}
				}
				ordinal++
				prefix := fresh(fmt.Sprintf("_go4tSample%d_", ordinal))
				testName := fresh("testSample_" + law.Name)
				g := sampleGenerator{p: p, module: module, prefix: prefix, refs: make(map[string]string), generators: make(map[string]string)}
				generated, err := g.file(file, law, info, sample.Count, testName)
				if err != nil {
					return err
				}
				module.Files = append(module.Files, generated)
				if module != entry {
					wrapperName := fresh(testName + "_imported")
					wrapper, err := parser.Parse("package " + entry.Name + "\nfunc " + wrapperName + "() IO[Bool] { sampleModule." + testName + "() }\n")
					if err != nil {
						return err
					}
					wrapper.Name = "<sample " + law.Name + ">"
					wrapper.Imports = []ast.Import{{Alias: "sampleModule", Path: module.Path, Resolved: module.Path}}
					entry.Files = append(entry.Files, wrapper)
				}
			}
		}
	}
	return nil
}

type sampleGenerator struct {
	p          *types.Program
	module     *ast.Module
	prefix     string
	refs       map[string]string
	imports    []ast.Import
	generators map[string]string
	observers  map[string]string
	functions  strings.Builder
}

func (g *sampleGenerator) reference(name string) string {
	owner := g.p.Owners[name]
	if owner == "" || owner == g.module.Path {
		return sourceName(g.p, name)
	}
	alias := g.refs[owner]
	if alias == "" {
		alias = fmt.Sprintf("%smodule%d", g.prefix, len(g.refs))
		g.refs[owner] = alias
		g.imports = append(g.imports, ast.Import{Alias: alias, Path: owner, Resolved: owner})
	}
	return alias + "." + sourceName(g.p, name)
}

func (g *sampleGenerator) typeSource(typ types.Type) string {
	con, args := typeHead(typ)
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = g.typeSource(arg)
	}
	if _, tuple := types.IsTuple(con); tuple {
		return "(" + strings.Join(parts, ", ") + ")"
	}
	name := g.reference(con.Name)
	if len(parts) != 0 {
		name += "[" + strings.Join(parts, ", ") + "]"
	}
	return name
}

func (g *sampleGenerator) generator(typ types.Type, depth int) string {
	key := fmt.Sprintf("%s/%d", types.TypeString(typ), depth)
	if name := g.generators[key]; name != "" {
		return name
	}
	name := fmt.Sprintf("%sgen%d", g.prefix, len(g.generators))
	g.generators[key] = name
	con, args := typeHead(typ)
	call := func(t types.Type, d, offset int) string {
		return fmt.Sprintf("%s(seed, sample, offset * 131 + %d)", g.generator(t, d), offset+1)
	}
	var body string
	switch con.Name {
	case "Int", "Float", "String":
		body = g.prefix + con.Name + "(seed, sample, offset)"
	case "Bool":
		body = g.prefix + "choice(seed, sample, offset, 2) == 0"
	case "Unit":
		body = g.reference("unit") + "()"
	case "Map":
		mapFromList := g.reference(sampleMapFromList(g.p))
		var arms []string
		for n := range 4 {
			pairs := make([]string, n)
			for i := range pairs {
				pairs[i] = "(" + call(args[0], depth, 2*i) + ", " + call(args[1], depth, 2*i+1) + ")"
			}
			pattern := strconv.Itoa(n)
			if n == 3 {
				pattern = "_"
			}
			arms = append(arms, pattern+" => "+mapFromList+"(["+strings.Join(pairs, ", ")+"])")
		}
		body = "match " + g.prefix + "choice(seed, sample, offset, 4) { " + strings.Join(arms, "; ") + " }"
	default:
		if _, tuple := types.IsTuple(con); tuple {
			fields := make([]string, len(args))
			for i, arg := range args {
				fields[i] = call(arg, depth, i)
			}
			body = "(" + strings.Join(fields, ", ") + ")"
		} else {
			data := g.p.DataTypes[con.Name]
			var choices []string
			for _, ctor := range data.Ctors {
				fields := sampleFields(data, args, ctor)
				possible := true
				for _, field := range fields {
					possible = possible && sampleInhabited(g.p, field, depth-1)
				}
				if !possible {
					continue
				}
				values := make([]string, len(fields))
				for i, field := range fields {
					values[i] = call(field, depth-1, i)
					if ctor.FieldNames != nil {
						values[i] = ctor.FieldNames[i] + ": " + values[i]
					}
				}
				value := g.reference(ctor.Name)
				if ctor.FieldNames != nil {
					value += "{" + strings.Join(values, ", ") + "}"
				} else if len(values) != 0 {
					value += "(" + strings.Join(values, ", ") + ")"
				}
				choices = append(choices, value)
			}
			var arms []string
			for i, choice := range choices {
				pattern := strconv.Itoa(i)
				if i == len(choices)-1 {
					pattern = "_"
				}
				arms = append(arms, pattern+" => "+choice)
			}
			body = fmt.Sprintf("match %schoice(seed, sample, offset, %d) { %s }", g.prefix, len(choices), strings.Join(arms, "; "))
		}
	}
	fmt.Fprintf(&g.functions, "func %s(seed Int, sample Int, offset Int) %s { %s }\n", name, g.typeSource(typ), body)
	return name
}

func (g *sampleGenerator) file(original *ast.File, law *ast.LawDecl, info *types.LawInfo, count int64, testName string) (*ast.File, error) {
	var source, body strings.Builder
	fmt.Fprintf(&source, "package %s\n", g.module.Name)
	for _, scalar := range []string{"Int", "Float", "String"} {
		fmt.Fprintf(&source, "extern func %s%s(seed Int, sample Int, offset Int) %s = %q.Sample%s\n", g.prefix, scalar, scalar, "github.com/reoring/go4t/rt/testrt", scalar)
	}
	fmt.Fprintf(&source, "extern func %schoice(seed Int, sample Int, offset Int, count Int) Int = %q.SampleChoice\n", g.prefix, "github.com/reoring/go4t/rt/testrt")
	fmt.Fprintf(&source, "extern func %sfloatEqual(a Float, b Float) Bool = %q.SampleFloatEqual\n", g.prefix, "github.com/reoring/go4t/rt/testrt")
	fmt.Fprintf(&source, "extern func %srun(count Int, check func(Int, Int) String) IO[Bool] = %q.Sample\n", g.prefix, "github.com/reoring/go4t/rt/testrt")
	fmt.Fprintf(&source, "extern func %simpossible() String = %q.SampleImpossible\n", g.prefix, "github.com/reoring/go4t/rt/testrt")
	equal := g.reference("eq")
	if con, _ := typeHead(types.AnnOf(info.Decl.LHS).Type); con != nil && con.Name == "Float" {
		equal = g.prefix + "floatEqual"
	}
	fmt.Fprintf(&source, "func %ssides() Bool { %s(unit(), unit()) }\n", g.prefix, equal)
	bindings := make([]string, len(law.Params))
	displays := []string{strconv.Quote(law.Name + ": ")}
	for i, param := range law.Params {
		bindings[i] = fmt.Sprintf("%sarg%d", g.prefix, i)
		fmt.Fprintf(&body, "let %s = %s(seed, sample, %d)\n", bindings[i], g.generator(info.ParamTypes[i], sampleDepth), i)
		separator := ""
		if i != 0 {
			separator = ", "
		}
		displays = append(displays, strconv.Quote(separator+param.Name+" = "), g.observer(info.ParamTypes[i], sampleDepth)+"("+bindings[i]+")")
	}
	if len(bindings) == 0 {
		displays = append(displays, strconv.Quote("(no bindings)"))
	}
	fmt.Fprintf(&body, "if %ssides(%s) { \"\" } else { %s }", g.prefix, strings.Join(bindings, ", "), strings.Join(displays, " + "))
	source.WriteString(g.functions.String())
	fmt.Fprintf(&source, "func %scheck(seed Int, sample Int) String { %s }\n", g.prefix, body.String())
	fmt.Fprintf(&source, "func %s() IO[Bool] { %srun(%d, %scheck) }\n", testName, g.prefix, count, g.prefix)
	file, err := parser.Parse(source.String())
	if err != nil {
		return nil, fmt.Errorf("sample %s generation: %w", law.Name, err)
	}
	file.Name = "<sample " + law.Name + ">"
	file.Imports = append(append([]ast.Import(nil), original.Imports...), g.imports...)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name == g.prefix+"sides" {
			fn.Params = law.Params
			fn.Body.Result.(*ast.Call).Args = []ast.Expr{law.LHS, law.RHS}
		}
	}
	return file, nil
}
