// Command gen writes the deterministic shared inputs for the runtime comparisons.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: gen OUTPUT-DIRECTORY")
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	for name, text := range map[string]string{
		"sortints":     "1000000 1\n",
		"mapheavy":     "1000000\n",
		"mapheavy-mut": "1000000\n",
		"nbody":        "1000000\n",
		"treefold":     "20\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(text), 0o644); err != nil {
			panic(err)
		}
	}
	file, err := os.Create(filepath.Join(dir, "wordfreq.txt"))
	if err != nil {
		panic(err)
	}
	out := bufio.NewWriter(file)
	words := make([]string, 4096)
	for i := range words {
		words[i] = fmt.Sprintf("word%05d", i)
	}
	seed := int64(17)
	// Two million ten-byte tokens, including one separator: exactly 20 MB.
	for i := range 2000000 {
		seed = seed * 48271 % 2147483647
		separator := " "
		if i%16 == 15 {
			separator = "\n"
		}
		if _, err := out.WriteString(words[seed%4096] + separator); err != nil {
			panic(err)
		}
	}
	if err := out.Flush(); err != nil {
		panic(err)
	}
	if err := file.Close(); err != nil {
		panic(err)
	}
	fmt.Println("generated wordfreq=20000000 bytes, sortints/mapheavy/nbody=1000000, treefold=20")
}
