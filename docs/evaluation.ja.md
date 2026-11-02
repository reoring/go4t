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
一方で適用範囲の境界がはっきりしました。インフラ層は状態・キャッシュ・I/O が主体で、
純粋関数と網羅性の利点が小さく、Go binding 層が移植したコードより大きくなります。
結論が README の設計規則です。
**go4t は決定ロジックに限る。インフラは移植せず bind する。**
移植版は `experiment/client-go-gofx` ブランチ(`ports/client-go`)と、独立リポジトリ
[reoring/client-go-go4t](https://github.com/reoring/client-go-go4t) にあります。
`RESULTS.md` に検証内容、`CODE_STATS.md` に移植前後のコード統計を記録しています。
