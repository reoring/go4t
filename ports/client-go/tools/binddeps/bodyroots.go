package main

import (
 "encoding/json"
 "go/ast"
 "go/parser"
 "os"
 "path/filepath"
 "sort"
 "strconv"
 "strings"
)

// completeBodyRoots extends, rather than reconstructs, the accepted graph.
// Existing declarations and authority are immutable inputs to this pass.
func completeBodyRoots(path string) {
 data,err:=os.ReadFile(path);if err!=nil{panic(err)}
 var graph map[string]json.RawMessage
 if err=json.Unmarshal(data,&graph);err!=nil{panic(err)}
 var accepted struct { OperationSets struct { Required []string `json:"required_by_frozen_shared_contract"` } `json:"operation_sets"` }
 coverage,err:=os.ReadFile(filepath.Join(filepath.Dir(path),"../../deps/coverage.json"));if err!=nil{panic(err)}
 if err=json.Unmarshal(coverage,&accepted);err!=nil{panic(err)}
 var definitions map[string][]*Decl
 var files map[string]Source
 json.Unmarshal(graph["definitions"],&definitions)
 json.Unmarshal(graph["source_files"],&files)
 references:=map[string][]M{}
 err=filepath.WalkDir(root,func(path string,entry os.DirEntry,err error)error{
  if err!=nil{return err};if entry.IsDir(){if entry.Name()==".git"||entry.Name()=="testdata"{return filepath.SkipDir};return nil}
  if !strings.HasSuffix(path,".go")||strings.HasSuffix(path,"_test.go"){return nil}
  file,err:=parser.ParseFile(fs,path,nil,0);if err!=nil{return err}
  imports:=map[string]string{}
  for _,spec:=range file.Imports{p,_:=strconv.Unquote(spec.Path.Value);alias:=packageName(p);if spec.Name!=nil{alias=spec.Name.Name};if !strings.HasPrefix(p,"k8s.io/client-go"){imports[alias]=p}}
  rel,_:=filepath.Rel(root,path)
  inspect:=func(node ast.Node){if node==nil{return};ast.Inspect(node,func(node ast.Node)bool{
   selector,ok:=node.(*ast.SelectorExpr);if !ok{return true};ident,ok:=selector.X.(*ast.Ident);if !ok||ident.Obj!=nil{return true}
   p,ok:=imports[ident.Name];if !ok||!ast.IsExported(selector.Sel.Name){return true}
   id:=p+"."+selector.Sel.Name
   references[id]=append(references[id],M{"source":"k8s.io/client-go/"+filepath.ToSlash(rel),"line":fs.Position(selector.Pos()).Line})
   return true
  })}
  for _,decl:=range file.Decls{switch d:=decl.(type){case *ast.FuncDecl:if d.Body!=nil{inspect(d.Body)};case *ast.GenDecl:for _,spec:=range d.Specs{if value,ok:=spec.(*ast.ValueSpec);ok{for _,expr:=range value.Values{inspect(expr)}}}}}
  return nil
 });if err!=nil{panic(err)}
 queue:=[]string{};for id:=range references{queue=append(queue,id)};sort.Strings(queue)
 seen:=map[string]bool{};unresolved:=[]string{}
 for len(queue)>0{id:=queue[0];queue=queue[1:];if seen[id]{continue};seen[id]=true
  // The accepted signature closure already includes these declarations.
  if _,ok:=definitions[id];ok{continue}
  i:=strings.LastIndex(id,".");if i<0{continue};p,name:=id[:i],id[i+1:]
  if strings.HasPrefix(p,"k8s.io/client-go"){continue}
  pkg:=load(p);decls:=pkg.decls[name];if len(decls)==0{unresolved=append(unresolved,id);continue}
  definitions[id]=decls
  for _,decl:=range decls{queue=append(queue,decl.Refs...)}
  for _,method:=range pkg.methods[name]{if method.Exported{if _,ok:=definitions[method.ID];!ok{definitions[method.ID]=[]*Decl{method};queue=append(queue,method.Refs...)}}}
 }
 for key,source:=range sources{if _,ok:=files[key];!ok{files[key]=source}}
 sort.Strings(unresolved)
 put:=func(key string,value any){data,err:=json.Marshal(value);if err!=nil{panic(err)};graph[key]=data}
 if _,exists:=graph["frozen_required_operations"];!exists{put("frozen_required_operations",accepted.OperationSets.Required)}
 put("definitions",definitions);put("source_files",files);put("body_root_references",references);put("body_root_unresolved",unresolved)
 put("body_root_policy","Imported exported SelectorExpr references in pinned non-test bodies and initializers, all source variants; lexical local identifiers excluded. Required consumer operations remain separately frozen; method closure is availability only.")
 output,err:=json.MarshalIndent(graph,"","  ");if err!=nil{panic(err)}
 if err=os.WriteFile(path,append(output,'\n'),0644);err!=nil{panic(err)}
}
