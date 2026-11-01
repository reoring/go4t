package rt

import (
	"slices"
	"testing"
)

func TestIntListShapes(t *testing.T) {
	installTestShapes(t)
	for _, values := range [][]int64{nil, {}, {0}, {-9223372036854775808, -1, 0, 9223372036854775807}} {
		list := ListFromInts(values)
		if got := ListToInts(list); !slices.Equal(got, values) {
			t.Errorf("list round trip = %v, want %v", got, values)
		}
		if len(values) == 0 {
			if _, ok := list.(testNil); !ok {
				t.Fatalf("empty list = %#v", list)
			}
		} else {
			// A Go caller may mutate its slice; neither bridge shares a mutable
			// slice with a generated list.
			first := values[0]
			converted := ListToInts(list)
			converted[0] = 17
			if got := list.(testCons).head; got != first {
				t.Fatal("output slice aliases list")
			}
			values[0] = 23
			if got := list.(testCons).head; got != first {
				t.Fatal("input slice aliases list")
			}
		}
	}
}

func TestUnsetIntShapes(t *testing.T) {
	installTestShapes(t)
	for _, tc := range []struct {
		name   string
		shapes ShapeFuncs
		call   func()
		want   string
	}{
		{"constructors", ShapeFuncs{}, func() { ListFromInts(nil) }, "rt.Shapes is not initialized: Cons and Nil are required"},
		{"Cons", ShapeFuncs{Nil: Shapes.Nil}, func() { ListFromInts([]int64{1}) }, "rt.Shapes is not initialized: Cons and Nil are required"},
		{"Nil", ShapeFuncs{Cons: Shapes.Cons}, func() { ListFromInts(nil) }, "rt.Shapes is not initialized: Cons and Nil are required"},
		{"Uncons", ShapeFuncs{}, func() { ListToInts(testNil{}) }, "rt.Shapes is not initialized: Uncons is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Shapes = tc.shapes
			defer func() {
				if got := recover(); got != tc.want {
					t.Errorf("panic = %v, want %q", got, tc.want)
				}
			}()
			tc.call()
		})
	}
}
