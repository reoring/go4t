# ADR 0011: 境界は変換しない — 可逆な写像か handle、損失は名前付き関数だけ

Date: 2026-11-04
Status: Proposed

## Context

`examples/approval-controller` の apply を go4t に移した際(`a633d2e`)、`go:` binding に **skip されず、動くように見えて値が変わる**
穴が 3 つ出た。ADR 0009/0010 の穴(生成されない、vet が落ちる)より性質が悪い。

| Go 型 | 今の表現(`extern/mirror.go valueMirror`) | 失われるもの | 観測 |
| --- | --- | --- | --- |
| `json.Marshaler`/`*T Unmarshaler` を持つ struct(`metav1.Time`) | JSON wire 形 → `dyn.Value`(SPEC §8 表 1486 行、`mirror.go:259-263`) | 値の同一性。`metav1.Time.UnmarshalJSON` は `pt.Local()` を呼び、UTC `…Z` が `+09:00` になる | 既存 condition の timestamp を RFC3339 文字列のまま運んで回避 |
| `*[]T` / `*map` 引数 | `Option[List[T]]`(`mirror.go:304-309`、pointer = nullable) | 書き戻し。`meta.SetStatusCondition(*[]Condition, Condition)` は bind されるが slice のコピーに書くので**何も起きない** | go4t で不変の `setCondition` を再実装 |
| `error` | `rt.Failure{Kind, Message}`(`rt/failure.go:25-28`)。**cause フィールドは無い** | `errors.Is`/`errors.As` の同一性。`Failure.Is` は Kind から `os.ErrNotExist` 等 5 つの sentinel を再構成するだけ | 文字列前方一致で分類(soak、`d8505d5`) |

bind cache の実測(`~/.cache/go4t/bind`): metav1 module だけで `dyn.Value` が 309 サイト、`ObjectMeta.creationTimestamp` /
`deletionTimestamp` が `dyn.Value` なので **全 Pod/Node record に乗っている**。`Option[List|Map]` の引数は 62 本。

3 件の根は 1 つ。`valueMirror` は **Go 型 1 つに表現 1 つ**を構造規則で割り当てるが、各 Go 型は 2 つの目的
(JSON: 直列化と同一性、`*T`: nullable 読みと out-param、`error`: 分類と原因)を持ち、片方に潰している。

判定基準も 1 つで言える: **境界越えが可逆か**。all-exported struct の copy は可逆(隠れ状態なし)で record でよい。
JSON 往復は可逆性を証明できない(Go 自身が保証しない)。`*T` 引数の書き戻しは否定できない。error → sum は原因を捨てる。
ADR 0009(handle はアドレスで越える)と ADR 0010(upcast は名前付きの純粋関数、暗黙 subtyping なし)は既にこの方向で、
本 ADR はそれを単一規則に昇格させる。

既存で**意図的に lossy**な規則は 2 つあり、そのまま残す(法則が Int を要求するため、ADR 0004): `time.Duration` → `Int`、
`time.Time` → `Int`(UnixNano、location と monotonic を落とす。SPEC 1483-1484 行)。本 ADR は「表に lossy と書いてある規則」を
禁じない。禁じるのは「表に書かれていない損失」。

## Rule

> **境界は変換しない。可逆に写すか(スカラ、all-exported record の copy、handle のアドレス)、handle で同一性を保つ。
> 情報を失う操作は、名前でそれと分かる binding 関数(`…ToJson`/`…FromJson`、`…Parse`)か、SPEC の表に lossy と明記された行だけに置く。**

構造上は、`valueMirror` を **representation chooser**(可逆性だけで決める: scalar / record(copy) / handle(identity))と
**view generators**(json・フィールド accessor・メソッド・upcast・error 述語、それぞれ独立に binding を足す登録制)に分ける。
今の jsonscalar 分岐は「view の存在が表現を変える」形で、これが SRP 違反の正体。view を足しても chooser は変わらない(OCP)。

## Options

### 1. JSON Marshaler を持つ struct

- 1A. 現状維持(wire 形が表現)。却下: 往復の同一性を保証できない。`deriving Eq` が `dyn.Value` の構造比較になり、
  location 違いの同じ時刻を不等と判定する。
- 1B. `metav1.Time` だけ特例で `time.Time` 規則(Int)に寄せる。却下: `resource.Quantity`、`intstr.IntOrString`、
  `fieldsV1` も同じ分岐にいる。型名で分岐すると OCP に反する。
- **1C(採用). 表現は record/handle の通常規則、JSON は view**。
  - chooser は Marshaler を見ない。`metav1.Time` は `struct{ time.Time }` で promoted field が無い(`time.Time` は非公開フィールド)
    → **handle**。`Quantity` も handle。`IntOrString`(`Type IntOrString; IntVal int32; StrVal string`、全 exported)→ record。
  - view generator が `<t>ToJson(recv T) dyn.Value`(純粋、Marshal)と `<t>FromJson(v dyn.Value) IO[T]`(Unmarshal、
    失敗は IO failure、名前が lossy を示す)を生成する。既存の shim 変換コード(`go4t json scalar: …` panic)はこの 2 関数に移り、
    panic は IO failure になる。
  - record のフィールドに handle が入る(`ObjectMeta.creationTimestamp: Time`)。Go-owned record の handle フィールドは
    `71d060f` で既に成立。
  - **`deriving Eq` は handle フィールドを持つ record には生成しない**(`mirror.go:678`)。handle の等値性は identity しか無く、
    値の等値性を装うのは本 ADR の規則違反。`Debug` は残す(handle は型名を出す)。
  - `metav1.Time` は `time.Time` を埋め込んでいるので `timeTime(recv Time) Int` 相当のメソッド/フィールド経路で UnixNano に届く。
    射影法則で時刻を使うなら Int に落として書く(ADR 0004 の線)。

### 2. ポインタ引数

- 2A. 現状維持(`Option[T]`)。却下: out-param は黙って無効。
- 2B. `*[]T`/`*map`/`*record` 引数を skip。安全だが `SetStatusCondition` のような標準の in-out API が全部消える。
- **2C(採用). 位置で意味を決める**:
  - **結果**の `*T` → `Option[T]`(nullable 読み、現状)。
  - **引数**の `*T`: T が handle → 同一性(現状)。T が scalar/record/collection → **`Ref[T]`**(`rt/ref.go`、IO-only cell)。
    shim は `RefGet` → Go 値へ copy → 呼び出し → 結果の Go 値を go4t 値へ copy → `RefSet`。関数は IO(既に IO でなければ IO に昇格)。
    `setStatusCondition(conds Ref[List[Condition]], c Condition) IO[Bool]` となり、Go の `&conditions` と同じ形で変更が戻る。
  - record **フィールド**の `*T` → `Option[T]`(現状)。フィールドは in-out の位置ではない。
  - 可変長 `...*T` は引数規則に従う。
  - [INFERENCE] `rt.Ref` は `any` を持つ cell で shim から読めるが、extern 側に `Ref[T]` の型名解決が無い。`Chan[T]` と同じ経路
    (SPEC 1480 行の opaque directional handle)で追加できる見込み。実装時に確認。

### 3. error の原因

- 3A. 現状維持(Kind + Message)。却下: `errors.Is(err, target)` を go4t から判定できず、文字列一致に戻る。
- 3B. go4t の `io.Failure` sum に全 Go sentinel を列挙。却下: 閉じた sum は OCP に反し、k8s の `StatusError` 系は列挙不能。
- **3C(採用). `rt.Failure` に `Cause error` を持たせ、go4t には handle として見せる**:
  - `ClassifyError` は Kind/Message を決めた後、元の error を `Cause` に保持する(既存 `*Failure` を wrap する場合は内側の Cause を保つ)。
    SPEC 2231 行「message/cause を置き換えない」の cause が実在するようになる。
  - go4t: `io.cause(e: io.Failure) Option[GoError]`(`GoError` は `error` の handle、prelude)。
  - **述語は bind する**: `go:k8s.io/apimachinery/pkg/api/errors` の `IsNotFound(error) bool` 等は今も通常関数として bind される
    (`goimport.go:181-222` は分類器として link もする)。`errors.Is(err, target error) bool` も `go:errors` から bind でき、
    `target` は Go 側 sentinel の handle(パッケージ変数 getter、ADR 0009 でアドレス経路)。
  - 分類器(`Kind` への写像)は残す。`io.NotFound` の match は今までどおり動き、**原因が欲しいときだけ** `io.cause` を取る。
  - `Failure.Is`(5 sentinel の再構成)は `Cause != nil` なら `errors.Is(Cause, target)` に委譲し、nil のときだけ今の表を使う。

## Decision

- 規則(上の Rule)を SPEC §8 冒頭の「境界の不変条件」として 1 段落で書く。表の各行はこの規則の具体化であり、
  lossy な行(`time.Duration`、`time.Time`)は「lossy」と明記する。
- 実装順は **3 → 2 → 1**(小さく可逆な順、各段で規則を SPEC に先に書く):
  1. **3C**: `rt.Failure.Cause`、`io.cause`、`GoError` handle。受け入れ: `examples/node-drain-typed` の `evict` が
     `io.NotFound` の match のまま動き、soak の文字列前方一致(`TooManyRequests:`)が `apierrors.IsTooManyRequests(cause)` に
     置き換わる。`rt` と `std/io` のテストに cause 保持(wrap 越し)を固定。
  2. **2C**: 引数位置の `*T` → `Ref[T]`。受け入れ: `approval-controller/decide/apply.gt` の自前 `setCondition` を
     `meta.setStatusCondition(ref, c)` に置き換え、oracle 6 本 + gate_order 無変更。cache の 62 本の署名変化を fixture で固定
     (`extraValueDeepCopyInto(recv List[String], p0 Ref[List[String]]) IO[Unit]`)。
  3. **1C**: jsonscalar の廃止と json view。受け入れ: `metav1.Time` が handle、`ObjectMeta.creationTimestamp` が handle フィールド、
     `deriving Eq` が落ちることを fixture で固定。`approval-controller` の RFC3339 文字列回避を削除し、condition の
     `lastTransitionTime` を handle のまま往復させて UTC が保たれることを `observe_inputs_test.go` の既存ケースで確認。
     `node-drain-typed` の 15 法則が無変更で通る(時刻を使う法則は無いことを先に確認)。`std/json` の `project` 法則は
     `…ToJson` を 1 回挟む形に書き換え、本数は減らさない。
- 各段で `go test ./rt/ ./std/... ./extern/ ./cmd/go4t/ ./codegen/ ./docs/` と node-drain-typed の warm check 時間(2.4 s 基準)。
- 触らないもの: `time.Time` → Int、`time.Duration` → Int(ADR 0004 の要請、lossy と明記して維持)。`--emit-go`(§8.7)の
  表現は本 ADR の対象外(export 側は go4t 型から Go 型への写像で、既に可逆)。

## Consequences

- SPEC §8 に不変条件が 1 段落増え、表の jsonscalar 行が削除、`<t>ToJson`/`<t>FromJson` 行と `Ref[T]` 引数行が追加、
  `time.Duration`/`time.Time` 行に「lossy」が付く。
- 既存の go4t コードで `dyn.Value` として読んでいた `creationTimestamp` 等は handle になる。examples 内の利用箇所は
  段 1 の実装前に grep して列挙し、受け入れに含める(現時点で把握しているのは approval-controller の condition timestamp のみ)。
- `deriving Eq` が落ちる record では `==` が型エラーになる。これは「同一性しか無いものに値の等値性を装わない」規則の帰結で、
  必要なら射影して比較する。
- `Ref[T]` 引数により、in-out API が IO に昇格する。純粋だった binding が IO になる例があれば fixture で列挙する。
- `rt.Failure` が元の error を保持するため、Failure の寿命の間 Go 側の error グラフが生存する。controller の reconcile 単位では
  問題にならないが、Failure を長期保持する設計(ログ蓄積)では cause が GC を妨げる。`io.cause` を取らない限り参照しない
  ので、必要なら `Cause` を弱参照にする選択肢を残す。
