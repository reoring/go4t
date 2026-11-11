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

## controller 全体を go4t で書く

上の結論の根拠は「binding 層が太る」でしたが、太さの正体は生成物(88k 行)をリポジトリに
コミットする運用でした。`import "go:net/http"` のように Go パッケージを import 時に bind し、
生成物を build 出力に閉じ込めてキャッシュする方式(ADR 0003)に変えると、std はほぼ丸ごと、
`client-go` の typed clientset も型由来の skip ゼロで届きました。
`examples/node-drain-typed` は observe/apply を含むコントローラ全体を go4t で書き、
kind クラスタで drain を実行しています。

**純粋/IO の境界は緩めていません。** handle を受け取る呼び出しは `IO` です。
当初は JSON 表現を持つ `metav1.Time` や `resource.Quantity` を `dyn.Value` に写し、
`corev1.Pod` を record として扱いました。この規則は後述の ADR 0011 で見直しています。
現在は JSON を明示的な変換用のビューとし、元の値は handle として保持します。

`corev1.Pod` から決定ロジックの `Pod` への純粋な変換には、さらに 5 本の法則があります。
`ownerReferences` のどこにあっても DaemonSet の所有を検出する法則はリスト帰納法、
mirror annotation が所有関係より優先される法則は symbolic `Map` を使います。
timestamp に依存しないことも検証しています。
`rapid` の生成器で `ownerReferences` の形を手で決めるのとは異なり、
法則はすべての形を対象にします。

残った課題は適用範囲ではなくコストです。ただし、以前の `run` の
cold 150 秒 / warm 85 秒は binding キャッシュ導入前の値です。
AMD Ryzen 9 7950X3D の Linux VM (30 vCPU)、Go 1.26.1 で、
ほかの処理が動く中、Go のビルドキャッシュを温めて再計測しました。
export data の取得をまとめると、binding キャッシュが空 / warm の `build` は
53.93 / 30.14 秒から 31.81 / 8.92 秒になりました。
末尾のコメントだけを変えた後は 36.08 秒から 8.72 秒になりました。
codegen の `go list` は 248 回から 1 回に減り、生成する Go は変わりません。
`check` は cold 28.88 秒 / warm 2.57 秒で、15 法則すべてが証明されました。
計測では、同時に行われるリポジトリの commit の影響を除くため、
変更前後とも VCS 情報の埋め込みを無効にしました。
`run`/`test`/`eval` の一時バイナリではこれを自動で無効にし、
配布用の `build` では Go の既定動作を保ちます。
[処理別の時間とコマンド](../examples/node-drain-typed/README.md#build-cost-remeasurement-2026-11-11)を参照してください。
controller は実行せず、ビルドだけを計測した値です。

## 普通のプログラム

**2026-11-04 に、docs と examples だけから書いた 2 本が初回実行で動きました。**

| プログラム | 内容 | 結果 | 証明した法則 |
| --- | --- | --- | --- |
| `examples/todo` | `add\|list [--all]\|done\|rm\|serve`、`TODO_FILE` に JSON 保存、導出した `json.Encode/Decode`、`std/http` の `GET/POST /todos`、`POST /todos/<id>/done`、`DELETE /todos/<id>`。`todo.gt` + `main.gt` は約 260 行 | CLI と HTTP は想定どおり。`check` の失敗は法則だけ | 3 |
| `logstat` | 新規エージェントによる未コミットの調査用プログラム。正規表現、`std/map` 集計、ソート済み表と `--json`、`import "go:path/filepath"` | 30 行中 25 行を処理し、5 行をスキップ。テキストと JSON は正しかった | 3 |

todo の `markDone(markDone(xs, id), id) == markDone(xs, id)` は、
symbolic record のネストした更新を統合できない証明器の不足を見つけました。
同日に SPEC §6.5 の規則を追加し、単一コンストラクタの更新を統合、
後の書き込みを優先し、フィールドを index 順にすると、リスト帰納法で証明できました。

文書に記載した制限も確認しました。`list.length` は accumulator fold のため、
長さ不変の法則には構造再帰の `count` を使います。
正規表現の具体例は `law depends on extern go4t/rt.RegexCaptures` で拒否され、
`go4t test` に書く必要がありました。
`s.next` が既存の `s.items` に含まれるケースを忘れた誤った法則も、
正規形が一致しないという診断で拒否されました。

当初不足していた可変参照、ファイルの行ストリーミング、実行ファイル生成は、
同日の追加実装で IO 専用 `Ref`、`foldLines`/`forEachLine`、
`go4t build` の実行ファイル出力として入りました。
ソースだけを出す指定は `--emit-go` です。方向付き channel と native Go binding も追加しました。
CI は `main.gt` 単体ではなく、複数ファイルの example ディレクトリを実行する形に直しました。
5 本の実行時間比較では Go と go4t の出力が一致しました。
朝の比率 1.75〜31.78x は、後述の改善で unboxed 1.6〜4x、
`mutmap` 5x、永続 Map 更新 19〜24x になりました。

## 規模: self-hosted compiler

**5.4k 行のコンパイラが、現行言語で自分自身を再生成します。**
2026-11-04 の確認時、`selfhost/` は baseline `6b189c0` では固定点に達していましたが、
HEAD では `parser.gt:757: expected expression, found "alias"` で失敗し、
prelude の参照先も存在しませんでした。

同日の修正は deep-worker エージェント 1 つで約 48 分でした。
4,426 → 5,447 行に増え、`modules.gt`、`derive_higher.gt`、`derive_json.gt` を追加しました。
`std/` とローカル `go4t.mod`、各種 deriving
(`Debug`/`Show`/`Functor`/`Foldable`/`Traversable`/`JSON`)、
複数制約、pipeline と section、`Float`、alias と共通フィールド、
リストパターン、局所再帰群、IO の暗黙 tail lift (ADR 0005) に対応しています。
`law` と `proof` は解析しますが、`import "go:…"` は
`selfhost does not bind Go packages` で拒否します。

`bash selfhost/bootstrap.sh` は主担当の実行で 29.5 秒でした。
34 example の stdout が `cmd/go4t` とバイト単位で一致しました。
非決定的な `concurrency` と `sales-report` は終了状態と行数で比較しました。
`go:` import、network listener、cluster、Go module 依存の 9 件は明示的にスキップし、
結果は `FIXED POINT: stage2 == stage3` でした。
残る制限は `bench/PERF.md` の性能と SPEC §9 の型表現力であり、
大きいプログラムを書けないという制限ではありません。

## 計測された制限への対応

残る限界は言語コアではなく値表現・ライブラリ表面・ツールチェーンにあり、全部数字つきで
[bench/PERF.md](../bench/PERF.md) にあります。

| 対象 | Go 比 | 性質 |
| --- | --- | --- |
| unboxed で済む計算(sort、Float ループ、木) | 1.6〜4x | native scalar 化で到達 |
| 永続 `Map` を更新しながら畳む処理 | 19〜24x | 永続性のコスト(探索経路コピー 44%、GC 50%)。B-tree 化は改善 18.7% に対して別の workload が悪化し、棄却 |
| `std/mutmap`(IO の可変 map) | 5.0x | 直接形式 IO lowering(ADR 0008)で 9.4x から。残りは AVL/GC/割付で IO の固定費は消えた |

Int/String の boxing は全カーネルで CPU の 4% 未満で、「boxed 値表現が原因」という仮説は
計測で否定しました。決定ロジックや controller の `Observed` にある Map は observe で一度
作って読むだけなので、更新コストは払いません。
型システムは、式注釈・双方向推論・局所多相で消えるコードを数えて 0 箇所だったため変えて
いません。

**計測に基づいて実装を選びました。** native 比較・sort・scalar fold・numeric record の
非公開 accumulator により、sortints は 0.24 秒 (4.00x)、nbody は 0.30 秒 (1.58x)、
treefold は 1.75x でした。入力 record は不変のままです。
共有 immutable map entry 導入後も wordfreq は 2.45 秒 (約 27x)、
mapheavy は約 2.3 秒 (約 19x) でした。
order-8 B-tree は mapheavy が 18.7% 改善した一方、
wordfreq が 5.5% 悪化し、割り当て量も増えたため採用しませんでした。
replacement metadata の再利用も試して採用しませんでした。

| 対応 | 観測と判断 |
| --- | --- |
| 型の使い勝手 | 式注釈・双方向推論・局所多相で改善する箇所は 0。69 箇所の if-else に対し `prelude.when`/`prelude.unless` を追加。引数は strict のままで、選ばれなかった IO は実行しない |
| generic Go 関数 | 呼び出し箇所で具体化し、skip は slices 40→0、maps 10→0、sync 3→1、sets 25→16、workqueue 90→76。制約は使用箇所で検査し、generic 呼び出しと callback は IO。generic receiver method は未対応 (ADR 0006) |
| sampled law と帰納法 | `sample(N)` は `go4t test` で実行し、`tested, not proved` と表示。rewrite lemma には使えない。`induction` は網羅的なコンストラクタ帰納法と構造的に小さい仮説の適用を行い、各 arm の tactic 指定と、各 arm に `refl` を使う省略形を用意 (ADR 0007) |
| direct-style IO | List/Map/Array の要素 unboxing は採用せず、builtin IO の `do` を逐次 Go 文に変換。top-level の自己 tail call は cancellation 検査付き loop (ADR 0008) |

`mutmap` の初期値は 1.13 秒 (9.42x) でした。
ADR 0008 前の profile は tree 割り当て約 14%、IO closure 約 17%、`Some` 6%、GC 35% でした。
直接形式への変更では mapheavy-mut が 1.04 → 0.55 秒 (5.00x)、
peak RSS が 385 → 113 MB となり、`rt.Bind` が profile から消えました。
目標の 4x には達していません。残りは AVL、GC、比較、node と `Some` の割り当て、
Int boxing などデータ構造側のコストです。
評価順序と `tryIO` の捕捉範囲を保ち、`fork`/`foldLines`/`tryIO` 内の
非 tail 再帰は通常の呼び出しを使います。
決定的な CI example 41 本の stdout、law harness、self-hosting 固定点で一致を確認しました。
次の改善は IO 負荷の高い workload で必要になるまで保留です。

## 第三者が使って分かった課題

**4 本とも最初の実行で動きましたが、最初の証明検査やテスト呼び出しまで成功したとは限りません。**
2026-11-04、新規エージェントが実装を読まず、docs と examples だけから
logstat、ghstars、csvpivot、turnstile を書きました。
記録は検証用 checkout の `.tmp/probe-{logstat,ghstars,csvpivot,turnstile}/FRICTION.md`
にあります。ignored artifact のため、配布物には含まれません。

各 `Verdict:` 行を最初の分類で 1 回だけ数えました。
プログラム数、失敗コマンド数、独立した不具合数ではなく、観測の件数です。

| プログラム | 初回成功 | 言語の不足 | ライブラリの不足 | 文書の不足 | 自分の誤り | その他 | 合計 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| logstat | 3 | 1 | 1 | 1 | 0 | 1 | 7 |
| ghstars | 0 | 3 | 2 | 0 | 3 | 0 | 8 |
| csvpivot | 0 | 2 | 2 | 0 | 0 | 2 | 6 |
| turnstile | 4 | 0 | 1 | 0 | 3 | 0 | 8 |

「その他」には logstat の総合評価と csvpivot の「まだ障害なし」「文書どおりの境界」を含みます。
turnstile の「自分の誤り」2 件には文書の不足という副分類もあります。
誤った法則の診断に成功した 1 件にも、生成 binding を期待するなら言語の不足、という条件があります。
副分類は重複計上していません。ghstars と csvpivot の初回実行成功は
`Verdict:` 行の外にあるため、表の初回成功は 0 です。

| 課題 | 対応・判断 |
| --- | --- |
| JSON の Int 精度 | turnstile で `9007199254740993` が `9007199254740992` になった。同日に Float 経由の符号化をやめ、Int を正確に保持するよう修正 |
| HTTP loopback と動的 port | ghstars は全 interface と事前指定 port しか使えなかった。同日に host 指定 API と port zero の割当済み port 通知を追加 |
| 文書 | 未対応のリスト append 記法を `list.append` に修正。Int 境界 helper は `std/float` と明記。最小 Int literal、条件式を等式の項に使うときの括弧、証明失敗と sampled counterexample の違い、生成 Go 名の調べ方、全体読み込みと streaming の改行処理の一致を説明 |
| 残る制限 | Float の法則と sort は普遍的証明の対象外。Float 加算は実際に順序に依存する。標準 CSV codec と URL parser はない。調査では Int cents の証明、明示的 sampling、アプリ側の CSV/URL parser を使い、証明済みとは扱わなかった |

## 長時間稼働の検証: node-drain-typed (2026-11-04)

**3 時間で 839 cycle がすべて drain と復旧に成功しました。**
それまでは 1 reconcile を超える運用の検証がありませんでした。
`examples/node-drain-typed/soak` は専用 kind cluster
(worker 1 台、2 replica の Deployment、`minAvailable: 1` の PodDisruptionBudget) で、
cordon → `MarkDrained` まで evict → `--release` → 代替 pod の待機を繰り返します。

| 指標 | 値 |
| --- | --- |
| cycle | 839、すべて drain・復旧 |
| cycle あたり reconcile | 平均 4.25、最大 16 |
| cycle あたり実時間 | 平均 0.41 秒、最大 20.8 秒 (PDB back-off) |
| 1 reconcile の最大 RSS | 35.8〜38.4 MB、3 時間を通じて横ばい |
| PDB による eviction 拒否 | 49、すべて `TooManyRequests:` prefix で分類し再試行 |
| 未分類 stderr | `pods "…" not found` が 8 回 (exit 1、次の reconcile は成功) |

8 回の失敗だけが新たな問題でした。observe と apply の間に ReplicaSet が切り替わり、
観測した pod が消えていました。API error は既に `io.NotFound` と分類されていましたが、
example がそれを失敗として返していました。
`evict` は、消えた pod を処理済みとして扱うよう修正しました。
observe が見つからない node を扱うのと同じ判断です。

`IoError` に `Throttled` コンストラクタは追加しません。
49 回の back-off を prefix で曖昧さなく分類できたためです。
cordon は merge patch、eviction は create で、いずれも resourceVersion を持たないため
`Conflict` は観測されませんでした。`Update` を使う controller が現れるまで追加しません。
メモリと reconcile 時間に継続的な増加はありませんでした。
なお prefix による 429 分類は記録時の実装です。
現在の `evict` は保持された `io.cause` と `apierrors.isTooManyRequests` を使います (ADR 0011 §3C)。

## 既存 controller を層ごとに移行する (2026-11-04)

**Go は 271 → 88 行になり、元の 6 テストは変えていません。**
`examples/approval-controller` は AppThrust の controller を抽出し、
3 段階で `observe`・`decide`・`apply` を go4t に移しました。
Go に残るのは reconcile loop、manager の設定、controller-runtime client です。
export 側で `*kubernetes.Clientset`、`client.Client`、
`*unstructured.Unstructured` を同一性を保って渡す必要がありました (SPEC §8.7)。
`examples/node-drain-shell` では同じ構成を kind cluster で検証しています。

各段階で、未対応ではなく「呼べるが値を失う」binding 規則が見つかりました。

| 発見箇所 | 情報が失われる規則 | 修正 |
| --- | --- | --- |
| observe | upcast が同一 package 内だけを探し、`unstructured.Unstructured` を `client.Object` に渡せない | ADR 0009: direct import 間の双方向 upcast |
| 生成 shim の vet | package 変数・field・receiver が lock を持つ struct をコピー | ADR 0009: address-of shim、lock の値渡しは skip |
| list option | named string の `client.InNamespace` に upcast がない | ADR 0010: scalar/collection implementer の upcast |
| apply | `meta.SetStatusCondition(*[]Condition, …)` がコピーを変更 | ADR 0011: 引数 `*T` は `Ref[T]` |
| apply | `metav1.Time` を JSON に変換し、`UnmarshalJSON` が location を変更 | ADR 0011: JSON は `timeToJson`/`timeFromJson` によるビュー、値そのものは handle |
| soak | `apierrors.IsTooManyRequests(error)` にメッセージから作り直した error を渡す | ADR 0011: error は `GoError` handle、`Failure` は cause を保持 |

**境界は情報を失う変換をしません (ADR 0011)。**
全 field が export された struct はコピーし、それ以外は同一性を保持します。
情報が失われる操作は `…FromJson`・`…Parse` のような名前付き関数か、
`time.Time` → `Int` のように表で lossy と明記されたものだけです。
各修正は生成 shim を実行する fixture に固定しました。
node-drain-typed の 15 法則と warm check 2.4 秒は、どの変更でも変わりませんでした。
