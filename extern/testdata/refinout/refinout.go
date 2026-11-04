package refinout

import "errors"

type Rec struct {
	Count int
	Flag  *bool
}

func Push(xs *[]string, s string) { *xs = append(*xs, s) }
func Bump(m *map[string]int, k string) {
	if *m == nil {
		*m = make(map[string]int)
	}
	(*m)[k]++
}
func SetFlag(b *bool) { *b = true }
func Fill(r *Rec) {
	r.Count = 42
	flag := true
	r.Flag = &flag
}
func Peek(xs *[]string) int { return len(*xs) }
func Many(ps ...*int) {
	for i, p := range ps {
		*p += i + 1
	}
}
func Bytes(b *[]byte) { *b = append(*b, '!') }
func Result() *int   { n := 7; return &n }
func MutateError(n *int) error {
	*n++
	return errors.New("changed before failure")
}
func MutateAbsent(n *int) (int, bool) { *n++; return 0, false }
func Invoke(n *int, f func(*int))     { f(n) }
func InvokeMany(f func(...*int)) (int, int) {
	a, b := 10, 20
	f(&a, &b)
	return a, b
}
func Returned() func(*int) { return func(n *int) { *n++ } }

// Generic pointer parameters retain their template's Option representation.
func Generic[T any](p *T) *T { return p }
func GenericSlice[T any](p *[]T, v T) { *p = append(*p, v) }
