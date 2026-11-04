package q

import _ "go4t/extern/testdata/upcastfixture/transitive"

type Impl struct { private int }

func (*Impl) Speak() string { return "q" }

type Record struct { N int }

func (Record) Speak() string { return "record" }

type Generic[T any] struct { N T }

func (*Generic[T]) Speak() string { return "generic" }

type Constraint interface { ~int }

type Scalar string

func (Scalar) Speak() string { return "scalar" }
