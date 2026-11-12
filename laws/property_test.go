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

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/types"
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
	laws, sides, comparisons, rejected, floatComparisons := 0, 0, 0, 0, 0
	skipped := 0
	floatSamples := make(map[uint64]bool)
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
					want := propertyRejection(filepath.Base(fixture), name)
					if want == "" || !strings.Contains(err.Error(), want) {
						t.Fatalf("unexpected rejection of %s: %v", name, err)
					}
					rejected++
					t.Logf("%s: intentional non-normalizable fixture: %v", name, err)
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
					e.assumptions = variant.assumptions
					if variant.normal == nil {
						if err := e.assumePrecondition(law, variant.env); err != nil {
							t.Fatal(err)
						}
					}
					assumptions := e.assumptions
					seeds := propertyAssumptionSeeds(assumptions)
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
					satisfied := 0
					e.assumptions = nil
					for sample := range propertySamples {
						e.fuel, e.nextID = StepLimit, nextID
						// Longer runtime-oracle substitutions are not proof goals:
						// allow their concrete readback more fuel, without changing
						// the production proof budget or open normalization.
						if strings.HasPrefix(filepath.Base(fixture), "list_models") {
							e.fuel *= 100
						}
						e.solutions = make(map[int]*term)
						var sigma []string
						hasFloat := false
						for j, param := range variant.params {
							var v *term
							if strings.HasPrefix(filepath.Base(fixture), "list_models") {
								v = propertyModelValue(t, e, rng, param.Type, sample, j)
							} else {
								v = propertyValue(t, e, rng, param.Type, propertyDepth, sample, j)
							}
							v = propertySeedValue(v, seeds, sample-5)
							hasFloat = propertyFloatSamples(v, floatSamples) || hasFloat
							e.solutions[variant.vars[j].id] = v
							sigma = append(sigma, param.Name+"="+propertyLiteral(t, e, v))
						}
						holds, err := propertyAssumptionsHold(e, assumptions)
						if err != nil {
							t.Fatalf("%s/%s: evaluating assumptions: %v", target.law, variant.name, err)
						}
						if !holds {
							skipped++
							continue
						}
						satisfied++
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
							hasFloat = propertyFloatSamples(v, floatSamples) || hasFloat
							literal := propertyLiteral(t, e, v)
							args = append(args, literal)
							sigma = append(sigma, param.Name+"="+literal)
						}
						// Float is opaque to normalization. Observe float-bearing
						// residuals with the same runtime Debug dictionary as the
						// original expression, without adding a Float primitive.
						opaque := false
						for _, value := range observed {
							opaque = propertyFloatSamples(value, nil) || opaque
						}
						var shown *term
						if !opaque {
							fn, err := e.function(target.observer, nil)
							if err != nil {
								t.Fatal(err)
							}
							shown, err = e.apply(fn, observed)
							if err == nil {
								shown, err = e.normal(shown)
							}
							if err != nil {
								t.Fatalf("%s/%d/%s sample=%d σ={%s}: readback failed: %v", target.law, target.side, variant.name, sample, strings.Join(sigma, ", "), err)
							}
						}
						result := propertyResult{name: fmt.Sprintf("%s/%d/%s/%d", target.law, target.side, variant.name, sample), source: termSource, sigma: strings.Join(sigma, ", "), normal: e.display(normal), runtimeIndex: runtimeLines, residualIndex: -1}
						calls = append(calls, runner+"("+strings.Join(args, ", ")+")")
						runtimeLines++
						if !opaque && shown.kind == "string" {
							result.want = shown.name
						} else {
							// Unsupported map comparators and opaque Float fields
							// use the residual oracle, independently of the source.
							if !opaque && !propertyNeutralMap(closed) {
								t.Fatalf("%s: readback did not close: n=%s; result=%s", result.name, e.display(normal), e.display(shown))
							}
							observations := make([]string, len(observed))
							for j, value := range observed {
								observations[j] = propertyTermSource(t, e, value)
							}
							calls = append(calls, target.observer+"("+strings.Join(observations, ", ")+")")
							result.residualIndex = runtimeLines
							runtimeLines++
						}
						results = append(results, result)
						if hasFloat {
							floatComparisons++
						}
					}
					if err := propertyRequireSamples(target.law+"/"+variant.name, satisfied); err != nil {
						t.Fatal(err)
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
			// A kill ceiling for a hung program, not a budget: a shared CI runner
			// needs well over a minute for the largest fixture.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
				if result.residualIndex >= 0 {
					want = lines[result.residualIndex]
				}
				if lines[result.runtimeIndex] != want {
					t.Errorf("%s: normalization mismatch\nt = %s\nn = %s\nσ = {%s}\nruntime tσ: %q\nevaluator nσ: %q", result.name, result.source, result.normal, result.sigma, lines[result.runtimeIndex], want)
				}
			}
			comparisons += len(results)
			t.Logf("%d law sides, %d runtime comparisons", len(targets), len(results))
		})
	}
	for _, edge := range []float64{math.NaN(), math.Copysign(0, -1), math.Inf(1), math.Inf(-1), 0, 1, -1, 1e308, 5e-324} {
		if !floatSamples[math.Float64bits(edge)] {
			t.Errorf("Float edge %g was not sampled", edge)
		}
	}
	if floatComparisons == 0 {
		t.Error("no Float-bearing substitutions were compared")
	}
	t.Logf("open-term property: laws=%d side/partial-form targets=%d substitutions=%d per target comparisons=%d skipped=%d Float-bearing substitutions=%d intentional rejections=%d runtime=%s", laws, sides, propertySamples, comparisons, skipped, floatComparisons, rejected, time.Since(started))
}

func TestPropertyFloatTerms(t *testing.T) {
	e := newEvaluator(loadFixture(t, "projection.gt"))
	rng := rand.New(rand.NewPCG(2119, 4))
	sources := []string{"(0.0 / 0.0)", "(-0.0)", "(1.0 / 0.0)", "(-1.0 / 0.0)", "0.0", "1.0", "-1.0", "1e+308", "5e-324"}
	for offset := range sources {
		for sample := range sources {
			value := propertyValue(t, e, rng, types.Float, propertyDepth, sample, offset)
			want := sources[(sample+offset)%len(sources)]
			if got := propertyLiteral(t, e, value); got != want {
				t.Errorf("sample=%d offset=%d: source=%s, want %s", sample, offset, got, want)
			}
			if got := propertyTermSource(t, e, value); got != want {
				t.Errorf("residual source=%s, want %s", got, want)
			}
			normal, err := e.normal(value)
			if err != nil || normal != value {
				t.Fatalf("Float sample was reduced: %v, %v", normal, err)
			}
			copy := &term{kind: "float", float: value.float}
			if !alphaEqual(value, copy, nil) || termKey(value) != termKey(copy) {
				t.Fatal("identical Float bits must retain term identity")
			}
		}
	}
	values := []float64{math.NaN(), math.Float64frombits(math.Float64bits(math.NaN()) + 1), math.Copysign(0, -1), 0, 1, -1, math.Inf(1), math.Inf(-1)}
	for i, a := range values {
		for j, b := range values {
			left, right := &term{kind: "float", float: a}, &term{kind: "float", float: b}
			if alphaEqual(left, right, nil) != (i == j) || (termKey(left) == termKey(right)) != (i == j) {
				t.Errorf("Float term identity conflates bits %016x and %016x", math.Float64bits(a), math.Float64bits(b))
			}
		}
	}
}

type propertyTarget struct {
	law, runner, observer, source string
	side                          int
	expr                          ast.Expr
	extra                         []parameter
}

type propertyResult struct {
	name, source, sigma, normal, want string
	runtimeIndex, residualIndex       int
}

type propertyVariant struct {
	name, source string
	normal       *term
	env          *environment
	params       []parameter
	vars         []*term
	assumptions  *assumption
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
				branch := propertyVariant{name: parent.name + "/" + param.Name + "=" + e.sourceName(ctor.Name), env: child(nil)}
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
	case "Int", "Float", "Bool", "String", "Unit", "Map":
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
		if sample%2 == 0 {
			return integer(int64(rng.IntN(3) - 1))
		}
		if rng.IntN(3) == 0 {
			return integer(edges[rng.IntN(len(edges))])
		}
		return integer(int64(rng.Uint64()))
	case "Float":
		edges := [...]float64{math.NaN(), math.Copysign(0, -1), math.Inf(1), math.Inf(-1), 0.0, 1.0, -1.0, 1e308, 5e-324}
		if sample < len(edges) {
			return &term{kind: "float", float: edges[(sample+offset)%len(edges)]}
		}
		if rng.IntN(3) == 0 {
			return &term{kind: "float", float: edges[rng.IntN(len(edges))]}
		}
		return &term{kind: "float", float: math.Float64frombits(rng.Uint64())}
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
		if sample%2 == 0 {
			return text([]string{"", "a", "b"}[rng.IntN(3)])
		}
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
		index = (sample + offset) % len(eligible)
	}
	ctor := eligible[index]
	fields := make([]*term, len(ctor.Fields))
	for i, field := range propertyFields(data, args, ctor) {
		fields[i] = propertyValue(t, e, rng, field, depth-1, sample, offset+i)
	}
	return &term{kind: "ctor", ctor: ctor, args: fields}
}

// Give every list-model equation Nil, singleton, long and random tails after
// open normalization. Nested elements retain the bounded ADT generator; Int
// fields retain its forced MinInt64/MaxInt64/0/1/-1 prefix and random bits.
func propertyModelValue(t *testing.T, e *evaluator, rng *rand.Rand, typ types.Type, sample, offset int) *term {
	con, args := typeHead(typ)
	if con == nil || con.Name != "List" {
		return propertyValue(t, e, rng, typ, propertyDepth, sample, offset)
	}
	lengths := [...]int{0, 1, 2, 8, 32, 64}
	size := rng.IntN(17)
	if sample < len(lengths) {
		size = lengths[(sample+3*offset)%len(lengths)]
	}
	list := e.listNil()
	for i := range size {
		item := propertyValue(t, e, rng, args[0], propertyDepth-1, sample, offset+i)
		list = e.listCons(item, list)
	}
	return list
}

func propertyFloatSamples(value *term, samples map[uint64]bool) bool {
	if value.kind == "float" {
		if samples != nil {
			samples[math.Float64bits(value.float)] = true
		}
		return true
	}
	found := false
	for _, arg := range value.args {
		found = propertyFloatSamples(arg, samples) || found
	}
	for _, arm := range value.arms {
		found = propertyFloatSamples(arm.guard, samples) || found
		found = propertyFloatSamples(arm.body, samples) || found
	}
	return found
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
		return "(" + strconv.FormatInt(value.number, 10) + ")"
	case "float":
		switch {
		case math.IsNaN(value.float):
			return "(0.0 / 0.0)"
		case math.IsInf(value.float, 1):
			return "(1.0 / 0.0)"
		case math.IsInf(value.float, -1):
			return "(-1.0 / 0.0)"
		case value.float == 0 && math.Signbit(value.float):
			return "(-0.0)"
		}
		literal := strconv.FormatFloat(value.float, 'g', -1, 64)
		if !strings.ContainsAny(literal, ".e") {
			literal += ".0"
		}
		return literal
	case "bool":
		return strconv.FormatBool(value.flag)
	case "string":
		// go4t supports JSON-style escapes, not Go's \xNN/\a/\v.
		return propertyQuote(value.name)
	case "unit":
		return "unit()"
	case "tuple":
		return "(" + strings.Join(args, ", ") + ")"
	case "map":
		return propertyMapSource(t, e, value, propertyLiteral)
	case "ctor":
		name := propertyReference(e, value.ctor.Name)
		if len(args) == 0 {
			return name
		}
		if value.ctor.FieldNames != nil {
			for i, field := range value.ctor.FieldNames {
				args[i] = field + ": " + args[i]
			}
			return name + "{" + strings.Join(args, ", ") + "}"
		}
		return name + "(" + strings.Join(args, ", ") + ")"
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
		return "func(" + strings.Join(params, ", ") + ") " + propertyType(e, fn.Ret)
	}
	con, args := typeHead(typ)
	params := make([]string, len(args))
	for i, arg := range args {
		params[i] = propertyType(e, arg)
	}
	if _, ok := types.IsTuple(con); ok {
		return "(" + strings.Join(params, ", ") + ")"
	}
	name := propertyReference(e, con.Name)
	if len(params) != 0 {
		name += "[" + strings.Join(params, ", ") + "]"
	}
	return name
}

func propertyParams(e *evaluator, params []parameter, leadingComma bool) string {
	parts := make([]string, len(params))
	for i, param := range params {
		parts[i] = param.Name + " " + propertyType(e, param.Type)
	}
	result := strings.Join(parts, ", ")
	if leadingComma && len(params) != 0 {
		result = ", " + result
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
			return "debug(" + expr + ")"
		}
	}
	data := e.p.DataTypes[con.Name]
	if data == nil {
		t.Fatalf("no structural debug observer for %s", typ)
	}
	var arms []string
	for _, ctor := range data.Ctors {
		names := make([]string, len(ctor.Fields))
		parts := []string{propertyQuote(e.sourceName(ctor.Name) + "(")}
		for i, field := range propertyFields(data, args, ctor) {
			names[i] = fmt.Sprintf("propertyField%d", i)
			if i != 0 {
				parts = append(parts, propertyQuote(", "))
			}
			parts = append(parts, propertyObservation(t, e, field, names[i], extra))
		}
		parts = append(parts, propertyQuote(")"))
		pattern := propertyReference(e, ctor.Name)
		if len(names) != 0 {
			pattern += "(" + strings.Join(names, ", ") + ")"
		} else {
			parts = []string{propertyQuote(e.sourceName(ctor.Name))}
		}
		arms = append(arms, pattern+" => "+strings.Join(parts, " + "))
	}
	return "(match " + expr + " { " + strings.Join(arms, "; ") + " })"
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
	if decl.Requires != nil {
		start = propertyOffset(source, decl.Requires.GetEnd())
	}
	end := propertyOffset(source, decl.End)
	body := source[start:end]
	tokens, err := lexer.New(body).All()
	if err != nil {
		panic(err)
	}
	depth, opened, first, delimiter, last := 0, false, 0, 0, 0
	for _, token := range tokens {
		if !opened {
			if token.Text == "{" {
				opened = true
				first = propertyOffset(body, token.End)
			}
			continue
		}
		switch token.Text {
		case "(", "[", "{":
			depth++
		case ")", "]":
			depth--
		case "}":
			if depth == 0 {
				last = propertyOffset(body, token.Pos)
			} else {
				depth--
			}
		case "==":
			if depth == 0 {
				delimiter = propertyOffset(body, token.Pos)
			}
		}
	}
	if side == 0 {
		return strings.TrimSpace(body[first:delimiter])
	}
	return strings.TrimSpace(body[delimiter+2 : last])
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
	if file == "projection_bad.gt" && law == "stampSelfEqual" {
		return "Float is not supported in laws"
	}
	if file != "rejected.gt" {
		return ""
	}
	return map[string]string{
		"clonedPrimitive":   "law depends on extern github.com/reoring/go4t/rt.EqInt",
		"strictArgument":    "law depends on extern example.invalid/foreign.Read",
		"foreignDependency": "law depends on extern example.invalid/foreign.Read",
		"higherOrder":       "higher-order",
		"io":                "law depends on IO",
		"loopDependency":    "step limit",
		"divZero":           "division by zero",
		"divVariable":       "nonconstant divisor",
	}[law]
}

// The caller clears the hypothesis stack before closing these terms: otherwise
// an assumption could establish itself and admit a violating substitution.
func propertyAssumptionsHold(e *evaluator, assumptions *assumption) (bool, error) {
	for a := assumptions; a != nil; a = a.parent {
		condition, err := e.normal(e.instantiate(a.condition))
		if err != nil {
			return false, err
		}
		if !alphaEqual(condition, a.value, nil) {
			return false, nil
		}
	}
	return true, nil
}

func propertyRequireSamples(name string, satisfied int) error {
	if satisfied == 0 {
		return fmt.Errorf("%s: no substitution satisfies the assumptions", name)
	}
	return nil
}

func TestPropertyRequiresSatisfyingSubstitution(t *testing.T) {
	// Deliberately outside the incomplete precondition refutation fragment.
	p := loadSource(t, "package main\nlaw impossible(a Int) requires 0 <= a && a <= 10 && a * 2 == 21 { a == a }\nproof impossible(a) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }")
	e := newEvaluator(p)
	law, err := e.extractLaw("impossible")
	if err != nil {
		t.Fatal(err)
	}
	env, _ := propertyEnvironment(e, law.Params)
	if err := e.assumePrecondition(law, env); err != nil {
		t.Fatal(err)
	}
	assumptions := e.assumptions
	e.assumptions = nil
	holds, err := propertyAssumptionsHold(e, assumptions)
	if err != nil || holds {
		t.Fatalf("unsatisfiable fixture: holds=%v err=%v", holds, err)
	}
	if err := propertyRequireSamples("impossible", 0); err == nil {
		t.Fatal("empty satisfying sample set must fail, not silently pass")
	}
}

// Retain the five forced Int edges, then try literals actually mentioned by
// hypotheses. Full-width randomness alone will never exercise e.g. k == 2.
func propertyAssumptionSeeds(assumptions *assumption) map[string][]*term {
	seen := make(map[string]*term)
	var visit func(*term)
	visit = func(value *term) {
		if value.kind == "int" || value.kind == "string" {
			seen[termKey(value)] = value
		}
		for _, arg := range value.args {
			visit(arg)
		}
		for _, arm := range value.arms {
			visit(arm.pattern)
			visit(arm.guard)
			visit(arm.body)
		}
	}
	for a := assumptions; a != nil; a = a.parent {
		visit(a.condition)
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seeds := make(map[string][]*term)
	for _, key := range keys {
		value := seen[key]
		seeds[value.kind] = append(seeds[value.kind], value)
	}
	return seeds
}

func propertySeedValue(value *term, seeds map[string][]*term, index int) *term {
	if index < 0 {
		return value
	}
	if values := seeds[value.kind]; index < len(values) {
		return values[index]
	}
	// Generated constructor/tuple fields are independent values. Do not mutate
	// map keys: map generation must retain its own sorting/deduplication.
	if value.kind == "ctor" || value.kind == "tuple" {
		for i, arg := range value.args {
			value.args[i] = propertySeedValue(arg, seeds, index)
		}
	}
	return value
}
