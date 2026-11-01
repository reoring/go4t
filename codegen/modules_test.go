package codegen

import (
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "gofx/modules"
 "gofx/types"
)

func TestGenerateFilesAndRuntimeShapes(t *testing.T) {
 loaded,err:=modules.Load([]string{"../examples/modules"});if err!=nil{t.Fatal(err)}
 program,err:=types.Check(loaded);if err!=nil{t.Fatal(err)}
 generated,err:=Generate(program,"main");if err!=nil{t.Fatal(err)}
 if len(generated)!=len(loaded){t.Fatalf("got %d files for %d modules",len(generated),len(loaded))}
 geometry:=string(generated["geometry.go"])
 if !strings.Contains(geometry,"func geometry_squaredDistance_")||!strings.Contains(geometry,"func geometry_translate_")||strings.Contains(geometry,"func main()") {t.Fatalf("geometry ownership:\n%s",geometry)}
 main:=string(generated["main.go"])
 for _,text:=range []string{"rt.Shapes = rt.ShapeFuncs", "list_List_Cons", "prelude_Option_Some", "result_Result_Ok", "Tuple2", "geometry_squaredDistance_"} {if !strings.Contains(main,text){t.Errorf("entry missing %q",text)}}
 dir:=t.TempDir();root,err:=filepath.Abs("..");if err!=nil{t.Fatal(err)}
 generated["go.mod"]=[]byte("module moduletest\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => "+root+"\n")
 for name,source:=range generated {if err:=os.WriteFile(filepath.Join(dir,name),source,0644);err!=nil{t.Fatal(err)}}
 command:=exec.Command("go","run",".");command.Dir=dir
 output,err:=command.CombinedOutput();if err!=nil{t.Fatalf("generated modules: %v\n%s",err,output)}
 want:="point: (3, 4)\nsquared distance: 25\ntranslated: Point{x: 4, y: 6}\nlocal map: 42\n"
 if string(output)!=want{t.Fatalf("output = %q, want %q",output,want)}
}
