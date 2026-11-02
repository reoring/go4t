# ADR 0002: 法則検査での `Map` の記号的扱いと、スタック箇所の診断

Date: 2026-11-02
Status: Accepted

## Context

ADR 0001 の実地評価（`examples/node-drain-controller`、法則 10 本）で「足りない」と判定された部分のうち、改善可能なものを設計する。

| 足りない部分 | 本 ADR の扱い |
| --- | --- |
| `Map` を持つ観測状態が法則の外になる（`std/map` 0/26 汚染） | **対象**。記号的正規形と代数規則 |
| `refl` が閉じない時に 700 文字超の正規形を読む必要がある | **対象**。止まっている部分項と両辺の最初の相違点を名指しする |
| 再帰呼び出しの unfold が非対称で `refl` が閉じない | 別途修正済み（ADR 0001 Follow-up）。本 ADR の前提 |
| 文字列の中身に依存する分岐 | **対象外**。文字列の記号推論は入れない。設計規律: 文字列解析は Go シェルで行い ADT として `Observed` に渡す（README の境界規則） |
| 比較の順序推論、前提付き法則 | **対象外**（ADR 0001 保留のまま） |
| Go シェル側の手書き | 対象外（ミラー型のドキュメントコメントのみ別途実施） |
| `Set` / `Array` | **対象外**。`Set` は本設計を `Map[K, Unit]` 相当で再利用できるが需要が出るまで保留 |

### 確認済みの事実

- `Map[K, V]` は comparator 順序付き永続 map（SPEC §2 L209）。`std/map/map.gt` の extern は全て `cmp func(K, K) Ordering` を取る `*By` 版（`mapInsertBy`, `mapLookupBy`, `mapDeleteBy`, `mapMemberBy`, `mapFromListBy`, `mapUnionBy`）か、順序に依存しない版（`emptyMap`, `mapSize`, `mapToList`, `mapKeys`, `mapValues`, `mapMap`, `mapFold`）。`Ord` 版（`mapInsert` 等 L34–48）は `.gt` で `func(a, b) => compare(a, b)` を渡すラッパ。`mapAdjust`/`mapAlter`/`groupBy`/`Eq[Map]`（L54–93）も `.gt` で定義されており、下位の規則から自動的に簡約される。
- `mapToList`/`mapKeys`/`mapValues`/`mapFold`/`mapMap` は昇順（SPEC §10 L1870–1877）。`mapFromList` は後勝ち、`mapUnion` は左優先。
- 決定コアが Map に使うのは `lookup`/`member`/`size`/`fold` 程度（ADR 0001 保留節）。
- ADR 0001 の汚染分類は「ホワイトリスト extern = 全引数が定数のときのみ `rt` 実体で畳む」のみ。記号的に扱う extern の分類は無い。

## Options

### Map の扱い

- **A. 定数畳み込みのみ**（`std/string` と同じ）: 閉じた Map 項しか畳めない。Map を量化する法則（`lookup(insert(m, k, v), k) == Some(v)`）は何も証明できない。却下。
- **B. 記号的正規形 + 代数規則**（採用）: 以下に規定。
- **C. 汚染のまま**: 現状。実コントローラで Map を観測状態に持つ部分が法則の外になる。

### 診断

- **D. 正規形の全文表示のみ**（現状）。
- **E. 止まっている部分項と最初の相違点を名指し**（採用）。正規形全文は残す。
- F. 正規形の深さ制限付き省略表示: 情報が落ちるので E の補助に留め、第一版では入れない。

## Decision

### 1. Map の記号的正規形

評価器の項に `map` 種を追加する。

- **リテラル map** `map{k1: v1, …, kn: vn}`: 全キーが**定数**（Int / String / Bool / それらのタプル）で、キーの昇順に正規化され重複なし。順序は `rt` の比較と同じ（Int は数値順、String はバイト順、Bool は false < true、タプルは辞書式）。
- **挿入チェーン** `insert(m, k, v)`: `k` が記号的（自由変数を含む）な挿入は中立ノードとして残す。正規化規則:
  - `insert(map{…}, c, v)` で `c` 定数 → リテラルに吸収（後勝ち）。
  - `insert(insert(m, k, v1), k, v2)` で `k` が α 同値 → `insert(m, k, v2)`（シャドウ）。
  - 異なる記号キー同士は並べ替えない（順序は意味を持たないが正規形の一意性のために挿入順を保つ）。
- **空 map** は `map{}`。

### 2. 代数規則（`cmp` が `Ord` 辞書の `compare` のときのみ）

規則が使えるのは、`*By` の `cmp` 引数が `func(a, b) => compare(a, b)` の形（`std/map` の `Ord` ラッパが渡すもの）で、その `Ord` 辞書が prelude の Int/String/Bool インスタンス、derived `Ord`、またはそれらのタプル／合成であるときに限る。利用者定義の `cmp` や手書き `Ord` インスタンスを持つキー型では、全演算が中立のまま（汚染ではなく「進まない」）。根拠: 規則はキーの等価性を `Ord` の上位クラス `Eq` の `==` で判定するため、`compare(a, b) == EQ ⇔ a == b` が成り立つ必要があり、これは prelude と derived インスタンスでのみ保証される。

以下 `k == k'` は `Eq[K]` 辞書の `eq`（Int なら `rt.EqInt(k, k')`）に正規化される Bool 項。

| 演算 | 規則 |
| --- | --- |
| `lookup(map{}, k)` | `None` |
| `lookup(map{c1: v1, …, cn: vn}, k)` | `k` 定数なら決定。`k` 記号的なら `if k == c1 { Some(v1) } else if … else { None }`（map サイズで有界） |
| `lookup(insert(m, k, v), k')` | `if k' == k { Some(v) } else { lookup(m, k') }` |
| `member(m, k)` | `lookup` と同形（`true`/`false`） |
| `delete(map{…}, c)` 定数 | リテラルから除去 |
| `delete(insert(m, k, v), k')` | `if k' == k { delete(m, k') } else { insert(delete(m, k'), k, v) }` |
| `size(map{…})` | 定数 |
| `size(insert(m, k, v))` | `if member(m, k) { size(m) } else { size(m) + 1 }` |
| `fromList(xs)` | `xs` がリテラルリストで全キー定数ならリテラル map。それ以外は `fold` で `insert` に展開（`fromList(Cons((k, v), r)) → insert(fromList(r), k, v)` を後勝ちに合わせた順で） |
| `toList` / `keys` / `values` / `fold` / `mapMap` / `union` | リテラル map 同士（`union` は両方リテラル）のみ畳む。`insert` チェーンに対しては中立 |

`mapAdjust`/`mapAlter`/`groupBy`/`Eq[Map]`/`Debug[Map]` は `.gt` 定義を unfold するだけで上の規則に落ちる。`Eq[Map]` は `mapToList` 経由なのでリテラル map 同士でのみ決まる（記号的 map の等価は定義的等価＝正規形一致で判定する。ADR 0001 の `==` の意味と同じ）。

`if k' == k` の条件は ADR 0001 の `if` 戦術の仮定と α 同値で一致するので、利用者は `if k == k2 { refl } else { refl }` の形で証明する。これは `freshness` のガードと同じ使い勝手。

### 3. 汚染分類の追加

ADR 0001 の 2 分類（汚染 / ホワイトリスト定数畳み込み）に **「評価器がモデル化する extern」** を加える。`std/map` の extern 13 個がこの分類に入る（`mapInsertBy` 等は `cmp` が上記の形のときのみ規則が適用され、そうでなければ中立。どちらも汚染ではない）。到達範囲表（`docs/adr/0001-laws-std-reach.md`）に分類列を追加し、`std/map` が対象内になることを示す。

### 4. 健全性の検証

ADR 0001 の開いた項の性質テストをそのまま使う: Map を含む法則の両辺と部分証明の残留目標を正規化し、ランダム代入（キーは衝突が起きるよう小さな値域から生成、記号キー同士の等価・不等価の両方を踏む）で `go4t run` の `debug` 出力と比較する。これが「規則と `rt` 実装の一致」の検証そのもの。

### 5. スタック箇所の診断

`refl` 失敗時のメッセージに 2 行を追加する（既存の `LHS:` / `RHS:` 全文は残す）:

- `stuck on: <項>`: 各辺について、正規化を止めている**最外の中立な被除去項**（`match`/`if` のスクラッティニー、または引数が中立な再帰呼び出し）を 1 つ。両辺で同じなら 1 行。例: `stuck on: o#1.initial`。利用者はこれを `match`/`if` の対象にすればよい。
- `differs at: <LHS 部分項> vs <RHS 部分項>`: 両辺の正規形を同時に走査し、最初に α 同値でなくなる最小の部分項の組。例: `differs at: _firstEvictable(rest#3) vs match rest#3 { … }`。

両方とも正規形の走査だけで求まり、評価器の規則は変えない。

## Consequences

- 肯定: Map を観測状態に持つ決定コアの法則が書ける。到達点は ADR 0001 の実地評価で述べた「一階データ + Map で閉じ、文字列解析をシェルに押し出したコントローラ」。
- 肯定: 証明の手数は「記号キーごとの `if k == k'` の場合分け」で増えるが、既存の戦術で足りる。
- 否定: 手書き `Ord` のキー型では Map 演算が進まない。診断 `stuck on:` にその旨は出ないので、SPEC の制限節に記す。
- 否定: `fold`/`toList` 系は `insert` チェーンに対して中立のまま。集計を含む法則はリテラル map か `match` で具体化した map でしか閉じない。
- 否定: 評価器に Map 固有の規則が 10 本前後入る。性質テストが各規則を踏むことを担保する必要がある（代入生成器がキー衝突を起こす値域を使う）。

### 実地評価（2026-11-02、ゾーン配置の決定コア、`capacity`/`used` が `Map[String, Int]`、法則 6 本）

| 回 | 題材 | 法則 | 書いたまま通過 | 回避で通過（原因 → 対応） | 設計上の対象外 |
| --- | --- | --- | --- | --- | --- |
| 1 | attachment-repair（実コントローラ） | 4 | 2 | 2（レコード更新の射影 → 修正済み） | 0 |
| 2 | node-drain（新規） | 10 | 8 | 2（opaque 規則 → 修正済み） | 0 |
| 3 | zone placement（Map） | 6 | 3 | 2（`mapDelete` の記号簡約、比較の向き → 修正済み） | 1（`insertCommutes`: Map 値等式、意図どおり不成立） |
| 4 | admission（4 ゲートを `Result` の `flatMap`/`map` で連鎖） | 7 | 5（+2 は利用者側の誤り: 偽の法則 1、戦術順序 1） | 0（評価器起因なし） | 0 |
| 5 | node-drain V2（if↔タプル `match`、`match`↔`result.maybe`、first-fit 再帰↔`head∘filter` に書き換え）と `decide(o) == decideV2(o)` + 補題 | 2 | 2（Bool のリテラル `match` は意図的な制限、`if` へ書き換え） | 0 | 0 |

- Map 越しの帰納法則（`emptyCapacityRejects(o, zones)`: `emptyMap` ならどんな fallback 列でも `Reject`）が素直に通った。`room(o, zone)` のような lookup 2 本のタプル `match` も `if` 1 つで畳めた。
- 4 回目: `Result` の `flatMap` 連鎖は prelude の Monad 辞書解決 → `match` → `Err` 早期帰還で無加工に簡約。derived `Eq` の `Result[Reason, Int]` 比較も通る。偽の法則 `fullPassAdmits`（`q < 0` で quota ゲートが拒否）を `stuck on: if (q#3 < 0) …` が反例条件として示した＝検査器が利用者の誤りを捕まえた初例。ゲート順を入れ替える破壊で 7 本中 6 本が落ちる。
- 戦術順序（`if` 仮定 → `match` 分割で仮定が外れる）は修正: 仮定の照合時に現在の解で再正規化して比較する（`laws/normal.go` `assume`、再帰防止フラグ付き）。順序はもう問わない。fixture `assume_order.gt`。
- 4 回とも健全性の欠陥は出ておらず、全て「正しいのに閉じない」完全性側。`stuck on:` により原因特定は 1 分程度。
- 5 回目: リファクタの安全網として機能。1 本の法則が全分岐を踏み、構造の違いを正規化が吸収。残ったのは本質的な場合分け（Bool 2 × node 3 × deadline 2 × blocked 2 × 補題）で証明約 25 行＝仕様の複雑さそのもの。std の `filter`/`head` は構造再帰なので IH が噛む（fold ベースの `length` とは違う）。4 回目以降、利用者は評価器のソースを一度も開いていない。fixture `refactor_v2.gt`。
- **判定（2026-11-02）**: 「評価器起因ゼロが 2 回連続」を性格の異なる題材（Result 連鎖／リファクタ同値）で満たした。健全性欠陥は 5 回ゼロ。**「決定コアだけを書く言語」という自己定義の範囲では使える。** 使える範囲: 一階 ADT + レコード + List + Option/Result + Map で閉じる決定コア、`o{...}` 全称の不変条件、ゲート順・早期帰還・冪等性・分岐被覆、構造再帰上の帰納（std の `filter`/`head`/`append` 含む）、二つの `decide` の同値性。使えない（宣言済み）: Set/Array、文字列の中身の推論、比較の順序推論、`requires`、fold ベース std 関数への帰納、Map 値そのものの等式。「使える」に含めないもの: Go シェル側は楽になっていない（目標外）、落ちたときの正規形は長い、証明の行数は分岐数に比例する。

## Follow-up

- [x] 実装（2026-11-02、`laws/mapterm.go` ほか）: `map` 項種（ソート済み定数キーのリテラル + 記号キーの `insert` チェーン）、規則表どおりの `lookup`/`member`/`delete`/`size`/`fromList`、リテラル限定の `toList`/`keys`/`values`/`fold`/`mapMap`/`union`、`cmp` が prelude/derived `Ord` の `compare` ラムダのときのみ適用。第三分類 `modeled` を到達範囲表に追加し `std/map` 26/26 対象内。fixture `map.gt`/`map_neutral.gt`/`map_decide.gt`/`map_quota.gt`。性質テストは Map 値と小さいキー値域を生成し 0 不一致。付随: 停止性証明書を instance method にも拡張（再帰 derived `Ord` キー、例 `List[Int]`、が中立のまま無限 unfold しない）。
- [x] 実装（2026-11-02、`laws/diagnose.go`）: `stuck on:` / `differs at:`。`cancel_bad1` は `differs at: Pending(n#2) vs Cancelled(n#2)`、自由レコードフィールドの未分割は `stuck on: o#1.mode`、中立再帰呼び出しは減少位置の引数を表示。
- [x] 付随の評価器規則（2026-11-02、実地スモークで判明）: (1) スクラッティニーの精緻化 — スタックした `match s` の各 arm 内で `s ≡ パターン` を、スタックした `if c` の各分岐で `c ≡ true/false` を仮定環境に積む（`laws/patterns.go`, `eval.go`, `resume.go`）。同じ `o.mode` を二重に `match` する `decide` が `refl` で閉じる。(2) 反射的比較 — `EqInt/EqString/EqBool(a, a) → true`、`LtInt/LtString(a, a) → false`（α 同値な被演算子、`laws/primitives.go`）。`mapLookup(mapInsert(m, k, q), k)` 後の `q >= q` が決まり、`map_quota.gt` の `grantedAllows` が `refl` で閉じる。
- [x] 到達範囲表の分類列、SPEC §6.5（unfold 規則・精緻化・Map 節・診断・制限）、`docs/stdlib.md` の `std/map` 行、ADR 0001 保留節。
- [ ] `Set` は需要が出た時に本設計を `Map[K, Unit]` として再利用する。
