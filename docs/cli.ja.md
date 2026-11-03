# CLI リファレンス

[English](cli.md)

| コマンド | 目的 |
| --- | --- |
| `go4t check [--full-types] FILE.gt\|DIR...` | 読み込み・型検査・法則と証明の検証。warning では失敗しない。foreign ABI はここでは検査しない。 |
| `go4t build [--full-types] [-o DIR] FILE.gt\|DIR...` | go4t module ごとに 1 Go ファイルを 1 Go パッケージへ出力。 |
| `go4t build --emit-go DIR --package NAME FILE.gt\|DIR...` | 型付き export を持つ import 可能な Go パッケージを出力(§8.7)。 |
| `go4t run [--full-types] FILE.gt\|DIR... [-- args...]` | ビルドして実行。stdin を継承、終了コードを保持。 |
| `go4t test FILE.gt\|DIR...` | `Bool` または `IO[Bool]` を返す引数なしの `testName` 関数を実行。 |
| `go4t eval 'EXPR' [FILE.gt\|DIR...]` | 純粋値または IO 結果を評価。import は最初のファイルから。 |
| `go4t bind IMPORT [FILE.gt\|DIR...] [-o FILE.gt\|DIR] [--prefix STR] [--shim DIR]` | 確認用または従来の連携用に宣言と任意の Go shim を生成。 |
| `go4t fmt [-w\|-d\|-l] FILE...\|DIR` | 正規整形。`-l` は差分があれば 1 で終了。 |
| `go4t init <module-path>` / `go4t get <path>@<version>` | プロジェクトと依存の管理。 |
| `go4t lsp` | stdio 言語サーバ(診断、hover、定義、シンボル)。 |

証明を検証するのは `check` だけです。`build`、`run`、`test`、`eval` は法則と証明を
生成コードに含めませんが、構文・型検査は行います。

`go4t get <path>@<version>` は `go get` を実行し、選択されたバージョンを
`go4t.mod` に記録します。binding は生成しません。`import "go:<path>"` を使うと、
プログラムの読み込み時に必要な Go パッケージの binding が生成されます。

終了コード:通常の IO エラーは 1、捕捉されない action panic は 2。プログラム内での回復は
`io.tryIO(action) : IO[Result[io.IoError, A]]`。

## ドキュメントのテスト

`go test ./docs ./std` が SPEC、tutorial、stdlib、`docs/laws.md`、英語 README の
すべての `go4t` fence を取り出し、`go4t check` と同じローダと型検査器にかけます。
`go4t error` fence は拒否されなければなりません。

`selfhost/` は別管理の進行中セルフホストコンパイラで、リリース対象ではありません。
