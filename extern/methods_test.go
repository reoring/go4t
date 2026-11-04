package extern

import (
	"go/types"
	"strings"
	"testing"
)

func TestMethodBindingBoundaries(t *testing.T) {
	var loader Loader
	pkg, err := loader.Load("go4t/extern/testdata/methodfixture")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	previous := ""
	for _, method := range pkg.Methods {
		name := method.TypeName + "." + method.Name
		if seen[name] || name < previous {
			t.Errorf("duplicate or unsorted method %s after %s", name, previous)
		}
		seen[name], previous = true, name
		if strings.HasPrefix(name, "Alias.") || strings.HasPrefix(name, "hidden.") || method.Name == "hidden" {
			t.Errorf("enumerated non-exported/nonnamed type or private method %s", name)
		}
		switch name {
		case "Widget.Count":
			if _, ok := method.Receiver.(*types.Pointer); !ok {
				t.Errorf("pointer method receiver is %s", method.Receiver)
			}
		case "Widget.Echo", "Outer.Visible":
			if _, ok := method.Receiver.(*types.Named); !ok {
				t.Errorf("value method receiver is %s", method.Receiver)
			}
		}
	}
	for _, name := range []string{"Widget.Count", "Widget.Echo", "Outer.Visible", "Generic.Name"} {
		if !seen[name] {
			t.Errorf("missing method %s", name)
		}
	}
	decls, source, _, err := BindWithShim(pkg, BindOptions{Float: true}, "example/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type Widget = Widget{name: String} deriving Eq, Debug", "type Outer = OuterHandle",
		`extern func widgetCount(recv Widget) Int = "example/shim".Widget_Count`,
		"extern func widgetEcho(recv Widget, p0 String) String",
		"extern func widgetLookup(recv Widget, p0 Bool) Option[String]",
		"extern func widgetRead(recv Widget) IO[String]",
		"extern func widgetWords(recv Widget, p0 List[String]) List[String]",
		"extern func widgetNumbers(recv Widget, p0 List[Int]) List[Int]",
		"extern func widgetAliasWords(recv Widget, p0 List[String]) List[String]",
		"extern func widgetAliasNumbers(recv Widget, p0 List[Int]) List[Int]",
		"extern func widgetUnit(recv Widget, p0 Unit) Unit",
		"extern func widgetFloat(recv Widget, p0 Float) Float",
		"// skipped: Generic.Name (generic function)",
		"// skipped: Constraint.Text (generic function)",
		"extern func widgetCallback(recv Widget, p0 func() String) String",
		"extern func widgetPointer(recv Widget, p0 Ref[String]) IO[String]",
		"extern func widgetVariadic(recv Widget, p0 List[String]) String",
		"extern func widgetVoid(recv Widget) Unit",
		"extern func widgetMultiple(recv Widget) (String, String)",
		"extern func widgetResultPointer(recv Widget) Option[Widget]",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	if strings.Count(decls, "type Widget =") != 1 {
		t.Errorf("handle declared multiple times:\n%s", decls)
	}
	for _, forbidden := range []string{"func Constraint_Text", "func Generic_Name"} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("unsupported shim %s:\n%s", forbidden, source)
		}
	}
	prefixed, _, _, err := BindWithShim(pkg, BindOptions{Prefix: "native"}, "example/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prefixed, "extern func nativeWidgetEcho(recv Widget, p0 String) String") ||
		!strings.Contains(prefixed, "// skipped: Widget.Float (unsupported parameter type float64)") {
		t.Errorf("prefix/Float options ignored:\n%s", prefixed)
	}
	// Loading methods must never change the existing function-only output.
	withMethods := Bind(pkg, BindOptions{})
	pkg.Methods = nil
	if withoutMethods := Bind(pkg, BindOptions{}); withMethods != withoutMethods {
		t.Errorf("Bind output depends on methods:\n%s", withMethods)
	}
}
