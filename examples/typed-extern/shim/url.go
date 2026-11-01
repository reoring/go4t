package shim

import "net/url"

// ParseURL creates an opaque handle. The generated method shims recover *url.URL
// from this erased value; gofx must not inspect or construct the handle itself.
func ParseURL(raw string) (any, error) {
	return url.Parse(raw)
}
