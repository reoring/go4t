// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
// OS/path effects only. Loading, merge, migration and mutation policy are native.
package host
import("context";"os";"path/filepath";"runtime";"strings";"gofx/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
func NewError(message string) any {return hostdeps.ErrorNew(message)}
func ReadFile(path string) rt.IO{return func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};b,e:=os.ReadFile(path);return rt.Tuple2Of(string(b),hostdeps.ErrorOptionFromGo(e)),nil}}
func WriteFile(path,content string,mode int64) rt.IO{return func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};return hostdeps.ErrorOptionFromGo(os.WriteFile(path,[]byte(content),os.FileMode(mode))),nil}}
func MkdirAll(path string,mode int64) rt.IO{return func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};return hostdeps.ErrorOptionFromGo(os.MkdirAll(path,os.FileMode(mode))),nil}}
func Stat(path string) rt.IO{return func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};s,e:=os.Stat(path);dir:=false;mode:=int64(0);if e==nil{dir=s.IsDir();mode=int64(s.Mode().Perm())};return rt.Tuple3Of(dir,mode,hostdeps.ErrorOptionFromGo(e)),nil}}
func Remove(path string) rt.IO{return func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};return hostdeps.ErrorOptionFromGo(os.Remove(path)),nil}}
func CreateExclusive(path string) rt.IO{return func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};f,e:=os.OpenFile(path,os.O_CREATE|os.O_EXCL,0);if e==nil{e=f.Close()};return hostdeps.ErrorOptionFromGo(e),nil}}
func Getenv(name string) rt.IO{return func(context.Context)(any,error){return os.Getenv(name),nil}}
func WorkingDirectory() rt.IO{return func(context.Context)(any,error){s,e:=os.Getwd();return rt.Tuple2Of(s,hostdeps.ErrorOptionFromGo(e)),nil}}
func IsAbs(path string) bool{return filepath.IsAbs(path)}
func Join(base,path string) string{return filepath.Join(base,path)}
func Dir(path string) string{return filepath.Dir(path)}
func Relative(base,path string) rt.IO{return func(context.Context)(any,error){s,e:=filepath.Rel(base,path);return rt.Tuple2Of(s,hostdeps.ErrorOptionFromGo(e)),nil}}
func Separator() string{return string(os.PathSeparator)}
func OS() string{return runtime.GOOS}
func SplitList(value string) []string {return filepath.SplitList(value)}
func IsPermission(value any) bool {e:=hostdeps.ErrorToGo(value);return os.IsPermission(e)}
func HasSeparator(value string) bool{return strings.ContainsRune(value,os.PathSeparator)}
func IsNotExist(value any) bool {e:=hostdeps.ErrorToGo(value);return os.IsNotExist(e)}
