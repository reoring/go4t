// Copyright 2026 The gofx Authors.
// SPDX-License-Identifier: Apache-2.0
// Compare complete observed traces; never projects away headers, errors, or fields.
package main

import (
 "bytes"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "sort"
 "strconv"
)

func load(path string)(any,error){
 raw,err:=os.ReadFile(path);if err!=nil{return nil,err}
 decoder:=json.NewDecoder(bytes.NewReader(raw));decoder.UseNumber()
 var value any;if err=decoder.Decode(&value);err!=nil{return nil,err}
 var extra any;if err=decoder.Decode(&extra);err!=io.EOF {if err==nil{return nil,fmt.Errorf("%s: multiple JSON values",path)};return nil,err}
 return value,nil
}
func compare(path string,want,got any,failures *[]string){
 switch expected:=want.(type){
 case map[string]any:
  actual,ok:=got.(map[string]any);if !ok{*failures=append(*failures,fmt.Sprintf("%s: expected object, got %T",path,got));return}
  keys:=make([]string,0,len(expected)+len(actual));for k:=range expected{keys=append(keys,k)};for k:=range actual{if _,ok:=expected[k];!ok{keys=append(keys,k)}};sort.Strings(keys)
  for _,key:=range keys{w,wp:=expected[key];g,gp:=actual[key];p:=path+"["+strconv.Quote(key)+"]";if !wp{*failures=append(*failures,p+": unexpected field")}else if !gp{*failures=append(*failures,p+": missing field")}else{compare(p,w,g,failures)}}
 case []any:
  actual,ok:=got.([]any);if !ok{*failures=append(*failures,fmt.Sprintf("%s: expected array, got %T",path,got));return}
  if len(expected)!=len(actual){*failures=append(*failures,fmt.Sprintf("%s: expected %d items, got %d",path,len(expected),len(actual)))}
  for i,w:=range expected{if i<len(actual){compare(fmt.Sprintf("%s[%d]",path,i),w,actual[i],failures)}}
 default:
  // UseNumber preserves exact integers, including values beyond binary64.
  if fmt.Sprintf("%T",want)!=fmt.Sprintf("%T",got)||want!=got{*failures=append(*failures,fmt.Sprintf("%s: expected %#v, got %#v",path,want,got))}
 }
}
func run()error{
 if len(os.Args)!=3{return fmt.Errorf("usage: compare EXPECTED.json NATIVE.json")}
 expected,err:=load(os.Args[1]);if err!=nil{return err};actual,err:=load(os.Args[2]);if err!=nil{return err}
 var failures []string;compare("$",expected,actual,&failures)
 if len(failures)!=0{for _,failure:=range failures{fmt.Fprintln(os.Stderr,failure)};return fmt.Errorf("%d trace differences",len(failures))}
 fmt.Println("native trace matches every expected field and ordered event");return nil
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
