// Copyright The Kubernetes Authors. Licensed under Apache-2.0.
// Extract typed getters from the upstream clientset Interface, not directory names.
package main

import (
 "encoding/json"
 "fmt"
 "go/ast"
 "go/parser"
 "go/token"
 "os"
 "path"
 "strconv"
 "strings"
)

type getter struct {
 Method string `json:"method"`
 Type string `json:"type"`
}

func extract(source string) (map[string]getter, error) {
 file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
 if err != nil { return nil, err }
 imports := map[string]string{}
 for _, spec := range file.Imports {
  importPath, err := strconv.Unquote(spec.Path.Value)
  if err != nil { return nil, err }
  name := path.Base(importPath)
  if spec.Name != nil { name = spec.Name.Name }
  imports[name] = importPath
 }
 var iface *ast.InterfaceType
 for _, declaration := range file.Decls {
  general, ok := declaration.(*ast.GenDecl)
  if !ok || general.Tok != token.TYPE { continue }
  for _, spec := range general.Specs {
   typ := spec.(*ast.TypeSpec)
   if typ.Name.Name != "Interface" { continue }
   if iface != nil { return nil, fmt.Errorf("%s: ambiguous clientset Interface", source) }
   iface, ok = typ.Type.(*ast.InterfaceType)
   if !ok { return nil, fmt.Errorf("%s: clientset Interface is not an interface", source) }
  }
 }
 if iface == nil { return nil, fmt.Errorf("%s: missing clientset Interface", source) }
 result := map[string]getter{}
 for _, field := range iface.Methods.List {
  signature, ok := field.Type.(*ast.FuncType)
  if !ok || len(field.Names) != 1 { return nil, fmt.Errorf("%s: unsupported clientset Interface member", source) }
  if signature.Results == nil || len(signature.Results.List) != 1 { return nil, fmt.Errorf("%s: invalid getter %s result", source, field.Names[0].Name) }
  selector, ok := signature.Results.List[0].Type.(*ast.SelectorExpr)
  if !ok { return nil, fmt.Errorf("%s: getter %s result is not a selector", source, field.Names[0].Name) }
  qualifier, ok := selector.X.(*ast.Ident)
  if !ok { return nil, fmt.Errorf("%s: invalid getter %s qualifier", source, field.Names[0].Name) }
  importPath, ok := imports[qualifier.Name]
  if !ok { return nil, fmt.Errorf("%s: unresolved getter %s import", source, field.Names[0].Name) }
  if !strings.HasPrefix(importPath, "k8s.io/client-go/kubernetes/typed/") { continue }
  if signature.Params.NumFields() != 0 || signature.Results.NumFields() != 1 { return nil, fmt.Errorf("%s: invalid typed getter %s signature", source, field.Names[0].Name) }
  if previous, exists := result[importPath]; exists { return nil, fmt.Errorf("%s: ambiguous typed getter for %s: %s and %s", source, importPath, previous.Method, field.Names[0].Name) }
  result[importPath] = getter{Method: field.Names[0].Name, Type: selector.Sel.Name}
 }
 return result, nil
}

func main() {
 if len(os.Args) != 2 { fmt.Fprintln(os.Stderr, "usage: clientset_getters <upstream-root>"); os.Exit(1) }
 result, err := extract(path.Join(os.Args[1], "kubernetes/clientset.go"))
 if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
 if err := json.NewEncoder(os.Stdout).Encode(result); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
