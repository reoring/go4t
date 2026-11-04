# 検証記録: どこで使われ、何が分かったか

[English](evaluation.md)

## 同じ決定ロジックでの Go と法則の比較

README の比較表の最後の行には注釈が要ります。node-drain の `decide` を素の Go でも書いて比較しました
(Go 版と `rapid` テストは ADR 0002 に記録しており、リポジトリには同梱していません)。
決定ロジック本体の行数は同じで、型の利点は確かにあるものの小さいものでした。
`pgregory.net/rapid` による property-based testing は、試した 3 つのバグ注入を既定の 100 回ですべて検出しました:

- DaemonSet を退避する
- 期限チェックを PDB チェックの後ろへ動かす
- `>=` → `>`(`now == at` の境界でだけ露呈する)

生成器が 0・空文字列・同じ値を意図的に優先するためです。
法則にしかないのは、確率ではなく証明であることです。seed も試行回数も生成器の偏りも考える必要がありません。
そのために第二言語のコストを払うかどうかは、分岐の深さ、順序を変える頻度、
ランダムテストが通ったという結果をどれだけ信用するかで決まります。
`if` が 2 つで済む `decide` なら Go のままで十分です。

## 移植と法則の検証

サンプルを書くのではなく、実際のコードを移植して検証しました。

**実際の controller 2 本を移植。** `examples/approval-controller` と
`examples/attachment-repair-controller` の決定ロジックを go4t に移し、元の Go テスト(6 本 + 22 本)を
一切変えずにそのまま通しました。approval では、元テストが一度も実行していなかった分岐を
網羅性検査が見つけ、扱うまでコンパイルを拒否しました。attachment では、観測 → 認可 → compare-and-swap 削除 → 公開という
複数ステップのワークフローを純粋な `Decide(Observed) Action` 一つにまとめ、Go 側が reconcile ごとに
最大 3 回呼ぶ形にしました。

**法則は 5 つの決定ロジックで続けて試してから**「使える」と判断しました。

| 題材 | 法則 | 正規化器の不足(正しい法則なのに証明が閉じない) |
| --- | --- | --- |
| 前述の attachment-repair controller | 4 本 | 1 件見つかり修正 |
| node-drain controller(`examples/node-drain-controller`、一から作成。撤回 → ノード不在 → cordon → 期限 → PDB → 最初に退避可能な pod の順) | 10 本 | 1 件見つかり修正 |
| ゾーン配置(`Map` で容量と使用量を管理。フォールバックリストの帰納法を含む) | 6 本 | 1 件見つかり修正 |
| admission(4 つのゲートを `Result` で連鎖) | 7 本 | なし |
| node-drain を構造から書き換え、`decide(o) == decideV2(o)` を証明 | 1 本 | なし |

見つかった不足はすべて修正し、テストに固定しました。「誤った法則が通る」健全性の欠陥は一度も出ていません。
正規化器は、全法則の両辺と証明途中のゴールを約 75,000 件のランダム代入で
`go4t run` の実行結果と照合する property test で検証しています。
admission の回では、書き手が誤った法則を検査器が初めて検出しました。
`fullPassAdmits` が負の quota は拒否されることを忘れていたもので、
診断は具体例ではなく `stuck on: if (q < 0) …` と条件を示しました。
Go + `rapid` との比較で自分たちの主張を切り下げた経緯も含め、記録は
[`docs/adr/0001-equational-laws.md`](adr/0001-equational-laws.md) と
[`docs/adr/0002-laws-map-and-diagnostics.md`](adr/0002-laws-map-and-diagnostics.md) にあります。

**移植で毎回同じ書き換えが起きました。** Go の `(value, ok)`、フラグ、sentinel 値がコンストラクタになります:

| 元の Go の書き方 | 変更後 | 効果 |
| --- | --- | --- |
| `gatesValid: Bool` + `gates: List[Gate]` | `gates: List[GateEvidence]`、`GateEvidence = Gate \| InvalidGate` | 最初の失敗の順序が Go のループと一致 |
| 「≤ 0 は不正」付きの `Option[Int]` | `Ttl = NoTtl \| InvalidTtl \| TtlNanos(Int)` | 不正ケースを見落とせない |
| `present: Bool` + `Option[Int]` | `Timestamp = NoTimestamp \| InvalidTimestamp \| AtNanos(Int)` | ネストした if/match が 1 つの match に、20 → 11 行 |
| `fromOption(gen, -1)` | `gen != Some(plan.generation)` | sentinel 値なし |

**`client-go` の 11 万行移植**は製品ではなくスケール実験として行いました。
コンパイラ修正 3 件(テストハーネスの分割、返り値位置の `match` と `if`/ブロックを
クロージャ化せず直接 lowering する変更。いずれも言語に取り込み済み)の後、コンパイルは通りました。
一方でこの時点では適用範囲の境界が出ました。インフラ層は状態・キャッシュ・I/O が主体で、
純粋関数と網羅性の利点が小さく、Go binding 層が移植したコードより大きくなりました。
当時の結論は「go4t は決定ロジックに限る。インフラは移植せず bind する」でしたが、
この結論は後日の計測で覆っています(次節)。
移植版は `experiment/client-go-gofx` ブランチ(`ports/client-go`)と、独立リポジトリ
[reoring/client-go-go4t](https://github.com/reoring/client-go-go4t) にあります。
`RESULTS.md` に検証内容、`CODE_STATS.md` に移植前後のコード統計を記録しています。

## 「決定ロジックしか書けない」の反証

上の結論の根拠は「binding 層が太る」でしたが、太さの正体は生成物(88k 行)をリポジトリに
コミットする運用でした。`import "go:net/http"` のように Go パッケージを import 時に bind し、
生成物を build 出力に閉じ込めてキャッシュする方式(ADR 0003)に変えると、std はほぼ丸ごと、
`client-go` の typed clientset も型由来の skip ゼロで届きました。
`examples/node-drain-typed` は observe/apply を含むコントローラ全体を go4t で書き、
kind クラスタで drain を実行しています。

重要なのは、このとき純粋/IO の境界を一度も緩めていないことです。
handle(共有状態)を受け取る呼び出しは全部 `IO`、Go の plain struct は record として値コピー、
という規則で、法則の対象断片は 1 行も変わりませんでした。
逆に `Time`/`Quantity` のような JSON 表現を持つ型を値として写す規則(ADR 0004 と
`json.Marshaler` キー)により `corev1.Pod` が record になり、
Kubernetes オブジェクトから `Observed` への射影関数に法則が付くようになりました
(「DaemonSet に所有される pod は `DaemonSet` に射影される」など 5 本)。
法則の対象面積が実際に広がった唯一の場所です。

## 普通のプログラムと規模

CLI+HTTP+永続化の todo、ログ集計の logstat、第三者(docs だけを読んだ fresh agent)による
ghstars/csvpivot/turnstile は、いずれも初回の実行で動きました。
規模は 5.4k 行の self-hosting コンパイラ(字句・構文・型クラス付き型検査・codegen)が
現行言語で自分を再生産し、CI が毎回 bootstrap を回しています。
第三者の摩擦で見つかった JSON の Int 丸め(`9007199254740993` が Float 経由で崩れる)は
`JInt` の分離で同日に修正し、`std/json` 経由で読んだ Int に対する法則が実行と一致するように
なりました。

## 計測された限界

残る限界は言語コアではなく値表現・ライブラリ表面・ツールチェーンにあり、全部数字つきで
[bench/PERF.md](../bench/PERF.md) にあります。

| 対象 | Go 比 | 性質 |
| --- | --- | --- |
| unboxed で済む計算(sort、Float ループ、木) | 1.6〜4x | native scalar 化で到達 |
| 永続 `Map` を更新しながら畳む処理 | 19〜24x | 永続性の値段(探索経路コピー 44%、GC 50%)。B-tree 化は -18.7% で棄却 |
| `std/mutmap`(IO の可変 map) | 5.0x | 直接形式 IO lowering(ADR 0008)で 9.4x から。残りは AVL/GC/割付で IO の固定費は消えた |

Int/String の boxing は全カーネルで CPU の 4% 未満で、「boxed 値表現が原因」という仮説は
計測で否定しました。決定ロジックや controller の `Observed` にある Map は observe で一度
作って読むだけなので、更新コストは払いません。
型システムは、式注釈・双方向推論・局所多相で消えるコードを数えて 0 箇所だったため変えて
いません。

詳細(英語): [evaluation.md](evaluation.md) の各節。
