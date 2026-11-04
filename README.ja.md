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
すべての `t`, `at`, `now` について失敗します。

## なぜ Go だけでは足りないか

Go は外側(I/O、クライアント、配線)を書く言語としては優れ、決定ロジックを書く言語としては弱いです。
reconciler、ワークフローエンジン、ポリシー評価、料金ルールのいずれでも、
難しいのは部分的にしか分かっていない多くの事実をもとにした判断です。
Go はその事実を `(value, ok)`、sentinel 値(`-1` など)、nil になりうるポインタ、bool フラグで表し、
判断は早期 return の連鎖になります。すべてのケースを扱えているかはテストでしか示せません。

| 決定ロジックで必要なこと | go4t | Go |
| --- | --- | --- |
| N 個のうちどれか、という状態 | 直和型(`A \| B(Int) \| C`) | const + コメント、または `*T` + `bool` |
| 「すべてのケースを扱ったか?」 | 網羅性検査付きの `match`、欠けたパターンを列挙するコンパイルエラー | `default:` を書いて済ませる |
| エラーをデータとして扱う | `Result[E, A]`、`Option[A]`、`do` 記法による逐次処理 | `(T, error)` と `if err != nil` |
| 決定ロジックが I/O をしない保証 | 作用は `IO[A]` 型の値。純粋関数からは実行できない | 規律 |
| ネストしたデータの等価・順序・デバッグ表示 | `deriving Eq, Ord, Debug, Show` | 手書きか `reflect.DeepEqual` |
| コンテナに依存しない汎用的な操作 | 型クラス、高カインド多相(`Functor`、`Monad`、`Monoid`) | 型ごとにコピー |
| 「この不変条件は全入力で成り立つ」 | `law` + `proof`、コンパイル時に全値について検査 | 選んだ入力のテーブルテスト、またはランダム入力の property-based testing(`rapid`) |

すべて読みやすい Go にコンパイルされ、Go の速度で動き、依存は `go.mod` で管理します。
どの Go パッケージも生成された型付き binding 経由で呼べます。

ただし最後の行には注釈が要ります。同じ決定ロジックに対して、`rapid` による property-based testing は
試したバグ注入をすべて検出しました。法則が足すのは、確率ではなく証明であることです。
`if` が 2 つで済む `decide` なら Go のままで十分です。
Go との比較、実際の controller 2 本の移植、法則の検証 5 回の記録は
[docs/evaluation.ja.md](docs/evaluation.ja.md) にあります。

## ユースケース

go4t が向くのは、**入力の組み合わせが多く、判断の順序に意味がある決定ロジック**です。
Go 側で状態を集めて `Observed` を作り、go4t の `decide` が次の一手を返し、Go 側が実行します。

| ユースケース | 決定ロジックの中身 | 法則で固定できること | 例 |
| --- | --- | --- | --- |
| Kubernetes controller の reconcile | 観測したリソースの状態から、次に打つ API 操作を 1 つ選ぶ | 「cordon してから退避する」などの順序、冪等性、timeout の正確な境界 | [approval-controller](examples/approval-controller)、[attachment-repair-controller](examples/attachment-repair-controller)、[node-drain-controller](examples/node-drain-controller) |
| admission / 認可のゲート | 複数のチェックを順に通し、最初に失敗した理由を返す | ゲートの順序、負の quota など境界値の拒否 | ADR 0002 の admission の回 |
| 配置・スケジューリング | 容量と使用量の `Map` から配置先を選び、フォールバックする | 満杯のゾーンには置かない、容量がなければどのフォールバック列でも拒否 | `laws/testdata/placement.gt`、ADR 0002 のゾーン配置 |
| 複数ステップのワークフロー | 観測 → 認可 → compare-and-swap 削除 → 公開のどの段階かを判断し、次のステップを返す | 段階の判断に漏れがない(網羅性)、ステップの順序 | [attachment-repair-controller](examples/attachment-repair-controller) |
| ポリシー評価・料金ルール | 部分的にしか分からない事実(期限、TTL、フラグ)から判定する | 期限の `>=` / `>` の境界、不正入力のケースを落とさない | 冒頭の `freshness` |
| リファクタリングの安全な実施 | 既存の `decide` を構造から書き換える | `decide(o) == decideV2(o)` を全入力で証明 | `laws/testdata/refactor_v2.gt` |

向かないもの: I/O・キャッシュ・クライアントが主体の層。`if` が 2 つで済む判断。
詳細は [go4t でないもの](#go4t-でないもの) を参照。

## 言語の概要

- 代数的データ型、レコード(更新構文付き)、型シノニム、タプル、永続データ構造の `List`/`Map`/`Set`/`Array`
- Hindley–Milner 型推論(ランク 1 多相)、スーパークラスと高カインドを扱える型クラス、
  `deriving Eq, Ord, Show, Debug, Functor, Foldable, Traversable`、JSON `Encode`/`Decode`
- 網羅性検査付きの `match`:ガード、タプルパターン、リストパターン、パターン束縛の `let`
- 明示的な作用:`IO[A]`、`do` 記法風のブロック、`IoError` による型付きのエラー回復、
  キャンセル対応の fiber(`fork`/`await`/race/timeout)
- `std`:文字列、数値、コレクション、JSON、HTTP クライアント/サーバ、正規表現、
  遅延評価される値、`State`、並行処理、ファイル/プロセス I/O
- 双方向の Go 連携:`go4t bind` が任意の Go パッケージの型付き extern を生成、
  `go4t build --emit-go` が go4t の関数・レコード・直和型を自然な Go の型として export
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
条件の `if`、構造的帰納法を含む `rewrite` を組み合わせて証明します。法則の実行時コードは生成されません。
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

雛形:[examples/go-shell](examples/go-shell)。境界の規則、go4t から Go を呼ぶ `go4t bind`、
プロジェクトと依存の構成は [docs/go-interop.ja.md](docs/go-interop.ja.md) にあります。

## go4t でないもの

- **Go の代替ではない。** サービス、キャッシュ、クライアント、I/O が主体のものは Go に残す。
  bind して使い、移植しない。
- **エフェクトシステムの研究言語ではない。** 作用の型は `IO` 一つで、言語に可変参照はない。
- **証明支援系ではない。** 法則は関数を含まないデータ上の等式検査器で、戦術は固定。
  [docs/laws.ja.md](docs/laws.ja.md) を参照。
- **1.0 ではない。** 文法・std・ABI は上記の移植に耐える程度に安定し SPEC.md に記述されているが、
  まだ変わりうる。既知の制限は [SPEC §9](SPEC.md#9-known-limitations)。

## ドキュメント

| ドキュメント | 内容 |
| --- | --- |
| [Go 開発者向けチュートリアル](docs/tutorial.md) | 13 の短い章。全スニペットを CI で型検査(英語) |
| [言語仕様](SPEC.md) | 文法、型と作用の規則、deriving、export される Go API、境界表(英語) |
| [法則](docs/laws.ja.md) | 扱えるもの、扱えないもの、証明の書き方 |
| [Go 連携](docs/go-interop.ja.md) | 埋め込みの規則、`go4t bind`、プロジェクトと依存 |
| [CLI リファレンス](docs/cli.ja.md) | 全コマンドと終了コード |
| [Examples](examples/README.ja.md) | 実行できる例と実行コマンド |
| [検証記録](docs/evaluation.ja.md) | 実際の controller の移植、法則の検証 5 回、Go/`rapid` との比較、`client-go` のスケール実験 |
| [標準ライブラリ](docs/stdlib.md) | ソースの宣言一覧から生成(英語) |
| [エディタ設定](editors/README.md) | stdio LSP と VS Code クライアント |
| [設計履歴](DESIGN.md) | 実装スライスごとの契約。食い違う場合は SPEC が正 |

## ライセンス

Apache License 2.0。[LICENSE](LICENSE) を参照。
