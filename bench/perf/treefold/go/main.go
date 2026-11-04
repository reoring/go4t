package main

import (
	"fmt"
	"os"
)

type tree struct{ left, right *tree }

func build(depth int) *tree {
	if depth == 0 {
		return nil
	}
	return &tree{build(depth - 1), build(depth - 1)}
}

func count(t *tree) int64 {
	if t == nil {
		return 1
	}
	return 1 + count(t.left) + count(t.right)
}

func main() {
	var depth int
	if _, err := fmt.Fscan(os.Stdin, &depth); err != nil {
		panic(err)
	}
	if depth < 0 {
		panic("expected nonnegative depth")
	}
	fmt.Println(count(build(depth)))
}
