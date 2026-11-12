# ADR 0004: Float を含むデータ型を法則の量化変数に許す

## Summary (English)

Context: A `dyn.Value` constructor contains `Float`, so transitive field inspection
rejected laws over `corev1.Pod` even when projections ignored timestamps.
The alternatives are retaining that exclusion or treating hidden Float-bearing
constructor fields as opaque while still rejecting exposed Float expressions.
Decision: Adopt the latter in `lawHasFloat` and `unsupportedType`; do not descend
into constructor fields for Float checks, but retain recursive `IO` inspection.
Exposed types such as `Float`, `List[Float]`, and `func(Float) …` remain rejected.
Consequences: Projection laws can quantify Float-bearing data without reasoning
about Float values; functions that read Float remain outside the trusted fragment.
Derived `Eq` reaching Float still fails, while definitional equality is unaffected.
The open-term property harness includes NaN, signed zero, and infinities;
recorded execution comparisons found no mismatches.
Status: Accepted and verified on 2026-11-03; Float reduction rules remain deferred
to a separate ADR if a concrete need appears.

Date: 2026-11-03
Status: Accepted (verified 2026-11-03)

## Context

ADR 0003 で `metav1.Time`/`resource.Quantity` 等が `dyn.Value` に写り、`corev1.Pod` が record になった。
`dyn.Value` は `Float(Float)` コンストラクタを持つため、`Pod` を引数に取る法則は `creationTimestamp` に触れなくても
`laws do not support Float` で拒否される(`types/laws.go:lawHasFloat`、`laws/laws.go:unsupportedType` がデータ型の
フィールドを推移的に走査するため)。射影 `project(corev1.Pod) Pod` に法則を書けない。

### 確認済みの事実

- 正規化器は Float に簡約規則を持たない。`ast.FloatLit` は `laws/eval.go:353` で `unsupported law expression` エラー。
  `Eq[Float]` の実体 `rt.EqFloat` は `laws/primitives.go:155-164` の `primitive()` に無く、`extern()` は
  `law depends on extern go4t/rt.EqFloat` を返す。いずれも **stuck 項ではなくエラー**で、証明は失敗する。
- `&&` は短絡する(`laws/primitives.go:254-257`): 左が `false` なら右を評価しない。derived `Eq` の `&&` 連鎖は
  Float フィールドより前のフィールドが不一致なら `false` に落ちる。
- 法則の `lhs == rhs` は定義的等式(正規形の α 同値)で、`Eq` 辞書を経由しない。`project` が Float フィールドを
  読まなければ、`project(m{creationTimestamp: t}) == project(m)` は両辺とも同じ正規形になる。

## Decision

1. **Float 型の式は従来どおり拒否。Float を含むだけのデータ型は許す。**
   `lawHasFloat` と `unsupportedType` の Float 判定はデータ型のコンストラクタフィールドへ降りない。
   `Float`、`List[Float]`、`Option[Float]`、`func(Float) …` のように型引数として露出する Float は拒否のまま。
   `IO` の判定は従来どおりフィールドへ降りる(変更しない)。
2. 法則の結果型(`lhs == rhs` の型)も同じ規則。Float 型の式は拒否されるので結果型が Float になることはない。
3. 依存関数のシグネチャ(`laws/termination.go:148`)と本文の式(`:200`)も同じ規則。Float を**読む**関数は
   その式で拒否され、法則の依存にできない。これは「証明できない」側で、健全性に影響しない。

## 健全性

主張: この緩和で、実行時に偽になる等式が `go4t check` を通ることはない。

Float 値の間に等式を作る経路は (a) Float 式の評価、(b) `Eq`/`Ord` 辞書の unfold が `rt.EqFloat`/`rt.LtFloat` に
到達すること、の 2 つだけ。(a) は型検査で拒否、(b) は `extern()` のエラーで証明が失敗する。従って Float 値は
量化変数の中に不透明な部分項として存在するだけで、正規化器がそれについて何かを結論することはない。
`NaN == NaN` が実行時 `false` であることと矛盾する証明は構成できない。

副作用: `o == o` のような record 全体の `Eq` を使う法則は、Float フィールドに到達した時点でエラーになる
(定義的等式 `refl` は影響を受けない)。

## 検証

`laws/property_test.go` の open-term property harness は正規形を `go4t run` の結果と照合する。
Float の生成器を追加し、**NaN・-0.0・±Inf を必ず含める**(素朴な生成は NaN を引かない)。Float 項は
ビット同一性で比較する不活性な項(簡約規則なし)。

結果: `laws=244 targets=1752 substitutions=50 comparisons=87600 Float-bearing substitutions=842`、不一致なし。
従来の「Float skip」カウンタは消えた(スキップではなく照合される)。std 到達目録(ADR 0001)は
eligible 210 → 225、Float 起因の汚染 59 → 28(`std/json` が 0/39 → 15/24)。

受け入れテスト `laws/testdata/projection.gt`: `dyn.Value` を含む record から `PodKind` へ射影する関数と
3 本の法則(DaemonSet owner → `DaemonSet`、owner 無し → `Managed`、timestamp を差し替えても射影は不変)が
`refl` で閉じる。`projection_bad.gt`: Float を読む法則 `m.stamp == m.stamp` は拒否される。実測では
`rt.EqFloat` より手前、derived `Eq[dyn.Value]` の `Float` 腕にある Float 型の式で
`law stampSelfEqual: Float is not supported in laws` となる。到達順が違うだけで同じ障壁。

## Follow-up

- [ ] Float の簡約規則(定数畳み込み、IEEE 比較)は入れない。需要が出たら別 ADR。
