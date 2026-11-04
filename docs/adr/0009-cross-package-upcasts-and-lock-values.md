# ADR 0009: package 跨ぎの upcast と、lock を含む値の境界越え

Date: 2026-11-04
Status: Proposed (design review 1 回反映)

## Context

`examples/approval-controller` の observe を go4t に移した際(`4eeb131`)、binder の限界が 2 つ出た。
どちらも `go:` import の生成規則(SPEC §8)の問題で、言語コアには触れない。

### 1. upcast が所有パッケージに閉じている

`extern/upcasts.go:30-49`: パッケージ P を bind するとき、upcast `<t>As<I>` の候補は
**source T = P の exported 型(record / handle / jsonscalar)**、**target I = P と P の直接 import に宣言された
method set を持つ interface** に限り、binding は P の module に置かれる。

`controller-runtime/pkg/client` の `Client.Get(ctx, ObjectKey, Object, ...GetOption)` は bind される
(`clientGet(recv Client, NamespacedName, Object, List[GetOption]) IO[Unit]`)。しかし渡したい
`unstructured.Unstructured` は `apimachinery/.../unstructured` の型で、同パッケージは controller-runtime を
import しないため `unstructured` を bind しても `unstructuredAsObject` は生まれず、`client` を bind するときは
source が `client` の型に限られるのでそこにも生まれない。結果:

```
probe.gt:10:83: type mismatch: expected client.Object, got unstructured.Unstructured
```

回避は Go 側に `fetch/fetch.go`(Get/List の生呼び出し 29 行)を置き、`go:approvalctl/fetch` で import した。
「interface を定義する側のパッケージが、実装型のパッケージを import している」のは Go の普通の形
(`client` は `unstructured` を直接 import している。非テスト 1 ファイル、テスト 5 ファイル)で、k8s に限らない。

### 2. 生成 shim が lock を含む struct を値コピーする

`go vet -copylocks` を `examples/approval-controller/_decide_gen` に掛けると 11 件。**全部が 3 つの生成経路**で、
いずれも「Go 関数が T を値で受け渡す署名」ではない:

| 経路 | 生成箇所 | 例 | vet |
| --- | --- | --- | --- |
| パッケージ変数の getter | `extern/values.go:74` → `shim.go:107` `return &v` | `AllocatorPool() any { return go4tMirror115From(go2.AllocatorPool) }`(`var AllocatorPool sync.Pool`)、`ForkLock()`(`sync.RWMutex`) | 変数を値で読んでから箱詰め |
| 値 receiver の選択 | `extern/methods.go:31`「両 method set にあれば値 receiver を優先」 | `Recorder_ResultChan`/`Recorder_Stop` が `go4tMirror24To(recv)` で `*p` を deref(`watch.Recorder` は `sync.Mutex` を持つ) | receiver に値コピー |
| struct handle のフィールド getter | `shim.go:465-473` | 観測例なし(同じ形) | フィールドを値で読む |

どの経路にも **vet が通り、かつ identity を保つ shim が存在する**: 変数は `&go2.ForkLock`、フィールドは
`&recv.Field`、receiver は `recv.(*T)`(handle は既に `*T` の箱)。SPEC §8(1583 行付近)は既に「パッケージ値の getter は
読み通さずに handle を返す」と書いており、値コピーはその規則の実装漏れ。

残るのは**本当に値で受け渡す署名**(`func f(m sync.Mutex)`、`func g() watch.Recorder`)だけで、これは Go で書いても vet が
同じ警告を出す。つまりその署名の値越えは Go 側でも誤りで、正しい shim は存在しない。

## Options

### 1. upcast の探索範囲

- **1A. 使用時に暗黙変換**: `T` を `I` 期待位置に渡したら compiler が upcast を挿入する。言語に subtyping が入る。
  `readerAsReader` を明示生成にした判断(型の同一性は go4t 側で決める、SPEC §8 冒頭)に反する。却下。
- **1B. 全閉包 × 全閉包**: P の依存閉包の全 exported 型 × 全 interface。`client.Object` の実装は k8s.io/api の全型なので
  `client` の binding に数百の `xxxAsObject` が付き、生成量が閉包サイズに比例する。却下。
- **1C. 対称な直接 import 規則(採用)**: P を bind するとき、source T と target I は**それぞれ P または P の直接 import**に
  宣言された型で、**少なくとも一方は P に宣言されている**。今日の規則(T ∈ P、I ∈ P ∪ imports)に「I ∈ P、T ∈ imports」を
  足しただけ。binding は **interface 側 = P の module** に置く(`client.unstructuredAsObject`)。source 側 module に置くと
  source パッケージの生成物が「誰が自分を import しているか」に依存し、パッケージ単位のキャッシュが壊れる。

1C の詳細(レビューで出た穴を規則として固定する):

- **探索順序**: 外来 source は `types.Implements` を**先に**判定し、成立した型だけ `resolve` する。今の `upcasts.go:49` は
  resolve が先で、`mirror.go:65-86` が resolve のたびに import alias を登録し `mirror.go:536-541` が全部 emit するため、
  同じ順序で外来 source を回すと `client` の module が直接 import 31 本(`net/http`、`reflect`、`sync`、`rest`…)を
  全部 import して推移的に bind し始める。Implements 先行なら、新たに import される module は実装型を持つものだけ。
- **規模**: `client` の直接 import は 31 パッケージ、exported 型 約 330(metav1 69、runtime 51、net/http 30、rest 27)。
  走査コストは |外来 T| × |P の I| 回の `Implements`。生成されるのは `client.Object`/`ObjectList` の実装
  (`Unstructured`、`UnstructuredList`、`PartialObjectMetadata(List)`、`metav1.List`、`metav1.Status` 程度)。
- **名前**: 既存は target 名の衝突だけ所有パッケージ接尾辞で曖昧解消する(`upcasts.go:80-101`)。source が複数パッケージに
  またがると `rest.Request`/`http.Request` のような **source 名の衝突**が起き、binding 名 `requestAsI` が同点で
  「先着優先」(SPEC 1535 行付近)により片方が黙って落ちる。規則: source 名が複数の候補で重なるときは target と同じ
  所有接尾辞を source にも付け(`restRequestAsI` / `httpRequestAsI`)、**P 自身の型は接尾辞なしで常に勝つ**。
- **対象外**: generic 型(`exportedNamedTypes` の `TypeParams().Len()==0`、ADR 0006 と整合)、method set を持たない
  interface、`internal`/`vendor`/`C` の直接 import(`mirror.go:92-101`、`load.go:133` の既存除外)。
- **型の同一性**: `resolve` は型構造で決まり、所有 module は命名と `importable` にしか効かないので、`Unstructured` は
  `unstructured` module と `client` module で同じ handle になる(`client` の生成 binding は既に
  `go_…_unstructured.Unstructured` を参照している)。import 辺は Go の import DAG をなぞるだけで循環は増えない。
- **キャッシュ鍵**: `cmd/go4t/bindcache.go:67-88` は P 自身の export data(`loader.ExportHash`)しか鍵に入れていない。
  今日の規則も target I を直接 import から取っている以上、直接 import の内容に依存しているが、鍵には入っていない
  (Go の build ID 連鎖に暗黙に頼っている)。1C で依存が強まるので、**直接 import の export hash を鍵に加える**
  (loader は `l.exports[dep]` を持っている)。`docs/cli.md` の鍵の説明もそれに合わせる。
- 名前付きスカラ下地の実装型(`client.InNamespace string`、`Limit`、`MatchingLabels` など)は `upcasts.go:48` の
  source 種別制限(record/handle/jsonscalar)で今も対象外で、1C でも変えない。`List` の namespace 指定は
  `newListOptions()` → `listOptionsSetNamespace` → `listOptionsAsListOption` の handle 経路で既に届く。
  スカラ実装型を upcast 対象にするかは別 ADR。

### 2. lock を含む値

- **2A'. 格納クラスで分ける(採用)**:
  - **アドレスを取れる格納**(パッケージ変数、struct handle のフィールド、struct handle の receiver)は**コピーせずアドレスを取る**:
    変数 getter は `&go2.Var`、フィールド getter は `&recv.Field`、receiver は `*T` の method set にある限り
    ポインタ receiver(`methods.go:31` の「値 receiver 優先」を struct handle については廃止)。lock の有無に関係なく
    適用する。identity が保たれる分、意味も良くなる(変数を読み通さない、SPEC §8 の既存文言どおり)。
  - **本当に値で受け渡す署名**(引数・戻り値・コンテナ要素として lock を含む struct の**値**)は
    `// skipped: Name (copies lock value T)`。`*T` はこれまでどおり handle。
- 2B. 値を受けたら箱詰めして二度と値に戻さない: 受け取り時のコピーが残り vet は消えない。却下。
- 2C. 生成物に `//nolint`・vet 除外: 原因(Go 側で未定義動作になりうるコピー)が残る。却下。

lock の判定は vet の copylocks と**同じ条件**にする: `Implements(*T, sync.Locker) && !Implements(T, sync.Locker)`
(埋め込み `sync.Locker` interface や値 receiver の `Lock`/`Unlock` は lock ではない)、struct フィールド・配列要素・
埋め込みを再帰。`sync.Locker` は `types.NewInterfaceType` で合成し、追加依存を持たない。
record は現規則では lock を含み得ない(lock 型は非公開フィールドを持つので、それを含む struct は handle になる。SPEC 1488 行付近)。

## Decision

- 1C と 2A' を実装する。言語・型規則・runtime 表現は変えない。変更箇所:
  `extern/upcasts.go`(候補列挙、Implements 先行、source 接尾辞)、`extern/values.go` / `extern/methods.go` /
  `extern/shim.go:465-473`(アドレス取得)、`extern/mirror.go`(lock 判定と skip 理由)、`cmd/go4t/bindcache.go`(鍵)、
  SPEC §8 の upcast 行と 1529-1531 行の散文、SPEC 1490 行付近の値/ポインタ行、`docs/go-interop.md:70-71`、`docs/cli.md` 鍵の説明。
  ADR 0003 §4b はこの ADR で一部置き換えと注記。
- 受け入れ:
  - `examples/approval-controller`: `fetch/fetch.go` を削除。`observe.gt` は `unstructured.newUnstructured()` →
    `unstructuredSetGroupVersionKind(obj, schema.GroupVersionKind{…})` → `io.tryIO(client.clientGet(c, types.NamespacedName{…},
    client.unstructuredAsObject(obj), []))` で読み、`Err(io.NotFound(_))` を missing に写す(`api/errors.IsNotFound` の
    分類器は `cmd/go4t/goimport.go:181-222` が既に link する)。List は `newListOptions()`/`listOptionsSetNamespace`/
    `listOptionsAsListOption` と `client.unstructuredListAsObjectList(list)`。oracle の `TestApproval*` 6 本と `gate_order_test`
    は無変更で通る。
  - 同例で**生成物込み** `go vet ./...` が copylocks 0 件。`_decide_gen/` と `*_gen/` ignore はそのまま(生成物をコミットしない
    運用は変えない)。README の「vet から外す理由」の一文を削る。
  - `extern` fixture: (a) interface を持つ P が実装型を持つ Q を直接 import するケースで `P.qTypeAsI` が生成され、
    source 名が P 自身の型と重なるケースで P の型が無接尾辞で勝つ; (b) `sync.Mutex` を含む struct の **変数 getter /
    フィールド getter / メソッド** がアドレス経由で生成され vet が通る; (c) 同 struct を**値**で返す/受ける関数が
    `copies lock value` で skip される; (d) 埋め込み `sync.Locker` だけの struct は lock 扱いにならない。
  - `go4t check examples/node-drain-typed` の warm(1.8 s)と cold(44 s)を実装前後で計測し、有意に伸びない。
  - `examples/go-import`、`node-drain-typed` の `.gt` は lock 値を使う binding を呼んでいない(確認済み)ので挙動差なし。
- 触らないもの: `--emit-go`(§8.7)は extern を export しない(SPEC 1762 行付近)ので、upcast の露出という論点は存在しない。
  Go 側は interface 代入で済む。

## Consequences

- SPEC §8 upcast 行: 「T の所有 module に、その Go パッケージと import 内の interface を対象に生成」→
  「T と I の少なくとも一方を宣言するパッケージ P の module に、他方が P かその直接 import に宣言されている場合に生成。
  source 名が重なるときは所有接尾辞、P 自身の型が優先」。
- SPEC §8 値/ポインタ行: 「パッケージ変数・フィールド・receiver はアドレス経由。lock を含む struct の値の受け渡しは skip」。
- 1C で `client` のような interface 中心パッケージの binding が増えるが、上限は直接 import 内の実装型数で、閉包には依らない。
  Implements 先行により新たに import される module は実装型を持つものだけ。
- 2A' で、これまで bind できていた lock 値の署名が skip に変わる。vet が指摘するとおり Go 側でも誤りだった形なので互換性の後退とは
  扱わない。変数・フィールド・receiver はアドレス経由に変わるが、identity が保たれる方向の変化で、呼び出し側の go4t コードは変わらない。
- キャッシュ鍵に直接 import の export hash が入るため、依存更新時に P の binding も再生成される(今まで暗黙だった依存が明示になる)。
