# go4t — Go for types

[English](README.md)

go4t は、通常の Go にコンパイルされる小さな静的型付き関数型言語です。目的は一つ、
**Go プログラムの決定ロジック(decision core)を書くこと**です。観測した状態から次の一手を決める部分を、
すべての状態に名前があり、すべてのケースが検査され、I/O が一切起きず、
**決定の不変条件を法則として書き、`go4t check` が全入力について検証する**形で書きます。

外側は Go のまま、決定ロジックだけを go4t で書きます。サービスを go4t で書き直すのではなく、
`if ok { … } else if … { return … }` の連鎖を go4t に移し、それについて成り立つべきことを書き、
Go から型付き関数として呼びます。

```go4t
package decide

type Ttl       = NoTtl | InvalidTtl | TtlNanos(Int)                 deriving Eq, Debug
type Timestamp = NoTimestamp | InvalidTimestamp | AtNanos(Int)      deriving Eq, Debug
type Freshness = Fresh(Int) | TtlInvalid | TimestampMissing
               | TimestampInvalid | Stale                            deriving Eq, Debug

func freshness(ttl Ttl, observedAt Timestamp, nowNanos Int) Freshness {
  match (ttl, observedAt) {
    (NoTtl, _)                                          => Fresh(0)
    (InvalidTtl, _)                                     => TtlInvalid
    (TtlNanos(_), NoTimestamp)                          => TimestampMissing
    (TtlNanos(_), InvalidTimestamp)                     => TimestampInvalid
    (TtlNanos(t), AtNanos(at)) if nowNanos >= at + t    => Stale
    (TtlNanos(t), AtNanos(at))                          => Fresh(at + t - nowNanos)
  }
}

func main() IO[Unit] { println(debug(freshness(TtlNanos(10), AtNanos(5), 12))) }
```

`go4t build --emit-go decide --package decide decide/decide.gt` がこれを Go パッケージに変換します。
Go 側は入力を埋めて、結果で `switch` するだけです。

```go
switch step := decide.Freshness(ttl, observedAt, now.UnixNano()).(type) {
case decide.FreshnessStale:
    return requeue(ctx, obj)
case decide.FreshnessTtlInvalid:
    return reject(ctx, obj, "evidence TTL invalid")
// …
}
```

`match` の arm を 1 本消すと、`go4t check` は
`missing: (TtlNanos(_), InvalidTimestamp)` で失敗します。法則
`isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now >= at + t)` を足せば、
ガードを別のチェックの後ろへ動かしたとき、テーブルテストが並べた数件ではなく
すべての `t`, `at`, `now` について失敗します。価値の中心はこれです。
ただし、このうちどこまでが Go でも実現できるかは、下で検証しています。

## なぜ Go だけでは足りないか

Go は外側(I/O、クライアント、配線)を書く言語としては優れ、決定ロジックを書く言語としては弱いです。
reconciler、ワークフローエンジン、ポリシー評価、料金ルールのいずれでも、
難しいのは部分的にしか分かっていない多くの事実をもとにした判断です。
Go はその事実を `(value, ok)`、sentinel 値(`-1` など)、nil になりうるポインタ、bool フラグで表し、
判断は早期 return の連鎖になります。すべてのケースを扱えているかはテストでしか示せず、
テストが全組み合わせを列挙することは稀です。

go4t が決定ロジックに与えるもの:

| 決定ロジックで必要なこと | go4t | Go |
| --- | --- | --- |
| N 個のうちどれか、という状態 | 直和型(`A \| B(Int) \| C`) | const + コメント、または `*T` + `bool` |
| 「すべてのケースを扱ったか?」 | 網羅的 `match`、欠けたパターンを列挙するコンパイルエラー | `default:` を書いて済ませる |
| エラーをデータとして扱う | `Result[E, A]`、`Option[A]`、`do` による逐次化 | `(T, error)` と `if err != nil` |
| 決定ロジックが I/O をしない保証 | 効果は `IO[A]` 値。純粋関数は実行できない | 規律 |
| ネストしたデータの等価・順序・デバッグ表示 | `deriving Eq, Ord, Debug, Show` | 手書きか `reflect.DeepEqual` |
| コンテナに依存しない汎用的な操作 | 型クラス、高階種(`Functor`、`Monad`、`Monoid`) | 型ごとにコピー |
| 「この不変条件は全入力で成り立つ」 | `law` + `proof`、コンパイル時に全値について検査 | 選んだ入力のテーブルテスト、またはランダム入力の property-based testing(`rapid`) |

すべて読みやすい Go にコンパイルされ、Go の速度で動き、依存は `go.mod` で管理します。
どの Go パッケージも生成された型付き binding 経由で呼べます。

**ただし最後の行には注釈が要ります。** node-drain の `decide` を素の Go でも書いて比較しました
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

## どこで使われ、何が分かったか

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
| 上記の attachment-repair controller | 4 本 | 1 件見つかり修正 |
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
[`docs/adr/0001-equational-laws.md`](docs/adr/0001-equational-laws.md) と
[`docs/adr/0002-laws-map-and-diagnostics.md`](docs/adr/0002-laws-map-and-diagnostics.md) にあります。

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
結論が上の設計規則です。
**go4t は決定ロジックに限る。インフラは移植せず bind する。**
移植版は `experiment/client-go-gofx` ブランチ(`ports/client-go`)と、独立リポジトリ
[reoring/client-go-go4t](https://github.com/reoring/client-go-go4t) にあります。
`RESULTS.md` に検証内容、`CODE_STATS.md` に移植前後のコード統計を記録しています。

## 言語の概要

- 代数的データ型、不変更新付きレコード、透過的 alias、タプル、永続 `List`/`Map`/`Set`/`Array`
- Hindley–Milner 推論(rank-1 多相)、superclass と高階種を持つ型クラス、
  `deriving Eq, Ord, Show, Debug, Functor, Foldable, Traversable`、JSON `Encode`/`Decode`
- ガード・タプル・リストパターン・パターン `let` を備えた網羅的 `match`
- 明示的な効果:`IO[A]`、`do` 風ブロック、型付き `IoError` 回復、
  キャンセル対応の fiber(`fork`/`await`/race/timeout)
- `std`:文字列、数値、コレクション、JSON、HTTP クライアント/サーバ、正規表現、
  遅延値、`State`、並行、ファイル/プロセス I/O
- 双方向の Go 連携:`go4t bind` が任意の Go パッケージの型付き extern を生成
  (opaque handle 上のメソッド含む)、`go4t build --emit-go` が go4t の関数・レコード・直和型を
  自然な Go の型として export
- ツール:`check`、`build`、`run`、`test`、`eval`、`fmt`、LSP サーバ、VS Code 拡張、
  `^~~~` スパン付きのソース位置診断
- Go 風の構文:`func`、`package`、`import`、波括弧、`|>` パイプライン、`_` 呼び出しプレースホルダ

完全な仕様:[SPEC.md](SPEC.md)。Go 開発者向け入門:[docs/tutorial.md](docs/tutorial.md)。
std の全宣言:[docs/stdlib.md](docs/stdlib.md)。(いずれも英語)

### 法則(Laws)

`law` に等式を書き、`go4t check` で選んだテスト入力だけでなく全入力について検証できます。

```go4t
package main
law distribute(a Int, b Int, c Int) { (a + b) * c == a * c + b * c }
proof distribute(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

等式は `Eq` インスタンスとは独立した定義的等価です。`refl`、コンストラクタの
`match`、条件の `if`、構造的帰納法を含む `rewrite` を組み合わせて証明します。
対象は条件を満たす純粋で停止する依存だけで、Float や信頼されない extern は
除外されます。`Map` は記号的にモデル化され、`mapLookup(mapInsert(m, k, v), k2)` は
キーが等しいかどうかのケース分けに簡約されます。法則の実行時コードは生成されません。
[チュートリアル §13](docs/tutorial.md#13-laws) に cancel の冪等性、
`freshness` の `staleIff`、リストの帰納法の例があり、`laws/testdata/placement.gt` と
`refactor_v2.gt` に Map を持つ決定ロジックとリファクタ同値の証明があります。

**扱えるもの:** 一階データ(ADT、レコード、`List`、`Option`、`Result`、`Map`)、
`o{field: v}` による「他のフィールドは任意」型の不変条件、ゲート順、早期 return、冪等性、
分岐の網羅、構造的帰納(std の `filter`/`head`/`append` を含む)、二つの `decide` の同値性。

**扱えないもの:** `Set`/`Array`、文字列の中身の推論、比較をまたぐ順序推論
(`a < b && b < c ⇒ a < c`)、前提条件(`requires`)、fold ベースの std 関数への帰納、
二つの記号的な Map の等式。

証明の長さは分岐数に比例しますが、それは仕様自体の複雑さであって証明器の制約ではありません。

## インストールとクイックスタート

**Go 1.22 以上**が PATH に必要です(go4t は Go を生成して `go build` を実行します)。

```sh
go install ./cmd/go4t
go4t eval '1 + 2'        # 3
```

バイナリはランタイムを埋め込んでいるので、`run`、`test`、`eval` は `go.mod` なしの任意の
ディレクトリで動きます。インストールしない場合は `go run ./cmd/go4t` を使ってください。

`hello/main.gt` を作ります:

```go4t
package main
import "std/string"
func main() IO[Unit] {
  let names = [" Ada ", " Bob "] |> map(string.trim)
  println("Hello, " + string.join(" and ", names) + "!")
}
```

```sh
go4t check hello/main.gt
go4t run hello/main.gt        # Hello, Ada and Bob!
go4t build hello/main.gt      # hello/main_gen/ に Go ソースを出力
```

## Go プログラムへの埋め込み

想定している使い方は次のとおりです。

1. `decide/decide.gt` に決定ロジックを書く:判断に必要な状態だけを写した `Observed` レコード、
   アクションの直和型、`func decide(o Observed) Action`。
2. 生成:`go4t build --emit-go decide --package decide decide/decide.gt`
   (`//go:generate` に書く)。パッケージは `Decide`、全レコード・直和型の Go ミラー
   (`ObservedGo`、`ActionRollout{}`、`TtlTtlNanos{F0: …}`)、`Main() rt.IO` を export します。
3. Go 側:外部の状態を読み、`Observed` を組み立て、`Decide` を呼び、結果で switch し、効果を実行する。
   `decide` の中では I/O が起きないことをコンパイラが保証します。

境界をきれいに保つ規則(SPEC §8.7、tutorial §12):

- 判断に必要なフィールドだけを `Observed` に写す。呼び出しごとにレコードがコピーされるため。
- sentinel 値を境界に渡さない。`Option[Int]` はポインタなしのミラーで渡し、
  `-1` に変換せず `Some(x)` と比較する。
- 期間とタイムスタンプは `Int` ナノ秒で渡す(`time.Duration` と `UnixNano` の内部表現と同じ)。
  境界で丸めが起きない。
- リスト全体にかかる妥当性フラグは、要素ごとの直和型にする。「最初に失敗する要素」が
  元の Go のループと厳密に一致する。

雛形:[examples/go-shell](examples/go-shell)。実際の移植例:
[examples/approval-controller](examples/approval-controller)、
[examples/attachment-repair-controller](examples/attachment-repair-controller)。

## go4t から Go を呼ぶ

```sh
go4t bind strings --prefix str          # extern 宣言を出力
go4t bind modernc.org/sqlite -o bindings.gt
```

生成される extern は関数、opaque handle 上のメソッド、スライス、Option、エラーを扱えます。
ミラーできないシグネチャは、誤った binding を出す代わりにスキップコメントになります。
[examples/sql-service](examples/sql-service) はこの方法で組んだ SQLite + Prometheus metrics の HTTP サービス、
[examples/typed-extern](examples/typed-extern) は ABI アダプタの例です。
`std/http` に binding は不要です。

## プロジェクトと依存

go4t ライブラリは `go.mod`、`go4t.mod`、`.gt` ファイルを持つ Go module です。

```sh
go4t init example.com/myapp            # go.mod + go4t.mod を作成
go4t get example.com/lib@v1.0.0        # go get して go4t.mod に記録
```

バージョン選択とチェックサムは Go が担当し、`go4t.mod` は言語 import を列挙します。
import はファイル単位で修飾付き(`import "std/io"` → `io.readFile`)、暗黙に入るのは小さな
core prelude だけです。解決順:import 元 module のソースルート、`go4t.mod` の require、
`GO4T_PATH`、埋め込み std。オフラインでローカル replace を使う構成は [examples/deps](examples/deps) を参照。

## CLI

| コマンド | 目的 |
| --- | --- |
| `go4t check [--full-types] FILE.gt\|DIR...` | 読み込み・型検査・法則と証明の検証。warning では失敗しない。foreign ABI はここでは検査しない。 |
| `go4t build [--full-types] [-o DIR] FILE.gt\|DIR...` | go4t module ごとに 1 Go ファイルを 1 Go パッケージへ出力。 |
| `go4t build --emit-go DIR --package NAME FILE.gt\|DIR...` | 型付き export を持つ import 可能な Go パッケージを出力(§8.7)。 |
| `go4t run [--full-types] FILE.gt\|DIR... [-- args...]` | ビルドして実行。stdin を継承、終了コードを保持。 |
| `go4t test FILE.gt\|DIR...` | `Bool` または `IO[Bool]` を返す引数なしの `testName` 関数を実行。 |
| `go4t eval 'EXPR' [FILE.gt\|DIR...]` | 純粋値または IO 結果を評価。import は最初のファイルから。 |
| `go4t bind IMPORT [-o FILE] [--prefix STR]` | Go extern 宣言を生成。 |
| `go4t fmt [-w\|-d\|-l] FILE...\|DIR` | 正規整形。`-l` は差分があれば 1 で終了。 |
| `go4t init <module-path>` / `go4t get <path>@<version>` | プロジェクトと依存の管理。 |
| `go4t lsp` | stdio 言語サーバ(診断、hover、定義、シンボル)。 |

証明を検証するのは `check` だけです。`build`、`run`、`test`、`eval` は法則と証明を
生成コードに含めませんが、構文・型検査は行います。

終了コード:通常の IO エラーは 1、捕捉されない action panic は 2。プログラム内での回復は
`io.tryIO(action) : IO[Result[io.IoError, A]]`。

## Examples

コマンドは checkout ルートから実行します。ネットワークを使う例は明記しています。
主要な例のみ示します。全例は英語版 [README.md](README.md#examples) の表を参照してください。

| 例 | 内容 | 実行 |
| --- | --- | --- |
| [go-shell](examples/go-shell) | 最小の Go 外側 + go4t 決定ロジックの雛形 | `go4t run examples/go-shell/core.gt` |
| [approval-controller](examples/approval-controller) | 実際の controller。元テスト 6 本無変更 | `cd examples/approval-controller && go generate ./... && go test ./...` |
| [attachment-repair-controller](examples/attachment-repair-controller) | 複数ステップのワークフローの決定ロジックを go4t 化。元テスト 22 本無変更 | `cd examples/attachment-repair-controller && go generate ./... && go test ./...` |
| [node-drain-controller](examples/node-drain-controller) | 一から書いた controller。検証済み法則 10 本(cordon 先行、DaemonSet 透過、正確な timeout) | `go4t check examples/node-drain-controller/decide/decide.gt && cd examples/node-drain-controller && go generate ./... && go test ./...` |
| [sql-service](examples/sql-service) | `bind` 経由の SQLite + Prometheus HTTP サービス | README 参照 |
| [typed-extern](examples/typed-extern) | 生成 binding、ABI アダプタ、opaque method handle | `go4t run examples/typed-extern` |
| [deps](examples/deps) | ローカル replace を使うオフライン依存 | `go4t run examples/deps/app` |
| [calc](examples/calc) | `Result` エラー付き式インタプリタ | `go4t run examples/calc/main.gt` |
| [cards](examples/cards) | ポーカー役判定、derived 順序、seed 付き `State` | `go4t run examples/cards/main.gt` |
| [scheduler](examples/scheduler) | カスタム順序、ガード、上限付きスケジューリング | `go4t run examples/scheduler/main.gt` |
| [sales-report](examples/sales-report) | 並列取得、型付き IO 回復、フォールバック | `go4t run examples/sales-report` |
| [http](examples/http) | `std/http` サーバ/クライアント、JSON ルート、キャンセル(loopback) | `go4t run examples/http/main.gt -- 8080` |
| [concurrency](examples/concurrency) | fork/await、順序付き並列、race、timeout | `go4t run examples/concurrency/main.gt` |
| [json-deriving](examples/json-deriving) | JSON スキーマ導出とネストしたエラーパス | `go4t run examples/json-deriving/main.gt` |
| [diagnostics](examples/diagnostics) | 網羅性・alias 不一致の診断(`bad_*.gt` は意図的に失敗) | `go4t run examples/diagnostics/main.gt` |
| [cli-tools](examples/cli-tools) | 純粋/IO テストと eval。`go4t test examples/cli-tools/main.gt` も | `go4t run examples/cli-tools/main.gt` |
| [bank](examples/bank)、[inventory](examples/inventory)、[orders](examples/orders) | HTTP 通知付きドメイン検証(**ネットワーク**) | `go4t run examples/<name>/main.gt` |

失敗を確認する例:`go4t test examples/cli-tools/failing.gt` は 1 で終了、
`go4t check examples/diagnostics/bad_hint.gt` は 1 で終了、
`go4t check examples/diagnostics/bad_unused.gt` は warning を出して 0 で終了します。

## go4t でないもの

- **Go の代替ではない。** サービス、キャッシュ、クライアント、I/O が主体のものは Go に残す。
  bind して使い、移植しない。
- **効果システムの研究言語ではない。** 効果型は `IO` 一つで、言語に可変参照はない。
  共有可変状態が必要な設計なら、その状態は境界の Go 側に置く。
- **証明支援系ではない。** 法則は一階データ上の等式検査器で、戦術は固定。限界は
  [法則](#法則laws) の節を参照。controller の決定の順序を固定し安全にリファクタリングするには
  足りるが、算術や文字列の性質を証明するものではない。
- **1.0 ではない。** 文法・std・ABI は上記の移植に耐える程度に安定し SPEC.md に記述されているが、
  まだ変わりうる。既知の制限は [SPEC §9](SPEC.md#9-known-limitations):rank-1 多相のみ、
  多引数型クラスなし、文字型なし、選択的 unboxing、`check` は foreign Go シグネチャを
  検証できない(`build`/`run` は検証する)。

## ドキュメントとエディタ

- [Go 開発者向けチュートリアル](docs/tutorial.md) — Go 外側 / go4t 決定ロジックの分担と法則を扱う 13 の短い章。
  全スニペットを CI で型検査。
- [言語仕様](SPEC.md) — 文法、型と効果の規則、CLI、deriving、export される Go API、境界表。
- [標準ライブラリ](docs/stdlib.md) — ソースの宣言一覧から生成。
- [エディタ設定](editors/README.md) — stdio LSP と VS Code クライアント(`go4t.executablePath`)。
- [設計履歴](DESIGN.md) — 実装スライスごとの契約。食い違う場合は SPEC が正。

ドキュメントの回帰テスト:`go test ./docs ./std` が SPEC、tutorial、stdlib、英語 README の
すべての `go4t` fence を取り出し、`go4t check` と同じローダと型検査器にかけます。
`go4t error` fence は拒否されなければなりません。

`selfhost/` は別管理の進行中セルフホストコンパイラで、リリース対象ではありません。

## ライセンス

Apache License 2.0。[LICENSE](LICENSE) を参照。
