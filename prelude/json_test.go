package prelude

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
	"gofx/parser"
	"gofx/types"
)

func TestJSONInstancesAndDeriving(t *testing.T) {
	checks := []struct{ name, expr string }{
		{"Int", `roundTrip(-42) && roundTrip(0) && roundTrip(9007199254740991)`},
		{"Float", `roundTrip(1.25) && roundTrip(-2e-20)`},
		{"String", `roundTrip("日😀\n\"\\") && roundTrip("")`},
		{"Bool", `roundTrip(true) && roundTrip(false)`},
		{"Unit", `roundTrip(unit()) && toJson(encode(unit())) == "null"`},
		{"Option", `roundTrip(Some("x")) && roundTrip(none) && toJson(encode(none)) == "null"`},
		{"List", `roundTrip([1, 2, 3]) && roundTrip(empty)`},
		{"Array", `roundTrip(arrayFromList([true, false])) && roundTrip(arrayFromList(empty))`},
		{"Map", `roundTrip(mapFromList([("z", 1), ("a", 2)])) && roundTrip(emptyMapValue)`},
		{"Map sorts", `toJson(encode(mapFromList([("z", 1), ("a", 2)]))) == "{\"a\":2,\"z\":1}"`},
		{"pair", `roundTrip((1, "x"))`},
		{"tuple arities", `serialRoundTrip((1, "x", true)) && serialRoundTrip((1, "x", true, 1.5, unit(), Some(2), [3]))`},
		{"Json", `serialRoundTrip(json) && toJson(encode(json)) == "{\"z\":null,\"a\":[true,2.5,\"é\"]}"`},
		{"record order", `toJson(encode(Item("x", 2, 1.5))) == "{\"sku\":\"x\",\"qty\":2,\"price\":1.5}"`},
		{"record", `roundTrip(Item("x", 2, 1.5))`},
		{"generic recursive", `roundTrip(Fork(1, [Tip, Fork(2, [])]))`},
		{"generic record", `roundTrip(Box(Some(1), [(true, "x")]))`},
		{"nullary", `roundTrip(Pending) && toJson(encode(Pending)) == "{\"$tag\":\"Pending\"}"`},
		{"positional", `roundTrip(Shipped("track")) && toJson(encode(Shipped("track"))) == "{\"$tag\":\"Shipped\",\"$values\":[\"track\"]}"`},
		{"sum record", `roundTrip(Named("x")) && toJson(encode(Named("x"))) == "{\"$tag\":\"Named\",\"name\":\"x\"}"`},
		{"root error", `intResult(JStr("x")) == Err("expected Int at $")`},
		{"fraction error", `intResult(JNum(1.5)) == Err("expected Int at $")`},
		{"option error", `optionResult(JNum(1.0)) == Err("expected String at $")`},
		{"nested path", `errorOf(orderResult(badOrder)) == "expected String at $.items[2].sku"`},
		{"array path", `errorOf(arrayResult(JArr([JNum(1.0), JStr("bad")]))) == "expected Int at $[1]"`},
		{"map path", `errorOf(mapResult(badMap)) == "expected Int at $[\"a.b\"]"`},
		{"pair path", `errorOf(pairResult(JArr([JNum(1.0), JBool(true)]))) == "expected String at $[1]"`},
		{"tuple length", `errorOf(pairResult(JArr([JNum(1.0)]))) == "expected array of length 2 at $"`},
		{"tuple extra", `errorOf(pairResult(JArr([JNum(1.0), JStr("x"), JNull]))) == "expected array of length 2 at $"`},
		{"tag error", `errorOf(statusResult(badTag)) == "expected Status constructor at $[\"$tag\"]"`},
		{"values path", `errorOf(statusResult(badValues)) == "expected String at $[\"$values\"][0]"`},
		{"missing field", `errorOf(itemResult(missing)) == "expected field at $.sku"`},
		{"non object", `errorOf(itemResult(JNull)) == "expected object at $"`},
		{"extra fields", `isOk(itemResult(extra))`},
	}
	var source, want strings.Builder
	source.WriteString(`package main
type Item = Item{sku: String, qty: Int, price: Float} deriving Encode, Decode, Eq
type Order = Order{items: List[Item]} deriving Encode, Decode
type Status = Pending | Shipped(String) | Named{name: String} deriving Encode, Decode, Eq
type Tree[A] = Tip | Fork(A, List[Tree[A]]) deriving Encode, Decode, Eq
type Box[A, B] = Box{value: A, values: List[B]} deriving Encode, Decode, Eq
func roundTrip[A Encode + Decode + Eq](a A) Bool {
  let result: Result[String, A] = bind(parseJson(toJson(encode(a))), decode)
  match result { Err(_) => false; Ok(b) => eq(a, b) }
}
func serialRoundTrip[A Encode + Decode](a A) Bool {
  let result: Result[String, A] = bind(parseJson(toJson(encode(a))), decode)
  match result { Err(_) => false; Ok(b) => toJson(encode(a)) == toJson(encode(b)) }
}
func errorOf[A](r Result[String, A]) String { match r { Err(e) => e; Ok(_) => "success" } }
func intResult(j Json) Result[String, Int] { decode(j) }
func optionResult(j Json) Result[String, Option[String]] { decode(j) }
func orderResult(j Json) Result[String, Order] { decode(j) }
func arrayResult(j Json) Result[String, Array[Int]] { decode(j) }
func mapResult(j Json) Result[String, Map[String, Int]] { decode(j) }
func pairResult(j Json) Result[String, (Int, String)] { decode(j) }
func statusResult(j Json) Result[String, Status] { decode(j) }
func itemResult(j Json) Result[String, Item] { decode(j) }
func main() IO[Unit] {
  let none: Option[String] = None
  let empty: List[Int] = []
  let emptyMapValue: Map[String, Int] = emptyMap()
  let json = fromResult(parseJson("{\"z\":null,\"a\":[true,2.5,\"é\"]}"), JNull)
  let badOrder = fromResult(parseJson("{\"items\":[{\"sku\":\"a\",\"qty\":1,\"price\":2},{\"sku\":\"b\",\"qty\":1,\"price\":2},{\"sku\":false}]}"), JNull)
  let badMap = fromResult(parseJson("{\"a.b\":false}"), JNull)
  let badTag = fromResult(parseJson("{\"$tag\":\"Lost\"}"), JNull)
  let badValues = fromResult(parseJson("{\"$tag\":\"Shipped\",\"$values\":[false]}"), JNull)
  let missing = fromResult(parseJson("{}"), JNull)
  let extra = fromResult(parseJson("{\"price\":2,\"extra\":true,\"qty\":1,\"sku\":\"x\"}"), JNull)
  do {
`)
	for _, check := range checks {
		fmt.Fprintf(&source, "println(%q + show(%s))\n", check.name+": ", check.expr)
		fmt.Fprintf(&want, "%s: true\n", check.name)
	}
	source.WriteString("}\n}\n")
	var files []*ast.File
	for i, text := range []string{Source, source.String()} {
		file, err := parser.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		file.Name = []string{"prelude.gfx", "json_test.gfx"}[i]
		files = append(files, file)
	}
	program, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, generated, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("JSON program: %v\n%s", err, output)
	}
	if string(output) != want.String() {
		t.Errorf("JSON checks:\n%s\nwant:\n%s", output, want.String())
	}
}
