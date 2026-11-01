package rt

// Option is the natural Go representation of a go4t Option. Its zero value is None.
type Option[T any] struct {
	Value   T
	Present bool
}

func Some[T any](value T) Option[T] { return Option[T]{Value: value, Present: true} }
func None[T any]() Option[T]       { return Option[T]{} }

// Result carries either Ok (IsOk) or Err. The inactive field is ignored.
type Result[E, A any] struct {
	Err  E
	Ok   A
	IsOk bool
}

// Tuple2 through Tuple8 are the natural Go representations of go4t tuples.
type Tuple2[A, B any] struct {
	F0 A
	F1 B
}

type Tuple3[A, B, C any] struct {
	F0 A
	F1 B
	F2 C
}

type Tuple4[A, B, C, D any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
}

type Tuple5[A, B, C, D, E any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
	F4 E
}

type Tuple6[A, B, C, D, E, F any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
	F4 E
	F5 F
}

type Tuple7[A, B, C, D, E, F, G any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
	F4 E
	F5 F
	F6 G
}

type Tuple8[A, B, C, D, E, F, G, H any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
	F4 E
	F5 F
	F6 G
	F7 H
}
