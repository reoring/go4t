package bindfixture

import "errors"

func Greeting(name string) string          { return "Hello, " + name }
func Length(values []int64) int64          { return int64(len(values)) }
func LookupCode(name string) (int64, bool) { return int64(len(name)), name != "" }
func Read(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty name")
	}
	return name, nil
}
func Repeat(s string, n int) string    { return s }
func Pointer(p *int) int               { return *p }
func Variadic(values ...string) string { return "" }
func Generic[T any](value T) T         { return value }
func hidden()                          {}
