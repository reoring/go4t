# Formatter example

The original `before.gt` input was:

```go4t
// Formatting keeps this comment.
package main
func twice(x Int) Int{x*2}
func main() IO[Unit]{do{println(show(21|>twice));pure(unit())}}
```

From the repository root, this command exited 0 and printed the following diff:

```sh
go run ./cmd/go4t fmt -d examples/fmt/before.gt
```

```diff
--- examples/fmt/before.gt
+++ examples/fmt/before.gt
@@ -1,4 +1,13 @@
-// Formatting keeps this comment.
-package main
-func twice(x Int) Int{x*2}
-func main() IO[Unit]{do{println(show(21|>twice));pure(unit())}}
+// Formatting keeps this comment.
+package main
+
+func twice(x Int) Int {
+  x * 2
+}
+
+func main() IO[Unit] {
+  do {
+    println(show(21 |> twice))
+    pure(unit())
+  }
+}
```

The checked-in `before.gt` is now canonical: it was included in the final
repository example/prelude formatting pass, so `fmt -l examples` remains clean.
To reproduce the nonempty diff, save the original input above to a temporary
`.gt` file and run `go4t fmt -d` on that file. Running `-d` on the checked-in
canonical file correctly prints nothing.

Observed commands after reformatting, all exit 0:

```sh
go run ./cmd/go4t fmt -w examples prelude/prelude.gt
go run ./cmd/go4t fmt -l examples
go run ./cmd/go4t fmt -l prelude/prelude.gt
go run ./cmd/go4t run examples/fmt/before.gt
```

Both `-l` commands produced no output. Running the example printed:

```text
42
```

With no flag, `fmt` prints canonical source without writing. `-w` writes changed
files, `-d` prints a unified diff without writing, and `-l` lists changed paths
without writing (exit 1 if any are listed, otherwise 0). These flags are mutually
exclusive. Directories are traversed recursively for `.gt` files.
