# go4t — Go for types

[English](README.md)

go4t は、通常の Go にコンパイルされる小さなプログラミング言語です。
reconciler の判断、認可のチェック、料金ルールなど、
*すべてのケースを正しく扱うこと*が重要な部分を書きます。
go4t には、Go にはない次の 3 つがあります。

1. **状態をデータで表す。** 値は名前の付いたケースのいずれかです
   (`NoTtl | InvalidTtl | TtlNanos(Int)`)。
   `match` にケースが欠けていると、足りないパターンを示すコンパイルエラーになります。
2. **純粋なコードと作用を分ける。** I/O を行う関数は `IO[A]` 型の値を返します。
   純粋関数からは呼び出せません。コンパイラがこの制約を検査します。
3. **法則を書く。** 純粋なコードについて等式を書きます。たとえば
   「TTL が経過したときに限り stale」は
   `isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now - at >= t)` です。
   `go4t check` は、法則が明記した範囲の**すべての**入力について証明します。
   テストに列挙した入力だけの検査ではありません。

それ以外は、意図的に普段の Go と同じにしています。
出力は読みやすい Go で、`go build` でビルドでき、依存は `go.mod` で管理します。
ビルド時に生成するバインディングを通じて、任意の Go パッケージを呼べます
(`import "go:net/http"`)。
プログラム全体を go4t で書いても、Go プログラムを残して関数を 1 つだけ移してもかまいません。

## 2 分で試す

PATH に **Go 1.22 以上**が必要です。

```sh
go install github.com/reoring/go4t/cmd/go4t@latest
go4t eval '1 + 2'        # 3
```

`hello/main.gt` を作ります。

```go4t
package main
import "std/string"
func main() IO[Unit] {
  let names = [" Ada ", " Bob "] |> map(string.trim)
  println("Hello, " + string.join(" and ", names) + "!")
}
```

```sh
go4t check hello/main.gt      # type-check (and prove laws, if any)
go4t run hello/main.gt        # Hello, Ada and Bob!
go4t build hello/main.gt      # builds ./main; keeps Go sources in hello/main_gen/
```

Go を知っている方には、[チュートリアル](docs/tutorial.md)があります。
Go での書き方を出発点に、何が変わるかを 18 の短い章で説明します。

## 小さな決定を端から端まで

次は go4t が対象とするコードの例です。
controller が、観測した情報をまだ有効とみなせるか判断します。
TTL とタイムスタンプは、それぞれ未指定や不正な値の可能性があります。

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

注目する点は 3 つです。

- 入力は `*time.Time` と `bool` の組み合わせではなく、**直和型**です。
  関数が扱うべき組み合わせがすべてシグネチャに表れています。
- `(TtlNanos(_), InvalidTimestamp)` の分岐を削除すると、`go4t check` は
  `missing: (TtlNanos(_), InvalidTimestamp)` で失敗します。
- **法則**を追加すると、`Stale` のガードを別のチェックの後ろへ動かしたときや
  `>=` を `>` に変えたとき、すべての `t`、`at`、`now` を対象とする検証が失敗します。
  証明は通常、単語 1 つ (`refl`) かケース分けで書けます。

  ```text
  func plausible(t Int, at Int, now Int) Bool {
    0 <= t && t <= 1000000000000 && 0 <= at && at <= now && now <= 4000000000000000000
  }
  law staleOnceElapsed(t Int, at Int, now Int) requires plausible(t, at, now) {
    isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now - at >= t)
  }
  proof staleOnceElapsed(t, at, now) { if now - at >= t { refl } else { refl } }
  ```

  範囲は通常の `Bool` 関数として 1 度だけ書き、必要な法則すべてで共有します。
  `requires` 句は飾りではありません。`Int` は桁あふれで折り返すため、
  「経過した」が文字どおりの意味を持つのは `at + t` があふれない範囲だけです。
  この句を消すと、`go4t check` は法則を退け、反例を表示します:
  `counterexample: t = 9223372036854775807, at = 1, now = 0`。

Go から使うには、`go4t build --emit-go decide --package decide decide/decide.gt`
を実行します。通常は `//go:generate` 行から実行します。
生成されたパッケージは各関数と、各レコード・直和型に対応する通常の Go の型を公開します。
Go 側は結果に応じて `switch` します。
(`freshness` は型 `Freshness` を返すため、公開される関数名は `FreshnessFn` です。
関数名が `decide` なら `decide.Decide` になります。)

```go
switch step := decide.FreshnessFn(ttl, observedAt, now.UnixNano()).(type) {
case decide.FreshnessStale:
    return requeue(ctx, obj)
case decide.FreshnessTtlInvalid:
    return reject(ctx, obj, "evidence TTL invalid")
// …
}
```

[examples/go-decision-template](examples/go-decision-template) は、この決定をそのまま
コピーして使える Go モジュールにしたものです。`.gt` ファイル 1 つ、`//go:generate` 1 行、
その前に置く Go の API、Go のテストで構成されています。

### 同じ決定を、I/O も go4t で

決定の外側(シェル)を Go で書く必要はありません。作用は `IO[A]` 型の値で、
`do` ブロックがそれを順に実行します。`x <- action` は作用を実行して結果を束縛し、
式をそのまま書けばそれは純粋な値です。「外の世界を読む」ことと
「読んだものを `Timestamp` に解釈する」ことは別の仕事なので、別の関数にします。

```go4t
package main
import "std/io"
import "std/string"

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

// Reading the file is IO; deciding what the text means is pure.
func observedAt(path String) IO[Timestamp] {
  do {
    read <- io.tryIO(io.readFile(path))
    match read {
      Err(io.NotFound(_)) => NoTimestamp
      | Err(e)            => io.ioRaise(e)
      | Ok(text)          => match string.parseInt(string.trim(text)) {
        Some(nanos) => AtNanos(nanos)
        None        => InvalidTimestamp
      }
    }
  }
}

func main() IO[Unit] {
  do {
    at     <- observedAt("/var/run/last-seen")
    millis <- io.nowMillis()
    match freshness(TtlNanos(30 * 1000000000), at, millis * 1000000) {
      Fresh(left)      => println("fresh for ${left / 1000000} ms more")
      Stale            => println("stale: re-observe")
      TimestampMissing => println("never observed")
      | other          => io.ioFail("invalid input: " + debug(other))
    }
  }
}
```

`io.tryIO` は失敗しうる作用を `Result` に変えるので、ファイルが無いことを
例外ではなく `NoTimestamp` というケースとして扱えます。それ以外の失敗は
`io.ioRaise` でそのまま投げ直します。`match` の腕は、純粋な値(`NoTimestamp`)でも
作用(`io.ioFail(...)`)でもかまいません。`IO` が期待される位置ではどちらも書けます。
ファイルが無ければ `never observed`、最近の UnixNano が入っていれば
`fresh for 29170 ms more` と表示し、不正な内容なら `invalid input: TimestampInvalid`
で終了コード 1 になります。`freshness` は 1 行も変わっておらず、法則もそのまま成り立ちます。

## Go プログラムへの組み込み方

どこまで移すかは選べます。次の 3 つの構成には、いずれも実行できる例があり、
同じ決定ロジックを共有しています。

| 構成 | Go に残す部分 | go4t が担当する部分 | 例 |
| --- | --- | --- | --- |
| 外側を Go、決定ロジックを go4t にする | クライアント、reconcile ループ、観測と操作の適用 | `decide` | [go-shell](examples/go-shell)、[node-drain-controller](examples/node-drain-controller) |
| 外側を Go、controller を go4t にする | クライアントとループ | Go のクライアントをネイティブハンドルとして受け取る、公開 IO 関数の `observe`、`decide`、`apply` | [node-drain-shell](examples/node-drain-shell)、[approval-controller](examples/approval-controller) (実際の controller。Go は 271 行から 88 行になり、6 つのテストは無変更) |
| プログラム全体を go4t にする | なし。`main` も go4t | すべて。Go パッケージは `import "go:…"` で呼ぶ | [node-drain-typed](examples/node-drain-typed) (kind クラスタで 3 時間の連続稼働試験を実施) |

境界の規則は単純です。**情報が失われる変換はしません。**
すべてのフィールドが公開されている Go の構造体は、レコードとしてコピーします。
それ以外 (`*kubernetes.Clientset`、`error`、構造体内の `time.Time` など) は、
同一性を保った不透明なハンドルとして渡します。
情報を失う可能性がある操作には名前の付いた関数があり、意図して呼び出します。
詳細、使い方、生成される名前は [docs/go-interop.md](docs/go-interop.md) にあります。

## なぜ Go だけでは足りないか

reconciler、ワークフローエンジン、ポリシー評価、料金ルールでは、
部分的にしか分からない多くの事実をもとにした判断が難しい部分です。
Go はその事実を `(value, ok)` の組、sentinel 値、nil になりうるポインタ、
bool フラグで表します。判断は早期 return の連鎖になり、
すべてのケースを扱えているかはテストでしか示せません。

| 決定ロジックで必要なこと | go4t | Go |
| --- | --- | --- |
| N 個の名前付きケースのいずれかである状態 | 直和型 (`A \| B(Int) \| C`) | const + コメント、または `*T` + `bool` |
| 「すべてのケースを扱ったか?」 | 網羅性検査付きの `match`。欠けたパターンを列挙するコンパイルエラー | `default:` を書いて済ませる |
| エラーをデータとして扱う | `Result[E, A]`、`Option[A]`、`do` による逐次処理 | `(T, error)` と `if err != nil` |
| 決定ロジックが I/O をしない保証 | 作用は `IO[A]` 型の値。純粋関数からは実行できない | 規律 |
| ネストしたデータの等価・順序・デバッグ表示 | `deriving Eq, Ord, Debug, Show` | 手書きか `reflect.DeepEqual` |
| コンテナに対する汎用的な操作 | 型クラス、高カインド (`Functor`、`Monad`、`Monoid`) | 型ごとにコピー |
| 「この不変条件はすべての入力で成り立つ」 | `law` + `proof`。コンパイル時にすべての値について検査 | 選んだ入力のテーブルテスト、またはランダム入力の property-based testing (`rapid`) |

最後の行には注釈が要ります。同じ決定ロジックに対して、
`rapid` による property-based testing は、試したバグ注入をすべて検出しました。
法則が加えるのは、確率ではなく証明です。
`if` が 2 つで済む `decide` なら、Go のままで十分です。

証明が届いて生成器が届かないのは、入力の形を人が決めなければならない場面です。
node-drain controller は DaemonSet の Pod に手を出してはならず、
Pod は owner reference の*どれか 1 つ*が DaemonSet なら DaemonSet の Pod です。

```text
law daemonSetOwnerAnywhere(prefix List[meta.OwnerReference], owner meta.OwnerReference, rest List[meta.OwnerReference]) {
  ownedByDaemonSet(append(prefix, Cons(owner{kind: "DaemonSet"}, rest))) == true
}
```

`meta.OwnerReference` は `import "go:…"` で束縛した `k8s.io/apimachinery` の実際の型です。
この法則は、すべての prefix、すべての suffix、owner の他のすべてのフィールドについて成り立ちます。
証明は `prefix` に対する帰納法です
([examples/node-drain-typed](examples/node-drain-typed/decide.gt))。
同じ性質を `rapid` で試すには、まず owner reference のリストの生成器を書く必要があり、
検査できるのはその生成器が作るリストについてだけです。

**速度。** 実行コストは推測ではなく計測しています ([bench/PERF.md](bench/PERF.md))。
値をボックス化せずに済むコードの実行時間は、同等の Go の 1.6〜4 倍です。
ホットループ内で永続 `Map` を更新すると 19〜24 倍になります。
これは他の永続コレクションと同じく、永続性のためのコストです。
IO 専用の代替である `std/mutmap` は 5 倍です。
決定ロジックや controller はマップを*読む*ため、この更新コストはかかりません。

## 法則の概要

法則は純粋なコードについての等式で、証明にはその検査方法を書きます。
たいていの証明は短く書けます。

```go4t
package main
law distribute(a Int, b Int, c Int) { (a + b) * c == a * c + b * c }
proof distribute(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

証明は `refl`、コンストラクタの `match`、ガードの `if`、`rewrite`、
`induction` を組み合わせて書きます。
証明済みの法則から実行時コードは生成されません。
法則は一階のデータ (直和型、レコード、リスト、マップ、`Int`、`String` など) と、
それらを扱う純粋関数を対象とします。I/O は対象外です。
また、不透明な Go 関数に依存する等式は、サンプルによる検査 (`sample(N)`) だけが可能です。
`go4t test` で実行しますが、証明済みとはみなしません。

法則には前提条件 (`requires 0 <= a && a < limit`) を書けます。
`Int` は 64 ビットで折り返すため、比較を判定するのは、仮定から桁あふれが起きないと分かる場合だけです。
`a + 1 > a` は、範囲の指定がなければ退けられ、指定があれば証明できます。
証明に失敗すると、`go4t check` は両辺が食い違う具体的な引数を探して表示します。
法則が誤っている場合と、証明が足りない場合を見分けられます。

法則で表せることと表せないことは [docs/laws.md](docs/laws.md)、
「証明済み」が何に依拠し、それを実際のランタイムとどう突き合わせているかは
[docs/laws-trust.md](docs/laws-trust.md)、
具体例は [チュートリアル §14](docs/tutorial.md#14-laws) にあります。

## 使われてきた場所

- **Kubernetes controller。** AppThrust プラットフォームで本番利用されている
  controller を 2 つ移植しました。元の Go のテストを正しさの基準として残しています
  ([approval-controller](examples/approval-controller)、
  [attachment-repair-controller](examples/attachment-repair-controller))。
  node-drain controller には前述の 3 つの構成があり、全体を go4t で書いたものは、
  3 時間の連続稼働試験で 839/839 回の drain/recover サイクルを実行しました。
- **通常のプログラム。** CLI ツール、HTTP サービス、そしてコンパイラを見たことのない
  コーディングエージェント 4 本がドキュメントだけを頼りに書いたプログラムがあります
  ([docs/evaluation.md](docs/evaluation.md) に、つまずいた点と変更内容を記録しています)。
  上記はすべてプロジェクト自身の手によるもので、外部の利用者はまだいません。
- **go4t 自身。** `selfhost/` は go4t で書かれた 5.4k 行の go4t→Go コンパイラです。
  自分自身の出力を再現でき (`bash selfhost/bootstrap.sh`)、CI で検査しています。

法則では、これまで次の性質を固定してきました。

| 対象 | 固定した性質 |
| --- | --- |
| 操作の順序 | 「cordon してから evict する」 |
| 繰り返し実行 | 冪等性 |
| 期限 | `>=` / `>` の正確な境界 |
| ゾーンの選択 | 「満杯のゾーンは選ばない」 |
| pod の変換 | 「DaemonSet に所有される pod は `DaemonSet` に写す」 |
| リファクタリング | 変更前後で `decide(o) == decideV2(o)` |

## 言語の概要

- 代数的データ型、イミュータブルな更新ができるレコード、タプル、
  永続データ構造の `List`/`Map`/`Set`/`Array`。
- Hindley–Milner 型推論。スーパークラスと高カインドを扱える型クラス。
  `deriving Eq, Ord, Show, Debug, Functor, Foldable, Traversable`、JSON の `Encode`/`Decode`。
- 網羅性検査付きの `match`。ガード、タプル・リストのパターン、パターン束縛の `let`。
- 明示的な作用: `IO[A]`、`do` ブロック、`IoError` による型付きのエラー回復、
  キャンセル対応の fiber (`fork`/`await`/race/timeout)。IO 専用の `Ref`、`MutMap`、チャネル。
- `std`: 文字列、数値、コレクション、JSON、HTTP クライアント/サーバ、正規表現、
  遅延評価される値、`State`、ファイル・プロセス I/O。
- 双方向の Go 連携: `import "go:…"` が、任意の Go パッケージの関数、メソッド、
  フィールド、ジェネリクス、アップキャストをビルド時にバインドします。
  `--emit-go` は go4t の関数・レコード・直和型を通常の Go として公開します。
- Go 風の構文: `func`、`package`、`import`、波括弧、`|>` パイプライン。
- ツール: `check`、`build`、`run`、`test`、`eval`、`fmt`、LSP サーバ、VS Code 拡張。
  診断にはソースの該当範囲を表示します。

## 現状と go4t でないもの

- **1.0 ではありません。** 現在のリリースは
  [v0.1.0](https://github.com/reoring/go4t/releases/tag/v0.1.0) です。
  `@latest` は最新のタグを、`@v0.1.0` はそのバージョンを固定します。
  文法、標準ライブラリ、Go ABI は、上記の移植に使える程度に安定しており、
  [SPEC.md](SPEC.md) に記述しています。ただし、リリース間で変わる可能性があります。
  既知の制限は [SPEC §9](SPEC.md#9-known-limitations) にあります。
- **Go の代替ではありません。** Go プログラムは残し、利点のある部分を移してください。
  法則は I/O について何も保証しません。
- **証明支援系ではありません。** 法則は一階のデータについての等式検査器で、
  使える証明戦術は固定されています。
- **エフェクトシステムの研究言語ではありません。** 作用の型は `IO` の 1 つです。
- **どこでも高速なわけではありません。** ホットループに永続 `Map` を入れる前に、
  上の「速度」を参照してください。

## ドキュメント

| ドキュメント | 内容 |
| --- | --- |
| [Go 開発者向けチュートリアル](docs/tutorial.md) | 18 の短い章。すべての go4t スニペットを CI で型検査。 |
| [言語仕様](SPEC.md) | 文法、型と作用の規則、deriving、公開する Go API、境界表。 |
| [法則](docs/laws.md) | 法則で扱えること、扱えないこと、証明の書き方。 |
| [「証明済み」が依拠するもの](docs/laws-trust.md) | 法則検査器が信頼している部分と、それぞれの検査方法。 |
| [Go 連携](docs/go-interop.md) | 3 つの移行構成、境界の規則、`--emit-go`、`go:` import と依存。 |
| [CLI リファレンス](docs/cli.md) | 全コマンドと終了コード。 |
| [例](examples/README.md) | 実行できるすべての例とコマンド。 |
| [検証記録](docs/evaluation.md) | 本番 controller の移植、法則の検証、Go/`rapid` の比較、連続稼働試験、移行の記録。 |
| [アーキテクチャの決定](docs/adr/README.md) | 各規則を採用した理由。 |
| [標準ライブラリ](docs/stdlib.md) | ソースの宣言一覧から生成。 |
| [エディタ設定](editors/README.md) | stdio LSP と VS Code クライアント。 |
| [設計履歴](docs/history/DESIGN.md) | 実装スライスごとの契約。食い違うときは SPEC が優先。 |

## ライセンス

Apache License 2.0。[LICENSE](LICENSE) を参照。
