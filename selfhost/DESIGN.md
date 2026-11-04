# selfhost — go4t compiler written in go4t

Goal: `selfhost/*.gt` is a go4t→Go transpiler that compiles itself. Fixed point:
stage1 (built by the Go compiler) compiles `selfhost/*.gt` → stage2 Go source; stage2
binary compiles the same source → stage3; `gofmt`-normalised stage2 == stage3.

Authority: `SPEC.md` for behaviour; the Go implementation (`lexer/`, `parser/`, `types/`,
`codegen/`) is the reference port source. `rt/` stays in Go (it is the runtime, not the
compiler) and is shared by both compilers.

## Pipeline (domain model)

```
String ──lex──▶ List[Token] ──parse──▶ File ──check──▶ Program ──generate──▶ String (Go)
```

| Stage | Owner file | Entry | Notes |
|---|---|---|---|
| lex | `lexer.gt` | `lex(src String, base Pos) CompileResult[List[Token]]` | layout insertion, Float literals, interpolation parts |
| parse | `parser.gt` | `parseFile(name String, src String) CompileResult[File]` | imports, aliases, local functions, sections; laws/proofs parse without runtime declarations |
| modules | `modules.gt` | `loadProgram(inputs List[String]) IO[List[File]]` | lexical directory loading, checkout std, local `go4t.mod`, file-local qualification |
| types | `types.gt` | `unify`, `zonk`, `instantiate`, `generalize`, `resolveSType`, `defaultMetas`, `typeString` | immutable substitution; transparent alias expansion with cycle checks |
| decls | `decls.gt`, `derive_higher.gt`, `derive_json.gt` | `checkProgram(files List[File]) CompileResult[Program]` | environment, dictionaries, structural/higher-kinded/JSON derivation |
| infer | `infer.gt` | `inferFuncBody(info FuncInfo) Check[Core]`, `inferInstanceMethod(inst InstanceInfo, method MethodInfo, im InstMethod) Check[Core]` | expressions, common fields, coverage, recursion, IO-tail lifting and generalisation |
| codegen | `cg_decls.gt`, `cg_expr.gt` | `generate(prog Program, goPackage String) CompileResult[String]` | erased Go, recursive local closures, runtime shape registration |
| driver | `main.gt` | `main` | `prog FILE.gt\|DIR... [-o OUT.go]`; no external prelude file |

Immutability forces one deviation from the Go design: the checker does **not** annotate
the AST. It produces a separate dictionary-explicit IR (`core.gt`). Codegen reads only
Core, never the AST or types. All desugarings listed at the top of `core.gt` are the
checker's job.

## Shared internal contracts

- `base.gt`: `CompileResult[A] = CompileErr(Diag) | CompileOk(A)` (Monad),
  `Diag{pos, msg}`, `TreeMap[K, V]`
  (persistent AA tree: `mapEmpty/mapInsert/mapLookup/mapMember/mapToList/mapFromList/
  mapKeys/mapValues/mapUnion/mapMap/mapSize`), list helpers (`range`, `nth`, `indexOf`,
  `zipWithIndex`, `all`, `any`, `last`, `unzip`, `foldResult`, `duplicates`, `lookupAssoc`).
- `ast.gt`: `Pos`, `Token{kind, text, pos, parts}`, `TokKind`, `InterpPart`, `SType`,
  `TypeParam{constraints: List[String], ...}`, `Param`, `Import`, `File{imports, ...}`,
  `Decl`, `AliasDecl`, `ClassDecl`, `MethodSig`, `InstanceDecl`, `Constraint`,
  `InstMethod`, `TypeDecl`, `CtorDecl`, `FuncDecl`, `ExternRef`, `Block`,
  `Stmt`, `LetTarget`, `Expr`, `Arm`, `DoStmt`, `Pattern`, `exprPos`, `patPos`, `stypePos`.
- `ty.gt`: `Kind`, `Type` (TCon/TVar/TMeta/TApp/TFunc, flat `tApp`), builtin
  constructors (`tInt`…`tTuple`, `isTupleCon`), `Scheme{tparams, constraints, typ}`,
  `ClassInfo`, `MethodInfo`, `DataInfo`, `CtorInfo`, `InstanceInfo`, `FuncInfo`, `Env`,
  `emptyEnv`, `instanceKey`, `DictRef` (DInstance/DParam/DSuper).
- `core.gt`: `Core`, `CPat`, `CArm`, `primNames`, `CoreCtor`, `CoreData`, `CoreClass`,
  `CoreMethodSig`, `CoreInstance`, `CoreMethod`, `CoreFunc` (CoreFunc/CoreExtern),
  `Program`.
- `check.gt`: `Check[A]` monad over `CheckState{nextMeta, subst, warnings, env}`:
  `runCheck`, `initialState`, `checkFail`, `checkWarn`, `getState/putState/modifyState`,
  `askEnv/setEnv/modifyEnv`, `liftResult`, `freshMeta`, `freshName`, `bindMeta`,
  `lookupMeta`, `mapMCheck`, `forEachCheck`, `foldCheck`, `whenCheck`.

The rebase migrates these contracts and all slice drivers together. `Import` remains
file-scoped; module qualification precedes checking and does not mutate source files.

## Function-level interfaces between slices

```
// types.gt (W3)
zonk(t Type) Check[Type]                                   // apply subst deeply
unify(pos Pos, expected Type, actual Type) Check[Unit]     // "type mismatch: expected T, got U"
typeString(t Type) String                                  // same rendering as Go types.TypeString
kindOf(t Type) Kind
freeMetas(t Type) Check[List[Int]]                         // of the zonked type, ascending, unique
instantiate(sch Scheme) Check[(Type, List[(String, Type)])] // fresh metas; constraints as (class, meta)
generalize(fixed List[Int], constraints List[(String, Type)], t Type) Check[(Scheme, List[(String, Type)])]
     // metas not in `fixed` become tparams (names "t0","t1",… in order of appearance);
     // constraints on them become scheme constraints; the rest are returned unchanged
substTVars(mapping TreeMap[String, Type], t Type) Type          // rigid-var substitution (no Check)
resolveSType(scope TreeMap[String, Type], t SType) Check[Type]  // names→kinds via env.typeKinds; arity/kind errors
defaultMetas(metas List[Type], origins TreeMap[Int, (Pos, String)], constraints List[(String, Type)]) Check[Unit]
     // §5: `metas` are the (zonked, still unresolved) TMeta nodes of the body; `constraints` the
     // unresolved (class, type) obligations. Each meta: arrow kind -> error; appears in a constraint
     // -> error "unresolved class constraint C[?n]" (defaults.go/expr.go wording); else bind to Unit and
     // warn at origins[id] = (pos, what) with "type of <what> defaulted to Unit; add an annotation or
     // use the value"; warnings in pos order. Infer computes `origins` (priorities of
     // defaults.go prepareDefaults) and passes its leftover constraints.

// decls.gt (W4)
checkProgram(files List[File]) CompileResult[Program]
buildEnv(files List[File]) Check[Unit]
resolveDict(given List[(String, String)], className String, t Type) Check[DictRef]
     // given = (class, rigid tvar name) constraints in scope; t may be unzonked
superclassChain(className String) Check[List[String]]       // [className, super, super's super …]

// infer.gt (W5)
inferFuncBody(info FuncInfo) Check[Core]
inferInstanceMethod(inst InstanceInfo, method MethodInfo, im InstMethod) Check[Core]

// cg_expr.gt (W7)
type Gen[A] = CGGen(state.State[Int, A])                                 // fresh-name counter
type GenCtx = GenCtx{instances: TreeMap[String, CoreInstance], classes: TreeMap[String, CoreClass],
                     datas: TreeMap[String, CoreData], externs: TreeMap[String, String],
                     receiver: Option[String], receiverDicts: List[(String, String)]}
     // externs: func name -> "ext_N.GoName"; receiver = Some("dictself") inside instance methods
genExpr(ctx GenCtx, e Core) Gen[String]
genDict(ctx GenCtx, d DictRef) Gen[String]
genArms(ctx GenCtx, scrutinee String, arms List[CArm]) Gen[String]
collectTupleArities(e Core) List[Int]
usesOrdering(e Core) Bool
mangle(name String) String          // name + "_"
exported(name String) String        // first rune upper-cased
ctorGoName(data String, ctor String) String   // data + "_" + ctor
instanceGoName(className String, typeCon String) String  // class + "_" + typeCon

// cg_decls.gt (W6)
generate(prog Program, goPackage String) CompileResult[String]
```

## Go output conventions

- Header `// Code generated by go4t. DO NOT EDIT.`, `package P`, imports `"go4t/rt"` and
  extern aliases `ext_1`, `ext_2`, … in order of first appearance over funcs sorted by name.
- Data type `T` → `type T interface { isT() }`; constructor `C` → `type T_C struct { F0 any; … }`
  with `func (*T_C) isT() {}`. `rt.Shapes` init when both `List` and `Option` exist.
- Class → Go interface (embedding the superclass); methods receive dictionaries
  named `dict_<parameter>_<class>`, so independent constraints cannot collide.
- Instance → struct `Class_TypeCon` embedding its superclass and context dictionaries;
  methods `func (dictself Class_TypeCon) Exported(dict…, p_ any, …) any`.
- Function `f` → `func f_(dict_P_Class Class, …, x_ any, …) any`; erased runtime externs
  delegate to their imported Go function.
- `func main() { rt.RunMain(main_()) }`; `TupleN` structs for every used arity; `ord_lt/le/gt/ge`
  helpers when any ordering comparison is generated.
- Expressions: locals mangled; constructors `&T_C{F0: …}`; match → `func() any { … }()` with
  type switches / field tests; `+` on Int `rt.AddInt`, String `rt.AddString`; etc. Read
  `codegen/expr.go` for every case.

Byte-identical output with the Go compiler is a stretch goal, not a requirement.
Required: generated Go builds and every in-scope example passes the comparison
contract. Deterministic outputs compare byte-for-byte; explicitly listed
nondeterministic examples compare successful exit status and line count.

## Verification

- `bash selfhost/bootstrap.sh` builds stage1 through `build --emit-go`, exercises
  all in-scope examples, and compares gofmt-normalised stage2/stage3.
- Directory selection matches CI: use directory form when exactly one immediate
  `.gt` file defines `func main`; otherwise use `main.gt` plus `gostd.gt` when present.
  `args.txt` and `input.txt` feed every execution. Input redirection occurs inside
  the serialization runner so it cannot consume the program's stdin.
- Each reference executes once. Unlisted examples require exact stage1 stdout.
  `NONDETERMINISTIC` explicitly lists `concurrency` (elapsed milliseconds),
  `sales-report` (random EUR fallback and elapsed milliseconds), and `sales-service`
  (`rates Nms`; currently network-skipped), with a reason beside each entry.
  Listed examples require successful exits and equal line counts and print
  `ok (nondeterministic: exit+line count)`. `http-service` has no timing output
  and is not in that list. There is no sampling-based classification or retry.
- `python3 selfhost/dev/run.py <slice>` builds the existing focused drivers in a
  temporary single-directory module and removes its fixtures afterward.
- `python3 selfhost/dev/lexer_compare.py` compares lexer output against the Go lexer.
- `python3 selfhost/dev/regression.py` checks import rejection, alias cycles, duplicate
  constraints, malformed exponents, invalid placeholders, interpolation checking, and
  a compiled program combining aliases, implicit IO and recursive dictionaries.

## Implementation choices and boundaries

- Internal compiler data use `CompileResult` and `TreeMap`, not builtin/standard names.
- AST constructors retain family prefixes (`E*`, `P*`, `ST*`, `T*`, `C*`, `CP*`, `Tk*`).
- Library references use file-local explicit imports. Module names are qualified before
  checking; lexical bindings and entry declarations take precedence over implicit names.
- Counters and substitutions remain in `CheckState` / `Gen`; `Gen` deliberately wraps
  `state.State[Int, A]` to keep its distinct Monad instance.
- Laws and proofs are parsed and omitted from the executable program.
- `import "go:..."` is rejected with `selfhost does not bind Go packages`. The driver
  does not implement the Go package binder or typed foreign adapters.
- Source resolution covers nearest local `go4t.mod` roots and checkout `std/`.
  Go module `require` resolution, `GO4T_PATH`, embedded-stdlib fallback and complete
  filesystem identity canonicalisation are not implemented by this driver.
- Aliases expand transparently; source-written alias presentation provenance is not
  retained in selfhost diagnostics.
- The lexer treats braces as layout resets, like the Go lexer; record-brace newline
  tolerance remains the parser's responsibility.

## Status

`bash selfhost/bootstrap.sh` exits **0** and prints
`FIXED POINT: stage2 == stage3`. Explicit nondeterministic classification avoids
the coincident-timing failures observed with the discarded sampling-based policies.

- **S1 passes:** sixteen go4t compiler sources compile through the current `cmd/go4t`
  and build a stage1 executable. The keyword, deriving and std-import drift is removed.
- **S2 passes:** all 36 in-scope examples compile, build and execute successfully.
  Thirty-four outputs match byte-for-byte. `concurrency` and `sales-report` pass
  the explicitly authorised exit+line-count comparison.
- **S3 passes:** `FIXED POINT: stage2 == stage3`.

| Example | S2 |
|---|---|
| alias-fields | ok |
| bank | ok |
| calc | ok |
| cards | ok |
| cli-tools | ok |
| codegen-quality | ok |
| collections | ok |
| concurrency | ok (nondeterministic: exit+line count) |
| deriving | ok |
| diagnostics | ok |
| eqord | ok |
| float-num | ok |
| inventory | ok |
| io-effects | ok |
| json-deriving | ok |
| lists | ok |
| local-recursion | ok |
| lsp | ok |
| modules | ok |
| monoid | ok |
| multi-constraint | ok |
| orders | ok |
| parser | ok |
| perf | ok |
| pipe-partial | ok |
| records | ok |
| ref | ok |
| routes | ok |
| sales-report | ok (nondeterministic: exit+line count) |
| scheduler | ok |
| state | ok |
| stdlib-extras | ok |
| strings | ok |
| todo | ok |
| tree | ok |
| wc | ok |

Nine explicit skips remain: `deps`, `go-chan`, `go-import`, `http`, `http-service`,
`node-drain-typed`, `sales-service`, `sql-service`, `typed-extern`. `bootstrap.sh`
prints a reason for each, including the nested `deps/app/main.gt` entry.

Focused evidence: 39 type cases; 35 inference cases; parser files, 21 expression
shapes, 5 structural checks and 17 negative diagnostics; declaration and codegen
drivers; 62-file lexer corpus, 809 layout/interpolation/error/stress cases,
3 base-position checks and 7 compiler-input lexer comparisons.

Differences from the Go compiler that are accepted: instance emission order (Core sorts by
`instanceKey`), extern parameter names (`a0…`), fresh-name scheme (`_go4tN`), stderr
warning paths. Generated Go is therefore not byte-identical to `cmd/go4t build`.

## Rebase list

| Surface | Landed implementation |
|---|---|
| keyword/import drift | explicit std imports; reserved identifiers renamed; internal Result/Map renamed |
| deriving | Debug, enum/newtype Show, Functor/Foldable/Traversable, JSON Encode/Decode |
| multi-constraint | ordered `A Show + Ord`; class-qualified dictionary fields and parameters |
| pipe-partial | pipelines, placeholder calls, operator sections and projection sections |
| IO effects | std Result/IoError, cancellation-aware shared runtime, implicit `ioPure` at expected IO tails |
| Float | literals/exponents, Float patterns, arithmetic and runtime operations |
| collections | opaque Map/Set/Array/Fiber/Ref types; Option/Result/tuple/Ordering/JSON/IoError shapes |
| modules | lexical directory input, std checkout loading, local manifests and file-scoped aliases |
| aliases and records | transparent aliases, alias-cycle validation, common-field access/update, positional record construction |
| patterns | list patterns and Boolean exhaustiveness inside tuples |
| recursion | consecutive local function groups and polymorphic recursive lambda lets |
| laws/proofs | syntax consumed without generated runtime declarations |
| bootstrap | explicit skips, CI entrypoint selection, argument/stdin fixtures, explicit nondeterministic classification, fixed point |

The historical baseline `6b189c0` passed seventeen examples. It is no longer needed
to build or bootstrap selfhost; `GO4T_PRELUDE` has no role.
