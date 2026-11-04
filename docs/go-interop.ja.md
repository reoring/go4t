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

Go のチャネルは `Chan[T]`、`RecvChan[T]`、`SendChan[T]` に対応します。
`std/chan` を import すると、キャンセルに対応した送受信と、IO として遅延実行する
close を使えます。閉じたチャネルからの受信は `None` を返します。
双方向ハンドルの操作権限は `asRecv` と `asSend` で明示的に制限します。
`make` は go4t の fiber 用チャネルを作ります。Go API に渡す場合は、
`signal.newSignalChan` などの生成されたコンストラクタを使ってください。
コンストラクタは IO の実行時にチャネルを確保し、要素には通常の引数・戻り値と同じ
変換を適用します。[examples/go-chan](../examples/go-chan) に `time.after` と
producer/consumer fiber の例があります。要素型を変換できないチャネルは
引き続きスキップされます。

Go のジェネリック関数は import 時にテンプレートとして登録され、
呼び出し箇所の具体型から Go の型引数を決めます。
Int、Float、Bool、String はそれぞれ `int64`、`float64`、`bool`、`string` に対応し、
List と Map は要素型にも同じ変換を適用します。
外部の record と handle は元の Go の型を保持します。
Go の型制約を検証し、go4t の Eq/Ord で代用しません。
型引数が決まらない場合、変換できない型、制約違反は使用箇所でエラーにします。
Go の型引数を明示的に書く必要はありません。
context、error、Option への変換は、型引数の置換後ではなく元の Go 宣言で決まります。
ジェネリック関数とそのコールバックはすべて IO です。
`sync.onceValue` が返す関数も IO を返します。
トップレベルの多相関数は必要な具体型ごとに生成します。
ローカルな多相ラムダの型引数が未確定のまま残る場合は、具体型の注釈が必要です。
`maps.keys` の iterator 戻り値は List になりますが、
`slices.sorted` のような iterator 引数と、ジェネリックな構造体・interface・receiver は未対応です。
`slices.sort` はコピーを変更して `IO[Unit]` を返し、呼び出し元の List は変更しません。
パッケージのテンプレートと具体型ごとの shim は別々にキャッシュします。
呼び出し側の変更でパッケージのキャッシュは無効になりません。
[examples/go-generics](../examples/go-generics) と
[ADR 0006](adr/0006-generic-go-functions.md) を参照してください。


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
