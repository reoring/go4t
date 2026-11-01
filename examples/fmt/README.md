# Formatter example

The original `before.gfx` input was:

```gofx
// Formatting keeps this comment.
package main
func twice(x Int) Int{x*2}
func main() IO[Unit]{do{println(show(21|>twice));pure(unit())}}
```

From the repository root, this command exited 0 and printed the following diff:

```sh
go run ./cmd/gofx fmt -d examples/fmt/before.gfx
```

```diff
--- examples/fmt/before.gfx
+++ examples/fmt/before.gfx
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

The checked-in `before.gfx` is now canonical: it was included in the final
repository example/prelude formatting pass, so `fmt -l examples` remains clean.
To reproduce the nonempty diff, save the original input above to a temporary
`.gfx` file and run `gofx fmt -d` on that file. Running `-d` on the checked-in
canonical file correctly prints nothing.

Observed commands after reformatting, all exit 0:

```sh
go run ./cmd/gofx fmt -w examples prelude/prelude.gfx
go run ./cmd/gofx fmt -l examples
go run ./cmd/gofx fmt -l prelude/prelude.gfx
go run ./cmd/gofx run examples/fmt/before.gfx
```

Both `-l` commands produced no output. Running the example printed:

```text
42
```

With no flag, `fmt` prints canonical source without writing. `-w` writes changed
files, `-d` prints a unified diff without writing, and `-l` lists changed paths
without writing (exit 1 if any are listed, otherwise 0). These flags are mutually
exclusive. Directories are traversed recursively for `.gfx` files.
