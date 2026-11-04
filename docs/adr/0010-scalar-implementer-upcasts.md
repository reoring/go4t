# ADR 0010: スカラ・コレクション下地の実装型を upcast の source にする

Date: 2026-11-04
Status: Proposed

## Context

ADR 0009 で package 跨ぎの upcast が入り、`client.unstructuredAsObject` が生成されるようになった。
同 ADR は「名前付きスカラ下地の実装型は `upcasts.go` の source 種別制限で対象外、別 ADR」とした。

controller-runtime の list/delete オプションはほぼ全部その形:

```go
type InNamespace string                 // func (n InNamespace) ApplyToList(*ListOptions)
type Limit int64
type Continue string
type MatchingLabels map[string]string
type MatchingFields fields.Set          // map[string]string
type HasLabels []string
```

go4t 側ではこれらは `valueMirror`(`extern/mirror.go:296-303`)で**下地の mirror に潰れる**
(`InNamespace` → `String`、`MatchingLabels` → `Map[String, String]`)。`m.typ` には名前付き型が残っているが、
`upcasts.go:89` が `record`/`handle`/`jsonscalar` 以外を source から外すので、`inNamespaceAsListOption` は生まれない。
`approval-controller` の observe は `newListOptions()` → `listOptionsSetNamespace` → `listOptionsAsListOption` という
3 段の handle 経路で namespace を指定している(`4eeb131`、`f36e1c6`)。Go で 1 トークン `client.InNamespace(ns)` のものが
go4t では 3 呼び出し + 可変 handle で、ユーザーの最初の摩擦になる。

Go 側で見れば、これらは「値型に method set が付いた実装」で、struct 実装型と扱いが違う理由は無い。
違うのは go4t 側の表現だけ: struct は handle/record という**名前付きの型**だが、スカラ下地は**名前を失う**。

## Options

- **A. スカラ下地の named 型を独立の go4t 型(newtype)として宣言する**: `InNamespace` を `type InNamespace = …` のような
  opaque 型にし、`inNamespace(s String) InNamespace` の構築関数と `inNamespaceAsListOption` を生成。
  正確だが、今まで `String` に潰れていた型が全部 opaque になり、`Limit` を `Int` として受けている既存の呼び出しが壊れる。
  `types.NamespacedName.Namespace` のように値として扱いたい named string は多数あり、変更範囲が広すぎる。却下。
- **B. 下地表現のまま、upcast 関数だけを生成する(採用)**: 表現は `String`/`Map[String,String]` のまま、
  `inNamespaceAsListOption(s String) ListOption` を生成する。shim は `go2.InNamespace(v.(string))` のように**下地から named 型に変換して**
  interface に入れる。go4t 側に新しい型は増えず、既存の呼び出しは変わらない。失われるのは「`InNamespace` と `Continue` を取り違えない」
  静的保証だが、それは今日既に `String` に潰れている時点で失われている(この ADR で悪化しない)。
- C. 何もしない(handle 経路で十分): `ListOptions` に対応する handle 経路があるのは `ListOption` だけで、`DeleteAllOfOption` 等
  同形の interface は controller-runtime 以外にもある(`grpc.CallOption` の実装型など)。汎用の経路を持つべき。却下。

B の詳細:

- **source の条件**: ADR 0009 の候補集合(P の型 + P の直接 import の型、少なくとも一方が P)のうち、`valueMirror` が
  `int`/`float`/`bool`/`string`/`bytes`/`slice`/`map` に解決する named 型で、値または `*T` が target interface を実装するもの。
  `pointer`/`parameter` 下地は対象外(`*T` の下地を持つ named 型は Go 側でも稀)。
- **名前と署名**: 既存規則どおり `<t>As<I>(recv <下地 gt>) I`。receiver は下地の go4t 型(`String`、`Map[String, String]`、
  `List[String]`、`Int`)。同名規則(target 名の衝突は所有接尾辞、source 名の衝突は foreign 側に接尾辞、P 自身が勝つ)は ADR 0009 と同じ。
- **shim**: 既存の `<kind>To` 変換で下地の Go 値を得て、`go2.T(value)` で named 型へ変換し、pointer implements なら `&` を取ってから
  interface に代入。コレクション下地はいつもどおりコピー(`Map` → 新しい `map[string]string`)なので aliasing は無い。
- **純粋性**: 既存 upcast と同じく純粋関数(`IO` でない)。下地からの変換も interface への代入も効果を持たない。
- **除外**: `jsonscalar`(既に対象)、`handle`/`record`(既に対象)、lock を含むもの(スカラ下地には存在しない)。
  method set を持たない interface、generic は ADR 0009 と同じ除外。
- **ADR 0009 の探索順序**: `Implements` 先行は維持。スカラ下地の `resolve` は module import を増やさない(下地は prelude 型)ので、
  コストは `Implements` の回数だけ。

## Decision

- B を実装する。変更は `extern/upcasts.go:89` の種別制限(`scalarMirror(m) || kind ∈ {bytes, slice, map}` を許可)と、
  それに対応する shim 生成(`extern/upcasts.go` の emit 側で receiver 変換後に named 型へキャスト)、SPEC §8 の upcast 行 1 文、
  `docs/go-interop.md` の 1 文。
- 受け入れ:
  - `examples/approval-controller/decide/observe.gt` の operations list が `client.clientList(c, …, [client.inNamespaceAsListOption(ns)])`
    になり、`newListOptions`/`listOptionsSetNamespace`/`listOptionsAsListOption` の 3 段が消える。oracle 6 本 + gate_order は無変更で通る。
  - `extern` fixture: `type Tag string` と `type Labels map[string]string` が `Opt` interface を実装し、`tagAsOpt(String) Opt`、
    `labelsAsOpt(Map[String, String]) Opt` が生成されて Go 側で元の named 型として受け取れる(`switch o.(type) { case Tag: … }`)。
    pointer receiver で実装する `type Counter int` も `*Counter` に箱詰めされて通る。
  - `go4t check examples/node-drain-typed` の warm 時間が有意に伸びない(直接 import のスカラ named 型は多いが `Implements` は安価)。
- 触らないもの: A の newtype 化。必要になるのは「同じ下地の実装型を取り違える」事故が実際に起きてからで、
  その時は `jsonscalar` のような別 kind として扱う設計になる。

## Consequences

- SPEC §8 の upcast 行: source の種類に「スカラ・bytes・slice・map 下地の named 型(receiver は下地の go4t 型)」を追加。
- `client` の binding に `inNamespaceAsListOption`、`limitAsListOption`、`continueAsListOption`、`matchingLabelsAsListOption`、
  `hasLabelsAsListOption`、`matchingFieldsAsListOption` と同じ型の `…AsDeleteAllOfOption` が増える。上限は直接 import と P 自身の
  named 非 struct 型の数で、閉包には依らない。
- 下地に潰れた型の静的区別は無いままだが、この ADR 以前から無かったので後退ではない。
