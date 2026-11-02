package laws

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"go4t/ast"
	"go4t/lexer"
	"go4t/modules"
	"go4t/types"
)

const propertySamples = 50
const propertyDepth = 4

// TestOpenTermProperty checks normalization, not the truth of the fixture laws:
// deliberately false equalities must preserve both sides independently. Normalize
// with fresh variables FIRST, then substitute; evaluating the AST twice would
// miss precisely the open-term bugs this test is intended to catch.
//
// A deterministic seed and a forced scalar edge prefix make failures repeatable.
// One go4t run per fixture batches every law/side/substitution into lists, so the
// Go toolchain cost does not grow with the sample count. The runtime channel is
// debug output (including the program's own derived/custom Debug dictionaries).
// Function-bearing results without Debug are observed structurally, applying
// their functions to additional generated arguments before calling debug.
func TestOpenTermProperty(t *testing.T) {
	started := time.Now()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("open-term property needs the Go toolchain: %v", err)
	}
	if output, err := exec.Command(goTool, "version").CombinedOutput(); err != nil {
		t.Skipf("Go toolchain is unavailable: %v: %s", err, output)
	}
	t.Setenv("GO4T_PATH", "")
	fixtures, err := filepath.Glob("testdata/*.gt")
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("finding law fixtures: %v (%d files)", err, len(fixtures))
	}
	laws, sides, comparisons, rejected, floats := 0, 0, 0, 0, 0
	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			p := loadFixture(t, filepath.Base(fixture))
			e := newEvaluator(p)
			var names []string
			for name := range p.Laws {
				names = append(names, name)
			}
			sort.Strings(names)
			var targets []propertyTarget
			for _, name := range names {
				law, err := e.extractLaw(name)
				if err == nil {
					// Rejected dependencies are still sampled when normalization
					// succeeds, as before; certified laws use production unfolding.
					_ = e.checkDependencies(p.Laws[name], p.Proofs[name])
					e.fuel = StepLimit
					open, _ := propertyEnvironment(e, law.Params)
					_, _, err = e.equation(law, open)
				}
				if err != nil {
					if strings.Contains(err.Error(), "Float") {
						floats++
						t.Run(name, func(t *testing.T) { t.Skipf("Float law: %v", err) })
					} else {
						want := propertyRejection(filepath.Base(fixture), name)
						if want == "" || !strings.Contains(err.Error(), want) {
							t.Fatalf("unexpected rejection of %s: %v", name, err)
						}
						rejected++
						t.Logf("%s: intentional non-normalizable fixture: %v", name, err)
					}
					e = newEvaluator(p)
					continue
				}
				if want := propertyRejection(filepath.Base(fixture), name); want != "" {
					t.Fatalf("%s unexpectedly became normalizable; remove its rejection expectation and sample it", name)
				}
				laws++
				for side, expr := range []ast.Expr{law.LHS, law.RHS} {
					targets = append(targets, propertyTarget{law: name, side: side, expr: expr})
				}
				e = newEvaluator(p)
			}
			if len(targets) == 0 {
				return
			}
			original, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			main := p.Funcs["main"].Decl
			source := string(original[:propertyOffset(string(original), main.GetPos())]) + string(original[propertyOffset(string(original), main.GetEnd()):])
			var helpers strings.Builder
			for i := range targets {
				target := &targets[i]
				law, _ := e.extractLaw(target.law)
				target.source = propertyLawSource(string(original), p.Laws[target.law].Decl, target.side)
				target.observer = fmt.Sprintf("propertyObserve%d", i)
				observation := propertyObservation(t, e, types.AnnOf(target.expr).Type, "value", &target.extra)
				fmt.Fprintf(&helpers, "\nfunc %s(value %s%s) String { %s }\n", target.observer, propertyType(e, types.AnnOf(target.expr).Type), propertyParams(e, target.extra, true), observation)
				target.runner = fmt.Sprintf("propertySide%d", i)
				params := append(append([]parameter(nil), law.Params...), target.extra...)
				extraNames := make([]string, len(target.extra))
				for j, param := range target.extra {
					extraNames[j] = param.Name
				}
				extra := ""
				if len(extraNames) != 0 {
					extra = ", " + strings.Join(extraNames, ", ")
				}
				fmt.Fprintf(&helpers, "func %s(%s) String { %s((%s)%s) }\n", target.runner, propertyParams(e, params, false), target.observer, target.source, extra)
			}
			dir := t.TempDir()
			file := filepath.Join(dir, "property.gt")
			// Retype the helpers with the fixture, preserving dictionary identity.
			propertyWrite(t, file, source+helpers.String()+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			loaded, err := modules.Load([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			p, err = types.Check(loaded)
			if err != nil {
				t.Fatalf("checking property observers: %v", err)
			}
			var batches strings.Builder
			var results []propertyResult
			runtimeLines := 0
			for i, target := range targets {
				e = newEvaluator(p)
				law, err := e.extractLaw(target.law)
				if err != nil {
					t.Fatal(err)
				}
				_ = e.checkDependencies(p.Laws[target.law], p.Proofs[target.law])
				e.fuel = StepLimit
				target.expr = []ast.Expr{law.LHS, law.RHS}[target.side]
				variants := propertyVariants(t, e, law)
				variants = append(variants, propertyProofVariants(t, e, law, target.side)...)
				var calls []string
				for v, variant := range variants {
					e.fuel = StepLimit
					value := variant.normal
					runner, termSource := target.runner, target.source
					if value == nil {
						value, err = e.eval(target.expr, variant.env, nil)
						if err != nil {
							t.Fatal(err)
						}
					} else {
						runner = fmt.Sprintf("propertyPartial%d_%d", i, v)
						termSource = variant.source
						params := append(append([]parameter(nil), variant.params...), target.extra...)
						var extra string
						for _, param := range target.extra {
							extra += ", " + param.Name
						}
						fmt.Fprintf(&helpers, "func %s(%s) String { %s((%s)%s) }\n", runner, propertyParams(e, params, false), target.observer, termSource, extra)
					}
					normal, err := e.normal(value)
					if err != nil {
						t.Fatalf("%s/%d/%s: %v", target.law, target.side, variant.name, err)
					}
					// Retain the open term unchanged across every substitution.
					nextID := e.nextID
					rng := rand.New(rand.NewPCG(2119, 4))
					for sample := range propertySamples {
						e.fuel, e.nextID = StepLimit, nextID
						e.solutions = make(map[int]*term)
						var sigma []string
						for j, param := range variant.params {
							v := propertyValue(t, e, rng, param.Type, propertyDepth, sample, j)
							e.solutions[variant.vars[j].id] = v
							sigma = append(sigma, param.Name+"="+propertyLiteral(t, e, v))
						}
						args := make([]string, 0, len(law.Params)+len(target.extra))
						if variant.normal == nil {
							for _, param := range law.Params {
								args = append(args, propertyLiteral(t, e, e.instantiate(variant.env.lookup(param.Name))))
							}
						} else {
							for _, variable := range variant.vars {
								args = append(args, propertyLiteral(t, e, e.instantiate(variable)))
							}
						}
						closed, err := e.normal(normal)
						if err != nil {
							t.Fatalf("%s/%d/%s sample=%d σ={%s}: %v", target.law, target.side, variant.name, sample, strings.Join(sigma, ", "), err)
						}
						observed := []*term{closed}
						for j, param := range target.extra {
							v := propertyValue(t, e, rng, param.Type, propertyDepth, sample, j+len(variant.params))
							observed = append(observed, v)
							literal := propertyLiteral(t, e, v)
							args = append(args, literal)
							sigma = append(sigma, param.Name+"="+literal)
						}
						fn, err := e.function(target.observer, nil)
						if err != nil {
							t.Fatal(err)
						}
						shown, err := e.apply(fn, observed)
						if err == nil {
							shown, err = e.normal(shown)
						}
						if err != nil {
							t.Fatalf("%s/%d/%s sample=%d σ={%s}: readback failed: %v", target.law, target.side, variant.name, sample, strings.Join(sigma, ", "), err)
						}
						result := propertyResult{name: fmt.Sprintf("%s/%d/%s/%d", target.law, target.side, variant.name, sample), source: termSource, sigma: strings.Join(sigma, ", "), normal: e.display(normal), runtimeIndex: runtimeLines, residualIndex: -1}
						calls = append(calls, runner+"("+strings.Join(args, ", ")+")")
						runtimeLines++
						if shown.kind == "string" {
							result.want = shown.name
						} else {
							// Unsupported comparators and nonliteral key types must
							// stay neutral, even closed. Observe the evaluator's
							// residual with rt, independently of the original side.
							if !propertyNeutralMap(closed) {
								t.Fatalf("%s: readback did not close: n=%s; result=%s", result.name, e.display(normal), e.display(shown))
							}
							observations := make([]string, len(observed))
							for j, value := range observed { observations[j] = propertyTermSource(t, e, value) }
							calls = append(calls, target.observer+"("+strings.Join(observations, ", ")+")")
							result.residualIndex = runtimeLines
							runtimeLines++
						}
						results = append(results, result)
					}
					e.solutions = make(map[int]*term)
					sides++
				}
				fmt.Fprintf(&batches, "func propertyBatch%d() IO[Unit] { propertyPrint([%s]) }\n", i, strings.Join(calls, ",\n"))
			}
			batches.WriteString("func propertyPrint(lines List[String]) IO[Unit] { match lines { Nil => ioPure(unit()); Cons(line, rest) => do { println(line); propertyPrint(rest) } } }\nfunc main() IO[Unit] { do {\n")
			for i := range targets {
				fmt.Fprintf(&batches, "propertyBatch%d();\n", i)
			}
			batches.WriteString("ioPure(unit()) } }\n")
			propertyWrite(t, file, source+helpers.String()+batches.String())
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, goTool, "run", "./cmd/go4t", "run", file)
			cmd.Dir = ".."
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("go run ./cmd/go4t run %s: %v\n%s", file, err, stderr.String())
			}
			lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
			if len(lines) != runtimeLines {
				t.Fatalf("runtime returned %d lines, want %d\nstderr: %s", len(lines), runtimeLines, stderr.String())
			}
			for _, result := range results {
				want := result.want
				if result.residualIndex >= 0 { want = lines[result.residualIndex] }
				if lines[result.runtimeIndex] != want {
					t.Errorf("%s: normalization mismatch\nt = %s\nn = %s\nσ = {%s}\nruntime tσ: %q\nevaluator nσ: %q", result.name, result.source, result.normal, result.sigma, lines[result.runtimeIndex], want)
				}
			}
			comparisons += len(results)
			t.Logf("%d law sides, %d runtime comparisons", len(targets), len(results))
		})
	}
	t.Logf("open-term property: laws=%d side/partial-form targets=%d substitutions=%d per target comparisons=%d Float skips=%d intentional rejections=%d runtime=%s", laws, sides, propertySamples, comparisons, floats, rejected, time.Since(started))
}

type propertyTarget struct {
	law, runner, observer, source string
	side int
	expr ast.Expr
	extra []parameter
}

type propertyResult struct {
	name, source, sigma, normal, want string
	runtimeIndex, residualIndex int
}

type propertyVariant struct {
	name, source string
	normal *term
	env *environment
	params []parameter
	vars []*term
}

func propertyEnvironment(e *evaluator, params []parameter) (*environment, []*term) {
	env := child(nil)
	vars := make([]*term, len(params))
	for i, param := range params {
		vars[i] = e.fresh(param.Name)
		env.values[param.Name] = vars[i]
	}
	return env, vars
}

// Include every one-constructor Match refinement, plus nested refinements of
// constructor fields. These are partial proofs: unsplit recursive tails/Option
// fields stay open during normalization, rather than being pre-generated data.
func propertyVariants(t *testing.T, e *evaluator, law lawGoal) []propertyVariant {
	t.Helper()
	env, vars := propertyEnvironment(e, law.Params)
	result := []propertyVariant{{name: "open", env: env, params: law.Params, vars: vars}}
	var split func(propertyVariant, int)
	split = func(parent propertyVariant, depth int) {
		if depth == 0 {
			return
		}
		for i, param := range parent.params {
			data, args := propertyData(e, param.Type)
			if data == nil {
				continue
			}
			for _, ctor := range data.Ctors {
				branch := propertyVariant{name: parent.name+"/"+param.Name+"="+e.sourceName(ctor.Name), env: child(nil)}
				fields := make([]*term, len(ctor.Fields))
				for j, field := range propertyFields(data, args, ctor) {
					name := fmt.Sprintf("%sField%d", param.Name, j)
					fields[j] = e.fresh(name)
					branch.params = append(branch.params, parameter{Name: name, Type: field})
					branch.vars = append(branch.vars, fields[j])
				}
				for j, other := range parent.params {
					if j != i {
						branch.params = append(branch.params, other)
						branch.vars = append(branch.vars, parent.vars[j])
					}
				}
				e.solutions[parent.vars[i].id] = &term{kind: "ctor", ctor: ctor, args: fields}
				for _, original := range law.Params {
					branch.env.values[original.Name] = e.instantiate(parent.env.lookup(original.Name))
				}
				delete(e.solutions, parent.vars[i].id)
				result = append(result, branch)
				// Nested matches are needed for Box[Option[Int]], but splitting
				// all sibling parameters again produces redundant cross products.
				if len(law.Params) == 1 && data.Name != "List" {
					split(branch, depth-1)
				}
			}
		}
	}
	split(result[0], 2)
	return result
}

func propertyData(e *evaluator, typ types.Type) (*types.DataType, []types.Type) {
	con, args := typeHead(typ)
	if con == nil {
		return nil, nil
	}
	return e.p.DataTypes[con.Name], args
}

func propertyFields(data *types.DataType, args []types.Type, ctor *types.Ctor) []types.Type {
	sub := make(map[*types.TVar]types.Type, len(args))
	for i, arg := range args {
		sub[data.TParams[i]] = arg
	}
	fields := make([]types.Type, len(ctor.Fields))
	for i, field := range ctor.Fields {
		fields[i] = substituteType(field, sub)
	}
	return fields
}

// A constructor is eligible only if every field is inhabitable within the
// remaining depth. This handles recursive and mutually-recursive ADTs without
// assuming their first constructor is the base case; List/Option use this same
// DataType path, not special generators.
func propertyInhabited(e *evaluator, typ types.Type, depth int) bool {
	con, args := typeHead(typ)
	if con == nil {
		return false
	}
	switch con.Name {
	case "Int", "Bool", "String", "Unit", "Map":
		return true
	}
	if _, ok := types.IsTuple(con); ok {
		for _, arg := range args {
			if !propertyInhabited(e, arg, depth) {
				return false
			}
		}
		return true
	}
	if depth < 0 {
		return false
	}
	data := e.p.DataTypes[con.Name]
	if data == nil {
		return false
	}
	for _, ctor := range data.Ctors {
		possible := true
		for _, field := range propertyFields(data, args, ctor) {
			possible = possible && propertyInhabited(e, field, depth-1)
		}
		if possible {
			return true
		}
	}
	return false
}

func propertyValue(t *testing.T, e *evaluator, rng *rand.Rand, typ types.Type, depth, sample, offset int) *term {
	t.Helper()
	con, args := typeHead(typ)
	if con == nil {
		t.Fatalf("no first-order generator for %s", typ)
	}
	switch con.Name {
	case "Int":
		edges := [...]int64{math.MinInt64, math.MaxInt64, 0, 1, -1}
		if sample < len(edges) {
			return integer(edges[(sample+offset)%len(edges)])
		}
		if sample%2 == 0 { return integer(int64(rng.IntN(3)-1)) }
		if rng.IntN(3) == 0 {
			return integer(edges[rng.IntN(len(edges))])
		}
		return integer(int64(rng.Uint64()))
	case "Bool":
		if sample < 2 {
			return boolean((sample+offset)%2 == 0)
		}
		return boolean(rng.IntN(2) == 0)
	case "String":
		edges := [...]string{"", "a", "\"\\\n\t", "日本語𝄞", "\x00\r"}
		if sample < len(edges) {
			return text(edges[(sample+offset)%len(edges)])
		}
		if sample%2 == 0 { return text([]string{"", "a", "b"}[rng.IntN(3)]) }
		alphabet := []rune("ab \"\\\n\t日本語𝄞")
		chars := make([]rune, rng.IntN(12))
		for i := range chars {
			chars[i] = alphabet[rng.IntN(len(alphabet))]
		}
		return text(string(chars))
	case "Unit":
		return &term{kind: "unit"}
	case "Map":
		return propertyMapValue(t, e, rng, args, depth, sample, offset)
	}
	if _, ok := types.IsTuple(con); ok {
		fields := make([]*term, len(args))
		for i, arg := range args {
			fields[i] = propertyValue(t, e, rng, arg, depth, sample, offset+i)
		}
		return &term{kind: "tuple", args: fields}
	}
	data := e.p.DataTypes[con.Name]
	if data == nil || depth < 0 {
		t.Fatalf("no bounded generator for %s at depth %d", typ, depth)
	}
	var eligible []*types.Ctor
	for _, ctor := range data.Ctors {
		possible := true
		for _, field := range propertyFields(data, args, ctor) {
			possible = possible && propertyInhabited(e, field, depth-1)
		}
		if possible {
			eligible = append(eligible, ctor)
		}
	}
	if len(eligible) == 0 {
		t.Fatalf("type %s has no inhabitant within depth %d", typ, depth)
	}
	index := rng.IntN(len(eligible))
	if sample < len(eligible) {
		index = (sample+offset)%len(eligible)
	}
	ctor := eligible[index]
	fields := make([]*term, len(ctor.Fields))
	for i, field := range propertyFields(data, args, ctor) {
		fields[i] = propertyValue(t, e, rng, field, depth-1, sample, offset+i)
	}
	return &term{kind: "ctor", ctor: ctor, args: fields}
}

func propertyLiteral(t *testing.T, e *evaluator, value *term) string {
	t.Helper()
	switch value.kind {
	case "insert", "mapop", "mapfrom", "primitive":
		return propertyTermSource(t, e, value)
	}
	args := make([]string, len(value.args))
	for i, arg := range value.args {
		args[i] = propertyLiteral(t, e, arg)
	}
	switch value.kind {
	case "int":
		if value.number == math.MinInt64 {
			return "(-9223372036854775807 - 1)"
		}
		return "("+strconv.FormatInt(value.number, 10)+")"
	case "bool":
		return strconv.FormatBool(value.flag)
	case "string":
		// go4t supports JSON-style escapes, not Go's \xNN/\a/\v.
		return propertyQuote(value.name)
	case "unit":
		return "unit()"
	case "tuple":
		return "("+strings.Join(args, ", ")+")"
	case "map":
		return propertyMapSource(t, e, value, propertyLiteral)
	case "ctor":
		name := e.sourceName(value.ctor.Name)
		if len(args) == 0 {
			return name
		}
		if value.ctor.FieldNames != nil {
			for i, field := range value.ctor.FieldNames {
				args[i] = field+": "+args[i]
			}
			return name+"{"+strings.Join(args, ", ")+"}"
		}
		return name+"("+strings.Join(args, ", ")+")"
	}
	t.Fatalf("substitution is not closed first-order data: %s", e.display(value))
	return ""
}

func propertyQuote(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			if r < 32 {
				fmt.Fprintf(&out, "\\u%04x", r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

func propertyType(e *evaluator, typ types.Type) string {
	if fn, ok := types.Unalias(typ).(*types.TFunc); ok {
		params := make([]string, len(fn.Params))
		for i, param := range fn.Params {
			params[i] = propertyType(e, param)
		}
		return "func("+strings.Join(params, ", ")+") "+propertyType(e, fn.Ret)
	}
	con, args := typeHead(typ)
	params := make([]string, len(args))
	for i, arg := range args {
		params[i] = propertyType(e, arg)
	}
	if _, ok := types.IsTuple(con); ok {
		return "("+strings.Join(params, ", ")+")"
	}
	name := e.sourceName(con.Name)
	if len(params) != 0 {
		name += "["+strings.Join(params, ", ")+"]"
	}
	return name
}

func propertyParams(e *evaluator, params []parameter, leadingComma bool) string {
	parts := make([]string, len(params))
	for i, param := range params {
		parts[i] = param.Name+" "+propertyType(e, param.Type)
	}
	result := strings.Join(parts, ", ")
	if leadingComma && len(params) != 0 {
		result = ", "+result
	}
	return result
}

func propertyObservation(t *testing.T, e *evaluator, typ types.Type, expr string, extra *[]parameter) string {
	t.Helper()
	if fn, ok := types.Unalias(typ).(*types.TFunc); ok {
		args := make([]string, len(fn.Params))
		for i, param := range fn.Params {
			name := fmt.Sprintf("propertyInput%d", len(*extra))
			*extra = append(*extra, parameter{Name: name, Type: param})
			args[i] = name
		}
		return propertyObservation(t, e, fn.Ret, "("+expr+")("+strings.Join(args, ", ")+")", extra)
	}
	con, args := typeHead(typ)
	for _, inst := range e.p.Instances {
		if inst.Class.Name == "Debug" && inst.TypeCon == con {
			return "debug("+expr+")"
		}
	}
	data := e.p.DataTypes[con.Name]
	if data == nil {
		t.Fatalf("no structural debug observer for %s", typ)
	}
	var arms []string
	for _, ctor := range data.Ctors {
		names := make([]string, len(ctor.Fields))
		parts := []string{propertyQuote(e.sourceName(ctor.Name)+"(")}
		for i, field := range propertyFields(data, args, ctor) {
			names[i] = fmt.Sprintf("propertyField%d", i)
			if i != 0 {
				parts = append(parts, propertyQuote(", "))
			}
			parts = append(parts, propertyObservation(t, e, field, names[i], extra))
		}
		parts = append(parts, propertyQuote(")"))
		pattern := e.sourceName(ctor.Name)
		if len(names) != 0 {
			pattern += "("+strings.Join(names, ", ")+")"
		} else {
			parts = []string{propertyQuote(pattern)}
		}
		arms = append(arms, pattern+" => "+strings.Join(parts, " + "))
	}
	return "(match "+expr+" { "+strings.Join(arms, "; ")+" })"
}

func propertyOffset(source string, pos ast.Pos) int {
	line, col := 1, 1
	for offset, r := range source {
		if line == pos.Line && col == pos.Col {
			return offset
		}
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return len(source)
}

// The AST keeps semantic expression positions (a binary operator, for example).
// Locate the outer equality in the original token stream so the runtime oracle
// retains the exact source spelling rather than reusing evaluator serialization.
func propertyLawSource(source string, decl *ast.LawDecl, side int) string {
	start := propertyOffset(source, decl.Pos)
	end := propertyOffset(source, decl.End)
	body := source[start:end]
	tokens, err := lexer.New(body).All()
	if err != nil { panic(err) }
	depth, opened, first, delimiter, last := 0, false, 0, 0, 0
	for _, token := range tokens {
		if !opened {
			if token.Text == "{" { opened = true; first = propertyOffset(body, token.End) }
			continue
		}
		switch token.Text {
		case "(", "[", "{": depth++
		case ")", "]": depth--
		case "}":
			if depth == 0 { last = propertyOffset(body, token.Pos) } else { depth-- }
		case "==":
			if depth == 0 { delimiter = propertyOffset(body, token.Pos) }
		}
	}
	if side == 0 { return strings.TrimSpace(body[first:delimiter]) }
	return strings.TrimSpace(body[delimiter+2:last])
}

func propertyWrite(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

// These fixture sides deliberately have no admissible normal form. The oracle
// still checks false equations and terminating sides of invalid proof scripts.
func propertyRejection(file, law string) string {
	if file != "rejected.gt" { return "" }
	return map[string]string{
		"clonedPrimitive": "law depends on extern go4t/rt.EqInt",
		"strictArgument": "law depends on extern example.invalid/foreign.Read",
		"foreignDependency": "law depends on extern example.invalid/foreign.Read",
		"higherOrder": "higher-order",
		"io": "law depends on IO",
		"loopDependency": "step limit",
		"divZero": "division by zero",
		"divVariable": "nonconstant divisor",
	}[law]
}
