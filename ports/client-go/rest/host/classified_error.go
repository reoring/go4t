// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import "gofx/rt"

// A classified error retains an inspectable payload without automatically
// making that payload part of errors.Is/Unwrap semantics.
type classifiedError struct { classification string; message string; cause error }
func (e classifiedError) Error() string { return e.message }
func ClassifiedError(classification,message string,cause any) any {
 var err error
 if value,ok:=rt.OptionValue(cause);ok {err=unboxError(value)}
 return boxError(classifiedError{classification,message,err})
}
func ErrorHasClassification(value any,classification string) bool {
 err,ok:=unboxError(value).(classifiedError)
 return ok && err.classification==classification
}
func ClassifiedErrorCause(value any,classification string) any {
 err,ok:=unboxError(value).(classifiedError)
 if !ok || err.classification!=classification {return rt.NoneOf()}
 return errOption(err.cause)
}
