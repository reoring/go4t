package rt

import (
	"reflect"
	"slices"
	"testing"
)

type testCons struct{ head, tail any }
type testNil struct{}
type testSome struct{ value any }
type testNone struct{}
type testTuple2 struct{ a, b any }

func installTestShapes(t *testing.T) {
	t.Helper()
	previous := Shapes
	t.Cleanup(func() { Shapes = previous })
	Shapes = ShapeFuncs{
		Cons: func(h, tail any) any { return testCons{h, tail} },
		Nil:  func() any { return testNil{} },
		Some: func(a any) any { return testSome{a} },
		None: func() any { return testNone{} },
		Uncons: func(l any) (any, any, bool) {
			switch l := l.(type) {
			case testCons:
				return l.head, l.tail, true
			case testNil:
				return nil, nil, false
			default:
				panic("test shape: not a list")
			}
		},
		Tuple2: func(a, b any) any { return testTuple2{a, b} },
		Untuple2: func(p any) (any, any) {
			pair := p.(testTuple2)
			return pair.a, pair.b
		},
	}
}

func TestListShapes(t *testing.T) {
	installTestShapes(t)
	for _, values := range [][]string{nil, {}, {"one"}, {"first", "", "日本語", "last"}} {
		list := ListFromStrings(values)
		if len(values) == 0 {
			if _, ok := list.(testNil); !ok {
				t.Fatalf("empty list = %#v, want testNil", list)
			}
		} else if first := list.(testCons).head; first != values[0] {
			t.Errorf("first element = %v, want %q", first, values[0])
		}
		if got := ListToStrings(list); !slices.Equal(got, values) {
			t.Errorf("list round trip = %q, want %q", got, values)
		}
	}
}

func TestOptionShapes(t *testing.T) {
	installTestShapes(t)
	for _, value := range []any{nil, int64(42), "line", false} {
		if got := SomeOf(value); !reflect.DeepEqual(got, testSome{value}) {
			t.Errorf("SomeOf(%v) = %#v", value, got)
		}
	}
	if got := NoneOf(); got != (testNone{}) {
		t.Errorf("NoneOf() = %#v, want testNone", got)
	}
}

func TestUnsetShapesPanicClearly(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		name   string
		shapes ShapeFuncs
		call   func()
		want   string
	}{
		{"list constructors unset", ShapeFuncs{}, func() { ListFromStrings(nil) }, "rt.Shapes is not initialized: Cons and Nil are required"},
		{"Cons unset", ShapeFuncs{Nil: Shapes.Nil}, func() { ListFromStrings([]string{"x"}) }, "rt.Shapes is not initialized: Cons and Nil are required"},
		{"Nil unset", ShapeFuncs{Cons: Shapes.Cons}, func() { ListFromStrings(nil) }, "rt.Shapes is not initialized: Cons and Nil are required"},
		{"Uncons unset", ShapeFuncs{}, func() { ListToStrings(testNil{}) }, "rt.Shapes is not initialized: Uncons is required"},
		{"Some unset", ShapeFuncs{}, func() { SomeOf(int64(1)) }, "rt.Shapes is not initialized: Some is required"},
		{"None unset", ShapeFuncs{}, func() { NoneOf() }, "rt.Shapes is not initialized: None is required"},
		{"Tuple2 unset", ShapeFuncs{}, func() { Tuple2Of(int64(1), "two") }, "rt.Shapes is not initialized: Tuple2 is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			Shapes = tt.shapes
			defer func() {
				if got := recover(); got != tt.want {
					t.Errorf("panic = %v, want %q", got, tt.want)
				}
			}()
			tt.call()
		})
	}
}
