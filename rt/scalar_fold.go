package rt

// ListFoldScalar keeps a concrete accumulator across iterations. Values still
// cross the immutable List shape boundary; no list cells are copied or changed.
func ListFoldScalar[B any](xs any, init B, fold func(B, any) B) B {
	for {
		value, rest, ok := Shapes.Uncons(xs)
		if !ok {
			return init
		}
		init = fold(init, value)
		xs = rest
	}
}

// ArrayFoldScalar uses the same left-to-right order as ArrayFold without boxing
// its accumulator or adapting its callback on every element.
func ArrayFoldScalar[B any](a any, init B, fold func(B, any) B) B {
	for _, value := range a.(arrayValue) {
		init = fold(init, value)
	}
	return init
}

// Record folds mutate only a private value copy with no reference-bearing
// fields. The generated worker writes the next value into that copy, avoiding
// large struct argument/return copies at every step.
func ArrayFoldRecord[B any](a any, init B, fold func(*B, any)) B {
	for _, value := range a.(arrayValue) {
		fold(&init, value)
	}
	return init
}

func ListFoldRecord[B any](xs any, init B, fold func(*B, any)) B {
	for {
		value, rest, ok := Shapes.Uncons(xs)
		if !ok {
			return init
		}
		fold(&init, value)
		xs = rest
	}
}

// BoxRecord restores the pointer-shaped ABI after a by-value record fold.
func BoxRecord[T any](value T) any { return &value }
