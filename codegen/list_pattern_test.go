package codegen

import "testing"

func TestListPatternsRun(t *testing.T) {
	source := qualityProgram(t, "list-pattern.gt", `package main
func classify(xs List[Int]) Int {
  match xs {
    [] => 0
    [a] => a
    [a, b, c] => a + b + c
    [a, b, ..rest] => a + b + length(rest) * 100
  }
}
func sum(xs List[Int]) Int {
  match xs { [] => 0; [a, ..rest] => a + sum(rest) }
}
func nested(xs List[List[Int]]) Int {
  match xs { [[], [a, .._]] => a; _ => 0 }
}
func guarded(xs List[Int]) Int {
  match xs {
    [a, b] if a > b => a
    [a, b] => b
    [_ , ..] => 1
    [] => 0
  }
}
func six(xs List[Int]) Int {
  match xs { [a, b, c, d, e, f] => a + b + c + d + e + f; _ => 0 }
}
func main() IO[Unit] {
  do {
    println("${classify([])} ${classify([7])} ${classify([2, 3])} ${classify([1, 2, 3])} ${classify([1, 2, 3, 4])}")
    println("${sum([1, 2, 3, 4])} ${nested([[], [9, 8]])} ${nested([[], []])}")
    println("${guarded([3, 2])} ${guarded([2, 3])} ${guarded([1, 2, 3])} ${guarded([])}")
    println("${six([1, 2, 3, 4, 5, 6])} ${six([1, 2, 3, 4, 5])} ${six([1, 2, 3, 4, 5, 6, 7])}")
  }
}`)
	output, err := runQualityGo(t, source, false)
	want := "0 7 5 6 203\n10 9 0\n3 3 1 0\n21 0 0\n"
	if err != nil || output != want {
		t.Fatalf("output = %q, error = %v, want %q", output, err, want)
	}
}
