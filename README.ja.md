# go4t — Go for types

[English](README.md)

go4t は、通常の Go にコンパイルされる小さな静的型付き純粋関数型言語です。
特徴は、**純粋なコードについて等式で書いた法則を検証できること**です。
不変条件を等式で書くと、`go4t check` が全入力について検証します。
代数的データ型と網羅性検査付きのパターンマッチで状態を明示し、作用は `IO` で扱います。

`import "go:net/http"` のように、Go パッケージの binding を import 時に生成します。
IO 専用の可変参照(`Ref`)、チャネル、fiber、ファイルのストリーミング処理も使えます。
実際のクラスタで動かした Kubernetes controller は、観測から操作の適用まで go4t で書かれています。
通常の CLI・HTTP プログラムや、5.4k 行の自己ホストコンパイラの実績もあります。
プログラム全体を go4t で書くことも、次の決定関数のように純粋な部分だけを Go に組み込むこともできます。

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
すべての `t`, `at`, `now` について失敗します。

## なぜ Go だけでは足りないか

reconciler、ワークフローエンジン、ポリシー評価、料金ルールでは、
部分的にしか分からない多くの事実をもとにした判断が難しくなりがちです。
Go はその事実を `(value, ok)`、sentinel 値(`-1` など)、nil になりうるポインタ、bool フラグで表し、
判断は早期 return の連鎖になります。すべてのケースを扱えているかはテストでしか示せません。
go4t では状態とケースを明示し、純粋なコードについての等式を検証できます。

| 決定ロジックで必要なこと | go4t | Go |
| --- | --- | --- |
| N 個のうちどれか、という状態 | 直和型(`A \| B(Int) \| C`) | const + コメント、または `*T` + `bool` |
| 「すべてのケースを扱ったか?」 | 網羅性検査付きの `match`、欠けたパターンを列挙するコンパイルエラー | `default:` を書いて済ませる |
| エラーをデータとして扱う | `Result[E, A]`、`Option[A]`、`do` 記法による逐次処理 | `(T, error)` と `if err != nil` |
| 決定ロジックが I/O をしない保証 | 作用は `IO[A]` 型の値。純粋関数からは実行できない | 規律 |
| ネストしたデータの等価・順序・デバッグ表示 | `deriving Eq, Ord, Debug, Show` | 手書きか `reflect.DeepEqual` |
| コンテナに依存しない汎用的な操作 | 型クラス、高カインド多相(`Functor`、`Monad`、`Monoid`) | 型ごとにコピー |
| 「この不変条件は全入力で成り立つ」 | `law` + `proof`、コンパイル時に全値について検査 | 選んだ入力のテーブルテスト、またはランダム入力の property-based testing(`rapid`) |

すべて読みやすい Go にコンパイルされ、依存は `go.mod` で管理します。
Go パッケージは、import 時に生成する型付き binding 経由で呼べます。
実行コストは推測ではなく計測しています([bench/PERF.md](bench/PERF.md)):
値が unboxed のまま済むカーネルは Go の 1.6〜4x、永続 `Map` を更新しながら畳む処理は
19〜24x(永続データ構造の値段で、他の永続コレクションと同じ桁)、IO 限定の `std/mutmap` は
直接形式 IO lowering 後で 5x。決定ロジックや controller は Map を読むだけなので更新コストを
払いません。この先の最適化は、それを要求するワークロードが出るまで行いません。

ただし最後の行には注釈が要ります。同じ決定ロジックに対して、`rapid` による property-based testing は
試したバグ注入をすべて検出しました。法則が足すのは、確率ではなく証明であることです。
`if` が 2 つで済む `decide` なら Go のままで十分です。
Go との比較、実際の controller 2 本の移植、法則の検証 5 回の記録は
[docs/evaluation.ja.md](docs/evaluation.ja.md) にあります。

## ユースケース

go4t では CLI・HTTP プログラムや controller 全体を書けます。
作用は `IO` で扱い、純粋な部分について法則を検証します。
特に、**入力の組み合わせが多く、判断の順序に意味がある決定ロジック**で法則が役立ちます。
以下の例はこの使い方を示しています。go4t の `decide` を Go に組み込む構成も含みます。

| ユースケース | 決定ロジックの中身 | 法則で固定できること | 例 |
| --- | --- | --- | --- |
| Kubernetes controller の reconcile | 観測したリソースの状態から、次に打つ API 操作を 1 つ選ぶ | 「cordon してから退避する」などの順序、冪等性、timeout の正確な境界 | [approval-controller](examples/approval-controller)、[attachment-repair-controller](examples/attachment-repair-controller)、[node-drain-controller](examples/node-drain-controller) |
| controller 全体を go4t で | 上と同じ決定に加え、`observe`/`apply` も `go:k8s.io/client-go` に対して go4t で書く | 上に加えて射影の法則:「DaemonSet に所有される pod は `DaemonSet` に射影される」「mirror annotation が優先」 | [node-drain-typed](examples/node-drain-typed) |
| Go がクライアントを持つ go4t controller | Go で作った `*kubernetes.Clientset` を、go4t が export した `observe`/`apply`/`reconcile` の IO 関数に渡す | 決定と射影の同じ 15 法則。reconcile のループは Go で実行 | [node-drain-shell](examples/node-drain-shell) |
| admission / 認可のゲート | 複数のチェックを順に通し、最初に失敗した理由を返す | ゲートの順序、負の quota など境界値の拒否 | ADR 0002 の admission の回 |
| 配置・スケジューリング | 容量と使用量の `Map` から配置先を選び、フォールバックする | 満杯のゾーンには置かない、容量がなければどのフォールバック列でも拒否 | `laws/testdata/placement.gt`、ADR 0002 のゾーン配置 |
| 複数ステップのワークフロー | 観測 → 認可 → compare-and-swap 削除 → 公開のどの段階かを判断し、次のステップを返す | 段階の判断に漏れがない(網羅性)、ステップの順序 | [attachment-repair-controller](examples/attachment-repair-controller) |
| ポリシー評価・料金ルール | 部分的にしか分からない事実(期限、TTL、フラグ)から判定する | 期限の `>=` / `>` の境界、不正入力のケースを落とさない | 冒頭の `freshness` |
| リファクタリングの安全な実施 | 既存の `decide` を構造から書き換える | `decide(o) == decideV2(o)` を全入力で証明 | `laws/testdata/refactor_v2.gt` |

`if` が 2 つで済む判断なら、Go のままのほうが簡単かもしれません。
I/O はどちらの言語でも書けますが、法則の対象は純粋なコードだけです。
詳細は [go4t でないもの](#go4t-でないもの) を参照。

## 言語の概要

- 代数的データ型、レコード(更新構文付き)、型シノニム、タプル、永続データ構造の `List`/`Map`/`Set`/`Array`
- Hindley–Milner 型推論(ランク 1 多相)、スーパークラスと高カインドを扱える型クラス、
  `deriving Eq, Ord, Show, Debug, Functor, Foldable, Traversable`、JSON `Encode`/`Decode`
- 網羅性検査付きの `match`:ガード、タプルパターン、リストパターン、パターン束縛の `let`
- 明示的な作用:`IO[A]`、`do` 記法風のブロック、`IoError` による型付きのエラー回復、
  キャンセル対応の fiber(`fork`/`await`/race/timeout)
- `std`:文字列、数値、コレクション、JSON、HTTP クライアント/サーバ、正規表現、
  遅延評価される値、`State`、IO 専用の `Ref`/`MutMap`/チャネル、並行処理、ファイル/プロセス I/O
- 双方向の Go 連携:`import "go:net/http"` が Go パッケージの型付き binding を import 時に生成、
  `go4t build --emit-go` が go4t の関数・レコード・直和型を自然な Go の型として export
- Go のジェネリック関数は呼び出し箇所で単相化。ジェネリックな receiver のメソッドと、型引数を適用したジェネリックな名前付き型は引き続き対象外
- extern に依存する法則は `sample(N)` で `go4t test` の対象にできるが、証明にはならず、書き換え補題にも使えない
- ツール:`check`、`build`、`run`、`test`、`eval`、`fmt`、LSP サーバ、VS Code 拡張、
  `^~~~` スパン付きのソース位置診断
- Go 風の構文:`func`、`package`、`import`、波括弧、`|>` パイプライン、`_` 呼び出しプレースホルダ

### 法則(Laws)

`law` に等式を書き、`go4t check` で全入力について検証できます。

```go4t
package main
law distribute(a Int, b Int, c Int) { (a + b) * c == a * c + b * c }
proof distribute(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

等式は関数を含まないデータ上の定義的等価です。`refl`、コンストラクタの `match`、
条件の `if`、`rewrite`、`induction` を組み合わせて証明します。証明済みの法則の実行時コードは生成されません。
扱える範囲と限界は [docs/laws.ja.md](docs/laws.ja.md)、具体例は
[チュートリアル §13](docs/tutorial.md#13-laws) にあります。

## インストールとクイックスタート

**Go 1.22 以上**が PATH に必要です(go4t は Go を生成して `go build` を実行します)。

```sh
go install ./cmd/go4t
go4t eval '1 + 2'        # 3
```

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
go4t build hello/main.gt      # ./main をビルドし、Go ソースを hello/main_gen/ に残す
go4t build --emit-go generated hello/main.gt  # Go ソースのみ出力
```

## Go プログラムへの埋め込み

1. `decide/decide.gt` に決定ロジックを書く:判断に必要な状態だけを写した `Observed` レコード、
   アクションの直和型、`func decide(o Observed) Action`。
2. 生成:`go4t build --emit-go decide --package decide decide/decide.gt`
   (`//go:generate` に書く)。
3. Go 側:外部の状態を読み、`Observed` を組み立て、`Decide` を呼び、結果で switch し、作用を実行する。
   `decide` の中では I/O が起きないことをコンパイラが保証します。

雛形:[examples/go-shell](examples/go-shell)。シェル側も go4t で書くなら Go パッケージを直接
import します(`import "go:k8s.io/client-go/kubernetes"`)。handle と共有状態を観測する呼び出しは
`IO`、plain な Go struct は record になり、binding は build 時に生成・キャッシュされます。
境界の規則、`go:` import、プロジェクトと依存の構成は
[docs/go-interop.ja.md](docs/go-interop.ja.md) にあります。

## go4t でないもの

- **Go を置き換える必要はない。** 全体を go4t で書いても、既存の Go の基盤を残して
  純粋な部分だけを組み込んでもよい。法則は I/O の振る舞いを証明しない。
- **エフェクトシステムの研究言語ではない。** 作用の型は `IO` 一つ。
  可変参照、チャネルなど、共有状態を扱う操作は IO 専用。
- **証明支援系ではない。** 法則は関数を含まないデータ上の等式検査器で、戦術は固定。
  [docs/laws.ja.md](docs/laws.ja.md) を参照。
- **1.0 ではない。** 文法・std・ABI は上記の移植に耐える程度に安定し SPEC.md に記述されているが、
  まだ変わりうる。既知の制限は [SPEC §9](SPEC.md#9-known-limitations)。

## ドキュメント

| ドキュメント | 内容 |
| --- | --- |
| [Go 開発者向けチュートリアル](docs/tutorial.md) | 17 の短い章。すべての go4t スニペットを CI で型検査(英語) |
| [言語仕様](SPEC.md) | 文法、型と作用の規則、deriving、export される Go API、境界表(英語) |
| [法則](docs/laws.ja.md) | 扱えるもの、扱えないもの、証明の書き方 |
| [Go 連携](docs/go-interop.ja.md) | 埋め込みの規則、`go:` import、プロジェクトと依存 |
| [CLI リファレンス](docs/cli.ja.md) | 全コマンドと終了コード |
| [Examples](examples/README.ja.md) | 実行できる例と実行コマンド |
| [検証記録](docs/evaluation.ja.md) | 実際の controller の移植、法則の検証 5 回、Go/`rapid` との比較、`client-go` のスケール実験 |
| [標準ライブラリ](docs/stdlib.md) | ソースの宣言一覧から生成(英語) |
| [エディタ設定](editors/README.md) | stdio LSP と VS Code クライアント |
| [設計履歴](DESIGN.md) | 実装スライスごとの契約。食い違う場合は SPEC が正 |

## ライセンス

Apache License 2.0。[LICENSE](LICENSE) を参照。
