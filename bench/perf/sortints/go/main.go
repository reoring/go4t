package main

import (
	"fmt"
	"os"
	"slices"
)

func main() {
	var n int
	var seed int64
	if _, err := fmt.Fscan(os.Stdin, &n, &seed); err != nil { panic(err) }
	if n <= 0 || seed <= 0 || seed >= 2147483647 { panic("invalid size or seed") }
	values := make([]int64, n)
	for i := n - 1; i >= 0; i-- {
		seed = (seed * 48271) % 2147483647
		values[i] = seed
	}
	slices.Sort(values)
	var total int64
	for i := 0; i < n; i += 1000 { total += values[i] }
	fmt.Println(total)
}
