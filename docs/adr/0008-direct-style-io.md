# ADR 0008: IO の直接形式 lowering と値表現の計測結果

Date: 2026-11-04
Status: Accepted

## Context

`bench/PERF.md` の follow-up で、unboxed で済む計算(sortints 4.0x、nbody 1.6x、treefold 1.75x)と Map を使う計算
(mapheavy 19x、wordfreq 27x、mapheavy-mut 9.4x)の差が残った。当初の仮説は「boxed 値表現が原因」だった。
`bench/perf/results/2026-11-04-map2.json` の生 pprof を割付サイトごとに再集計した結果、仮説は否定された。

### 計測済みの事実(CPU サンプル中の割付経路、GOMAXPROCS=1)

| kernel | 最大の割付サイト | 内訳 |
|---|---|---|
| mapheavy(永続 Map) | `rt.collEntryBranch` **44.3%**(経路コピー) | `convT64`(Int boxing)1.75%、`mapValue` の struct boxing 1.75%、`Some` 0.9% |
| wordfreq | `collEntryBranch` 23.7%、`collOrder`(String 比較)24.5% 累積 | `Shapes.Cons` 4.0%、`mapValue` boxing 3.2%、`Some` 2.8%、`convTstring` 1.2%、`convT64` 0.8% |
| mapheavy-mut(`std/mutmap`) | 木ノード 13.9% | **`rt.Bind` クロージャ 8.2%**、`Some` 5.7%、do 継続 lambda 5.7% + 2.5%、`convT64` 2.5%; GC 35% |

- Int/String の boxing(`convT64`/`convTstring`)は全 kernel で **4% 未満**。要素の unboxed 化で得られるのは一桁 %。
- 永続 Map の支配項は経路コピー。これは永続性の定義どおりの費用で、order-8 B-tree(割付回数 -47%、バイト +7%)でも壁時間 -18.7% に留まった(棄却済み)。永続 Map と Go の可変 map の比(≈20x)は Clojure/Scala の永続コレクションと同じ桁で、表現の問題ではなく構造の選択。
- `mutmap` の残り 9.4x のうち、IO の実行そのもの(`rt.Bind` クロージャ + 継続 lambda + `Pure`)が約 **17%**、`Some` が 6%。

### IO の現在の lowering

`do { x <- a; rest }` は `(Monad_IO{}).Bind(a, func(x any) any { rest })` に落ちる(`codegen/bindings.go:doStatements`)。
`rt.Bind` は毎回 `IO` クロージャを 1 つ返し、実行時に継続 lambda を呼んでさらに `IO` を受け取る。IO 1 ステップにつき
クロージャ 2〜3 個と `any` boxing。`do` が末尾で自分を再帰呼び出しするループ(`insertAll(i+1)`)では、1 反復ごとに
この全部が割り付けられる。IO が主体のプログラム(サーバ、コントローラ、`mutmap` 利用コード)全てに効く固定費。

## Decision

1. **IO の直接形式 lowering を入れる。** `do` ブロックの Monad 辞書が `std/prelude` の `Monad[IO]` インスタンスに
   静的に確定しているとき、ブロック全体を 1 つの `rt.IO(func(ctx) (any, error) { … })` に落とす。各 `x <- a` は
   `x, err := rt.Run(ctx, a); if err != nil { return nil, err }` の直列文になり、`rt.Bind` と継続 lambda は生成しない。
   `IO` 戻り型の関数本文の末尾に自分自身への呼び出しがあるとき(IO 末尾再帰)は、既存の末尾ループ機構(`codegen/tail.go`)
   と同じ形で `for` ループに落とす。
2. **要素表現の unboxed 化は行わない。** List/Map/Array の要素を native にする変更は計測上 4% 未満の効果しかなく、
   erased ABI(辞書・extern・mirror・selfhost)全体に触れる。否定結果として記録し、再測定で 10% を超える kernel が
   出るまで着手しない。
3. **永続 Map の表現は変えない。** 更新主体のスループットは `std/mutmap`(IO)で取る、という ADR 0003 以来の
   「効果の世界に逃がす」方針を維持する。`mapValue` を pointer 形にして struct boxing(1.7〜3.2%)だけ取る。

## Rules

- 直接形式 lowering は **観測可能な意味を変えない**:
  - 遅延: `do` ブロックの評価は `rt.IO` 値を作るだけで、効果は実行時にしか起きない(SPEC §6.2)。
  - キャンセル: `rt.Bind` が各ステップの前後で行う `ctx.Err()` 検査を、直列文でも各ステップの前後で行う。
  - エラー分類: `rt.ClassifyError` を同じ位置で適用する。
  - `tryIO`/`conc.*`/`io.exit` などの extern は従来どおり `IO` 値を受け渡すので境界は不変。
- 適用条件は「`do` の `Monad` 辞書が `Monad_IO` の具体値」のみ。Option/Result/List/State の `do` は従来の `Bind` 経路。
  辞書が型変数(多相な `M[_] Monad` 関数内)なら従来経路。`prelude.when`/`mapM` 等の汎用コンビネータは変えない。
- IO 末尾再帰ループは、呼び出しが `do` の最終式、または `IO` 戻り型関数本文の末尾位置(`if`/`match` の腕を含む)にある
  自己呼び出しに限る。相互再帰は対象外。ループ化しても各反復で `ctx.Err()` を検査する(無限ループがキャンセル可能
  であることは既存の意味)。
- 法則との接点はない。`laws/` は `IO` を含む法則を `law depends on IO` で拒否し、記号評価器は AST 上で動く。
  property harness(`laws/property_test.go`)は生成プログラムの `debug` 出力だけを照合するので、
  **harness 全件通過を受け入れ条件に含める**(値表現・実行形式の変更が法則の健全性検証を兼ねる)。
- selfhost は従来の `Bind` 経路を出し続けてよい(生成 Go はもともと非同一。fixed point は selfhost 同士の比較)。

## Consequences

- 目標: `mapheavy-mut` 9.4x → **≤ 4x**、`examples/ref`/`http` 系の IO ループで割付回数が反復あたり定数個減る。
  目標値は `bench/budget.json` に反映する。
- 生成 Go は `do` ブロックごとに 1 関数になり、デバッグ時のスタックが浅くなる(副次効果)。
- 否定結果 2 件を `bench/PERF.md` に残す: 要素 unboxed 化(効果 <4%)、永続 Map の表現変更(B-tree -18.7%)。
- `docs/evaluation.md` の「boxed 値が支配的」という記述は誤りだったので訂正する: 支配項は永続 Map の経路コピーと
  IO 実行のクロージャ割付。

## Follow-ups

- `Some` の割付(5.7%)は erased ABI の制約。`lookup` の native 戻り経路(`MutMapLookupInt` 等)は kernel が出てから。
- IO 内の lambda(`forEach(xs, func(x) => …)`)の継続は lowering の対象外。高階 IO コンビネータ
  (`forEach`/`mapM`/`parMapIO`)の IO 特殊化は次の計測で判断する。
