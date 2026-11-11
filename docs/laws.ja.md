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

**扱えるもの:** 関数を含まないデータ(ADT、レコード、`List`、`Option`、`Result`、`Map`)、
`o{field: v}` による「他のフィールドは任意」型の不変条件、ゲート順、早期 return、冪等性、
分岐の網羅、構造的帰納、二つの `decide` の同値性。
構造的帰納には std の `filter`/`head`/`append` に加え、
`std/list` が定義する `length`、`sum`、`product`、`count`、`reverse`、
`zipWith`、`unzip`、`partition`、`flatten`、`intercalate`、`init`、`span`、
`findIndex`、`indexed` の構造的モデルも使えます。
たとえば長さを保存する map や append の法則は、ライブラリ関数のまま証明できます。
実行時の累積変数による実装は変わらず、Int の周回もモデルに反映されます。
同名のユーザー関数には適用せず、コールバックの依存検査も省略しません。
各式と対象外の理由は [ADR 0014](adr/0014-structural-models-for-std-list.md) に記載しています。

**扱えないもの:** `Set`/`Array`、文字列の中身の推論、比較をまたぐ順序推論
(`a < b && b < c ⇒ a < c`)、前提条件(`requires`)、他の fold の累積変数に関する自動的な一般化、
二つの記号的な Map の等式。

証明の長さは分岐数に比例しますが、それは仕様自体の複雑さであって証明器の制約ではありません。
