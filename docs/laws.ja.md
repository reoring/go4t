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
未簡約の項が残った候補やステップ数の上限に達した候補は捨てます。
証明が通った法則では、`requires` があるときだけ前提を満たす引数を探し、反例は探しません。

変数はソースの名前で表示し、一つの診断内で異なる変数の名前が衝突するときだけ `#n` を付けます。
多項式は `now - at` や `2 * a - b + 1` と表示します。
比較の向きは正規形のまま、不要な外側の括弧を省きます。
`stuck on:` の項は `LHS:` / `RHS:` と同じ表記です。

**扱えるもの:** 関数を含まないデータ(ADT、レコード、`List`、`Option`、`Result`、`Map`)、
`o{field: v}` による「他のフィールドは任意」型の不変条件、ゲート順、早期 return、冪等性、
分岐の網羅、構造的帰納、二つの `decide` の同値性、`requires` による前提条件付きの等式。
構造的帰納には std の `filter`/`head`/`append` に加え、
`std/list` が定義する `length`、`sum`、`product`、`count`、`reverse`、
`zipWith`、`unzip`、`partition`、`flatten`、`intercalate`、`init`、`span`、
`findIndex`、`indexed` の構造的モデルも使えます。
たとえば長さを保存する map や append の法則は、ライブラリ関数のまま証明できます。
実行時の累積変数による実装は変わらず、Int の周回もモデルに反映されます。
同名のユーザー関数には適用せず、コールバックの依存検査も省略しません。
各式と対象外の理由は [ADR 0014](adr/0014-structural-models-for-std-list.md) に記載しています。

**扱えないもの:** `Set`/`Array`、文字列の中身の推論、制限のない整数の順序演算、
他の fold の累積変数に関する自動的な一般化、二つの記号的な Map の等式。

証明の長さは分岐数に比例しますが、それは仕様自体の複雑さであって証明器の制約ではありません。

## 前提条件と順序

`law successor(a Int) requires a < 9223372036854775807 { (a + 1 > a) == true }`
は `proof successor(a) { refl }` で証明できます。`requires` は予約語ではありません。
Bool 型の前提条件には、等式の両辺と同じ純粋性・停止性・Float・一階の制約が適用されます。
証明は前提条件を真と仮定し、最上位の `&&` を個別の仮定に分けます。
帰納法の仮説を含む `rewrite` は、具体化した前提条件が現在の仮定の下で
`true` に簡約される場合に限り使えます。実行時の検査は生成されません。
`requires` と `sample` の併用はできません。

範囲は通常の Bool 関数として再利用できます。たとえば
`func inRange(t Int, at Int, now Int) Bool { 0 <= t && t <= 1000 && 0 <= at && at <= now && now <= 2000 }`
を定義し、各法則に `requires inRange(t, at, now)` と書きます。
関数を展開すると、範囲を直接書いたときと同じ仮定が得られます。

証明を始める前に、各条件を順に検査して仮定に加えます。
それまでの仮定から偽と分かる条件があれば、`requires` の位置にエラーを出します。

```text
law <name>: requires can never hold: <conjunct> is false given the conditions before it
```

最初の条件や前提全体が単独で偽になるときは、末尾が `is false` になります。
証明内の `if`・`match` の不可能な分岐には、この検査を適用しません。
検査は完全ではなく、`0 <= a && a <= 10 && a * 2 == 21` は検出できません。
探索候補には、正規化した前提中の Int・String 定数と、桁あふれしない範囲で Int の前後の値も加えます。
前提を満たす候補がなければ、証明失敗時の末尾は
`no arguments satisfying requires were found in 128 candidates: no counterexample could be tried`
になります。証明が通ったときは、代わりに警告を出します。

```text
warning: law <name>: no arguments satisfying requires were found in 128 candidates; the law may say nothing
```

狭い範囲では条件を満たす引数を探索で見逃すこともあるため、警告は終了ステータスを変えません。
引数の型が探索の対象外なら、試した候補数は 0 です。CLI とエディタの両方で警告を表示します。

順序推論は二段階です。

- **全順序:** 正規化した Int/String の項を内部構造に立ち入らず扱い、
  反射性・反対称性・推移性・全順序性と定数の順序を使います。
  `a < b && b < c` から `a < c` を導けますが、`a + 1` と `a` は別の項です。
- **ラップしない Int 演算:** 定数との比較から範囲を絞り、任意精度整数で区間解析します。
  両辺とその差、および単項式の途中の積が int64 に収まる場合だけ、
  比較を `0 ⋈ (q - p)` に正規化します。定数の差、または仮定した非負の一次式を
  最大二つ足して非負定数を加えた形を判定します。厳密な不等号には、
  厳密な不等号の仮定か正の定数が必要です。逆向きの規則で反証もできます。

完全な算術ソルバーではなく、非線形の判定、`/`・`%` の推論、文字列の算術は対象外です。
範囲の制約なしでは `a + 1 > a` は証明できず、
`now >= at + t` と `now - at >= t` も一致するとは限りません。
[TTL の例](../laws/testdata/requires_ttl.gt)では、範囲を指定して経過時間の法則を証明します。
