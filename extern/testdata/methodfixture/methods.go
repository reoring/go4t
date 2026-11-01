package methodfixture

import "go4t/rt"

type Widget struct{ Name string }

func NewWidget(name string) any { return Widget{Name: name} }
func NewPointer(name string) any { return &Widget{Name: name} }
func (w Widget) Echo(s string) string { return w.Name + s }
func (w *Widget) Count() int { return len(w.Name) }
func (w Widget) Lookup(ok bool) (string, bool) { return w.Name, ok }
func (w Widget) Read() (string, error) { return w.Name, nil }
func (w Widget) Words(xs []string) []string { return xs }
func (w Widget) Numbers(xs []int64) []int64 { return xs }
func (w Widget) Float(x float64) float64 { return x }
func (w Widget) Unit(x rt.Unit) rt.Unit { return x }
func (w Widget) Callback(f func() string) string { return f() }
func (w Widget) Pointer(x *string) string { return *x }
func (w Widget) Variadic(xs ...string) string { return w.Name }
func (w Widget) Void() {}
func (w Widget) Multiple() (string, string) { return w.Name, w.Name }
func (w Widget) ResultPointer() *Widget { return &w }
func (w Widget) hidden() string { return w.Name }

type hidden struct{}
func (hidden) Visible() string { return "promoted" }
type Outer struct{ hidden }

type Generic[T any] struct{}
func (Generic[T]) Name() string { return "generic" }

type Alias = Widget

type word = string
type number = int64

func (w Widget) AliasWords(xs []word) []word { return xs }
func (w Widget) AliasNumbers(xs []number) []number { return xs }

type Constraint interface {
	~string
	Text() string
}
