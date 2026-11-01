package std_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gofx/ast"
	"gofx/codegen"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestJSONInstancesAndDeriving(t *testing.T) {
	checks := []struct{ name, expr string }{
		{"Int", `roundTrip(-42) && roundTrip(0) && roundTrip(9007199254740991)`},
		{"Float", `roundTrip(1.25) && roundTrip(-2e-20)`},
		{"String", `roundTrip("日😀\n\"\\") && roundTrip("")`},
		{"Bool", `roundTrip(true) && roundTrip(false)`},
		{"Unit", `roundTrip(unit()) && codec.toJson(codec.encode(unit())) == "null"`},
		{"Option", `roundTrip(Some("x")) && roundTrip(none) && codec.toJson(codec.encode(none)) == "null"`},
		{"List", `roundTrip([1, 2, 3]) && roundTrip(empty)`},
		{"Array", `roundTrip(array.arrayFromList([true, false])) && roundTrip(array.arrayFromList(empty))`},
		{"Map", `roundTrip(map.mapFromList([("z", 1), ("a", 2)])) && roundTrip(emptyMapValue)`},
		{"Map sorts", `codec.toJson(codec.encode(map.mapFromList([("z", 1), ("a", 2)]))) == "{\"a\":2,\"z\":1}"`},
		{"pair", `roundTrip((1, "x"))`},
		{"tuple arities", `serialRoundTrip((1, "x", true)) && serialRoundTrip((1, "x", true, 1.5, unit(), Some(2), [3]))`},
		{"Json", `serialRoundTrip(json) && codec.toJson(codec.encode(json)) == "{\"z\":null,\"a\":[true,2.5,\"é\"]}"`},
		{"record order", `codec.toJson(codec.encode(Item("x", 2, 1.5))) == "{\"sku\":\"x\",\"qty\":2,\"price\":1.5}"`},
		{"record", `roundTrip(Item("x", 2, 1.5))`},
		{"generic recursive", `roundTrip(Fork(1, [Tip, Fork(2, [])]))`},
		{"generic record", `roundTrip(Box(Some(1), [(true, "x")]))`},
		{"nullary", `roundTrip(Pending) && codec.toJson(codec.encode(Pending)) == "{\"$tag\":\"Pending\"}"`},
		{"positional", `roundTrip(Shipped("track")) && codec.toJson(codec.encode(Shipped("track"))) == "{\"$tag\":\"Shipped\",\"$values\":[\"track\"]}"`},
		{"sum record", `roundTrip(Named("x")) && codec.toJson(codec.encode(Named("x"))) == "{\"$tag\":\"Named\",\"name\":\"x\"}"`},
		{"root error", `intResult(codec.JStr("x")) == Err("expected Int at $")`},
		{"fraction error", `intResult(codec.JNum(1.5)) == Err("expected Int at $")`},
		{"option error", `optionResult(codec.JNum(1.0)) == Err("expected String at $")`},
		{"nested path", `errorOf(orderResult(badOrder)) == "expected String at $.items[2].sku"`},
		{"array path", `errorOf(arrayResult(codec.JArr([codec.JNum(1.0), codec.JStr("bad")]))) == "expected Int at $[1]"`},
		{"map path", `errorOf(mapResult(badMap)) == "expected Int at $[\"a.b\"]"`},
		{"pair path", `errorOf(pairResult(codec.JArr([codec.JNum(1.0), codec.JBool(true)]))) == "expected String at $[1]"`},
		{"tuple length", `errorOf(pairResult(codec.JArr([codec.JNum(1.0)]))) == "expected array of length 2 at $"`},
		{"tuple extra", `errorOf(pairResult(codec.JArr([codec.JNum(1.0), codec.JStr("x"), codec.JNull]))) == "expected array of length 2 at $"`},
		{"tag error", `errorOf(statusResult(badTag)) == "expected Status constructor at $[\"$tag\"]"`},
		{"values path", `errorOf(statusResult(badValues)) == "expected String at $[\"$values\"][0]"`},
		{"missing field", `errorOf(itemResult(missing)) == "expected field at $.sku"`},
		{"non object", `errorOf(itemResult(codec.JNull)) == "expected object at $"`},
		{"extra fields", `result.isOk(itemResult(extra))`},
	}
	var source, want strings.Builder
	source.WriteString(`package main
import codec "std/json"
import "std/prelude"
import "std/map"
import "std/array"
import "std/result"
type Item = Item{sku: String, qty: Int, price: Float} deriving codec.Encode, codec.Decode, Eq
type Order = Order{items: List[Item]} deriving codec.Encode, codec.Decode
type Status = Pending | Shipped(String) | Named{name: String} deriving codec.Encode, codec.Decode, Eq
type Tree[A] = Tip | Fork(A, List[Tree[A]]) deriving codec.Encode, codec.Decode, Eq
type Box[A, B] = Box{value: A, values: List[B]} deriving codec.Encode, codec.Decode, Eq
func roundTrip[A codec.Encode + codec.Decode + Eq](a A) Bool {
  let result: Result[String, A] = prelude.bind(codec.parseJson(codec.toJson(codec.encode(a))), codec.decode)
  match result { Err(_) => false; Ok(b) => eq(a, b) }
}
func serialRoundTrip[A codec.Encode + codec.Decode](a A) Bool {
  let result: Result[String, A] = prelude.bind(codec.parseJson(codec.toJson(codec.encode(a))), codec.decode)
  match result { Err(_) => false; Ok(b) => codec.toJson(codec.encode(a)) == codec.toJson(codec.encode(b)) }
}
func errorOf[A](r Result[String, A]) String { match r { Err(e) => e; Ok(_) => "success" } }
func intResult(j codec.Json) Result[String, Int] { codec.decode(j) }
func optionResult(j codec.Json) Result[String, Option[String]] { codec.decode(j) }
func orderResult(j codec.Json) Result[String, Order] { codec.decode(j) }
func arrayResult(j codec.Json) Result[String, Array[Int]] { codec.decode(j) }
func mapResult(j codec.Json) Result[String, Map[String, Int]] { codec.decode(j) }
func pairResult(j codec.Json) Result[String, (Int, String)] { codec.decode(j) }
func statusResult(j codec.Json) Result[String, Status] { codec.decode(j) }
func itemResult(j codec.Json) Result[String, Item] { codec.decode(j) }
func main() IO[Unit] {
  let none: Option[String] = None
  let empty: List[Int] = []
  let emptyMapValue: Map[String, Int] = map.emptyMap()
  let json = result.fromResult(codec.parseJson("{\"z\":null,\"a\":[true,2.5,\"é\"]}"), codec.JNull)
  let badOrder = result.fromResult(codec.parseJson("{\"items\":[{\"sku\":\"a\",\"qty\":1,\"price\":2},{\"sku\":\"b\",\"qty\":1,\"price\":2},{\"sku\":false}]}"), codec.JNull)
  let badMap = result.fromResult(codec.parseJson("{\"a.b\":false}"), codec.JNull)
  let badTag = result.fromResult(codec.parseJson("{\"$tag\":\"Lost\"}"), codec.JNull)
  let badValues = result.fromResult(codec.parseJson("{\"$tag\":\"Shipped\",\"$values\":[false]}"), codec.JNull)
  let missing = result.fromResult(codec.parseJson("{}"), codec.JNull)
  let extra = result.fromResult(codec.parseJson("{\"price\":2,\"extra\":true,\"qty\":1,\"sku\":\"x\"}"), codec.JNull)
  do {
`)
	for _, check := range checks {
		fmt.Fprintf(&source, "println(%q + show(%s))\n", check.name+": ", check.expr)
		fmt.Fprintf(&want, "%s: true\n", check.name)
	}
	source.WriteString("}\n}\n")
	file, err := parser.Parse(source.String())
	if err != nil {
		t.Fatal(err)
	}
	file.Name = "json_test.gfx"
	loaded, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var paths []string
	for name, source := range generated {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, source, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", append([]string{"run"}, paths...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("JSON program: %v\n%s", err, output)
	}
	if string(output) != want.String() {
		t.Errorf("JSON checks:\n%s\nwant:\n%s", output, want.String())
	}
}
