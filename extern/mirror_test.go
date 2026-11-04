package extern

import (
	"strings"
	"testing"
)

func TestForeignMirrors(t *testing.T) {
	var loader Loader
	pkg, err := loader.Load("go4t/extern/testdata/mirrorfixture")
	if err != nil {
		t.Fatal(err)
	}
	source, shim, _, err := BindWithShim(pkg, BindOptions{Float: true}, "fixture/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`import dyn "std/dyn"`,
		"type Point = Point{x: Int, y: Int} deriving Eq, Debug",
		"type Record = Record{enabled: Bool, name: String, point: Point, maybe: Option[Point], wait: Int, at: Int, bytes: String, err: Option[String], labels: Map[String, Int]} deriving Eq, Debug",
		"type Counter = CounterHandle", "type Opaque = OpaqueHandle", "type Stateful = StatefulHandle",
		"type Labelled = Labelled{type_: String, match_: Int} deriving Eq, Debug", "type Reserved = ReservedHandle",
		"extern func echoRecord(p0 Record) Record", "extern func echoPointer(p0 Option[Point]) Option[Point]",
		"extern func echoDuration(p0 Int) Int", "extern func echoTime(p0 Int) Int", "extern func echoBytes(p0 String) String",
		"extern func echoCount(p0 Int) Int", "extern func echoMap(p0 Map[Int, List[Point]]) Map[Int, List[Point]]",
		"extern func withContext(p0 Record) IO[Record]", "extern func contextOnly() IO[String]", "extern func errorOnly(p0 Bool) IO[Unit]",
		"extern func variadic(p0 String, p1 List[Int]) Int", "extern func dynamic(p0 List[dyn.Value]) String", "extern func dynamicResult() dyn.Value",
		"extern func pureCallback(p0 Point, p1 func(Point) Point) Point", "extern func iOCallback(p0 Point, p1 func(Point) IO[Point]) IO[Point]",
		"extern func errorCallback(p0 func() IO[Unit]) IO[Unit]", "extern func voidCallback(p0 func() Unit) Unit",
		"extern func counterScan(recv Counter, count Int) IO[List[dyn.Value]]", "extern func counterAdd(recv Counter, p0 Int) IO[Int]",
		"// skipped: GenericIdentity (generic function)", "extern func unsupported(p0 Chan[Int]) IO[Int]",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("missing %q:\n%s", want, source)
		}
	}
	if strings.Count(source, "type Point =") != 1 {
		t.Errorf("duplicate Point declaration:\n%s", source)
	}
	if strings.Contains(source, "hidden") {
		t.Errorf("private field leaked:\n%s", source)
	}
	for _, want := range []string{"rt.IO(func(ctx context.Context)", "rt.ForeignRecord(", "rt.OptionValue(", "rt.DynListToGo(", "rt.DynListFromGo(", "value.(rt.IO)(a0)", "dest[i] = &values[i]"} {
		if !strings.Contains(string(shim), want) {
			t.Errorf("missing shim conversion %q:\n%s", want, shim)
		}
	}
}
