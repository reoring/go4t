// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import("io/fs";"os";"github.com/reoring/gofx/ports/client-go/host/deps")
func NotFoundError(path string)any{return hostdeps.ErrorFromGo(&os.PathError{Op:"open",Path:path,Err:fs.ErrNotExist})}
func AlreadyExistsError(path string)any{return hostdeps.ErrorFromGo(&os.PathError{Op:"open",Path:path,Err:fs.ErrExist})}
