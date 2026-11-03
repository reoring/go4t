# ADR 0005: IO 期待位置への暗黙 lift(`ioPure` の省略)

Date: 2026-11-04
Status: Accepted (verified 2026-11-04)

## Context

ADR 0003 で observe/apply を go4t で書けるようになり、`examples/node-drain-typed/main.gt` は 130 行中 `ioPure` が
12 回。パターンは 3 つに集約される(`examples/*.gt` の `ioPure(match …)` / `ioPure(if …)` 6 件を含む):

```go4t
ioPure(match listed { None => Nil; Some(l) => l.items })      // 1. 純粋な値を IO の末尾で返す
match action {
  Cordon => do { patchNode(…); ioPure(d) }                     // 2. 腕ごとに ioPure / do の混在
  Wait(_) => ioPure(d{lastBlock: None})
}
Ok(node) => ioPure(node)                                       // 3. tryIO の結果を分岐して戻す
```

TypeScript の `async` が消すのはこの種の明示的な lift で、`Promise` そのものではない。

### 確認済みの事実

- `do` は 1 つの Monad コンストラクタで desugar される(SPEC §4.8)。Option/Result/List/State でも使われるため、
  `do` の規則として lift を入れると他の Monad で意味が変わる。
- 期待型は関数本文の末尾・`if` 分岐・`match` 腕・`do` の最終式に流れる(SPEC §5.1「Expected types flow into …」)。
  暗黙 lift を置くべき位置はここで、期待型が `IO[T]` に**確定している**ときだけ適用できる。
- 純粋/IO の境界は型で表され、`laws/` は `IO` を含む型を `law depends on IO` で拒否する(`laws/laws.go:unsupportedType`)。
  lift は型検査後の desugar なので、法則の対象断片(純粋関数)には現れない。
- codegen は `IO` を `rt.IO` クロージャで表す。`ioPure(e)` の挿入は既存の呼び出しと同一。

## Options

- **A. 期待型 `IO[T]` への暗黙 lift(採用)**: 期待型が `IO[T]` の位置に型 `T` の式が来たら `ioPure(e)` に書き換える。
  `async` キーワードは導入しない。関数の戻り型 `IO[T]` が既に async 宣言の役割を果たす。
- B. `async func` + 暗黙 `IO[T]` 化: 戻り型を書かないので「この関数は効果を持つ」が署名から読めなくなる。却下。
- C. `<-` を `do` の外で許す: `do` の意味(1 Monad)を崩す。`do { }` を 1 段書く手間は残す。却下。
- D. `ioPure` を自動挿入せず、`match`/`if` の腕で `do` を省略できる糖衣だけ: 1 の場合に効かない。A に含まれる。

## Decision

### 1. 規則

式 `e` の検査で、期待型が `IO[T]`(`IO` が型コンストラクタとして確定、`T` は任意)であり、`e` の推論型 `U` が
`IO[_]` と単一化**できず**、`U` が `T` と単一化**できる**とき、`e` を `ioPure(e)` として検査する。

適用位置は期待型が流れる位置に限る: 関数本文の末尾式、`if`/`match` の各腕、`do` ブロックの最終式、
`let x: IO[T] = e` の右辺。引数位置・演算子オペランド・リスト要素には適用しない(SPEC §5.1 の infer-only 位置と同じ境界)。

### 2. 曖昧さの排除

- `U` が型変数(未確定)のときは lift しない。`ioPure` を自分で書く。推論順序に依存する lift を作らない。
- `U` が `IO[IO[T]]` のような二重 IO は lift の対象外(`IO[_]` と単一化できるため)。`join` は明示。
- `T` が `Unit` で `e` が `Unit` 型の純粋式(`unit()`)も lift する。副作用の無い `Unit` を IO にする点は `ioPure(unit())` と同じ。
- 診断: lift 後に型が合わない場合、エラーは**元の式の型**で報告する(`expected IO[String], got Int`)。
  lift を試みたことは報告しない。

### 3. 構文

変更なし。`format` は何も書き換えない(lift はソースに現れない)。LSP の hover は lift 後の型 `IO[T]` を出す。

### 4. 変えないもの

- `do` の desugar、`IO` の表現、`laws/` の判定、`bind`/`pure` の意味。
- `match` の腕で `do { … }` と純粋式が混在する場合、純粋式側だけ lift される。`do` を書いた腕は従来どおり。

## 健全性

lift は `ioPure(e)` の挿入と等価で、`ioPure` は `IO` の `pure`。型検査が通った式に対してのみ挿入するので、
挿入後のプログラムは利用者が手で `ioPure` を書いたプログラムと同一の型・同一の codegen 出力になる。
純粋関数の中に期待型 `IO[T]` は現れないので、法則の対象断片は変わらない。`if`/`match` の腕が片方 IO・片方純粋の
場合、純粋側の腕だけが `ioPure` に包まれ、その式を含む関数は IO になる — これは lift 以前から戻り型 `IO[T]` を
持っていた関数で、lift が純粋関数を IO に変えることはない。腕の中で呼ばれる純粋関数 `f` は従来どおり法則の対象。

## 検証

結果: `node-drain-typed/main.gt` の `ioPure` 12 → 0、`go-import/main.gt` 3 → 0、行数は不変(130 / 62)。
両例の `go4t build` 出力(`_gen/main.go`)は書き換え前後でバイト同一。構文変更なし。codegen で合成呼び出しの
`ioPure` を衛生的に命名し、利用者が `ioPure` を shadow しても捕まらないことをテストで固定。

- `examples/node-drain-typed/main.gt` と `examples/go-import/main.gt` を書き換え、`ioPure` の数(12 / 3)と行数の前後を記録。
  codegen 出力が書き換え前と同一(`go4t build` の `_gen` を diff)であることを確認する。
- `types` のテスト: lift される 4 位置、lift されない位置(引数、演算子)、型変数のとき lift しない、二重 IO は lift しない、
  診断が元の型で出る。
- `docs/tutorial.md` に 1 節、SPEC §4.8 と §5.1 に規則を追記。
