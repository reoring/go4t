# 法則: 扱える範囲と限界

[English](laws.md)

## 法則とは

`law` に等式を書き、`go4t check` で選んだテスト入力だけでなく全入力について検証できます。

```go4t
package main
law distribute(a Int, b Int, c Int) { (a + b) * c == a * c + b * c }
proof distribute(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

等式は `Eq` インスタンスとは独立した定義的等価です。`refl`、コンストラクタの
`match`、条件の `if`、構造的帰納法を含む `rewrite` を組み合わせて証明します。
対象は条件を満たす純粋で停止する依存だけで、信頼されない extern は除外されます。
Float 型の式は除外されますが、`dyn.Value` のように Float を含むだけのデータ型は量化変数に使えます。
`Map` は記号的にモデル化され、`mapLookup(mapInsert(m, k, v), k2)` は
キーが等しいかどうかのケース分けに簡約されます。法則の実行時コードは生成されません。
[チュートリアル §13](tutorial.md#13-laws) に cancel の冪等性、
`freshness` の `staleIff`、リストの帰納法の例があり、`laws/testdata/placement.gt` と
`refactor_v2.gt` に Map を持つ決定ロジックとリファクタ同値の証明があります。

`refl` に失敗すると、証明が止まった位置と両辺の正規形を表示し、反例を探します。
法則の元の引数に境界値などの 64 候補、固定 seed の乱数による 64 候補を代入します。
探索は正規化器の中で行い、候補ごとにステップ数の上限を設けます。Go ツールチェーンは不要です。
たとえば `(a + 1 > a) == true` は Int の桁あふれで成立しません:

```text
counterexample: a = 9223372036854775807
  LHS: false
  RHS: true
```

反例が見つからなければ、
`no counterexample in 128 candidates: the law may hold; this proof needs a case split, rewrite or induction`
と表示します。これは法則が正しいことの証明ではありません。
候補を生成できない型には `counterexample search skipped: no candidates for <type>` と表示します。
Float や handle を含む型、未対応の Map キー型、深さの上限内で値を作れない型は探索しません。
両辺が具体的な値まで正規化されて異なるときだけ反例を報告します。
未簡約の項が残った候補やステップ数の上限に達した候補は捨てます。証明が通れば探索しません。

変数はソースの名前で表示し、一つの診断内で異なる変数の名前が衝突するときだけ `#n` を付けます。
多項式は `now - at` や `2 * a - b + 1` と表示します。
比較の向きは正規形のまま、不要な外側の括弧を省きます。
`stuck on:` の項は `LHS:` / `RHS:` と同じ表記です。

**扱えるもの:** 関数を含まないデータ(ADT、レコード、`List`、`Option`、`Result`、`Map`)、
`o{field: v}` による「他のフィールドは任意」型の不変条件、ゲート順、早期 return、冪等性、
分岐の網羅、構造的帰納(std の `filter`/`head`/`append` を含む)、二つの `decide` の同値性。

**扱えないもの:** `Set`/`Array`、文字列の中身の推論、比較をまたぐ順序推論
(`a < b && b < c ⇒ a < c`)、前提条件(`requires`)、fold ベースの std 関数への帰納、
二つの記号的な Map の等式。

証明の長さは分岐数に比例しますが、それは仕様自体の複雑さであって証明器の制約ではありません。
