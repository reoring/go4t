# Examples

[English](README.md)

コマンドは checkout ルートから実行します。ネットワークを使う例は明記しています。
主要な例のみ示します。全例は [英語版](README.md) の表を参照してください。

| 例 | 内容 | 実行 |
| --- | --- | --- |
| [go-shell](go-shell) | 最小の Go 外側 + go4t 決定ロジックの雛形 | `go4t run examples/go-shell/core.gt` |
| [approval-controller](approval-controller) | 実際の controller。元テスト 6 本無変更 | `cd examples/approval-controller && go generate ./... && go test ./...` |
| [attachment-repair-controller](attachment-repair-controller) | 複数ステップのワークフローの決定ロジックを go4t 化。元テスト 22 本無変更 | `cd examples/attachment-repair-controller && go generate ./... && go test ./...` |
| [node-drain-controller](node-drain-controller) | 一から書いた controller。検証済み法則 10 本(cordon 先行、DaemonSet 透過、正確な timeout) | `go4t check examples/node-drain-controller/decide/decide.gt && cd examples/node-drain-controller && go generate ./... && go test ./...` |
| [sql-service](sql-service) | `go:` import を使う SQLite + Prometheus HTTP サービス | README 参照 |
| [go-import](go-import) | `go:net/http` / `go:strings` をビルド時に結合。タプル、ハンドルのフィールド、upcast、パッケージ変数(loopback) | `go4t run examples/go-import` |
| [todo](todo) | CLI、JSON ファイル保存、`std/http` ルート、JSON コーデック導出、検証済み法則 3 本(サーバはネットワーク使用) | `go4t run examples/todo -- list` |
| [typed-extern](typed-extern) | 生成 binding、ABI アダプタ、opaque method handle | `go4t run examples/typed-extern` |
| [deps](deps) | ローカル replace を使うオフライン依存 | `go4t run examples/deps/app` |
| [calc](calc) | `Result` エラー付き式インタプリタ | `go4t run examples/calc/main.gt` |
| [cards](cards) | ポーカー役判定、derived 順序、seed 付き `State` | `go4t run examples/cards/main.gt` |
| [scheduler](scheduler) | カスタム順序、ガード、上限付きスケジューリング | `go4t run examples/scheduler/main.gt` |
| [sales-report](sales-report) | 並列取得、型付きの IO エラー回復、フォールバック | `go4t run examples/sales-report` |
| [http](http) | `std/http` サーバ/クライアント、JSON ルート、キャンセル(loopback) | `go4t run examples/http/main.gt -- 8080` |
| [concurrency](concurrency) | fork/await、順序付き並列、race、timeout | `go4t run examples/concurrency/main.gt` |
| [json-deriving](json-deriving) | JSON スキーマ導出とネストしたエラーパス | `go4t run examples/json-deriving/main.gt` |
| [diagnostics](diagnostics) | 網羅性・型シノニム不一致の診断(`bad_*.gt` は意図的に失敗) | `go4t run examples/diagnostics/main.gt` |
| [cli-tools](cli-tools) | 純粋/IO テストと eval。`go4t test examples/cli-tools/main.gt` も | `go4t run examples/cli-tools/main.gt` |
| [bank](bank)、[inventory](inventory)、[orders](orders) | HTTP 通知付きドメイン検証(**ネットワーク**) | `go4t run examples/<name>/main.gt` |

失敗を確認する例:`go4t test examples/cli-tools/failing.gt` は 1 で終了、
`go4t check examples/diagnostics/bad_hint.gt` は 1 で終了、
`go4t check examples/diagnostics/bad_unused.gt` は warning を出して 0 で終了します。
