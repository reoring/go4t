# Go 連携: 埋め込み、binding、プロジェクト

[English](go-interop.md)

## Go プログラムに go4t を取り入れる 3 つの構成

移す範囲を選べます。同じ純粋な決定ロジックを、どの構成でも使えます。

| 構成 | client と loop の担当 | go4t で書く部分 | 法則の対象 | 例 |
| --- | --- | --- | --- | --- |
| Go 側で I/O、go4t で決定 | Go | `decide` だけ | 純粋な決定。Go 側の観測や作用は対象外 | [go-shell](../examples/go-shell)、[node-drain-controller](../examples/node-drain-controller)、[attachment-repair-controller](../examples/attachment-repair-controller) |
| Go 側で client と loop を保持 | Go。export した IO 関数に native の `*kubernetes.Clientset` や controller-runtime の `client.Client` handle を渡す | 観測、決定用データへの変換、決定、操作の適用 | 純粋な変換と決定。API の可用性や操作の成功は対象外 | [node-drain-shell](../examples/node-drain-shell) (kind で検証)、[approval-controller](../examples/approval-controller) (AppThrust の controller。Go reconciler は 271 → 88 行、元の 6 テストは無変更) |
| controller 全体を go4t で記述 | `import "go:k8s.io/client-go/kubernetes"` を使う go4t | client の準備、観測、決定、操作の適用、entry | 純粋な変換と決定。API の可用性や操作の成功は対象外 | [node-drain-typed](../examples/node-drain-typed) ([3 時間の連続稼働](../examples/node-drain-typed/soak): 839/839 cycle) |

## 境界の規則

**境界は情報を失う変換をしません。** 全 field が export され、ミラー可能な struct は
record としてコピーし、非公開の状態などミラーできないものは handle として保持します。
変換用のビューを追加しても、この表現は変わりません。
情報が失われる操作は `…ToJson`・`…FromJson`・`…Parse` のような名前付き関数か、
`time.Time` → UnixNano `Int` のように SPEC §8 の表で lossy と明記されたものだけです。
根拠は [ADR 0011](adr/0011-lossless-boundary.md) です。

| 帰結 | 契約 | 参照 |
| --- | --- | --- |
| handle の同一性 | `go:` handle は `--emit-go` を通っても native の `*pkg.T` (struct handle) または `pkg.T` (interface を含む)。record や container の内部でも同じ | [SPEC §8.7](../SPEC.md#87-exported-go-api)、[ADR 0011](adr/0011-lossless-boundary.md) |
| package 間の upcast | `client.unstructuredAsObject` などの生成変換は handle の同一性を保持。暗黙の部分型関係はない | [SPEC §8.3](../SPEC.md#83-externs-and-bind)、[ADR 0009](adr/0009-cross-package-upcasts-and-lock-values.md) |
| scalar implementer の upcast | `client.inNamespaceAsListOption(ns)` は `String` を受け取る。named scalar、bytes、slice、map の implementer は underlying value を使う | [SPEC §8.3](../SPEC.md#83-externs-and-bind)、[ADR 0010](adr/0010-scalar-implementer-upcasts.md) |
| 入出力 pointer | 値ミラーを持つ直接の `*T` 引数は非 null の `Ref[T]` と IO。error 時も書き戻す。戻り値と field は `Option[T]`。handle への pointer の同一性は変わらない | [SPEC §8.3](../SPEC.md#83-externs-and-bind)、[ADR 0011](adr/0011-lossless-boundary.md) |
| Go error | 引数は `GoError`、null 許容位置は `Option[GoError]`。末尾 error は IO の失敗となり、`io.tryIOCause` / `io.cause` が原因を保持。`apierrors.isNotFound(cause)` のような純粋な述語に渡せる | [SPEC §8.3](../SPEC.md#83-externs-and-bind)、[ADR 0011](adr/0011-lossless-boundary.md) |
| 明示的な JSON ビュー | JSON 対応型に `<t>ToJson` / `<t>FromJson` を追加しても通常のミラーは変わらない。`metav1.Time` は `dyn.Value` ではなく handle | [SPEC §8.3](../SPEC.md#83-externs-and-bind)、[ADR 0011](adr/0011-lossless-boundary.md) |
| handle を含む record | `Eq` と `Debug` は導出しない。record、container、`Option` を通じてネストした handle や `GoError` も対象 | [SPEC §8.3](../SPEC.md#83-externs-and-bind)、[ADR 0011](adr/0011-lossless-boundary.md) |
| export 名の衝突 | 関数が型やコンストラクタのミラーと衝突すると `Fn` を付ける (`nodeState` → `NodeStateFn`)。型の名前は保つ | [SPEC §8.7](../SPEC.md#87-exported-go-api) |

## Go プログラムへの埋め込み

1. `decide/decide.gt` に entry module を書きます。純粋な決定ロジックなら、必要な状態を写す
   `Observed` record、action の直和型、`func decide(o Observed) Action` を定義します。
2. `go4t build --emit-go decide_gen --package decide decide/decide.gt` で生成します
   (`//go:generate` に記載)。生成 package は `Decide`、到達可能な Go ミラー型
   (`Observed`、action のコンストラクタ)、`Main() rt.IO` を export します。
3. Go から生成 package を import し、`Observed` を埋めて `Decide` を呼び、選んだ作用を実行します。
   2 つ目の構成では、Go が持つ handle を export された observe/apply 関数に渡します。
   `IO[T]` の戻り値は `func(context.Context) (T, error)` として export されます。
   コンパイラが `decide` の純粋性を保証します。

境界を明確に保つための規則 (SPEC §8.7、tutorial §12):

- 決定に必要な状態だけを写します。
- sentinel 値を渡しません。`Option[Int]` は pointer を使わないミラーで渡し、
  `-1` で取り出すのではなく `Some(x)` と比較します。
- 決定に必要なら、期間と timestamp を明示的に `Int` ナノ秒へ変換します。
  timestamp の変換では location と monotonic metadata が失われます。
  native handle をそのまま渡す操作とは異なります。
- リストの横に置いた妥当性 flag は、要素ごとの直和型にします。
  これにより「順序どおりに最初の失敗を返す」動作が元の Go loop と一致します。

## go4t から Go を呼ぶ

**Go package を直接 import します。** binding は build 時に生成され、コミットしません。

```go4t
package main

import "std/io"
import "std/option"
import "go:net/url"

func main() IO[Unit] {
  do {
    parsed <- url.parse("https://go4t.example/guide")
    match parsed {
      None => io.println("no URL")
      Some(value) => io.println(url.urlHostname(value))
    }
  }
}
```

その他の binding 規則は [ADR 0003](adr/0003-import-time-go-bindings.md) と
[SPEC §8.3](../SPEC.md#83-externs-and-bind) にあります。

- 関数、method、package 変数、基本定数は extern になります。
  opaque struct の export field には `<type><Field>` getter と `<type>Set<Field>` setter、
  opaque struct には `new<Type>()` を生成します。
- 可変 handle を受け取る呼び出し (method receiver、引数、field getter) は `IO` です。
  handle は共有 Go 状態だからです。`GoError` は例外です。
  record、map、slice の値渡しは、共有状態を含まなければ純粋です。
- 入出力 cell は `std/ref` を import し、`ref.new(value)` で確保して Go に渡し、
  `ref.get(cell)` で読み戻します。`...*T` は `List[Ref[T]]` です。
  読み書きは個別に lock しますが、外部関数の呼び出し全体は atomic ではありません。
  native callback は入出力 pointer 引数に nil を渡せません。
- error message を明示的に取り出すには `io.goErrorMessage` を使います。
  直接の error 引数、callback の error 引数、`...error` の要素は nil を表せません。
  native callback から nil を渡すと `go4t error: nil error parameter` で panic します。
- `(A, B)` の戻り値は tuple、`(A, B, error)` は `IO[(A, B)]`、`(A, bool)` は
  `Option[A]` です。`iter.Seq[T]` の戻り値は `List[T]`、返された関数は go4t の関数になります。
  iterator 引数、generic receiver method、generic named struct/interface、未対応の channel 要素、
  error 以外が 3 個を超える戻り値は未対応です。
- Go の型は、その型を定義した package の module で一度だけ宣言します。
  `net/http` binding は `io.Reader` を `go_io.Reader` として参照し、loader が `go:io` を推移的に bind します。
  upcast には `strings.readerAsReader` や `goio.readCloserAsReader` があります。
  binding module P では、元の型と interface は P または direct import に属し、少なくとも一方は P の宣言である必要があります。
  外部の同名型には owner の修飾を付け (`qImplAsI`)、P 自身の型は修飾なし (`implAsI`) です。
- 標準ライブラリは `go.mod` なしで bind できます。第三者 package は build module の require が必要です。
  shim は ignored の build 出力 (`*_gen/`) 配下に書き、`go4t check` は何も書きません。
- struct handle の package 変数、field、method receiver は、lock の有無にかかわらず address を使います。
  lock を含む実際の値渡し引数、戻り値、container 要素、field setter は `copies lock value T` で skip し、
  `*T` は handle のままです。検出は Go vet と同じ pointer-only `sync.Locker` 実装の規則で、
  field、array、embedding を再帰的に調べます。`sync.Locker` interface の embedding だけでは lock としません。
- `<t>ToJson(recv T) dyn.Value` は純粋です。Marshal 失敗時は
  `go4t json view: <type>: <err>` で panic します。
  `<t>FromJson(v dyn.Value) IO[T]` は cancellation を検査し、変換 error を Go cause として保持します。
  名前の衝突時は既存の関数・method、accessor、constructor/value getter、upcast を優先し、
  skip した view には通常の衝突 comment を付けます。
  JSON の往復では location の正規化などで情報が失われることがあります。
  通常の record の受け渡しではこの経路を使いません。

実行できる例は [examples/go-import](../examples/go-import) です。

Go の channel は `Chan[T]`、`RecvChan[T]`、`SendChan[T]` に対応します。
`std/chan` を import すると、cancellation 対応の送受信と、IO として遅延実行する close を使えます。
閉じた channel からの受信は `None` を返します。`asRecv` と `asSend` で双方向 handle の権限を制限します。
`make` は go4t fiber 用の channel を作ります。Go API に渡すなら、
`signal.newSignalChan` などの生成 constructor を使います。
constructor は IO 内で確保し、要素には通常の引数・戻り値と同じ変換を適用します。
[examples/go-chan](../examples/go-chan) に `time.after` と producer/consumer fiber の例があります。
変換できない要素型の channel は引き続き skip します。

Go の generic 関数は template として bind し、型検査後に呼び出し箇所の具体型から実体化します。
Go の型引数を明示する必要はありません。Int、Float、Bool、String は
`int64`、`float64`、`bool`、`string` に対応し、List と Map はその対応を組み合わせます。
既知の外部 record/handle は元の Go の型を保持します。
Go の型制約を検査し、go4t の Eq/Ord で代用しません。
すべての generic 呼び出しと callback は IO で、`sync.onceValue` が返す closure も含みます。
未解決の型引数、未対応のミラー、制約違反は使用箇所でエラーにします。
トップレベルの多相 helper は必要な具体型ごとに生成します。
局所的な多相 lambda で外部引数の型が確定しなければ、具体型の注釈が必要です。
iterator 戻り値は List (`maps.keys`) ですが、iterator 引数 (`slices.sorted`) は未対応です。
`slices.sort` は境界で作ったコピーを変更して `IO[Unit]` を返し、呼び出し元の List は変更しません。
package template と型の組み合わせごとの shim は別々にキャッシュするため、
呼び出し側の変更で package cache は無効になりません。
[examples/go-generics](../examples/go-generics) と [ADR 0006](adr/0006-generic-go-functions.md) を参照してください。
context、error、optional-result の規則は、型引数の置換後ではなく元の Go 宣言で決まります。

`go4t bind` は宣言を確認するためのコマンドとして残っています。

```sh
go4t bind strings --prefix str          # prints extern declarations
go4t bind modernc.org/sqlite -o bindings.gt --shim shimdir
```

知らない生成名を使う前に、たとえば `go4t bind path/filepath` で宣言を確認できます。
`--prefix` を指定しないと、関数名は Go の先頭文字を小文字にします (`Base` → `base`)。
そのため `import "go:path/filepath"` では `filepath.base(path)` を使います。
receiver method を含む命名規則は [SPEC §8.3](../SPEC.md#83-externs-and-bind)、
確認用の flag は [CLI reference](cli.md) にあります。

ミラーできない signature は、誤った binding の代わりに skip comment を生成します。
[examples/sql-service](../examples/sql-service) は SQLite と Prometheus に `go:` import を使います。
[examples/typed-extern](../examples/typed-extern) は `go:net/url` 経由で実行し、
再生成した binding は確認用の出力として残します。`std/http` に binding は不要です。

## プロジェクトと依存

go4t ライブラリは `go.mod`、`go4t.mod`、`.gt` ファイルを持つ Go module です。

```sh
go4t init example.com/myapp            # creates go.mod + go4t.mod
go4t get example.com/lib@v1.0.0        # go get, then record in go4t.mod
```

Go がバージョン選択と checksum を担当し、`go4t.mod` は言語 import を列挙します。
`go4t get` は依存の更新だけを行います。`go:` import は必要時に binding を生成するため、
生成物をコミットする必要はありません。
import はファイル単位で修飾付き (`import "std/io"` → `io.readFile`) です。
暗黙に入るのは小さな core prelude だけです。
解決順は import 元 module の source root、`go4t.mod` の require、`GO4T_PATH`、埋め込み std です。
offline の local replace 構成は [examples/deps](../examples/deps) を参照してください。
