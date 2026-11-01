package format_test

import (
	"testing"

	"gofx/format"
)

func TestModuleAndLocalDeclarationGolden(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{
			"imports and qualified names",
			"package main\n// json codecs\nimport j \"std/json\" // codecs\nimport \"model\"\nalias Box[A]=model.Box[A]\nfunc value(x model.Box[Int]) j.Json {match x {model.Box(n)=>j.encode(n)}}",
			"package main\n\n// json codecs\nimport j \"std/json\" // codecs\nimport \"model\"\n\nalias Box[A] = model.Box[A]\n\nfunc value(x model.Box[Int]) j.Json {\n  match x {\n    model.Box(n) => j.encode(n)\n  }\n}\n",
		},
		{
			"alias and local function",
			"package main;alias Number=Int;func f(n Number) Number{func increment(x Number) Number{x+1};increment(n)}",
			"package main\n\nalias Number = Int\n\nfunc f(n Number) Number {\n  func increment(x Number) Number {\n    x + 1\n  }\n  increment(n)\n}\n",
		},
		{
			"qualified class constructor and deriving",
			"package main;import j \"std/json\";import m \"model\";type Box=Box{value:m.Value} deriving j.Encode,j.Decode;instance m.Render[Box]{render(x){m.render(x.value)}}",
			"package main\n\nimport j \"std/json\"\nimport m \"model\"\n\ntype Box = Box{value: m.Value} deriving j.Encode, j.Decode\n\ninstance m.Render[Box] {\n  render(x) {\n    m.render(x.value)\n  }\n}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := format.Source([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
			assertProperties(t, tc.source)
		})
	}
}
