// Copyright 2026 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
package host

import (
 "fmt"
 "path"
 "strings"
 "runtime"
 "os"
 "path/filepath"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
)

func boxError(err error) any { return deps.ErrorFromGo(err) }
func unboxError(value any) error { return deps.ErrorToGo(value) }
func Quote(value string) string { return fmt.Sprintf("%q", value) }
func Join(left, right string) string { return path.Join(left,right) }
func Lower(value string) string { return strings.ToLower(value) }
func Contains(value, part string) bool { return strings.Contains(value,part) }
func Trim(value string) string { return strings.TrimSpace(value) }
type suffixError struct { cause error; suffix string }
func (e suffixError) Error() string { return e.cause.Error()+e.suffix }
func (e suffixError) Unwrap() error { return e.cause }
func ErrorWithSuffix(cause any,suffix string) any { return boxError(suffixError{unboxError(cause),suffix}) }
func ProcessCommand() string { return os.Args[0] }
func CommandBase(value string) string { return filepath.Base(value) }
func OSName() string { return runtime.GOOS }
func Architecture() string { return runtime.GOARCH }
type sentinelError struct { identity string; message string }
func (e sentinelError) Error() string { return e.message }
func (e sentinelError) Is(other error) bool { value,ok:=other.(sentinelError);return ok&&e.identity==value.identity }
func SentinelError(identity,message string) any { return boxError(sentinelError{identity,message}) }
