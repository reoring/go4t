# CLI リファレンス

[English](cli.md)

| コマンド | 目的 |
| --- | --- |
| `go4t check [--full-types] FILE.gt\|DIR...` | 読み込み・型検査・法則と証明の検証。warning では失敗しない。foreign ABI はここでは検査しない。 |
| `go4t build [--full-types] [-o FILE] FILE.gt\|DIR...` | 実行ファイルをビルドし、生成した Go ソースを入力の隣に残す。 |
| `go4t build [--full-types] --emit-go DIR [--package NAME] FILE.gt\|DIR...` | Go ソースのみ出力。パッケージ名の既定値は `main`。別名を指定すると型付き export を生成(§8.7)。 |
| `go4t run [--full-types] FILE.gt\|DIR... [-- args...]` | ビルドして実行。stdin を継承、終了コードを保持。 |
| `go4t test FILE.gt\|DIR...` | `Bool` または `IO[Bool]` を返す引数なしの `testName` 関数を実行。 |
| `go4t eval 'EXPR' [FILE.gt\|DIR...]` | 純粋値または IO 結果を評価。import は最初のファイルから。 |
| `go4t bind IMPORT [FILE.gt\|DIR...] [-o FILE.gt\|DIR] [--prefix STR] [--shim DIR]` | 確認用または従来の連携用に宣言と任意の Go shim を生成。 |
| `go4t fmt [-w\|-d\|-l] FILE...\|DIR` | 正規整形。`-l` は差分があれば 1 で終了。 |
| `go4t init <module-path>` / `go4t get <path>@<version>` | プロジェクトと依存の管理。 |
| `go4t lsp` | stdio 言語サーバ(診断、hover、定義、シンボル)。 |

証明を検証するのは `check` だけです。`build`、`run`、`test`、`eval` は法則と証明を
生成コードに含めませんが、構文・型検査は行います。

`go4t build` はカレントディレクトリに実行ファイルを作ります。
名前は最初の入力ディレクトリ名、または最初の入力ファイル名から `.gt` を除いたものです。
`-o FILE` で出力先を指定できます。既存のディレクトリは指定できません。
生成した Go ソースは、ディレクトリ入力なら `DIR/_gen`、
ファイル入力なら `FILE_gen` に残ります。
`--emit-go DIR` はソースのみを出力し、`-o` とは併用できません。
`--package NAME` は `--emit-go` を指定したときだけ使えます。

ビルドには PATH 上の Go が必要です。
`go4t` を提供する Go module があれば、それを使います。
なければ、一時 module にコンパイラ内蔵のランタイムを配置します。
ユーザーの `go.mod` は変更しません。
Go のビルド診断は stderr に出力し、失敗時は非ゼロで終了します。

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
