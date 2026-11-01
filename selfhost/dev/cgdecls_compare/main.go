// Development-only declaration comparison: lower checked declaration metadata
// to Core fixtures, then compare formatted Go declaration projections. Bodies
// are deliberately outside this test; the Shape driver checks real Core bodies.
package main

import (
 "bytes"
 "fmt"
 goast "go/ast"
 "go/format"
 goparser "go/parser"
 "go/token"
 "os"
 "sort"
 "strconv"
 "strings"

 "gofx/ast"
 "gofx/parser"
 "gofx/prelude"
 "gofx/types"
)

func must(err error) { if err != nil { panic(err) } }
func q(s string) string { return strings.ReplaceAll(strconv.Quote(s), "$", "$$") }
func list(items []string) string { if len(items)==0 { return "Nil" }; return "["+strings.Join(items, ", ")+"]" }
func names(items []string) string { out:=make([]string,len(items)); for i,s:=range items {out[i]=q(s)}; return list(out) }
func constraints(cs []types.Constraint) string {
 out:=make([]string,len(cs)); for i,c:=range cs { out[i]="("+q(c.Arg.(*types.TVar).Name)+", "+q(c.Class)+")" }; return list(out)
}
func keys[V any](m map[string]V) []string { out:=make([]string,0,len(m)); for k:=range m {out=append(out,k)}; sort.Strings(out); return out }

func fixture() {
 source,err:=os.ReadFile("examples/monoid/main.gfx"); must(err)
 pf,err:=parser.Parse(prelude.Source); must(err)
 uf,err:=parser.Parse(string(source)); must(err)
 prog,err:=types.Check([]*ast.File{pf,uf}); must(err)
 var datas,classes,instances,funcs []string
 for _,n:=range keys(prog.DataTypes) {
  d:=prog.DataTypes[n]; var ctors []string
  for _,c:=range d.Ctors { ctors=append(ctors,fmt.Sprintf("CoreCtor{name: %s, arity: %d, fieldNames: %s}",q(c.Name),len(c.Fields),names(c.FieldNames))) }
  datas=append(datas,"CoreData{name: "+q(n)+", ctors: "+list(ctors)+"}")
 }
 for _,n:=range keys(prog.Classes) {
  c:=prog.Classes[n]; var methods []string
  for _,m:=range c.Methods { methods=append(methods,fmt.Sprintf("CoreMethodSig{name: %s, dictParams: %s, arity: %d}",q(m.Name),constraints(m.Scheme.Constraints),len(m.Scheme.Type.Params))) }
  super:="None"; if c.Super!="" {super="Some("+q(c.Super)+")"}
  classes=append(classes,"CoreClass{name: "+q(n)+", super: "+super+", methods: "+list(methods)+"}")
 }
 sort.Slice(prog.Instances,func(i,j int)bool { a,b:=prog.Instances[i],prog.Instances[j]; return a.Class.Name+"/"+a.TypeCon.Name < b.Class.Name+"/"+b.TypeCon.Name })
 for _,inst:=range prog.Instances {
  var methods []string
  for _,m:=range inst.Decl.Methods {
   var cs []types.Constraint; for _,sig:=range inst.Class.Methods {if sig.Name==m.Name {cs=sig.Scheme.Constraints}}
   methods=append(methods,"CoreMethod{name: "+q(m.Name)+", dictParams: "+constraints(cs)+", params: "+names(m.Params)+", body: CInt(0)}")
  }
  instances=append(instances,"CoreInstance{className: "+q(inst.Class.Name)+", typeCon: "+q(inst.TypeCon.Name)+", context: "+constraints(inst.Context)+", superArgs: Nil, methods: "+list(methods)+"}")
 }
 for _,n:=range keys(prog.Funcs) {
  f:=prog.Funcs[n]
  if ext:=f.Decl.Extern; ext!=nil {funcs=append(funcs,fmt.Sprintf("CoreExtern{name: %s, importPath: %s, goName: %s, arity: %d}",q(n),q(ext.ImportPath),q(ext.Name),len(f.Decl.Params))); continue}
  var params []string; for _,p:=range f.Decl.Params {params=append(params,p.Name)}
  body:="CInt(0)"; if n=="main" {body=`CPrim("ordLt", [CCtor("Ordering", "LT", Nil)])`}
  funcs=append(funcs,"CoreFunc{name: "+q(n)+", dictParams: "+constraints(f.Scheme.Constraints)+", params: "+names(params)+", body: "+body+"}")
 }
 text:="package main\nextern func writeFile(path String, content String) IO[Unit] = \"gofx/rt\".WriteFile\nextern func quote(s String) String = \"gofx/rt\".Quote\nfunc main() IO[Unit] {\nlet prog = Program{datas: "+list(datas)+", classes: "+list(classes)+", instances: "+list(instances)+", funcs: "+list(funcs)+", warnings: Nil}\nmatch generate(prog, \"main\") { Err(d) => ioFail(show(d)) | Ok(source) => do { writeFile(\"selfhost/dev/out/cgdecls_monoid/main.go\", source); println(\"cg_decls: monoid declaration fixture generated\") } }\n}\n"
 must(os.WriteFile("selfhost/dev/cg_decls_monoid_main.gfx",[]byte(text),0644))
 fmt.Printf("monoid fixture: %d data types, %d classes, %d instances, %d functions\n",len(datas),len(classes),len(instances),len(funcs))
}

func project(path, dest string, normalizeExternNames bool) {
 fset:=token.NewFileSet(); file,err:=goparser.ParseFile(fset,path,nil,goparser.ParseComments); must(err)
 for _,decl:=range file.Decls {
  fn,ok:=decl.(*goast.FuncDecl); if !ok {continue}
  if normalizeExternNames && fn.Body!=nil && len(fn.Body.List)==1 {
   if ret,ok:=fn.Body.List[0].(*goast.ReturnStmt); ok && len(ret.Results)==1 {
    if call,ok:=ret.Results[0].(*goast.CallExpr); ok {
     if sel,ok:=call.Fun.(*goast.SelectorExpr); ok {
      if id,ok:=sel.X.(*goast.Ident); ok && strings.HasPrefix(id.Name,"ext_") {
       n:=0; for _,field:=range fn.Type.Params.List {for _,name:=range field.Names {name.Name=fmt.Sprintf("a%d",n); n++}}
      }
     }
    }
   }
  }
  // Include all declaration signatures, init registration and ordering helpers;
  // remove executable function/method bodies from the declaration comparison.
  if fn.Name.Name!="init" && !strings.HasPrefix(fn.Name.Name,"ord_") {
   fn.Body=&goast.BlockStmt{}
  }
 }
 var out bytes.Buffer
 if normalizeExternNames {
  var declarations []string
  fmt.Fprintf(&out,"// Code generated by gofx. DO NOT EDIT.\npackage %s\n",file.Name.Name)
  for _,decl:=range file.Decls {
   var item bytes.Buffer; must(format.Node(&item,token.NewFileSet(),decl))
   if gen,ok:=decl.(*goast.GenDecl); ok && gen.Tok==token.IMPORT {out.Write(item.Bytes()); out.WriteByte('\n')} else {declarations=append(declarations,item.String())}
  }
  sort.Strings(declarations); out.WriteString(strings.Join(declarations,"\n\n"))
 } else {must(format.Node(&out,fset,file))}
 source,err:=format.Source(out.Bytes()); must(err); must(os.WriteFile(dest,source,0644))
}

func main() {
 if len(os.Args)==1 {fixture(); return}
 if len(os.Args)!=4 {panic("usage: compare INPUT OUTPUT raw|normalize")}
 project(os.Args[1],os.Args[2],os.Args[3]=="normalize")
}
