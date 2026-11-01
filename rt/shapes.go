package rt

// ShapeFuncs bridges the generated program's List, Option, Result, IoError, tuple and Ordering
// representations across the any ABI. Generated init functions register these operations
// before any runtime calls. Uncons returns ok=false for Nil and the head and tail for Cons.
type ShapeFuncs struct {
	Cons       func(h, t any) any
	Nil        func() any
	Some       func(a any) any
	None       func() any
	Ok         func(a any) any
	Err        func(e any) any
	IoError    func(kind string, msg string) any
	IoErrorOf  func(v any) (kind, msg string)
	Uncons     func(l any) (h, t any, ok bool)
	Tuple2     func(a, b any) any
	Untuple2   func(p any) (a, b any)
	OrderingOf func(o any) int // LT = -1, EQ = 0, GT = 1
	JNull      func() any
	JBool      func(any) any
	JNum       func(any) any
	JStr       func(any) any
	JArr       func(any) any
	JObj       func(any) any
	JsonView   func(any) (kind string, value any)
}

// Shapes holds the generated program's value constructors and destructors.
var Shapes ShapeFuncs

// ListFromStrings constructs a generated List[String], preserving input order.
func ListFromStrings(values []string) any {
	if Shapes.Cons == nil || Shapes.Nil == nil {
		panic("rt.Shapes is not initialized: Cons and Nil are required")
	}
	list := Shapes.Nil()
	for i := len(values) - 1; i >= 0; i-- {
		list = Shapes.Cons(values[i], list)
	}
	return list
}

// ListToStrings reads a generated List[String], preserving input order.
func ListToStrings(list any) []string {
	if Shapes.Uncons == nil {
		panic("rt.Shapes is not initialized: Uncons is required")
	}
	var values []string
	for {
		head, tail, ok := Shapes.Uncons(list)
		if !ok {
			return values
		}
		values = append(values, head.(string))
		list = tail
	}
}

// ListFromInts constructs a generated List[Int], preserving input order.
func ListFromInts(values []int64) any {
	if Shapes.Cons == nil || Shapes.Nil == nil {
		panic("rt.Shapes is not initialized: Cons and Nil are required")
	}
	list := Shapes.Nil()
	for i := len(values) - 1; i >= 0; i-- {
		list = Shapes.Cons(values[i], list)
	}
	return list
}

// ListToInts reads a generated List[Int], preserving input order.
func ListToInts(list any) []int64 {
	if Shapes.Uncons == nil {
		panic("rt.Shapes is not initialized: Uncons is required")
	}
	var values []int64
	for {
		head, tail, ok := Shapes.Uncons(list)
		if !ok {
			return values
		}
		values = append(values, head.(int64))
		list = tail
	}
}

// SomeOf constructs a generated Option containing value.
func SomeOf(value any) any {
	if Shapes.Some == nil {
		panic("rt.Shapes is not initialized: Some is required")
	}
	return Shapes.Some(value)
}

// NoneOf constructs a generated empty Option.
func NoneOf() any {
	if Shapes.None == nil {
		panic("rt.Shapes is not initialized: None is required")
	}
	return Shapes.None()
}

// OkOf constructs a generated successful Result.
func OkOf(value any) any {
	if Shapes.Ok == nil {
		panic("rt.Shapes is not initialized: Ok is required")
	}
	return Shapes.Ok(value)
}

// ErrOf constructs a generated failed Result.
func ErrOf(value any) any {
	if Shapes.Err == nil {
		panic("rt.Shapes is not initialized: Err is required")
	}
	return Shapes.Err(value)
}

// Tuple2Of constructs a generated two-element tuple.
func Tuple2Of(a, b any) any {
	if Shapes.Tuple2 == nil {
		panic("rt.Shapes is not initialized: Tuple2 is required")
	}
	return Shapes.Tuple2(a, b)
}
