package rt

import "runtime"

// A recovered Go panic is reported with Go's own text. For panic(nil) that
// text belongs to the Go runtime and has changed between releases.
var goNilPanicMessage = (&runtime.PanicNilError{}).Error()
