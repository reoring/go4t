# ADR 0003: import 時 Go binding と写像の拡張

Date: 2026-11-02
Status: Accepted (PoC verified 2026-11-02)

## Context

README の設計規則「go4t は決定ロジックに限る。インフラは移植せず bind する」は、client-go 移植
(`.wt/client-go/ports/client-go/CODE_STATS.md`) で **binding 層が go4t 側 44,114 行 + Go 側 43,825 行の
生成物としてリポジトリに入った**ことから導かれた。本 ADR は、その壁が「Go の型を写せない」ではなく
「生成物の置き場と運用」にあることを計測で確認し、go4t を一般のサービス記述にも使える形へ広げる設計を記す。

### 確認済みの事実

- `go4t bind IMPORT --shim DIR` の網羅率(2026-11-02、Go 標準ライブラリ): net/http 145 externs / 8 skip、
  os 130 / 1、io 61 / 4、strings 72 / 7、encoding/json 36 / 0、context 8 / 8。
  shim なし(自然 ABI のみ)では net/http 2 / 43、io 0 / 15 で、README 執筆時の感触はこちらに引きずられていた。
- 残る skip の理由は 3 種類: 多値返却(`strings.Cut`, `io.Pipe`, `context.WithCancel`)、`iter.Seq[T]` 返却、
  `func` 型の返却(`context.AfterFunc`, functional option)。chan は `Context.Done` 等 3 件。
- 構造体で private 状態を持つ型(`*http.Request`, `*http.Response`, `*http.Server`, `*os.File`)は handle
  (不透明)になる(`extern/mirror.go:valueMirror` L85–91 の「promoted fields が 0 なら handle」規則と、
  L101 の非公開 embedded pointer 規則)。handle の公開フィールド(`Request.Header`, `Request.URL`)には
  メソッド経由でしか触れない。
- 既存の binding 配置: `go4t get` / `go4t bind --module` が **プロジェクト直下 `go4tbind/<import path>/`** に
  `bindings.gt` + `go4t_shim_<base>.go` を書き、`import "go4tbind/database/sql"` で読む
  (`cmd/go4t/bind_module.go:bindPackages`、`examples/sql-service/README.md`)。モジュール配下の全パッケージを生成する。
- build の流れ(`cmd/go4t/main.go:run` L99–171): parse → check → 出力ディレクトリ決定 → build module 決定
  (`runtime.go:prepareBuildModule`。go.mod が無ければ一時モジュール `go4trun` を作り、foreign extern があればエラー)
  → codegen → `go build`。extern の Go import path は `bindings.gt` の文字列がそのまま codegen に渡る。
- import 解決は `modules/load.go:loader.importModule`: プロジェクト相対 → go4t.mod の require → `GO4T_PATH` → 埋め込み std。
  `modules` は `extern` に依存していない。

## Options

### A. 生成物の置き場

- **A1. 現状**: プロジェクト直下 `go4tbind/` に全パッケージ分をコミット。client-go で 88k 行。却下。
- **A2. import 時生成(採用)**: `import "go:net/http"` のように `go:` スキームで Go パッケージを直接 import する。
  `bindings.gt` はローダが生成してメモリ上/キャッシュで読み、shim Go は build 出力ディレクトリ配下
  `<out>/go4tbind/<import path>/` に書く。cgo が生成物を build に閉じ込めるのと同じ形。リポジトリには何も残らない。
- A3. `go4t get` 時に生成しキャッシュのみ: 生成タイミングが import と分かれると「bind を打つ手間」が残る。A2 の補助として
  クロスビルドキャッシュだけ後で入れる。

### B. 写像の拡張

- **B1(採用)**: 多値返却 `(A, B)` → go4t タプル `(A, B)`、`(A, B, error)` → `IO[(A, B)]`、`(A, bool)` は現状通り `Option[A]`。
  `iter.Seq[T]` → `List[T]`(呼び出し時に具体化)、`iter.Seq2[K, V]` → `List[(K, V)]`。`func` 型の返却 → 引数位置と
  同じ callback 写像。chan、generics 関数、`unsafe` は引き続き skip。
- B2. 全部 `dyn.Value` に落とす: 型が消える。却下。

### C. handle の公開フィールド

- **C1(採用)**: handle になった Named struct の公開フィールドごとに、既存の method 命名規則(`<type><Method>`)に合わせて
  getter `<type><Field>(recv T) F` と setter `<type>Set<Field>(recv T, value F) IO[Unit]` を生成する。
  フィールド型が写せなければそのフィールドだけ skip コメント。setter は可変操作なので `IO`。
- C2. handle を record に展開: private 状態を捨てるので既存規則どおり却下。

## Decision

### 1. ドメインモデル

- **GoPackage**(外部): Go import path で同定。export data から `extern.Package` が読む。本 ADR で変更なし。
- **Binding**(生成物): 1 GoPackage に対する `bindings.gt` 本文と shim Go 本文の組。
  同一性は `(Go version, go4t 自身のビルド, GoPackage の export data, shim Go import path)` で決まる純関数の出力。
  所有者は `extern`(生成)。配置は `cmd/go4t`(build 出力配下)。**リポジトリには属さない。**
- **GoImport**(言語側): `import "go:<path>"`。`modules` が解決するが、生成は `modules` が依存しない port
  `GoBinder` を通じて行う。

```go
// modules
type GoBinder interface {
    // Bind returns the go4t module (parsed bindings) for a Go import path.
    Bind(importPath string) (*ast.Module, error)
}
```

`cmd/go4t` が `extern` を使う adapter を実装して `modules.Load` に渡す。`check`/`lsp`/`fmt` は shim を書かない
adapter(extern の import path は `go4tbind/<path>` の仮値)で十分。`build`/`run` は shim を出力ディレクトリに書く
adapter を渡す。`modules` → `extern` の依存は作らない。

### 2. import 解決

- `go:` 接頭辞のみを GoImport と見なす。既存の `go4tbind/...`(A1)は PoC の間は残し、PoC 成功後に
  `go4t get` の自動 bind と `go4t bind --module` を外す(別 ADR で cutover)。
- 名前: `import "go:net/http"` は修飾子 `http`、`import gohttp "go:net/http"` で別名。パッケージ名は
  `bindPackages` と同じく `goIdentifier(path.Base)` + キーワード回避。
- 標準ライブラリは go.mod 無しでも bind できる(`run` の一時モジュール内で shim を書く)。
  第三者モジュールは build module の go.mod に require が必要。無ければ import 位置でエラー。

### 4b. 外部型の所有モジュールと upcast(PoC 中に判明)

PoC の受け入れ 1 は最初の実装で型エラーになった: `http.newRequest(_, _, p2 Reader)` の `Reader` は net/http の
bindings 内で再宣言された `io.Reader` の handle、`strings.newReader` の返す `Reader` は `*strings.Reader` の handle で、
SPEC §8「handle は binding モジュールごとに nominal」のため合成できない。決定:

- **外部型(handle も record も)はそれを所有する Go パッケージの `go:` モジュールでのみ宣言する。** 他パッケージの
  bindings は `import go_io "go:io"` を生成して `go_io.Reader` と修飾参照する。ローダは生成モジュールの import も
  再帰的に解決するので、`go:io` は利用者が import しなくても bind される。
- **upcast を生成する。** 具体型 T が所有パッケージおよびその import 先の公開 interface I を満たすとき
  `<t>As<I>(recv T) <pkg>.I` を恒等 shim として生成する(`readerAsReader: strings.Reader → io.Reader`、
  `readCloserAsReader: io.ReadCloser → io.Reader`)。go4t に subtyping を入れず、Kotlin/Java 境界と同じ明示変換にする。
- タプルは既存 ABI の arity 2・3 まで。4 以上は skip(Go API で稀)。
- 同じ Go パッケージを複数ファイルが import しても生成は 1 回(ローダの `seen`)。

### 3. build の順序

`main.go:run` を「出力ディレクトリと build module を先に確定 → parse/load(GoBinder が shim を書く)→ check → codegen」
に並べ替える。`prepareBuildModule` の foreign チェックは、GoImport が非 std パッケージを指したときに
adapter 側でエラーにするので、extern 走査は不要になる。

### 4. 写像(B1)と accessor(C1)

`extern/mirror.go:call` の `n > 1` 分岐をタプルに、`resolve(t, callback=false)` を result 位置でも callback を試す形に、
`iter.Seq`/`iter.Seq2` を `List` に写す。`extern/shim.go` はタプルの構築、`iter` の具体化、func 返却の wrap を emit。
accessor は `BindWithShim` が handle 型の Named struct を列挙して生成する。SPEC §8 の boundary table に行を足す。

### 5. 変えないもの

- 純粋/IO の境界、`laws/`、`IO` の表現、既存の record コピー境界(`docs/go-interop.md`)。
- 既存の「ctx 無し・error 無しの method は純粋」規則。handle に対する setter は IO にするが、既存の
  `serveMuxHandle(...) Unit` のような可変 method が純粋のままである矛盾は本 ADR では触らない(Follow-up)。

## Acceptance (PoC)

結果(2026-11-02):

1. `examples/go-import/main.gt`: `go:strings` の `cut` をタプルで受け、`strings.newReader |> readerAsReader` →
   `gohttp.newRequest` → `requestHeader`/`requestSetHeader` → `clientDo(defaultClient(), _)` →
   `goio.readAll(goio.readCloserAsReader(responseBody(_)))` で自前 `std/http` サーバを叩き
   `200 hello Ada via GET /greet` を出力。`go4t run` 8 秒(shim 29 パッケージ、2.6 MB、`_gen/` は gitignore 済み)。
2. `go4t check examples/go-import` は exit 0 で、`go4tbind/` は作られない(`examples/sql-service/go4tbind` は既存)。
3. `bind strings --shim`: skip 7 → 0。`bind net/http --shim`: 145 → 600 extern(accessor 込み)/ skip 5(chan と
   `[32]byte`、`map[string]func` のフィールド)。`context.WithCancel` は先頭 ctx 規則により `IO[(Context, func() Unit)]`。
4. `go test ./...` 通過。

PoC 中に追加した規則: 4b(所有モジュールと upcast)、`internal`/`vendor` 配下の型は写さない(protobuf の
`pragma.DoNotImplement` 経由で bind 不能パッケージに到達するため)、関数とメソッドの binding 名衝突は先勝ちで skip、
opaque struct の `new<Type>()`、パッケージ変数・基本型定数の getter。

### client-go 到達(2026-11-03)

`k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` 126 extern / 型由来 skip 0、`k8s.io/client-go/dynamic` 34 / 0
(3 段 interface 連鎖と `ListOptions` 等の record が全部届く)。`go:` import だけの `newUnstructured → setObject → getName`
スモークは `check` 3.5 秒・`run` 18 秒(推移 26 パッケージ)。ここで決めた規則:

- **JSON scalar**: `T: json.Marshaler` かつ `*T: json.Unmarshaler` な struct は record/handle より先に `dyn.Value` に写す
  (`metav1.Time`, `resource.Quantity`, `intstr.IntOrString`)。名前一致ではなく interface 実装で判定する。
  これで `ObjectMeta`・`Pod`・`Node`・`PodSpec` が record になり、typed clientset の値を純粋関数で読める。
- **名前の順位**: method/function > field accessor > constructor/value getter > upcast。負けた側は
  `// skipped: X (name n taken by Y)` で勝者を名指しする。走査順には依存しない。`meta/v1` の skip 62 → 8。
- **同名 interface への upcast** は所有パッケージの短い接尾辞で両方出す(`unstructuredAsMetaV1Object` / `unstructuredAsRuntimeObject`)。
- **キーワード名のフィールド**は `type_` のように `_` を付ける(パッケージ名と同じ規則)。`SELinuxOptions.Type` 1 つで
  `PodSpec` 全体が handle になっていた。`core/v1` の skip 0。

## Follow-up

- [ ] クロスビルドキャッシュ(A3)。net/http で推移的に 29 パッケージ・8 秒。キー: Go version + go4t build + export data。
- [ ] 同一 shim 内で同じ非修飾名を持つ別パッケージの record が `rt.ForeignRecord` のキーで衝突する(既存の制限)。
- [ ] `go4t get` 自動 bind / `go4t bind --module` / `examples/sql-service/go4tbind/` の cutover。
- [x] handle を受け取る呼び出しは全部 `IO`(SPEC §8): method receiver、引数(`List`/`Map`/`Option` の中も)、フィールド getter。
      理由は「変更する」ではなく「共有状態を読む」。record/map/slice はコピーなので純粋。constructor・package 変数 getter・
      upcast は handle を読まずに返すだけなので純粋。法則の対象となる純粋断片に共有状態を観測・変更する extern が入り込まない。
- [ ] `async` 構文糖(`IO` の暗黙 lift)。別 ADR。
