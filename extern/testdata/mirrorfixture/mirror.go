package mirrorfixture

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Embedded struct{ Enabled bool }
type Point struct {
	X int
	Y int
}
type Record struct {
	Embedded
	Name   string
	Point  Point
	Maybe  *Point
	Wait   time.Duration
	At     time.Time
	Bytes  []byte
	Err    error
	Labels map[string]int
	hidden string
}
type Count int32
type Opaque struct{ State any }
type Counter struct{ n int }
type Cycle struct{ Next *Cycle }
type Generic[T any] struct{ Value T }
type privateEmbedded struct{ Visible string }
type Flattened struct {
	privateEmbedded
	*Point
}
type Left struct {
	Same int
	L    int
}
type Right struct {
	Same int
	R    int
}
type Ambiguous struct {
	Left
	Right
	Own int
}
type PointFn func(Point) Point

func EchoRecord(v Record) Record { v.hidden = "not copied"; return v }
func MakeRecord() Record {
	return Record{Embedded: Embedded{true}, Name: "record", Point: Point{2, 3}, Maybe: &Point{4, 5}, Wait: 7, At: time.Unix(0, 11), Bytes: []byte("bytes"), Err: errors.New("field"), Labels: map[string]int{"x": 9}}
}
func MutateRecord(v Record) Record               { v.Bytes[0] = 'X'; v.Maybe.X = 99; v.Labels["x"] = 99; return v }
func MakeFlattened() Flattened                   { return Flattened{privateEmbedded: privateEmbedded{"visible"}} }
func EchoFlattened(v Flattened) Flattened        { return v }
func EchoAmbiguous(v Ambiguous) Ambiguous        { return v }
func PointSum(p Point) int                       { return p.X + p.Y }
func EchoPointer(p *Point) *Point                { return p }
func EchoDuration(v time.Duration) time.Duration { return v }
func EchoTime(v time.Time) time.Time             { return v }
func EchoBytes(v []byte) []byte                  { return v }
func EchoCount(v Count) Count                    { return v }
func EchoMap(v map[int][]Point) map[int][]Point  { return v }
func WithContext(ctx context.Context, v Record) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	return v, nil
}
func ContextOnly(ctx context.Context) string {
	if ctx == nil {
		panic("missing context")
	}
	return "context"
}
func HasDeadline(ctx context.Context) bool                   { _, ok := ctx.Deadline(); return ok }
func ContextOption(ctx context.Context, ok bool) (int, bool) { return 5, ok }
func ErrorOnly(fail bool) error {
	if fail {
		return errors.New("fixture failure")
	}
	return nil
}
func Variadic(prefix string, values ...int) int {
	n := len(prefix)
	for _, v := range values {
		n += v
	}
	return n
}
func Dynamic(values ...any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%T:%v", v, v)
	}
	return strings.Join(parts, "|")
}
func DynamicResult() any              { return uint16(42) }
func EchoDynamic(values ...any) []any { return values }
func DecodeDynamic() []any {
	return []any{int8(-1), uint32(2), float32(1.5), true, "s", []byte("b"), time.Time{}, nil, struct{ N int }{3}}
}
func PureCallback(v Point, f func(Point) Point) Point { return f(v) }
func NamedCallback(v Point, f PointFn) Point          { return f(v) }
func IOCallback(ctx context.Context, v Point, f func(context.Context, Point) (Point, error)) (Point, error) {
	return f(ctx, v)
}
func ErrorCallback(ctx context.Context, f func() error) error { return f() }
func VoidCallback(f func())                                   { f() }
func NewCounter() *Counter                                    { return &Counter{} }
func (c *Counter) Scan(dest ...any) error {
	for i, p := range dest {
		switch i % 3 {
		case 0:
			*p.(*any) = int64(12)
		case 1:
			*p.(*any) = []byte("scan")
		case 2:
			*p.(*any) = nil
		}
	}
	return nil
}
func (c *Counter) Add(ctx context.Context, n int) (int, error) { c.n += n; return c.n, ctx.Err() }
func NewOpaque() Opaque                                        { return Opaque{State: "opaque"} }
func OpaqueText(v Opaque) string                               { return v.State.(string) }
func GenericIdentity[T any](v T) T                             { return v }
func Unsupported(v chan int) int                               { return 0 }

// Keyword-named fields are renamed with a trailing underscore; other reserved
// spellings make the type opaque.
type Labelled struct {
	Type  string
	Match int
}

type Reserved struct{ Ext_ string }

func EchoLabelled(l Labelled) Labelled { return l }
func EchoReserved(r Reserved) Reserved { return r }
