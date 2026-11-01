package codegen

import (
	"bytes"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gofx/modules"
	"gofx/types"
)

func mirrorProgram(t *testing.T, files map[string]string) *types.Program {
	t.Helper()
	dir := t.TempDir()
	for name, source := range files {
		filename := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := modules.Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestGoMirrorConversions(t *testing.T) {
	program := mirrorProgram(t, map[string]string{
		"main.gfx": `package main
import "model"
type Tree = Leaf(Int) | Branch(Tree, Tree)
type Event = Message { text: String } | Empty
type Box = Box(Int)
type Unused = Unused(String)
alias Alias = Option[List[Int]]
func echoTree(t Tree) Tree { t }
func echoEvent(e Event) Event { e }
func echoBox(b Box) Box { b }
func echoAlias(a Alias) Alias { a }
func remote(r model.Remote) model.Remote { r }
func result(r Result[String, Option[Int]]) Result[String, Option[Int]] { r }
func strings(m Map[String, List[Int]]) Map[String, List[Int]] { m }
func ints(m Map[Int, Bool]) Map[Int, Bool] { m }
func floats(m Map[Float, String]) Map[Float, String] { m }
func bools(m Map[Bool, Int]) Map[Bool, Int] { m }
func set(s Set[String]) Set[String] { s }
func echoTuple2(t (Int, String)) (Int, String) { t }
func echoTuple3(t (Int, Int, Int)) (Int, Int, Int) { t }
func echoTuple4(t (Int, Int, Int, Int)) (Int, Int, Int, Int) { t }
func echoTuple5(t (Int, Int, Int, Int, Int)) (Int, Int, Int, Int, Int) { t }
func echoTuple6(t (Int, Int, Int, Int, Int, Int)) (Int, Int, Int, Int, Int, Int) { t }
func echoTuple7(t (Int, Int, Int, Int, Int, Int, Int)) (Int, Int, Int, Int, Int, Int, Int) { t }
func echoTuple8(t (Int, Int, Int, Int, Int, Int, Int, Int)) (Int, Int, Int, Int, Int, Int, Int, Int) { t }
func number(n Int) Int { n + 1 }
func text(s String) String { s }
func truth(b Bool) Bool { b }
func decimal(f Float) Float { f }
func unitValue(u Unit) Unit { u }
func later(t Tree) IO[Tree] { ioPure(t) }
func failed() IO[Int] { ioFail("failed export") }
func main() IO[Unit] { ioPure(unit()) }
`,
		"model/model.gfx": "package model\ntype Remote = Remote { value: Int }\nfunc secret() Int { 1 }\n",
	})
	generated, err := Generate(program, "core")
	if err != nil {
		t.Fatal(err)
	}
	exports := string(generated["gofx_export.go"])
	for _, absent := range []string{"type Unused ", "func Secret("} {
		if strings.Contains(exports, absent) {
			t.Errorf("unreachable/dependency export %q", absent)
		}
	}
	consumer := `package main
import (
 "context"
 "errors"
 "fmt"
 "math"
 "reflect"
 "mirrorcheck/core"
 "gofx/rt"
)
func equal(a, b any) { if !reflect.DeepEqual(a, b) { panic(fmt.Sprintf("round trip: %#v != %#v", a, b)) } }
func main() {
 tree := core.TreeBranch{F0: core.TreeLeaf{F0: 7}, F1: core.TreeLeaf{F0: 9}}
 equal(core.EchoTree(tree), tree)
 equal(core.EchoEvent(core.EventMessage{Text: "hello"}), core.EventMessage{Text: "hello"})
 equal(core.EchoEvent(core.EventEmpty{}), core.EventEmpty{})
 equal(core.EchoBox(core.Box{F0: 3}), core.Box{F0: 3})
 equal(core.Remote(core.ModelRemote{Value: 11}), core.ModelRemote{Value: 11})
 equal(core.EchoAlias(rt.Some([]int64{1, 2})), rt.Some([]int64{1, 2}))
 equal(core.EchoAlias(rt.None[[]int64]()), rt.None[[]int64]())
 ok := rt.Result[string, rt.Option[int64]]{Ok: rt.Some(int64(4)), IsOk: true}
 equal(core.Result(ok), ok)
 bad := rt.Result[string, rt.Option[int64]]{Err: "bad"}
 equal(core.Result(bad), bad)
 input := map[string][]int64{"z": {3}, "a": {1, 2}}
 copied := core.Strings(input)
 equal(copied, input)
 copied["a"][0] = 9; copied["new"] = nil
 if input["a"][0] != 1 || len(input) != 2 { panic("map/list aliasing") }
 equal(core.Strings(nil), map[string][]int64{})
 equal(core.Ints(map[int64]bool{3: true, -2: false}), map[int64]bool{3: true, -2: false})
 equal(core.Bools(map[bool]int64{true: 1, false: 2}), map[bool]int64{true: 1, false: 2})
 equal(core.Floats(map[float64]string{-1.5: "a", 2.5: "b"}), map[float64]string{-1.5: "a", 2.5: "b"})
 nan := core.Floats(map[float64]string{math.NaN(): "nan"}); if len(nan) != 1 { panic("NaN lost") }; for key, value := range nan { if !math.IsNaN(key) || value != "nan" { panic("NaN entry") } }
 func() { defer func() { if recover() != "gofx: duplicate NaN map key" { panic("wrong NaN collision panic") } }(); core.Floats(map[float64]string{math.NaN(): "one", math.NaN(): "two"}) }()
 set := map[string]struct{}{"b": {}, "a": {}}
 out := core.Set(set); equal(out, set); delete(out, "a"); if len(set) != 2 { panic("set aliasing") }
 equal(core.EchoTuple2(rt.Tuple2[int64, string]{F0: 7, F1: "x"}), rt.Tuple2[int64, string]{F0: 7, F1: "x"})
 equal(core.EchoTuple3(rt.Tuple3[int64, int64, int64]{F2: 3}).F2, int64(3))
 equal(core.EchoTuple4(rt.Tuple4[int64, int64, int64, int64]{F3: 4}).F3, int64(4))
 equal(core.EchoTuple5(rt.Tuple5[int64, int64, int64, int64, int64]{F4: 5}).F4, int64(5))
 equal(core.EchoTuple6(rt.Tuple6[int64, int64, int64, int64, int64, int64]{F5: 6}).F5, int64(6))
 equal(core.EchoTuple7(rt.Tuple7[int64, int64, int64, int64, int64, int64, int64]{F6: 7}).F6, int64(7))
 equal(core.EchoTuple8(rt.Tuple8[int64, int64, int64, int64, int64, int64, int64, int64]{F7: 8}).F7, int64(8))
 equal(core.Number(4), int64(5)); equal(core.Text("text"), "text"); equal(core.Truth(true), true); equal(core.Decimal(1.25), 1.25); equal(core.UnitValue(rt.Unit{}), rt.Unit{})
 value, err := core.Later(tree)(context.Background()); if err != nil { panic(err) }; equal(value, tree)
 _, err = core.Failed()(context.Background()); var failure *rt.Failure; if !errors.As(err, &failure) || err.Error() != "failed export" { panic("failure type/message") }
 ctx, cancel := context.WithCancel(context.Background()); cancel(); _, err = core.Later(tree)(ctx); if !errors.Is(err, context.Canceled) { panic("context lost") }
 func() { defer func() { if recover() != "invalid constructor" { panic("wrong invalid constructor panic") } }(); core.EchoTree(nil) }()
 fmt.Println("mirrors ok")
}
`
	runMirrorConsumer(t, generated, consumer)
}

func TestGoMirrorString(t *testing.T) {
	program := mirrorProgram(t, map[string]string{
		"main.gfx": `package main
import "model"
type Step = Cleanup(Int) | Skip | Named { label: String } deriving Debug
type Record = Record { names: List[String], step: Step } deriving Debug
type Box = Box(Int) deriving Debug
type Custom = Custom { secret: String }
instance Debug[Custom] { debug(c) { "custom:" + c.secret } }
type PlainRecord = PlainRecord { string: String }
type PlainBox = PlainBox(Int) deriving Show
type PlainSum = Detail(String) | NoDetails
type Phase = Pending | Ready
type CustomPhase = First | Last
instance Debug[CustomPhase] { debug(_p) { "custom phase" } }
func describeStep(s Step) String { debug(s) }
func describeRecord(r Record) String { debug(r) }
func describeBox(b Box) String { debug(b) }
func describeCustom(c Custom) String { debug(c) }
func describeRemote(r model.Remote) String { debug(r) }
func echoPlainRecord(r PlainRecord) PlainRecord { r }
func echoPlainBox(b PlainBox) PlainBox { b }
func echoPlainSum(s PlainSum) PlainSum { s }
func echoPhase(p Phase) Phase { p }
func describeCustomPhase(p CustomPhase) String { debug(p) }
func main() IO[Unit] { ioPure(unit()) }
`,
		"model/model.gfx": "package model\ntype Remote = Remote { value: Int } deriving Debug\n",
	})
	generated, err := Generate(program, "core")
	if err != nil {
		t.Fatal(err)
	}
	runMirrorConsumer(t, generated, `package main
import (
 "fmt"
 "mirrorcheck/core"
)
func equal(got, want string) { if got != want { panic(fmt.Sprintf("String: %q != %q", got, want)) } }
func main() {
 equal(fmt.Sprint(core.StepCleanup{F0: 7}), "Cleanup(7)")
 for _, step := range []core.Step{core.StepCleanup{F0: 7}, core.StepSkip{}, core.StepNamed{Label: "quoted\ntext"}} {
  equal(fmt.Sprint(step), core.DescribeStep(step))
 }
 record := core.Record{Names: []string{"cache", "secrets"}, Step: core.StepCleanup{F0: 7}}
 equal(fmt.Sprint(record), core.DescribeRecord(record))
 equal(fmt.Sprint(record), "Record{names: [\"cache\", \"secrets\"], step: Cleanup(7)}")
 box := core.Box{F0: 9}
 equal(fmt.Sprint(box), core.DescribeBox(box))
 equal(fmt.Sprint(box), "Box(9)")
 custom := core.Custom{Secret: "hidden"}
 equal(fmt.Sprint(custom), core.DescribeCustom(custom))
 equal(fmt.Sprint(custom), "custom:hidden")
 remote := core.ModelRemote{Value: 11}
 equal(fmt.Sprint(remote), core.DescribeRemote(remote))
 equal(fmt.Sprint(remote), "Remote{value: 11}")
 for _, plain := range []any{core.PlainRecord{String: "value"}, core.PlainBox{F0: 1}, core.PlainSumDetail{F0: "value"}, core.PlainSumNoDetails{}} {
  if _, ok := plain.(fmt.Stringer); ok { panic("String method without Debug") }
 }
 equal(fmt.Sprint(core.PhaseReady), "Ready")
 equal(fmt.Sprint(core.CustomPhaseLast), "Last")
 equal(core.DescribeCustomPhase(core.CustomPhaseLast), "custom phase")
 fmt.Println("mirrors ok")
}
`)
}

func runMirrorConsumer(t *testing.T, generated map[string][]byte, consumer string) {
	t.Helper()
	dir := t.TempDir()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, source := range generated {
		formatted, err := format.Source(source)
		if err != nil || !bytes.Equal(source, formatted) {
			t.Fatalf("%s not formatted: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "core", name), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, source := range map[string]string{
		"go.mod": "module mirrorcheck\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => " + root + "\n",
		"main.go": consumer,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"run", "."}, {"vet", "./core"}} {
		command := exec.Command("go", args...)
		command.Dir = dir
		out, err := command.CombinedOutput()
		if err != nil || args[0] == "run" && string(out) != "mirrors ok\n" {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
	}
}

func TestGoExportEligibility(t *testing.T) {
	program := mirrorProgram(t, map[string]string{"main.gfx": `package main
type Generic[A] = Generic(A)
func generic[A](a A) A { a }
func constrained[A Eq](a A) Bool { eq(a, a) }
func parameterised(a Generic[Int]) Generic[Int] { a }
func callback(f func(Int) Int) Int { f(1) }
func returned() func(Int) Int { func(x) => x }
func action(a IO[Int]) IO[Int] { a }
func nested(a Option[IO[Int]]) Option[IO[Int]] { a }
func keys(a Map[(Int, Int), Int]) Map[(Int, Int), Int] { a }
func setKeys(a Set[List[Int]]) Set[List[Int]] { a }
func array(a Array[Int]) Array[Int] { a }
extern func foreign(s String) String = "strings".ToUpper
func _private(n Int) Int { n }
func visible(n Int) Int { n }
func main() IO[Unit] { ioPure(unit()) }
`})
	generated, err := Generate(program, "core")
	if err != nil {
		t.Fatal(err)
	}
	source := string(generated["gofx_export.go"])
	for _, want := range []string{
		"generic (type parameters)", "constrained (type parameters)",
		"callback (parameter 1: function type)", "returned (result: function type)",
		"action (parameter 1: IO is only supported as the outer result)",
		"nested (parameter 1: IO is only supported as the outer result)",
		"keys (parameter 1: Map requires a scalar key)", "setKeys (parameter 1: Set requires a scalar key)",
		"array (parameter 1: unsupported type Array)", "foreign (extern function)",
	} {
		if strings.Count(source, "// not exported: "+want) != 1 {
			t.Errorf("missing unique skip %q:\n%s", want, source)
		}
	}
	if !strings.Contains(source, "func Parameterised(p0 GenericInt) GenericInt") {
		t.Fatalf("fully applied data type not exported:\n%s", source)
	}
	if strings.Contains(source, "private") || !strings.Contains(source, "func Visible(") || strings.Contains(source, "type Generic ") {
		t.Fatalf("eligibility:\n%s", source)
	}
	ordinary, err := Generate(program, "main")
	if err != nil || ordinary["gofx_export.go"] != nil {
		t.Fatalf("main package has export file: %v", err)
	}
}

func TestGoExportNameErrors(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"fields", "type Record = Record { s: Int, ſ: Int }\nfunc echo(r Record) Record { r }", "duplicate exported field S in Record"},
		{"String field", "type Record = Record { string: Int } deriving Debug\nfunc echo(r Record) Record { r }", "exported field String conflicts with String method in Record"},
		{"sum String field", "type Event = Message { string: Int } | Empty deriving Debug\nfunc echo(e Event) Event { e }", "exported field String conflicts with String method in Event"},
		{"function type", "type Box = Box(Int)\nfunc box(b Box) Box { b }", "Go export name collision: Box"},
		{"functions", "func s() Int { 1 }\nfunc ſ() Int { 2 }", "Go export name collision: S"},
		{"recursive value", "type Chain = Chain(Option[Chain])\nfunc echo(c Chain) Chain { c }", "recursive Go value type Chain"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			program := mirrorProgram(t, map[string]string{"main.gfx": "package main\n" + tt.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n"})
			_, err := Generate(program, "core")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want %s", err, tt.want)
			}
		})
	}
	program := mirrorProgram(t, map[string]string{"main.gfx": "package main\ntype Box = Box(Int)\nfunc echo(b Box) Box { b }\nfunc main() IO[Unit] { ioPure(unit()) }\n"})
	program.Modules = nil
	_, err := Generate(program, "core")
	if err == nil || !strings.Contains(err.Error(), "Go export name collision: Box") {
		t.Fatalf("no-module collision = %v", err)
	}
}

func TestGoParametricMirrors(t *testing.T) {
	program := mirrorProgram(t, map[string]string{"main.gfx": `package main
type Plan = Plan { generation: Int } deriving Eq, Debug
type Fetched[A] = NotReferenced | ReadError | Missing | Found(A) deriving Eq, Debug
type Pair[A, B] = Pair { first: A, second: B } deriving Debug
type Box[A] = Box(A) deriving Debug
type Tree[A] = Leaf(A) | Branch(List[Tree[A]]) deriving Debug
type Phase[A] = Pending | Ready
type Plain = Plain(Int)
type Fixed[A] = Fixed(A)
instance Debug[Fixed[Int]] { debug(_v) { "fixed int" } }
alias PlanAlias = Plan
alias Plans = Fetched[PlanAlias]
func echoFetched(v Fetched[Plan]) Fetched[Plan] { v }
func echoAlias(v Plans) Plans { v }
func describeFetched(v Fetched[Plan]) String { debug(v) }
func echoPair(v Pair[Int, Option[Bool]]) Pair[Int, Option[Bool]] { v }
func describePair(v Pair[Int, Option[Bool]]) String { debug(v) }
func echoStrings(v Fetched[List[String]]) Fetched[List[String]] { v }
func describeStrings(v Fetched[List[String]]) String { debug(v) }
func echoBoxes(v Box[Box[Int]]) Box[Box[Int]] { v }
func describeBoxes(v Box[Box[Int]]) String { debug(v) }
func echoTree(v Tree[Int]) Tree[Int] { v }
func describeTree(v Tree[Int]) String { debug(v) }
func echoPhase(v Phase[Int]) Phase[Int] { v }
func echoPlain(v Fetched[Plain]) Fetched[Plain] { v }
func describeFixed(v Fixed[Int]) String { debug(v) }
func echoUnmatched(v Fixed[Bool]) Fixed[Bool] { v }
func identity[A](a A) A { a }
func main() IO[Unit] { ioPure(unit()) }
`})
	generated, err := Generate(program, "core")
	if err != nil {
		t.Fatal(err)
	}
	exports := string(generated["gofx_export.go"])
	for _, want := range []string{
		"type FetchedPlan interface", "type FetchedPlanFound struct",
		"type FetchedListString interface", "type PairIntOptionBool struct",
		"func toGo_FetchedPlan(", "func fromGo_FetchedPlan(",
		"// not exported: identity (type parameters)",
	} {
		if strings.Count(exports, want) != 1 {
			t.Errorf("missing unique declaration %q:\n%s", want, exports)
		}
	}
	runMirrorConsumer(t, generated, `package main
import (
 "fmt"
 "reflect"
 "mirrorcheck/core"
 "gofx/rt"
)
func equal(a, b any) { if !reflect.DeepEqual(a, b) { panic(fmt.Sprintf("round trip: %#v != %#v", a, b)) } }
func main() {
 for _, value := range []core.FetchedPlan{core.FetchedPlanNotReferenced{}, core.FetchedPlanReadError{}, core.FetchedPlanMissing{}, core.FetchedPlanFound{F0: core.Plan{Generation: 7}}} {
  equal(core.EchoFetched(value), value)
  equal(core.EchoAlias(value), value)
  equal(fmt.Sprint(value), core.DescribeFetched(value))
 }
 for _, option := range []rt.Option[bool]{rt.Some(true), rt.None[bool]()} {
  pair := core.PairIntOptionBool{First: 7, Second: option}
  equal(core.EchoPair(pair), pair)
  equal(fmt.Sprint(pair), core.DescribePair(pair))
 }
 strings := core.FetchedListStringFound{F0: []string{"quoted\ntext", "next"}}
 copied := core.EchoStrings(strings).(core.FetchedListStringFound)
 equal(copied, strings)
 equal(fmt.Sprint(strings), core.DescribeStrings(strings))
 copied.F0[0] = "changed"
 equal(strings.F0[0], "quoted\ntext")
 boxes := core.BoxBoxInt{F0: core.BoxInt{F0: 9}}
 equal(core.EchoBoxes(boxes), boxes)
 equal(fmt.Sprint(boxes), core.DescribeBoxes(boxes))
 tree := core.TreeIntBranch{F0: []core.TreeInt{core.TreeIntLeaf{F0: 3}, core.TreeIntBranch{}}}
 equal(core.EchoTree(tree), tree)
 equal(fmt.Sprint(tree), core.DescribeTree(tree))
 equal(core.EchoPhase(core.PhaseIntReady), core.PhaseIntReady)
 equal(fmt.Sprint(core.PhaseIntReady), "Ready")
 plain := core.FetchedPlainFound{F0: core.Plain{F0: 5}}
 equal(core.EchoPlain(plain), plain)
 fixed := core.FixedInt{F0: 4}
 equal(fmt.Sprint(fixed), core.DescribeFixed(fixed))
 equal(fmt.Sprint(fixed), "fixed int")
 unmatched := core.FixedBool{F0: true}
 equal(core.EchoUnmatched(unmatched), unmatched)
 for _, value := range []any{plain, unmatched} {
  if _, ok := value.(fmt.Stringer); ok { panic("String without a resolvable Debug instance") }
 }
 fmt.Println("mirrors ok")
}
`)
}

func TestGoParametricMirrorErrors(t *testing.T) {
	for _, tt := range []struct {
		name, source, want string
		line               int
	}{
		{"unreachable declaration clash", "type Fetched[A] = Missing | Found(A)\ntype FetchedInt = Declared\nfunc echo(v Fetched[Int]) Fetched[Int] { v }", "Go export name collision: FetchedInt", 3},
		{"reachable declaration clash", "type FetchedInt = Declared\ntype Fetched[A] = Missing | Found(A)\nfunc echo(v Fetched[Int]) Fetched[Int] { v }\nfunc declared(v FetchedInt) FetchedInt { v }", "Go export name collision: FetchedInt", 2},
		{"recursive instance", "type Chain[A] = Chain(A, Option[Chain[A]])\nfunc echo(v Chain[Int]) Chain[Int] { v }", "recursive Go value type ChainInt", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			program := mirrorProgram(t, map[string]string{"main.gfx": "package main\n" + tt.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n"})
			_, err := Generate(program, "core")
			diagnostic, ok := err.(*types.Error)
			if !ok || diagnostic.Msg != tt.want || diagnostic.Pos.Line != tt.line || filepath.Base(diagnostic.File) != "main.gfx" {
				t.Fatalf("error=%v, want main.gfx:%d: %s", err, tt.line, tt.want)
			}
		})
	}
}

func TestGoParametricMirrorEligibility(t *testing.T) {
	program := mirrorProgram(t, map[string]string{"main.gfx": `package main
type Phantom[A] = Phantom
type Handler[A] = Handler(func(A) A)
func phantom(v Phantom[Array[Int]]) Phantom[Array[Int]] { v }
func handler(v Handler[Int]) Handler[Int] { v }
func main() IO[Unit] { ioPure(unit()) }
`})
	generated, err := Generate(program, "core")
	if err != nil {
		t.Fatal(err)
	}
	exports := string(generated["gofx_export.go"])
	for _, want := range []string{
		"// not exported: phantom (parameter 1: unsupported type Array)",
		"// not exported: handler (parameter 1: Handler: function type)",
	} {
		if !strings.Contains(exports, want) {
			t.Errorf("missing skip %q:\n%s", want, exports)
		}
	}
}
