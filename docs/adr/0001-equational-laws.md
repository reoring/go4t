# ADR 0001: 等式法則 (`law` / `proof`) を型システムとは別層で検査する

Date: 2026-11-02
Status: Accepted (2026-11-02; PoC 1–4 完了後に採択)

## Context

go4t は Go プログラムの決定コア（観測状態 → 次アクション）を、網羅的 `match`・純粋関数・`IO` 分離で書くための言語である（README, SPEC §1）。
現状の保証は「全ケースを扱っている」「I/O を行わない」までであり、「`decide` は冪等である」「`Stale` は `ttl` が有限のときだけ返る」のような**関数間の等式**はテストでしか担保できない。

[Bend2](https://bend2.dev/notes/what-is-bend2/) は同じ領域（ADT + `match` + `IO` 分離）に依存型を加え、`law` 宣言と同名の `def` で等式を証明し、コンパイラが実行前に検査する。記事の例:

```text
law cancel_idempotent:
  for order: Order
  {cancel(cancel(order)) == cancel(order) : Order}

def cancel_idempotent(order):
  match order:
    case Pending{number}: {==}
    case Cancelled{number}: {==}
    case Shipped{number}: {==}
```

これと同等の検査を go4t に加えたい。

### 確認済みの事実

| Bend2 が証明に依存する仕組み | go4t の現状 | 根拠 |
| --- | --- | --- |
| 依存型（型が値でインデックスされる、等式が型） | HM rank-1。型と項は分離。`unify.go` は構造的単一化のみ | SPEC §5.1, §9; `types/unify.go` |
| 型検査器内の項の正規化器（自由変数を含む項を簡約） | **存在しない**。`go4t eval` も Go にコンパイルして実行 | `types/`, `codegen/`, `cmd/go4t` に interpreter/normalize(項)/const-fold 相当なし |
| 全域性: 停止性検査 + `@unsafe` 汚染の伝播 | なし。一般再帰、正格引数のパニック（`ioPure(1 / 0)`）、trusted extern の純粋性違反が許される | SPEC §6.1, §9 |
| 網羅性検査 | **あり**。欠落パターンを列挙 | `types/coverage.go`, SPEC §5.2 |
| 純粋・正格・不変の意味論 | あり | SPEC §6.1 |
| `deriving` の展開 | AST 上の通常インスタンスに展開される。評価器は derived Eq/Ord/Debug を特別扱いせず `DictRef` 経由で簡約できる | `types/derive.go` `expandDeriving` |
| Int の意味論 | 符号付き int64 ラップ。$\mathbb{Z}/2^{64}$ は可換環なので `+ - *` の結合・交換・分配が厳密に成り立つ。`minInt() / -1 == minInt()` は panic しない | SPEC §10.1 L1575–1576 |
| Float の等価 | NaN は自分自身と等しくない | SPEC §10.1 L1578 |

型検査器の出力 `types.Program` は `Funcs map[string]*FuncInfo`（`Scheme` + `*ast.FuncDecl`）、`DataTypes`、`Instances`、辞書解決済みの参照注釈（`DictRef`、`RefKind`）を公開しており、推論を変更せずに後段が型付き AST を消費できる（`types/types.go` `Program`）。

注釈の十分性（`types/types.go` `Ann`, `codegen/expr.go` `identCall` で確認済み）:

- 全式に `Ann{Type, Ref, Inst, Dicts, Ctor, FieldIndex, FieldIndexes, RecordCases}` が付く（`AnnOf`）。`Check` 後に `TMeta` は残らない。
- 呼び出しの辞書は `Ann.Dicts` に `Scheme.Constraints` 順で入り、`RefMethod` は `Dicts[0]` が受け手の辞書、残りがメソッド自身の制約（`identCall` L273–278）。`DictInstance` は `Instance.Decl *ast.InstanceDecl` にメソッド本体を持ち、`Instance.Context`/`SuperArgs` で文脈辞書に辿れる。`DictParam` は関数 unfold 時に呼び出し側の `DictRef` へ置換する（評価器は辞書環境を持つ）。
- 比較 `== != < <= > >=` は `*ast.Binary` の `Ann.Dicts[0]` に Eq/Ord 辞書を持つ（`Ann` コメント L307）。算術 `+ - * /` は `types/expr.go` L359 で直接型付けされ辞書なし。
- レコード構築・射影・更新は `Ann.Ctor` + `FieldIndex(es)`、多コンストラクタ共通フィールドは `RecordCases` で位置が解決済み。
- extern は `FuncInfo.Decl.Extern != nil` で判別できる（`codegen/scalar.go` L166）。

**汚染解析への帰結**: プリミティブ型の `Eq`/`Ord`/`Show` インスタンスは `std/prelude/prelude.gt` L110–128 の extern（`rt.EqInt`, `rt.LtInt`, `rt.EqString`, `rt.LtString`, `rt.EqBool`, `rt.ShowInt` …）で実装されている。extern を一律汚染にすると、Int フィールドを持つ全レコードの derived Eq が対象外になり法則が書けない。したがって汚染解析は **既知の全域プリミティブ extern の固定ホワイトリスト**を持ち、評価器がそれらに組み込み意味論（Int/String/Bool の等価・順序・表示）を与える。ホワイトリストは `rt` 内の対象関数名で列挙し、利用者 extern は常に汚染とする。

### 制約

- README「What go4t is not」: Go の置き換えでも効果システム研究言語でもない。言語を小さく保つ。
- Go 生成コードの可読性・`--emit-go` の Go ミラー型・型クラス辞書・スカラー特殊化（`codegen/`）を壊さない。
- 証明の健全性は「検査器が使う簡約が実行時意味論と一致すること」に依存する。

## Options

### A. 依存型を導入する

型レベルに項を持ち込み、`types/`（約40ファイル）を依存型チェッカーに置き換え、`codegen/` に消去を実装する。

- 到達範囲: Bend2 と同等以上（`Vec(A, n)` のようなインデックス付き型も含む）。
- 代償: 型クラス辞書、特殊化、`--emit-go` ミラー型、LSP 型表示すべてに波及。言語の方向性（小さい決定コア言語）と矛盾。
- 判定: 却下。

### B. `law` / `proof` を型検査後の別層として追加する（採用候補）

HM 型検査は変更せず、`types.Program` を入力とする新パッケージが法則を検査する。法則は Go 生成に一切出さない。

- 到達範囲: 一階データ上の等式法則。Bend2 記事の例（冪等性、`cancel(Pending(n)) == Cancelled(n)` による欠陥の排除）はすべて表現可能。型にインデックスを持つ側（依存型本体）は対象外。
- 代償: 記号評価器・停止性検査・汚染解析を新規に書く。評価器の健全性が信頼の根拠になる。
- 判定: 採用。

### C. 性質テスト（QuickCheck 風）

`deriving Arbitrary` と `go4t test` 拡張。

- 到達範囲: 反例探索のみ。証明ではない。
- 代償: 最小。コンパイラ変更ほぼなし。
- 判定: 単独では不採用。ただし B の評価器の健全性検証（開いた項の性質テスト）に同じ生成器が必須なので、B の一部として実装する。比較・除算を含む法則の反例探索としての露出は別 ADR。

### D. 外部証明器（Lean 4 / Agda）へ翻訳する

go4t のデータ型・関数は正格・純粋・一階なので機械的に翻訳可能。証明は外部で書く。

- 到達範囲: 外部証明器の全能力。
- 代償: ツールチェーンが二つに割れる。Go コードと証明対象の一致を翻訳器の正しさに委ねる。利用者に別言語を要求する。
- 判定: 不採用。

## Decision

**B を採用する。** 法則検査層 `laws/`（仮）を `types.Check` の後段に置く。

### 構文（確定）

```text
law cancelIdempotent(order Order) {
  cancel(cancel(order)) == cancel(order)
}

proof cancelIdempotent(order) {
  match order {
    Pending(_)   => refl
    Cancelled(_) => refl
    Shipped(_)   => refl
  }
}
```

- `law NAME(params) { lhs == rhs }`: パラメータは `name Type` の単相な量化変数（型パラメータなし）。`lhs`, `rhs` は通常の式として型検査され、同じ型を持つことを要求する。`==` は区切り記号であり `Eq` インスタンスを要求しない。
- **`==` の意味**: 法則の等価は**定義的等価**（両辺の正規形の構文的 α 一致）であり、`Eq` インスタンスとは無関係である。手書き `Eq`（フィールド無視など）がある型でも、証明は正規形一致で判定し、実行時 `==` の結果は主張しない。`Float` は NaN で実行時 `x == x` が偽になる（SPEC §10.1 L1578）ため、正規形一致が実行時と矛盾する。第一版では `Float` を量化変数・両辺の型・部分項のいずれにも許さない。
- `proof NAME(x, y) { tactic }`: 同名 `law` の証明。パラメータは名前のみで、law のパラメータを位置で再束縛する（個数一致）。本体は以下の戦術のみからなる。`law` に `proof` がなければエラー、`proof` に `law` がなければエラー、重複はエラー。
- 文法（EBNF）:
  ```ebnf
  LawDecl   := 'law' Ident '(' [Param {',' Param}] ')' '{' Expr '==' Expr '}'
  ProofDecl := 'proof' Ident '(' [Ident {',' Ident}] ')' '{' Tactic '}'
  Tactic    := 'refl'
             | 'match' Ident '{' {Pattern '=>' Tactic} '}'
             | 'if' Expr '{' Tactic '}' 'else' '{' Tactic '}'
             | 'rewrite' Ident '(' [Expr {',' Expr}] ')' '{' Tactic '}'
  ```
  新キーワード: `law`, `proof`, `refl`, `rewrite`（std・examples・docs に同名識別子は存在しない）。`rewrite` の継続は `in` ではなくブロックで書く（`in` をキーワードにしない）。`match` の Pattern はコンストラクタ/レコード/タプルパターンと変数・`_` の組み合わせに限り、リテラル・ガード・`@` は不可。
- AST: `ast.LawDecl`, `ast.ProofDecl`, `ast.Tactic`（`TRefl`, `TMatch`/`TacticArm`, `TIf`, `TRewrite`）。`ast/ast.go` に確定済み。
- 検査器の出力: `types.Program.Laws map[string]*LawInfo{Decl, ParamTypes}` と `Proofs map[string]*ProofInfo{Decl, Law}`。law の両辺・`if` 条件・`rewrite` 引数は通常の式として注釈される。`match` のパターンは変数の型に対して検査され、束縛変数の型は `ProofInfo` 経由で参照できる。
- 法則検査の入口: `laws.CheckProgram(p *types.Program) []*types.Error`。`go4t check` が `types.Check` の直後に呼ぶ。`build`/`run`/`test`/`eval` は `LawDecl`/`ProofDecl` を無視する（codegen は出力しない）。`fmt` は整形し、LSP は診断・シンボルに含める。

### 戦術（最小集合）

| 戦術 | 意味 |
| --- | --- |
| `refl` | 現在の仮定の下で `lhs` と `rhs` が同じ正規形に簡約される |
| `match x { pat => proof, ... }` | 量化変数 `x` をコンストラクタで場合分けする。網羅性は既存の `coverage.go` で検査 |
| `if cond { proof } else { proof }` | 真偽値式で場合分けし、各分岐で `cond` を `true` / `false` に簡約する仮定を加える |
| `rewrite LAW(args) { proof }` | `LAW` は別 `law` または同名 law（帰納法仮説）。等式を引数で具体化し、その **lhs を rhs に（左→右）**、目標の両辺の**全出現**で置換してから `proof` を続ける。具体化後の lhs が単独の自由変数である `rewrite` は拒否する（全項を置換して任意の等式が閉じるため）。帰納法仮説は「先行引数不変 + 最初に変わる引数が外側 `match` の束縛変数」を要求する（Bend2 の規則） |

### 検査器の部品

1. **記号評価器**: 自由変数を含む型付き AST を簡約する。規則は β 簡約、`let`、`match` 分岐選択、レコード構築・射影・更新、Int/Bool/String 定数の畳み込み、型クラスメソッドの辞書経由の解決（`DictRef` を利用。`deriving` は `types/derive.go` `expandDeriving` で通常インスタンスに展開済みなので特別扱い不要）。意味論は SPEC §6.1（正格・不変・純粋）に従う。純粋かつ全域（部品 2, 3 で保証）な項は合流的なので、引数がスタックしていても関数を unfold し、束縛子の下でも簡約してよい。
   - **燃料**: 停止性検査が通る関数でも、自由変数を含む項の unfold は無限に進み得る（スタックした引数を再帰呼び出しに渡す場合など）。評価器はステップ上限を持ち、超過時は「正規化未完」診断を出す。
   - **α 同値**: 高階量化変数は禁止でも `map(fn(x) => …, xs)` の正規形にはラムダが残る。正規形比較は de Bruijn 化による α 同値で行う。η 同値は不要。
   - **変数捕獲**: `match x { Cons(h, t) => … }` のパターン変数、law の量化変数、本体の `let` 変数が衝突し得る。置換・unfold 時に束縛変数を常に fresh 化する。
   - **`if` 仮定のマッチ単位**: 比較ノードは正規化しないが、その**被演算子は環正規化後**に構文比較する（`now >= at + t` と `now >= t + at` を同一視）。比較ノード丸ごとの構文一致にはしない。
   - **`match` の部分的スタック判定**: arm は順に試し（SPEC §4.7 L540 の first-match）、各 arm のパターンを「一致確定」「不一致確定」「未知」の三値で判定する。不一致確定の arm は飛ばし、一致確定の arm（ガードが `true` に簡約されるもの）を選ぶ。未知の arm に達した時点でスタックとし、`refl` を失敗させて利用者に場合分けを要求する。これにより `freshness(NoTtl, at, now)` は第 2 要素が自由変数でも最初の arm で決まる。
   - **Int 環の正規化**: Int は int64 ラップ（SPEC §10.1 L1575）であり $\mathbb{Z}/2^{64}$ は可換環なので、`+ - *` と単項負号は結合・交換・分配が厳密に成り立つ。正規形として多項式正規形（単項式の係数付き和、変数順序固定）を採用する。これにより `1 + (length(t) + length(ys))` と `(1 + length(t)) + length(ys)` が一致し、帰納法則が閉じる。**比較 `< <= > >=` はラップ下で順序と整合しない**（`a + 1 > a` は `maxInt()` で偽）ため正規化せず、構文一致のみ。`/ %` も正規化しない。
2. **停止性検査**: Bend2 と同じ規則。再帰呼び出しの引数を左から右に見て、先行引数が不変で、ある引数がパラメータのパターン束縛部分項であること。対象は (a) `law` が推移的に参照する関数、(b) `proof` の自己再帰、(c) **`law` 参照グラフ**（`rewrite` で参照される law を呼び出し辺として `proof` を関数と同列に扱う）。(c) がないと law A が `rewrite B`、B が `rewrite A` で相互に閉じ偽が証明できる。一般の関数には課さない。
3. **汚染解析**: `extern`、`IO` を返す関数、パニックし得るプリミティブに推移的に依存する関数は法則の対象外とし、`law` がそれを参照したらエラーにする（Bend2 の `@unsafe` 依存拒否に相当）。除算・剰余は**除数が非ゼロ定数リテラルなら全域**（`minInt() / -1 == minInt()` も Go 仕様で panic しない、SPEC §10.1 L1576）として汚染しない。除数が変数のものは汚染する。巨大確保による panic（SPEC §10 L1597 相当）や非末尾再帰のスタック消費（§9）は Bend2 同様リソース例外として汚染対象にしない。
   - **ホワイトリスト（採択時の決定）**: prelude の 10 extern に加え、`std/string` が使う `rt` の純粋・全域な文字列 extern を対象内とし、評価器は**全引数が定数のときのみ**畳み込む（記号的な文字列推論はしない）。パニックし得るもの（添字範囲外など、SPEC §10 の該当行で確認）は除外。`std/map`・`std/set`・`std/array` は第一版では汚染のまま（Map の正規形設計が要るため別 ADR）。
4. **CLI**: `go4t check` が法則を検査し、失敗は通常の診断（`file:line:col: message` + `^~~~`）で報告する。`build` / `run` / `--emit-go` は `law` / `proof` を無視する。

### 対象外（明示）

- 比較 `< <= > >=` を含む等式は構文一致止まり。利用者がガードを `now >= at + t` と書き、法則側で `at + t <= now` と書けば一致せずスタックする。これは意図した制約。
- `/ %` を含む算術事実。`+ - *` の環等式は上記の正規化で扱う。
- 前提付き法則（`requires f(x) == C` ならば …）。第一版は Bool 等式へのエンコード（`isStale(freshness(..)) == (isFinite(ttl) && ...)`）で代替し、`if` 戦術と derived Eq の簡約で証明する。`requires` を将来足す場合、仮定側がコンストラクタ衝突（`Fresh(_) == Stale`）する分岐を閉じる **absurd 戦術**を同時に要する。
- 型にインデックスを持つ型（`Vec[A, n]`）。
- 高階関数を量化変数に取る法則（第一版は一階データと Int/Bool/String に限る。Float は上記の通り不可）。

### 到達範囲

通る法則（第一版の戦術で閉じる）:

| 種類 | 例 | 戦術 |
| --- | --- | --- |
| 冪等性・正規化 | `cancel(cancel(o)) == cancel(o)` | `match o` + `refl` |
| 分岐被覆 | `freshness(NoTtl, at, now) == Fresh(0)` | `refl`（部分的スタック判定） |
| 二つの `decide` の同値性 | `decideV2(obs) == decideV1(obs)` | `match obs` の入れ子 + ガードごとの `if` |
| ガード込み Bool 不変条件 | `isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now >= at + t)` | `if now >= at + t` 両分岐 `refl` |
| 一階リストの帰納法則 | `length(append(xs, ys)) == length(xs) + length(ys)`、`map` 融合、`append` 結合律 | `match xs` + `rewrite IH in refl`（環正規化） |
| 累積引数つき帰納 | `foldl` 系 | リストを先頭引数に置く（停止性規則「先行引数不変」のため、IH の後続引数のみ変えられる） |

通らない法則（設計どおり。一部は手動分解で回避可）:

- 比較の推移・順序推論（`a < b && b < c ⇒ a < c`）。`if` で両方仮定しても `a < c` は簡約しない。
- 除数が変数の除算を含む関数すべて。
- 前提付き法則。Bool 等式エンコードは結論側の型に `Eq` インスタンスがあるときのみ。
- `match f(x)` で `f(x)` がスタックする本体。`match x` で `f(x)` が簡約される場合のみ進む。
- 診断品質: 失敗時はスタックした正規形を表示するのみ。次の `match` / `if` は利用者が正規形を読んで決める。

## Consequences

- 肯定: 決定コアの法則（冪等性、不変条件、二つの decide 実装の同値性）をコンパイル時に検査できる。既存の網羅性検査・純粋性・正格意味論がそのまま健全性の前提になる。HM 推論と Go 生成は無変更。
- 肯定: 法則は Go 生成に出ないので、`--emit-go` の利用者には影響しない。
- 否定: 記号評価器が実行時意味論と乖離すれば証明が無効になる。健全性の本体は「開いた項の各簡約ステップが全ての代入で妥当」であり、閉じた項の `go4t run` 比較では検証できない。必須の検証は**開いた項の性質テスト**: 開いた項 $t$ を正規化して $n$ を得る → ランダム代入 $\sigma$ で $t\sigma$ を `go4t run`、$n\sigma$ を定数畳み込み → 一致を確認する。生成器は C（QuickCheck 風）の機構を評価器のテスト基盤として要する。
- 否定: 言語に `law` / `proof` / `refl` / `rewrite` が増える。SPEC の文法・診断・§9 の更新が必要。
- 否定: 汚染解析により std の一部（除算を使う関数など）は法則の対象外になる。対象範囲を `docs/stdlib.md` に表示する必要がある。
- 否定: 汚染解析が std の広い範囲（`/` や extern に触れる補助関数）を対象外にすると、決定コアごと法則の対象外になり得る。PoC の早期に対象可否表を出して実用性を判断する。

## Follow-up

- [x] PoC 1（2026-11-02、`laws/`）: Go API `laws.Check(p *types.Program, name, proof)` と戦術 `Refl{}` / `Match{Var, Arms}`。法則は `func law_NAME(params) Bool { lhs == rhs }` として通常の go4t 関数で書く（文法変更なし）。`cancel` 冪等性（3分岐 split）、`cancel(Pending(n)) == Cancelled(n)`（`Refl` のみ）、README `freshness` の `NoTtl`/`InvalidTtl` 分岐被覆（第2要素が自由変数でも `Refl` で閉じる）が通り、`cancel_bad1`（Cancelled→Pending）は冪等性で、`cancel_bad2`（`Cancelled(0)`）は `cancelPending` で失敗する。IO 依存・非ホワイトリスト extern・Float・燃料超過は拒否。発見: `types.Check` は `main` を必須とする（`types/check.go` L585–599）ので法則専用モジュールにも `main` が要る。失敗時は両辺のスタック正規形（unfold 済み `match` 全体）を表示するのみで、ADR の診断品質の制約どおり。
- [x] PoC 2（2026-11-02、`laws/`）: 戦術 `If{Hyp, Args, Then, Else}`。仮定の条件は検査済み go4t 関数 `hyp_NAME(params) Bool`（単相のみ）で書き、正規化した条件項と α 同値な Bool 部分項（unfold 後のガード・`if` 条件を含む）を分岐ごとに `true`/`false` に置換する。`law_staleIff: isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now >= at + t)` が `If` + 両分岐 `Refl` で通り、`Refl` 単独・被演算子を入れ替えた仮定（`now >= t + at`）はスタック（環正規化なし、設計どおり）。定数に決まる条件、arity/型不一致、多相仮定は拒否。分岐失敗は `if hyp then/else` を名指しする。観察: Int の `>=` は `Ord[Int]` 辞書経由で `compare` → `rt.LtInt`/`rt.EqInt` に unfold されるため、スタック正規形は比較ノードではなく展開済み項になる（診断の可読性に影響。PoC 3 の環正規化はこの展開内の被演算子に適用する必要がある）。
- [x] PoC 3（2026-11-02、`laws/`、`ring.go` `rewrite.go` `resume.go`）: Int 環の正規形（単項式の疎和、原子は順序付き、係数は int64 ラップ。比較・`/ %`・関数適用・残留 `match` は原子で、その被演算子は再帰的に環正規化）。`If` の被演算子入替仮定（`now >= t + at`）が通るようになった。戦術 `Rewrite{Law, Args, In}`: 左→右・全出現、lhs が単独自由変数なら拒否、帰納法仮説は「先行引数不変 + 最初に変わる引数が外側 `Match` の束縛変数」を要求（違反は `induction hypothesis must structurally decrease`）、law 参照グラフ（`Rewrite` 辺 + law 本体内の `law_*` 参照、通常の関数呼び出しは辺でない）の循環を拒否。他 law は再証明せず存在と非循環のみ検査。`len(app(xs, ys)) == len(xs) + len(ys)`、`app(xs, []) == xs`、`app` 結合律が `Match xs` + `Rewrite IH in Refl` で通り、`(a + b) * c == c * b + a * c` と int64 ラップ等式が `Refl` で通る。発見: (1) 誤法則 `len(app) == len(xs) + len(ys) + 1` は Cons ステップが自身の IH で閉じ、Nil 基底で失敗する（健全性には問題なし）。(2) std の `list.length` は累積引数付き `fold` 実装のため、IH（累積 0）が Cons ステップ（累積 1）に合わず閉じない。累積引数の一般化補題が要る＝ADR 到達範囲「累積引数つき帰納」の制約が std 関数に直接当たる。(3) パーサは `-9223372036854775808` リテラルを拒否する（`-9223372036854775807 - 1` と書く）。
- [x] PoC 4（2026-11-02、`laws/property_test.go`, `laws/property_proofs_test.go`）: 全 fixture の全 `law_*` の両辺、および部分証明（`Match`/`If`/`Rewrite` 途中）の残留目標を正規化してから、決定的 PCG 生成器でパラメータに 50 代入（Int は minInt/maxInt/0/±1 強制、Bool、String（制御文字・Unicode 含む）、`types.DataType` から深さ有界で生成した一階 ADT/List/Option/タプル）。`t σ` は fixture ごとに 1 回コンパイルした実プログラムを `go run ./cmd/go4t run` で実行して `debug` 出力、`n σ` は評価器で代入・定数畳み込みして同じ Debug 形式で描画し比較。430 対象 × 50 = 21,500 比較、**不一致 0**。実行時間約 33 秒（`go test ./laws/` 全体が約 32 秒になる）。Float 法則は skip、意図的に拒否される fixture は拒否理由を厳密に期待。
- [x] 汚染解析の std 到達範囲表（2026-11-02、`laws/taint_reach_test.go` が `docs/adr/0001-laws-std-reach.md` を生成・スナップショット検証）: 368 宣言中 152 対象内 / 216 汚染。第一理由: 非ホワイトリスト extern 79、Float 59、IO 45、推移 33、除算 0。`std/prelude` 47/47、`std/list` 67/70、`std/result` 18/19、`std/state` 10/10 は対象内。**`std/string` 0/32 と `std/map` 0/26 は全滅**（`rt` extern 実装のため）。`list.sort*` も array extern 経由で汚染。決定コア（コンストラクタで決まる決定・list/Option/Result・Int/Bool/String の Eq/Ord/Show・文字列結合）は対象内だが、`string.trim/split/startsWith/contains/parseInt` と `map` 一式が使えないのは実用上痛い。→ **ホワイトリストの拡張が必要**: `rt` の純粋・全域な string/map 関数に評価器の組み込み意味論（String は定数のときのみ畳み込み、Map は正規形として順序付き連想リストを持つなど）を与える。これは Consequences の「std の一部が対象外」を具体化したもので、採択時に範囲を決める。
- [x] `types.Program` の注釈が評価器の入力として十分かを確認した（2026-11-02）。結果は Context「確認済みの事実」末尾を参照。
- [x] 実装（2026-11-02、採択後）: 文法 `law`/`proof`/`refl`/`rewrite`（`lexer/`, `parser/`, `format/`, `ast/walk.go`, `ast/span.go`）、検査器（`types/laws.go`: `Program.Laws/Proofs`, `RefLaw`, `LawSymbol/ProofSymbol`、func/law は同一名前空間で重複拒否）、`laws.CheckProgram(p) []*types.Error`（PoC の Go API と `law_`/`hyp_` 規約は削除。停止性検査 `laws/termination.go` は SCC 単位で共通の減少位置を要求、law 参照グラフは依存順に証明し循環は SCC ごとに 1 回報告。`std/string` 32 extern を `go4t/rt` の実体で定数畳み込み: 到達範囲は 184/368 対象内、`std/string` 32/32）、`go4t check` 配線（`cmd/go4t/main.go`、失敗は戦術位置に `^~~~` 付き診断、exit 1。`build/run/test/eval` は無視、codegen 出力は law の有無で同一）、LSP 診断・シンボル、SPEC §6.5 / §8 / §9、tutorial §13、README、`docs/stdlib.md` の Law-eligible 列。検証: `go build ./... && go vet ./... && go test ./...` 全パス（`laws` 61s）。tutorial §13 と README の law 例 4 本を実 CLI `go4t check` で実行し exit 0、`cancel_bad1` は `17:53: law cancelIdempotent: stuck normal forms differ` を出力。
- 保留（2026-11-02 判断: 需要の証拠が出るまで ADR を立てない）:
  - `std/map`・`set`・`array`: 定数畳み込みのみの拡張は価値が低い（Map を使う法則は Map 自体を量化するため）。記号的正規形（順序付き連想リスト + `insert`/`lookup`/`delete` の書き換え規則、キー `Ord` 依存、`rt` との一致の性質テスト）が要る。**トリガー**: `Map` を引数に取る `decide` の法則を書く必要が出た時。実績の 2 コントローラ移植は `List` で Observed を表しており未発生。
  - 比較の順序推論: int64 ラップ下で素朴な線形算術は不健全（`a + 1 > a` が偽）。設計の主題は「非オーバーフローを仮定として持ち込む方法」になる。**トリガー**: `a < b && b < c ⇒ a < c` 型の不変条件を法則にする必要が出た時。
  - 変数除数の除算: 到達範囲表で汚染理由 0 件。**トリガー**: 汚染で拒否される決定コア関数が実際に出た時。
  - `requires` + absurd: 前提付き法則は現状 Bool 等式へのエンコードで代替可能。**トリガー**: 結論側に `Eq` がない等でエンコードできない具体例が出た時。
