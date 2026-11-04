package p

import (
	_ "go4t/extern/testdata/upcastfixture/q"
	_ "go4t/extern/testdata/upcastfixture/unused"
)

type I interface { Speak() string }

type Impl struct { private int }

func (*Impl) Speak() string { return "p" }
