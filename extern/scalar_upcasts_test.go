package extern

import (
	"strings"
	"testing"
)

func TestScalarSourceUpcasts(t *testing.T) {
	dir := bindingTestModule(t)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("go4t/extern/testdata/scalarupcastfixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", "native"} {
		decls, shim, _, err := BindWithShim(pkg, BindOptions{Prefix: prefix, Float: true}, "example.test/bindings/shim", "shim")
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ source, receiver string }{
			{"Tag", "String"},
			{"Labels", "Map[String, String]"},
			{"Names", "List[String]"},
			{"Counter", "Int"},
			{"QTag", "String"},
			{"Enabled", "Bool"},
			{"Ratio", "Float"},
			{"Bytes", "String"},
		} {
			name, _ := bindingName(tc.source+"AsOpt", prefix)
			want := "extern func " + name + "(recv " + tc.receiver + ") Opt = \"example.test/bindings/shim\"." + tc.source + "_AsOpt"
			if !strings.Contains(decls, want) {
				t.Errorf("missing %q:\n%s", want, decls)
			}
			if strings.Contains(decls, "type "+tc.source+" =") {
				t.Errorf("scalar source gained a distinct go4t type: %s", tc.source)
			}
		}
		if strings.Contains(decls, `import go_go4t_extern_testdata_scalarupcastfixture_q `) {
			t.Errorf("scalar source added an unnecessary binding import:\n%s", decls)
		}
		if prefix == "" {
			writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
		}
	}
	writeBindingTestFile(t, dir, "shim/upcasts_test.go", scalarUpcastRuntimeTest)
	runBindingTestGo(t, dir, "test", "./shim")
}

const scalarUpcastRuntimeTest = `package shim_test
import (
 "reflect"
 "testing"
 "go4t/rt"
 fixture "go4t/extern/testdata/scalarupcastfixture"
 q "go4t/extern/testdata/scalarupcastfixture/q"
 s "example.test/bindings/shim"
)
func TestScalarUpcasts(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  Uncons: func(v any) (any,any,bool) { xs:=v.([]any); if len(xs)==0 { return nil,nil,false }; return xs[0],xs[1:],true },
 }
 labels:=rt.ForeignMap([][2]any{{"app","worker"},{"team","infra"}})
 names:=[]any{"first","second"}
 for _,tc:=range []struct { name string; upcast func(any) any; input,want any }{
  {"tag",s.Tag_AsOpt,"blue","blue"},
  {"labels",s.Labels_AsOpt,labels,map[string]string{"app":"worker","team":"infra"}},
  {"names",s.Names_AsOpt,names,[]string{"first","second"}},
  {"counter",s.Counter_AsOpt,int64(17),17},
  {"qTag",s.QTag_AsOpt,"foreign","foreign"},
  {"enabled",s.Enabled_AsOpt,true,true},
  {"ratio",s.Ratio_AsOpt,2.5,2.5},
  {"bytes",s.Bytes_AsOpt,"\x00wire",[]byte("\x00wire")},
 } {
  t.Run(tc.name,func(t *testing.T) {
   o,ok:=tc.upcast(tc.input).(fixture.Opt)
   if !ok { t.Fatal("upcast did not return Opt") }
   var kind string
   var value,applied any
   cfg:=&fixture.Cfg{}
   o.Apply(cfg)
   switch v:=o.(type) {
   case fixture.Tag: kind,value,applied="tag",string(v),cfg.Tag
   case fixture.Labels: kind,value,applied="labels",map[string]string(v),cfg.Labels
   case fixture.Names: kind,value,applied="names",[]string(v),cfg.Names
   case *fixture.Counter: kind,value,applied="counter",int(*v),cfg.Counter
   case q.QTag: kind,value,applied="qTag",string(v),cfg.Tag
   case fixture.Enabled: kind,value,applied="enabled",bool(v),cfg.Enabled
   case fixture.Ratio: kind,value,applied="ratio",float64(v),cfg.Ratio
   case fixture.Bytes: kind,value,applied="bytes",[]byte(v),cfg.Bytes
   default: t.Fatalf("unexpected named implementation %T",o)
   }
   if kind!=tc.name || !reflect.DeepEqual(value,tc.want) || !reflect.DeepEqual(applied,tc.want) {
    t.Fatalf("upcast: kind=%s value=%#v applied=%#v, want %s %#v",kind,value,applied,tc.name,tc.want)
   }
  })
 }
 firstLabels:=s.Labels_AsOpt(labels).(fixture.Labels)
 firstLabels["app"]="changed"
 if s.Labels_AsOpt(labels).(fixture.Labels)["app"]!="worker" { t.Fatal("map upcast shared mutable storage") }
 rt.ForeignMapEach(labels,func(k,v any) { if k=="app" && v!="worker" { t.Fatal("map input changed") } })
 firstNames:=s.Names_AsOpt(names).(fixture.Names)
 firstNames[0]="changed"
 if names[0]!="first" || s.Names_AsOpt(names).(fixture.Names)[0]!="first" { t.Fatal("slice upcast shared mutable storage") }
 firstCounter:=s.Counter_AsOpt(int64(17)).(*fixture.Counter)
 *firstCounter=99
 if *s.Counter_AsOpt(int64(17)).(*fixture.Counter)!=17 { t.Fatal("pointer upcast reused its box") }
 firstBytes:=s.Bytes_AsOpt("wire").(fixture.Bytes)
 firstBytes[0]='x'
 if string(s.Bytes_AsOpt("wire").(fixture.Bytes))!="wire" { t.Fatal("byte upcast shared mutable storage") }
}
`
