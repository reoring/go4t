package main

import (
	"fmt"
	"os"
)

func main() {
	var n int64
	if _, err := fmt.Fscan(os.Stdin, &n); err != nil { panic(err) }
	if n <= 0 { panic("expected positive count") }
	values := make(map[int64]int64, n)
	for i := range n { values[i] = i*3 + 1 }
	hits := 0
	for i := range n {
		if value, ok := values[i]; ok && value == i*3+1 { hits++ }
	}
	fmt.Println(hits)
}
