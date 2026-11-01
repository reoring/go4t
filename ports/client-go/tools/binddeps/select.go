//go:build ignore

// Select the actual host source variants with Go's build-constraint evaluator.
package main

import (
 "encoding/json"
 "go/build"
 "os"
 "path/filepath"
)
func main(){
 data,err:=os.ReadFile(os.Args[1]);if err!=nil{panic(err)}
 var in struct{Sources map[string]struct{Path string `json:"path"`} `json:"source_files"`}
 if err=json.Unmarshal(data,&in);err!=nil{panic(err)}
 matches:=map[string]bool{}
 for id,s:=range in.Sources{ok,e:=build.Default.MatchFile(filepath.Dir(s.Path),filepath.Base(s.Path));if e!=nil{panic(e)};matches[id]=ok}
 json.NewEncoder(os.Stdout).Encode(struct{GOOS string `json:"goos"`;GOARCH string `json:"goarch"`;Compiler string `json:"compiler"`;ReleaseTags []string `json:"release_tags"`;Matches map[string]bool `json:"matches"`}{build.Default.GOOS,build.Default.GOARCH,build.Default.Compiler,build.Default.ReleaseTags,matches})
}
