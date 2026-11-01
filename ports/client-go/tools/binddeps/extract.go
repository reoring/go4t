package main

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "go/ast"
 "go/format"
 "go/parser"
 "go/token"
 "io"
 "os"
 "path/filepath"
 "sort"
 goruntime "runtime"
 "strconv"
 "strings"
)
type M = map[string]any
type Module struct { Path, Version, Dir, Sum, GoModSum string }
type Source struct { Path, SHA256, Package string }
type Decl struct { ID string `json:"id"`; Kind string `json:"kind"`; Source string `json:"source"`; Line int `json:"line"`; Exported bool `json:"exported"`; Shape any `json:"shape"`; Refs []string `json:"refs"`; Receiver string `json:"receiver,omitempty"` }
type Package struct { name string; decls map[string][]*Decl; methods map[string][]*Decl }
var modules []Module
var packages=map[string]*Package{}
var sources=map[string]Source{}
var issues=[]M{}
var fs=token.NewFileSet()
var root string
func text(n ast.Node) string { var b bytes.Buffer; if n!=nil { format.Node(&b,fs,n) }; return b.String() }
func sorted(m map[string]bool) []string { a:=[]string{}; for k:= range m { a=append(a,k) };sort.Strings(a);return a }
func directory(p string) string { if p=="k8s.io/client-go" {return root};if strings.HasPrefix(p,"k8s.io/client-go/"){return filepath.Join(root,strings.TrimPrefix(p,"k8s.io/client-go/"))}; for _,m:=range modules {if p==m.Path {return m.Dir};if strings.HasPrefix(p,m.Path+"/"){return filepath.Join(m.Dir,strings.TrimPrefix(p,m.Path+"/"))}};return filepath.Join(goruntime.GOROOT(),"src",p) }
func packageName(p string) string { d,err:=os.ReadDir(directory(p));if err==nil {for _,f:=range d {if strings.HasSuffix(f.Name(),".go")&&!strings.HasSuffix(f.Name(),"_test.go"){a,e:=parser.ParseFile(token.NewFileSet(),filepath.Join(directory(p),f.Name()),nil,parser.PackageClauseOnly);if e==nil{return a.Name.Name}}}};return filepath.Base(p) }
type Ctx struct { pkg string; imports map[string]string; params map[string]bool; refs map[string]bool }
func (c Ctx) typ(e ast.Expr) any { if e==nil{return nil};switch v:=e.(type) {
case *ast.Ident:
 if c.params[v.Name] {return M{"kind":"type_parameter","name":v.Name}}
 switch v.Name {case "any","error","bool","string","int","int8","int16","int32","int64","uint","uint8","uint16","uint32","uint64","uintptr","byte","rune","float32","float64","complex64","complex128","comparable","unsafe.Pointer":return M{"kind":"predeclared","name":v.Name}}
 id:=c.pkg+"."+v.Name;c.refs[id]=true;return M{"kind":"ref","id":id}
case *ast.SelectorExpr:
 if q,ok:=v.X.(*ast.Ident);ok {if p,ok:=c.imports[q.Name];ok {id:=p+"."+v.Sel.Name;c.refs[id]=true;return M{"kind":"ref","id":id}}}; return M{"kind":"unresolved_selector","syntax":text(e)}
case *ast.StarExpr:return M{"kind":"pointer","nilable":true,"element":c.typ(v.X)}
case *ast.ArrayType:if v.Len==nil{return M{"kind":"slice","nilable":true,"element":c.typ(v.Elt)}};return M{"kind":"array","length_expression":text(v.Len),"element":c.typ(v.Elt)}
case *ast.MapType:return M{"kind":"map","nilable":true,"key":c.typ(v.Key),"element":c.typ(v.Value)}
case *ast.ChanType:return M{"kind":"channel","nilable":true,"direction":int(v.Dir),"element":c.typ(v.Value)}
case *ast.FuncType:return c.funcType(v)
case *ast.StructType:return M{"kind":"struct","fields":c.fields(v.Fields,"field")}
case *ast.InterfaceType:return M{"kind":"interface","members":c.fields(v.Methods,"member")}
case *ast.Ellipsis:return M{"kind":"variadic","element":c.typ(v.Elt)}
case *ast.IndexExpr:return M{"kind":"instantiation","base":c.typ(v.X),"arguments":[]any{c.typ(v.Index)}}
case *ast.IndexListExpr:a:=[]any{};for _,x:=range v.Indices{a=append(a,c.typ(x))};return M{"kind":"instantiation","base":c.typ(v.X),"arguments":a}
case *ast.UnaryExpr:return M{"kind":"unary_constraint","operator":v.Op.String(),"element":c.typ(v.X)}
case *ast.BinaryExpr:return M{"kind":"binary_constraint","operator":v.Op.String(),"left":c.typ(v.X),"right":c.typ(v.Y)}
case *ast.ParenExpr:return c.typ(v.X)
default:return M{"kind":"unresolved_ast","syntax":text(e),"ast_type":fmt.Sprintf("%T",e)} }
}
func (c Ctx) fields(fl *ast.FieldList,role string) []any {out:=[]any{};if fl==nil{return out};for _,f:=range fl.List {names:=[]string{};for _,n:=range f.Names{names=append(names,n.Name)};tag:="";if f.Tag!=nil{tag,_=strconv.Unquote(f.Tag.Value)}; out=append(out,M{"names":names,"embedded":len(names)==0,"type":c.typ(f.Type),"tag":tag,"doc":f.Doc.Text(),"comment":f.Comment.Text(),"line":fs.Position(f.Pos()).Line})};return out}
func (c Ctx) funcType(f *ast.FuncType) any {n:=c;n.params=map[string]bool{};for k,v:=range c.params{n.params[k]=v};if f.TypeParams!=nil {for _,x:=range f.TypeParams.List{for _,k:=range x.Names{n.params[k.Name]=true}}};return M{"kind":"function","nilable":true,"type_parameters":n.fields(f.TypeParams,"type_parameter"),"parameters":n.fields(f.Params,"parameter"),"results":n.fields(f.Results,"result")}}
func receiver(e ast.Expr) string {switch x:=e.(type){case *ast.StarExpr:return receiver(x.X);case *ast.Ident:return x.Name;case *ast.IndexExpr:return receiver(x.X);case *ast.IndexListExpr:return receiver(x.X)};return ""}
func load(p string)*Package {if q,ok:=packages[p];ok{return q};q:=&Package{decls:map[string][]*Decl{},methods:map[string][]*Decl{}};packages[p]=q;d,err:=os.ReadDir(directory(p));if err!=nil{issues=append(issues,M{"kind":"missing_package","package":p,"error":err.Error()});return q};for _,entry:=range d{n:=entry.Name();if !strings.HasSuffix(n,".go")||strings.HasSuffix(n,"_test.go"){continue};path:=filepath.Join(directory(p),n);b,err:=os.ReadFile(path);if err!=nil{panic(err)};f,err:=parser.ParseFile(fs,path,b,parser.ParseComments|parser.AllErrors);if err!=nil{panic(err)};q.name=f.Name.Name;key:=p+"/"+n;sum:=sha256.Sum256(b);sources[key]=Source{Path:path,SHA256:hex.EncodeToString(sum[:]),Package:p};imports:=map[string]string{};for _,i:=range f.Imports {s,_:=strconv.Unquote(i.Path.Value);name:="";if i.Name!=nil{name=i.Name.Name}else{name=packageName(s)};imports[name]=s};for _,d:=range f.Decls {c:=Ctx{pkg:p,imports:imports,params:map[string]bool{},refs:map[string]bool{}};add:=func(name,kind string,pos token.Pos,shape any,recv string){id:=p+"."+name;if recv!=""{id=p+"."+recv+"."+name};v:=&Decl{ID:id,Kind:kind,Source:key,Line:fs.Position(pos).Line,Exported:ast.IsExported(name),Shape:shape,Refs:sorted(c.refs),Receiver:recv};if recv!=""{q.methods[recv]=append(q.methods[recv],v)}else{q.decls[name]=append(q.decls[name],v)}};switch v:=d.(type){case *ast.GenDecl:for _,sp:=range v.Specs{switch s:=sp.(type){case *ast.TypeSpec:c.refs=map[string]bool{};c.params=map[string]bool{};if s.TypeParams!=nil{for _,f:=range s.TypeParams.List{for _,n:=range f.Names{c.params[n.Name]=true}}};kind:="named";if s.Assign.IsValid(){kind="alias"};add(s.Name.Name,"type",s.Pos(),M{"kind":kind,"type_parameters":c.fields(s.TypeParams,"type_parameter"),"underlying":c.typ(s.Type),"doc":v.Doc.Text()+s.Doc.Text()},"")
case *ast.ValueSpec:for _,name:=range s.Names{c.refs=map[string]bool{};shape:=M{"kind":v.Tok.String(),"declared_type":c.typ(s.Type),"declaration":text(s)};if s.Type==nil{shape["type_information"]="unresolved_without_type_inference"};for _,value:=range s.Values{ast.Inspect(value,func(n ast.Node)bool{if x,ok:=n.(*ast.SelectorExpr);ok{if alias,ok:=x.X.(*ast.Ident);ok{if imp,ok:=imports[alias.Name];ok{c.refs[imp+"."+x.Sel.Name]=true}}};return true})};add(name.Name,v.Tok.String(),s.Pos(),shape,"")}}}
case *ast.FuncDecl:recv:="";if v.Recv!=nil{recv=receiver(v.Recv.List[0].Type);ast.Inspect(v.Recv.List[0].Type,func(n ast.Node)bool{switch x:=n.(type){case *ast.IndexExpr:if t,ok:=x.Index.(*ast.Ident);ok{c.params[t.Name]=true};case *ast.IndexListExpr:for _,a:=range x.Indices{if t,ok:=a.(*ast.Ident);ok{c.params[t.Name]=true}}};return true})};shape:=c.funcType(v.Type).(M);if v.Recv!=nil{shape["receiver"]=c.typ(v.Recv.List[0].Type)};add(v.Name.Name,"func",v.Pos(),shape,recv)
}}
};return q}
func main(){root=os.Args[1];f,e:=os.Open(os.Args[2]);if e!=nil{panic(e)};dec:=json.NewDecoder(f);for{var m Module;e=dec.Decode(&m);if e==io.EOF{break};if e!=nil{panic(e)};modules=append(modules,m)};sort.Slice(modules,func(i,j int)bool{return len(modules[i].Path)>len(modules[j].Path)})
 if len(os.Args)>3 {completeBodyRoots(os.Args[3]);return}
 roots:=[]*Decl{};filepath.WalkDir(root,func(path string,d os.DirEntry,e error)error{if e!=nil{panic(e)};if d.IsDir(){if d.Name()==".git"||d.Name()=="testdata"{return filepath.SkipDir};return nil};if !strings.HasSuffix(path,".go")||strings.HasSuffix(path,"_test.go"){return nil};rel,_:=filepath.Rel(root,filepath.Dir(path));p:="k8s.io/client-go";if rel!="."{p+="/"+filepath.ToSlash(rel)};load(p);return nil});for p,q:=range packages{if !strings.HasPrefix(p,"k8s.io/client-go"){continue};for _,ds:=range q.decls{for _,d:=range ds{if d.Exported{roots=append(roots,d)}}};for _,ds:=range q.methods{for _,d:=range ds{if d.Exported{roots=append(roots,d)}}}}
 sort.Slice(roots,func(i,j int)bool{if roots[i].ID==roots[j].ID{return roots[i].Source<roots[j].Source};return roots[i].ID<roots[j].ID})
 queue:=[]string{"k8s.io/apimachinery/pkg/api/errors.APIStatus"};seeded:=map[string]bool{};rootRefs:=map[string][]string{};for _,d:=range roots{for _,r:=range d.Refs{queue=append(queue,r);rootRefs[r]=append(rootRefs[r],d.ID)}};seen:=map[string]bool{};out:=map[string][]*Decl{};unresolved:=[]string{};for len(queue)>0{id:=queue[0];queue=queue[1:];if seen[id]{continue};seen[id]=true;i:=strings.LastIndex(id,".");if i<0{panic(id)};p,name:=id[:i],id[i+1:];q:=load(p);if !seeded[p]&&(strings.HasPrefix(p,"k8s.io/api/")||strings.HasPrefix(p,"k8s.io/apimachinery/")){seeded[p]=true;for n,decls:=range q.decls{if ast.IsExported(n)&&len(decls)>0&&decls[0].Kind=="type"{queue=append(queue,p+"."+n)}}};ds:=q.decls[name];if len(ds)==0{unresolved=append(unresolved,id);continue};if !strings.HasPrefix(p,"k8s.io/client-go"){out[id]=ds};for _,d:=range ds{queue=append(queue,d.Refs...)};for _,d:=range q.methods[name]{if d.Exported{if !strings.HasPrefix(p,"k8s.io/client-go"){out[d.ID]=append(out[d.ID],d)};queue=append(queue,d.Refs...)}}}
 sort.Strings(unresolved);enc:=json.NewEncoder(os.Stdout);enc.SetIndent("","  ");enc.Encode(M{"schema_version":"proposed-ast-1","roots":roots,"root_references":rootRefs,"definitions":out,"unresolved_references":unresolved,"source_files":sources,"modules":modules,"issues":issues,"policy":M{"parser":"go/parser ParseComments|AllErrors; AST only, no inferred types","platforms":"all non-test Go source variants retained, no build-tag evaluation","identity":"Go import path + declared name; methods append receiver + method; no per-consumer copies","closure":"all exported client-go declarations and methods, recursively named types and exported dependency methods; nonexported reachable fields/types included","presence":"pointer/slice/map/channel/function nilability exact in AST; json/protobuf tags and comments retained; omitempty does not imply Go pointer","unresolved":"inferred variable/constant types, interface/field promotion, constants evaluating array lengths and build-constraint selection require types-aware resolution","scope":"production signatures, includes generated families; test declarations and testdata excluded"}})
}
