package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type frequency struct {
	word  string
	count int
}

func main() {
	text, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	counts := make(map[string]int)
	for _, word := range strings.Fields(string(text)) {
		counts[word]++
	}
	ranked := make([]frequency, 0, len(counts))
	for word, count := range counts {
		ranked = append(ranked, frequency{word, count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].word < ranked[j].word
	})
	for _, entry := range ranked[:min(10, len(ranked))] {
		fmt.Printf("%s %d\n", entry.word, entry.count)
	}
}
