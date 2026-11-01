# Structural derivation and display

Run from the repository root:

```sh
go run ./cmd/gofx run examples/deriving/main.gfx
```

Observed output:

```text
display: (1 | (2 | 3))
debug: Branch(Leaf(1), Branch(Leaf(2), Leaf(3)))
mapped: (2 | (3 | 4))
foldr: 6
traverse: Some((10 | (20 | 30)))
failure: None
toList: [1, 2, 3]
foldl: 123
foldMapList: [1, 1, 2, 2, 3, 3]
strings: Leaf("hello\nworld")
```

`Tree[A]` derives `Functor`, `Foldable`, `Traversable`, and `Debug` over its last
parameter. Mapping preserves its shape; folding visits leaves in field order;
traversal sequences the supplied `Option` action and stops at `None`. The explicit
`Show[Tree[A]]` instance supplies the application-facing display independently of
constructor-oriented `Debug`. Interpolation still calls `Show`; `${debug(tree)}`
explicitly requests the structural representation, including quoted string fields.
