package rt

import (
	"math"
	"strings"
	"testing"
)

type testJSON struct {
	kind  string
	value any
}

func jsonShapes(t *testing.T) {
	collectionShapes(t)
	Shapes.JNull = func() any { return testJSON{kind: "null"} }
	Shapes.JBool = func(v any) any { return testJSON{"bool", v} }
	Shapes.JInt = func(v any) any { return testJSON{"integer", v} }
	Shapes.JNum = func(v any) any { return testJSON{"number", v} }
	Shapes.JStr = func(v any) any { return testJSON{"string", v} }
	Shapes.JArr = func(v any) any { return testJSON{"array", v} }
	Shapes.JObj = func(v any) any { return testJSON{"object", v} }
	Shapes.JsonView = func(v any) (string, any) { j := v.(testJSON); return j.kind, j.value }
}

func TestJSONParsePrint(t *testing.T) {
	jsonShapes(t)
	for _, tc := range []struct{ input, want string }{
		{`null`, `null`}, {`true`, `true`}, {`false`, `false`},
		{`-12.5e+2`, `-1250`}, {`1E-3`, `0.001`}, {`-0`, `0`}, {`-0.0`, `-0`},
		{`"a\n\t\r\b\f\\\"/"`, `"a\n\t\r\b\f\\\"/"`},
		{`"\u65e5\ud83d\ude00"`, `"日😀"`},
		{` {"z":[1,{"b":false,"a":null}],"a":"é"} `, `{"z":[1,{"b":false,"a":null}],"a":"é"}`},
		{`{"b":1,"a":2,"b":3}`, `{"b":3,"a":2}`},
		{`[]`, `[]`}, {`{}`, `{}`},
	} {
		t.Run(tc.input, func(t *testing.T) {
			result, ok := ParseJson(tc.input).(testOk)
			if !ok {
				t.Fatalf("parse failed: %#v", ParseJson(tc.input))
			}
			if got := ToJson(result.value); got != tc.want {
				t.Fatalf("print = %q, want %q", got, tc.want)
			}
		})
	}
	j := ParseJson(`{"z":[1,true],"a":{}}`).(testOk).value
	if got, want := ToJsonPretty(j), "{\n  \"z\": [\n    1,\n    true\n  ],\n  \"a\": {}\n}"; got != want {
		t.Fatalf("pretty = %q, want %q", got, want)
	}
}

func TestJSONExactIntegers(t *testing.T) {
	jsonShapes(t)
	for _, tc := range []struct {
		text  string
		value int64
	}{
		{"9007199254740993", 9007199254740993},
		{"-9223372036854775808", math.MinInt64},
		{"9223372036854775807", math.MaxInt64},
		{"0", 0},
		{"-1", -1},
	} {
		t.Run(tc.text, func(t *testing.T) {
			want := testJSON{"integer", tc.value}
			result, ok := ParseJson(tc.text).(testOk)
			if !ok || result.value != want {
				t.Fatalf("parse = %#v, want Ok(%#v)", result, want)
			}
			encoded := Shapes.JInt(tc.value)
			if got := ToJson(encoded); got != tc.text {
				t.Fatalf("print = %q, want %q", got, tc.text)
			}
			if got := ToJsonPretty(encoded); got != tc.text {
				t.Fatalf("pretty = %q, want %q", got, tc.text)
			}
			if got := ParseJson(ToJson(encoded)); got != (testOk{want}) {
				t.Fatalf("round trip = %#v, want Ok(%#v)", got, want)
			}
		})
	}
}

func TestJSONFloatLiteralFallback(t *testing.T) {
	jsonShapes(t)
	for _, tc := range []struct {
		text  string
		value float64
		intOK bool
	}{
		{"1e3", 1000, true},
		{"1E3", 1000, true},
		{"1.0", 1, true},
		{"1.5", 1.5, false},
		{"9007199254740992.0", 9007199254740992, true},
		{"-9007199254740992.0", -9007199254740992, true},
		{"9007199254740994.0", 9007199254740994, false},
		{"-9007199254740994.0", -9007199254740994, false},
		{"9223372036854775808", 9223372036854775808.0, false},
		{"-9223372036854775809", -9223372036854775808.0, false},
		{"-9223372036854775808.0", -9223372036854775808.0, false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			result, ok := ParseJson(tc.text).(testOk)
			if want := (testJSON{"number", tc.value}); !ok || result.value != want {
				t.Fatalf("parse = %#v, want Ok(%#v)", result, want)
			}
			want := any(testErr{"expected Int at $"})
			if tc.intOK {
				want = testOk{int64(tc.value)}
			}
			if got := JsonInt(tc.value); got != want {
				t.Fatalf("Float to Int = %#v, want %#v", got, want)
			}
		})
	}
}

func TestJSONParseErrors(t *testing.T) {
	jsonShapes(t)
	for _, input := range []string{"", " ", "true false", "null x", "[1,]", "{", "01", "NaN", "1e400", `{"x":}`, `"\x00"`} {
		if result, ok := ParseJson(input).(testErr); !ok || !strings.HasPrefix(result.value.(string), "invalid JSON: ") {
			t.Errorf("parse %q = %#v", input, ParseJson(input))
		}
	}
}

func TestJSONMapOrdering(t *testing.T) {
	jsonShapes(t)
	j := ParseJson(`{"z":1,"a":2}`).(testOk).value
	_, object := Shapes.JsonView(j)
	if got := collectionValues(MapKeys(object)); len(got) != 2 || got[0] != "a" || got[1] != "z" {
		t.Fatalf("map key order = %v", got)
	}
	// A normal Map value has sorted encoding, even when decoded from an ordered object.
	sorted := MapMap(object, func(v any) any { return v })
	if got := ToJson(Shapes.JObj(sorted)); got != `{"a":2,"z":1}` {
		t.Fatalf("map encoding = %s", got)
	}
	if got := ToJson(j); got != `{"z":1,"a":2}` {
		t.Fatalf("original mutated: %s", got)
	}
	reversed := MapFromList(MapToList(object), func(a, b any) any {
		return collectionOrdering{strings.Compare(b.(string), a.(string))}
	})
	if got := ToJson(Shapes.JObj(reversed)); got != `{"a":2,"z":1}` {
		t.Fatalf("custom-comparator map encoding = %s", got)
	}
}

func TestJSONMapNoopUpdatesDiscardOrder(t *testing.T) {
	jsonShapes(t)
	j := ParseJson(`{"z":1,"a":2}`).(testOk).value
	_, object := Shapes.JsonView(j)
	compare := func(a, b any) any {
		return collectionOrdering{strings.Compare(a.(string), b.(string))}
	}
	for name, value := range map[string]any{
		"delete absent": MapDelete(object, "absent", compare),
		"empty union":   MapUnion(object, EmptyMap(), compare),
		"self union":    MapUnion(object, object, compare),
	} {
		if got := ToJson(Shapes.JObj(value)); got != `{"a":2,"z":1}` {
			t.Errorf("%s retained source order: %s", name, got)
		}
	}
	if got := ToJson(j); got != `{"z":1,"a":2}` {
		t.Fatalf("no-op updates mutated source order: %s", got)
	}
}

func TestJSONNumberAndPathBoundaries(t *testing.T) {
	jsonShapes(t)
	for _, tc := range []struct {
		value float64
		text  string
	}{
		{38495, "38495"},
		{1e-6, "0.000001"},
		{1e-7, "1e-7"},
		{1e20, "100000000000000000000"},
		{1e21, "1e+21"},
		{math.Copysign(0, -1), "-0"},
	} {
		if got := ToJson(Shapes.JNum(tc.value)); got != tc.text {
			t.Fatalf("Float JSON encoding = %v, want %s", got, tc.text)
		}
	}
	for _, n := range []float64{1.25, math.Inf(1), math.Inf(-1), math.NaN(), 9007199254740994, -9007199254740994, -9223372036854775808.0, 9223372036854775808.0} {
		if got := JsonInt(n); got != (testErr{"expected Int at $"}) {
			t.Fatalf("int(%g) = %#v", n, got)
		}
	}
	for _, n := range []float64{-9007199254740992, 9007199254740992, -1, 0, 42} {
		if got := JsonInt(n); got != (testOk{int64(n)}) {
			t.Fatalf("int(%g) = %#v", n, got)
		}
	}
	if got := JsonErrorAt("expected String at $.name", ".items[2]"); got != "expected String at $.items[2].name" {
		t.Fatal(got)
	}
	if got := JsonErrorAt(`expected Int at $["x at $"]`, ".parent"); got != `expected Int at $.parent["x at $"]` {
		t.Fatal(got)
	}
	if got := JsonFieldPath("a.b"); got != `["a.b"]` {
		t.Fatal(got)
	}
	defer func() {
		if got := recover(); got != "cannot encode non-finite JSON number" {
			t.Fatalf("nonfinite panic = %v", got)
		}
	}()
	ToJson(Shapes.JNum(math.Inf(1)))
}
