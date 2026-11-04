package laws

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/std"
	"github.com/reoring/go4t/types"
)

type reachEntry struct {
	module, name, reason, category string
	node                           ast.Node
	deps                           []*reachEntry
}

// TestTaintReach is a static, whole-body taint inventory, not a termination
// checker or a claim that every eligible function's laws close with Refl.
func TestTaintReach(t *testing.T) {
	t.Setenv("GO4T_PATH", "")
	moduleSet := map[string]bool{}
	if err := fs.WalkDir(std.FS, ".", func(name string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(name, ".gt") {
			moduleSet["std/"+path.Dir(name)] = true
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range moduleSet {
		names = append(names, name)
	}
	sort.Strings(names)
	dir := t.TempDir()
	var source strings.Builder
	source.WriteString("package inventory\n")
	for _, name := range names {
		fmt.Fprintf(&source, "import %q\n", name)
	}
	source.WriteString("func main() IO[Unit] { ioPure(unit()) }\n")
	if err := os.WriteFile(filepath.Join(dir, "main.gt"), []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	mods, err := modules.Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check(mods)
	if err != nil {
		t.Fatal(err)
	}
	var entries []*reachEntry
	funcs := map[string]*reachEntry{}
	methods := map[*types.Instance]map[string]*reachEntry{}
	owners := map[ast.Node]string{}
	for _, mod := range p.Modules {
		if !moduleSet[mod.ImportPath] {
			continue
		}
		for _, file := range mod.Files {
			for _, decl := range file.Decls {
				owners[decl] = mod.ImportPath
			}
		}
	}
	for name, f := range p.Funcs {
		if module := owners[f.Decl]; module != "" {
			e := &reachEntry{module: module, name: p.ModuleNames[name], node: f.Decl}
			if e.name == "" {
				e.name = name
			}
			entries = append(entries, e)
			funcs[name] = e
		}
	}
	for _, inst := range p.Instances {
		module := owners[inst.Decl]
		if module == "" {
			continue
		}
		methods[inst] = map[string]*reachEntry{}
		for i := range inst.Decl.Methods {
			m := &inst.Decl.Methods[i]
			class, con := p.ModuleNames[inst.Class.Name], p.ModuleNames[inst.TypeCon.Name]
			if class == "" {
				class = inst.Class.Name
			}
			if con == "" {
				con = inst.TypeCon.Name
			}
			e := &reachEntry{module: module, name: class + "[" + con + "]." + m.Name, node: m}
			entries = append(entries, e)
			methods[inst][m.Name] = e
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].module != entries[j].module {
			return entries[i].module < entries[j].module
		}
		return entries[i].name < entries[j].name
	})
	for _, e := range entries {
		set := func(category, reason string) {
			if e.reason == "" {
				e.category, e.reason = category, reason
			}
		}
		if f, ok := e.node.(*ast.FuncDecl); ok {
			if reachTypeContains(p, p.Funcs[f.Name].Scheme.Type.Ret, types.IO) {
				set("IO result", "IO result type")
			}
			if reachTypeContains(p, p.Funcs[f.Name].Scheme.Type, types.Float) {
				set("Float", "Float in signature")
			}
			if f.Extern != nil {
				switch {
				case e.module == "std/map" && f.Extern.ImportPath == "github.com/reoring/go4t/rt" && modeledMap(f.Extern.Name):
					e.category = "modeled"
				case (e.module == "std/prelude" || e.module == "std/string") && primitive(f.Extern):
					e.category = "constant-folding"
				default:
					set("extern", "unlisted extern "+f.Extern.ImportPath+"."+f.Extern.Name)
				}
			}
		}
		var dictDeps func(types.DictRef, string)
		dictDeps = func(ref types.DictRef, method string) {
			if ref.Kind == types.DictSuper && ref.Super != nil {
				// The superclass implementation is selected by type constructor.
				base := ref.Super
				for base.Kind == types.DictSuper && base.Super != nil {
					base = base.Super
				}
				if base.Instance != nil {
					for inst := range methods {
						if inst.TypeCon == base.Instance.TypeCon && inst.Class.Name == ref.Class {
							dictDeps(types.DictRef{Instance: inst, Args: base.Args}, method)
						}
					}
				}
				return
			}
			if ref.Instance != nil {
				for name, dep := range methods[ref.Instance] {
					if method == "" || name == method {
						e.deps = append(e.deps, dep)
					}
				}
			}
			for _, arg := range ref.Args {
				dictDeps(arg, "")
			}
		}
		ast.Walk(e.node, func(node ast.Node) {
			x, ok := node.(ast.Expr)
			if !ok {
				return
			}
			a := types.AnnOf(x)
			if reachTypeContains(p, a.Type, types.IO) {
				set("IO result", "IO result in body")
			}
			if reachTypeContains(p, a.Type, types.Float) {
				set("Float", "Float use in body")
			}
			method := ""
			if id, ok := x.(*ast.Ident); ok {
				if a.Ref == types.RefFunc && funcs[id.Name] != nil {
					e.deps = append(e.deps, funcs[id.Name])
				}
				if a.Ref == types.RefMethod {
					method = id.Name
				}
			}
			if b, ok := x.(*ast.Binary); ok {
				switch b.Op {
				case "==", "!=":
					method = "eq"
				case "<", "<=", ">", ">=":
					method = "compare"
				}
				if (b.Op == "/" || b.Op == "%") && !nonzeroLiteral(b.R) {
					set("division/modulo", "division/modulo with nonconstant or zero divisor")
				}
			}
			for _, ref := range a.Dicts {
				dictDeps(ref, method)
			}
			for _, ref := range a.ShowDicts {
				dictDeps(ref, "show")
			}
			if a.Monad != nil {
				dictDeps(*a.Monad, "")
			}
		})
		sort.Slice(e.deps, func(i, j int) bool { return e.deps[i].module+"."+e.deps[i].name < e.deps[j].module+"."+e.deps[j].name })
	}
	// Synchronous propagation makes the chosen reason independent of map order:
	// shortest path to a direct taint, then lexical callee on equal-length paths.
	for {
		type pending struct{ e, dep *reachEntry }
		var next []pending
		for _, e := range entries {
			if e.reason == "" {
				for _, dep := range e.deps {
					if dep.reason != "" {
						next = append(next, pending{e, dep})
						break
					}
				}
			}
		}
		if len(next) == 0 {
			break
		}
		for _, x := range next {
			x.e.category = "transitive"
			x.e.reason = "depends on " + x.dep.module + "." + x.dep.name + " (" + x.dep.reason + ")"
		}
	}
	modeled := 0
	for _, entry := range entries {
		if entry.category == "modeled" {
			modeled++
		}
		if entry.module == "std/map" && entry.reason != "" {
			t.Errorf("std/map.%s is tainted: %s", entry.name, entry.reason)
		}
	}
	if modeled != 13 {
		t.Errorf("modeled extern count = %d, want 13", modeled)
	}
	document := reachMarkdown(entries, names)
	filename := filepath.Join("..", "docs", "adr", "0001-laws-std-reach.md")
	if os.Getenv("GO4T_UPDATE_TAINT_REACH") == "1" {
		if err := os.WriteFile(filename, []byte(document), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != document {
		t.Fatal("std reach inventory changed; review and regenerate with GO4T_UPDATE_TAINT_REACH=1 go test ./laws/ -run TaintReach")
	}
	t.Logf("classified %d declarations across %d modules", len(entries), len(names))
}

func reachTypeContains(p *types.Program, t types.Type, con *types.TCon) bool {
	seen := map[string]bool{}
	var contains func(types.Type) bool
	contains = func(t types.Type) bool {
		switch t := types.Unalias(t).(type) {
		case *types.TCon:
			if t == con {
				return true
			}
			if data := p.DataTypes[t.Name]; con != types.Float && data != nil && !seen[t.Name] {
				seen[t.Name] = true
				for _, ctor := range data.Ctors {
					for _, field := range ctor.Fields {
						if contains(field) {
							return true
						}
					}
				}
			}
		case *types.TApp:
			if contains(t.Fun) {
				return true
			}
			for _, arg := range t.Args {
				if contains(arg) {
					return true
				}
			}
		case *types.TFunc:
			if contains(t.Ret) {
				return true
			}
			for _, arg := range t.Params {
				if contains(arg) {
					return true
				}
			}
		}
		return false
	}
	return contains(t)
}

func reachMarkdown(entries []*reachEntry, modules []string) string {
	var out strings.Builder
	out.WriteString(`# Standard-library law taint reach

Generated by ` + "`GO4T_UPDATE_TAINT_REACH=1 go test ./laws/ -run TaintReach`" + `; without the variable the test asserts byte equality.

## Scope and interpretation

Every source top-level function (including externs) and checked instance method is listed; derived methods are included after types.Check expansion. Class signatures and constructors are not functions. **law-eligible means untainted by this inventory**, not proved terminating or provable with the current tactics. All syntactic branches and local function/lambda bodies are inspected, even unreachable ones. Nonzero integer literals and their unary negatives are the only safe divisors. IO inspection includes nested algebraic fields; Float inspection checks expressions and exposed type arguments, but not constructor fields (ADR 0004).

The constant-folding whitelist is the ten std/prelude externs plus all 32 std/string externs accepted by laws/primitives.go and laws/strings.go; a similarly named user extern is not trusted. String externs fold only when every argument is constant, using the real go4t/rt implementations. The 13 std/map externs form a separate **modeled** category: symbolic algebra is available only for the eligible Ord comparators in ADR 0002, and other comparators stay neutral rather than tainted. Direct reasons take precedence over transitive reasons: signature IO, signature Float, unlisted extern, then the first body IO/Float/division occurrence in AST order. Concrete checked dictionaries add dependencies on selected methods; dictionary arguments conservatively add all their methods. Abstract dictionary parameters and function parameters are conditional obligations, **not** dependencies on every possible instance: callers must supply eligible implementations. Recursion alone is not taint. Transitive reasons use the shortest discovered tainted-callee path with lexical ties.

## Decision-core judgement

ADR 0001's reach includes constructor case analysis, Bool guards, Int arithmetic and first-order list induction; eligibility does not remove its order-reasoning, accumulator-generalization, termination or higher-order-quantification restrictions. README lines 79–96 describe approval and attachment controllers as pure Decide(Observed) Action logic with constructor-based evidence, TTL and timestamp states. That style remains inside this taint boundary when observations are already parsed and effects stay in the Go shell.

List structure, filtering, folds, append, Option (in std/prelude) and Result combinators are largely eligible, conditional on their callbacks/dictionaries. Int/Bool/String Eq/Ord/Show basics and string concatenation stay eligible through the prelude whitelist. All std/string helpers, including trim/split/startsWith/contains/parseInt, are eligible; symbolic calls remain opaque until their arguments are constants. All std/map operations are eligible through modeled externs: lookup/member/insert/delete/size support symbolic maps, while ordered traversals require literal maps. Maps may therefore be fields of the decision core's algebraic observations. **The typical helper set is not wholly eligible:** set/array externs and list helpers converting through arrays remain tainted. Float expressions and IO stay outside; data types merely containing Float are allowed as quantified values.

## Counts by module

| Module | Eligible | Tainted | Total |
| --- | ---: | ---: | ---: |
`)
	totalGood, totalBad := 0, 0
	for _, module := range modules {
		good, bad := 0, 0
		for _, e := range entries {
			if e.module == module {
				if e.reason == "" {
					good++
				} else {
					bad++
				}
			}
		}
		fmt.Fprintf(&out, "| `%s` | %d | %d | %d |\n", module, good, bad, good+bad)
		totalGood += good
		totalBad += bad
	}
	fmt.Fprintf(&out, "| **Total** | **%d** | **%d** | **%d** |\n\n", totalGood, totalBad, totalGood+totalBad)
	out.WriteString("## First-reason breakdown\n\nCounts are disjoint, not all contributing causes. Transitive rows retain the ultimate direct cause in the detailed path.\n\n| First reason | Count |\n| --- | ---: |\n")
	for _, category := range []string{"extern", "IO result", "Float", "division/modulo", "transitive"} {
		count := 0
		for _, e := range entries {
			if e.category == category {
				count++
			}
		}
		fmt.Fprintf(&out, "| %s | %d |\n", category, count)
	}
	for _, module := range modules {
		fmt.Fprintf(&out, "\n## `%s`\n\n| Function / instance method | Classification | Category | First tainting reason |\n| --- | --- | --- | --- |\n", module)
		for _, e := range entries {
			if e.module != module {
				continue
			}
			status, category, reason := "law-eligible", e.category, "—"
			if category == "" {
				category = "source"
			}
			if e.reason != "" {
				status, reason = "tainted", e.reason
			}
			fmt.Fprintf(&out, "| `%s` | %s | %s | %s |\n", e.name, status, category, strings.ReplaceAll(reason, "|", "\\|"))
		}
	}
	return out.String()
}
