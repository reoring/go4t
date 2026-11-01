// Copyright 2015 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// http://www.apache.org/licenses/LICENSE-2.0
// Distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND.
// Package encoding exposes only generic byte/text/JSON scalar encoding, not selectors.
package encoding

import (
 "encoding/json"
 "fmt"
 "math"
 "strconv"
 "unicode/utf8"
)

func ByteString(n int64) string { return string([]byte{byte(n)}) }
func ByteValues(s string) []int64 {
 out := make([]int64,len(s))
 for i := range len(s) { out[i] = int64(s[i]) }
 return out
}
func RuneDescription(s string) string { r,_ := utf8.DecodeRuneInString(s); return fmt.Sprintf("%#U",r) }
func JSONString(s string) string { b,_ := json.Marshal(s); return string(b) }
func FloatText(f float64) string { return strconv.FormatFloat(f,'g',-1,64) }
func FloatFinite(f float64) bool { return !math.IsInf(f,0) && !math.IsNaN(f) }
func JSONFloat(f float64) string { b,err := json.Marshal(f); if err != nil { panic(err) }; return string(b) }
func JSONBytes(s string) string { b,_ := json.Marshal([]byte(s)); return string(b) }
