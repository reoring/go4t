# Go 連携: 埋め込み、binding、プロジェクト

[English](go-interop.md)

## Go プログラムへの埋め込み

想定している使い方は次のとおりです。

1. `decide/decide.gt` に決定ロジックを書く:判断に必要な状態だけを写した `Observed` レコード、
   アクションの直和型、`func decide(o Observed) Action`。
2. 生成:`go4t build --emit-go decide --package decide decide/decide.gt`
   (`//go:generate` に書く)。パッケージは `Decide`、全レコード・直和型の Go ミラー
   (`ObservedGo`、`ActionRollout{}`、`TtlTtlNanos{F0: …}`)、`Main() rt.IO` を export します。
3. Go 側:外部の状態を読み、`Observed` を組み立て、`Decide` を呼び、結果で switch し、作用を実行する。
   `decide` の中では I/O が起きないことをコンパイラが保証します。

境界をきれいに保つ規則(SPEC §8.7、tutorial §12):

- 判断に必要なフィールドだけを `Observed` に写す。呼び出しごとにレコードがコピーされるため。
- sentinel 値を境界に渡さない。`Option[Int]` はポインタなしのミラーで渡し、
  `-1` に変換せず `Some(x)` と比較する。
- 期間とタイムスタンプは `Int` ナノ秒で渡す(`time.Duration` と `UnixNano` の内部表現と同じ)。
  境界で丸めが起きない。
- リスト全体にかかる妥当性フラグは、要素ごとの直和型にする。「最初に失敗する要素」が
  元の Go のループと厳密に一致する。

雛形:[examples/go-shell](../examples/go-shell)。実際の移植例:
[examples/approval-controller](../examples/approval-controller)、
[examples/attachment-repair-controller](../examples/attachment-repair-controller)。

## go4t から Go を呼ぶ

```sh
go4t bind strings --prefix str          # extern 宣言を出力
go4t bind modernc.org/sqlite -o bindings.gt
```

生成される extern は関数、opaque handle 上のメソッド、スライス、Option、エラーを扱えます。
ミラーできないシグネチャは、誤った binding を出す代わりにスキップコメントになります。
[examples/sql-service](../examples/sql-service) はこの方法で組んだ SQLite + Prometheus metrics の HTTP サービス、
[examples/typed-extern](../examples/typed-extern) は ABI アダプタの例です。
`std/http` に binding は不要です。

## プロジェクトと依存

go4t ライブラリは `go.mod`、`go4t.mod`、`.gt` ファイルを持つ Go module です。

```sh
go4t init example.com/myapp            # go.mod + go4t.mod を作成
go4t get example.com/lib@v1.0.0        # go get して go4t.mod に記録
```

バージョン選択とチェックサムは Go が担当し、`go4t.mod` は言語 import を列挙します。
`go4t get` は依存の更新だけを行います。Go パッケージは `go:` import で必要時に
binding を生成するので、生成物をコミットする必要はありません。
import はファイル単位で修飾付き(`import "std/io"` → `io.readFile`)、暗黙に入るのは小さな
core prelude だけです。解決順:import 元 module のソースルート、`go4t.mod` の require、
`GO4T_PATH`、埋め込み std。オフラインでローカル replace を使う構成は [examples/deps](../examples/deps) を参照。
