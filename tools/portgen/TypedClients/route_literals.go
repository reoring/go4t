// Copyright The Kubernetes Authors. Licensed under Apache-2.0.
// Extract fluent request route literals with the Go parser, independently of formatting.
package main

import (
 "encoding/json"
 "fmt"
 "go/ast"
 "go/parser"
 "go/token"
 "os"
 "strconv"
)

type expansion struct {
 Source string `json:"source"`
 Name string `json:"name"`
 Body string `json:"body_source"`
}

func routes(input expansion) (map[string][]string, error) {
 file, err := parser.ParseFile(token.NewFileSet(), input.Source, "package extracted\nfunc expansion() {\n"+input.Body+"\n}", 0)
 if err != nil { return nil, err }
 result := map[string][]string{}
 ast.Inspect(file, func(node ast.Node) bool {
  if err != nil { return false }
  call, ok := node.(*ast.CallExpr)
  if !ok { return true }
  selector, ok := call.Fun.(*ast.SelectorExpr)
  if !ok || (selector.Sel.Name != "AbsPath" && selector.Sel.Name != "Resource") { return true }
  // Route setters belong to the fluent request chain, not package functions.
  if _, ok := selector.X.(*ast.CallExpr); !ok { return true }
  values := make([]string, len(call.Args))
  for i, argument := range call.Args {
   literal, ok := argument.(*ast.BasicLit)
   if !ok || literal.Kind != token.STRING {
    err = fmt.Errorf("%s.%s: nonliteral %s override", input.Source, input.Name, selector.Sel.Name)
    return false
   }
   values[i], err = strconv.Unquote(literal.Value)
   if err != nil { return false }
  }
  if len(values) == 0 || (selector.Sel.Name == "Resource" && len(values) != 1) {
   err = fmt.Errorf("%s.%s: invalid %s literal arity", input.Source, input.Name, selector.Sel.Name)
   return false
  }
  if previous, found := result[selector.Sel.Name]; found {
   if len(previous) != len(values) {
    err = fmt.Errorf("%s.%s: conflicting %s overrides", input.Source, input.Name, selector.Sel.Name)
    return false
   }
   for i := range previous {
    if previous[i] != values[i] {
     err = fmt.Errorf("%s.%s: conflicting %s overrides", input.Source, input.Name, selector.Sel.Name)
     return false
    }
   }
  }
  result[selector.Sel.Name] = values
  return true
 })
 return result, err
}

func main() {
 var inputs []expansion
 if err := json.NewDecoder(os.Stdin).Decode(&inputs); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
 outputs := make([]map[string][]string, len(inputs))
 for i, input := range inputs {
  value, err := routes(input)
  if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
  outputs[i] = value
 }
 if err := json.NewEncoder(os.Stdout).Encode(outputs); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
