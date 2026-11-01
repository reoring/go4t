# selfhost — gofx compiler written in gofx

Goal: `selfhost/*.gfx` is a gofx→Go transpiler that compiles itself. Fixed point:
stage1 (built by the Go compiler) compiles `selfhost/*.gfx` → stage2 Go source; stage2
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
| lex | `lexer.gfx` | `lex(src String, base Pos) Result[List[Token]]` | layout `;` insertion (§1.6), interpolation parts |
| parse | `parser.gfx` | `parseFile(name String, src String) Result[File]` | `Parser[A]` monad over `List[Token]`; interpolation parts are lexed+parsed in place |
| types | `types.gfx` | `unify`, `zonk`, `instantiate`, `generalize`, `resolveSType`, `defaultMetas`, `typeString` | substitution lives in `CheckState.subst`; `typeString` renders byte-for-byte like Go `TypeString` |
| decls | `decls.gfx` | `checkProgram(files List[File]) Result[Program]`, `buildEnv`, `resolveDict`, deriving | builds `Env`, assembles `Program` |
| infer | `infer.gfx` | `inferFuncBody(info FuncInfo) Check[Core]`, `inferInstanceMethod(inst InstanceInfo, method MethodInfo, im InstMethod) Check[Core]` | expressions, patterns, coverage, do, let-generalisation → Core |
| codegen | `cg_decls.gfx` + `cg_expr.gfx` | `generate(prog Program, goPackage String) Result[String]` ; `genExpr(ctx GenCtx, e Core) Gen[String]` | prints Core as Go using `gofx/rt` |
| driver | `main.gfx` | `main` | `prog FILE.gfx... [-o DIR]`; reads prelude from `GOFX_PRELUDE` or `prelude/prelude.gfx` |

Immutability forces one deviation from the Go design: the checker does **not** annotate
the AST. It produces a separate dictionary-explicit IR (`core.gfx`). Codegen reads only
Core, never the AST or types. All desugarings listed at the top of `core.gfx` are the
checker's job.

## Contracts (supervisor-owned, frozen for workers)

- `base.gfx`: `Result[A] = Err(Diag) | Ok(A)` (Monad), `Diag{pos, msg}`, `Map[K, V]`
  (persistent AA tree: `mapEmpty/mapInsert/mapLookup/mapMember/mapToList/mapFromList/
  mapKeys/mapValues/mapUnion/mapMap/mapSize`), list helpers (`range`, `nth`, `indexOf`,
  `zipWithIndex`, `all`, `any`, `last`, `unzip`, `foldResult`, `duplicates`, `lookupAssoc`).
- `ast.gfx`: `Pos`, `Token{kind, text, pos, parts}`, `TokKind`, `InterpPart`, `SType`,
  `TypeParam`, `Param`, `File`, `Decl`, `ClassDecl`, `MethodSig`, `InstanceDecl`,
  `Constraint`, `InstMethod`, `TypeDecl`, `CtorDecl`, `FuncDecl`, `ExternRef`, `Block`,
  `Stmt`, `LetTarget`, `Expr`, `Arm`, `DoStmt`, `Pattern`, `exprPos`, `patPos`, `stypePos`.
- `ty.gfx`: `Kind`, `Type` (TCon/TVar/TMeta/TApp/TFunc, flat `tApp`), builtin
  constructors (`tInt`…`tTuple`, `isTupleCon`), `Scheme{tparams, constraints, typ}`,
  `ClassInfo`, `MethodInfo`, `DataInfo`, `CtorInfo`, `InstanceInfo`, `FuncInfo`, `Env`,
  `emptyEnv`, `instanceKey`, `DictRef` (DInstance/DParam/DSuper).
- `core.gfx`: `Core`, `CPat`, `CArm`, `primNames`, `CoreCtor`, `CoreData`, `CoreClass`,
  `CoreMethodSig`, `CoreInstance`, `CoreMethod`, `CoreFunc` (CoreFunc/CoreExtern),
  `Program`.
- `check.gfx`: `Check[A]` monad over `CheckState{nextMeta, subst, warnings, env}`:
  `runCheck`, `initialState`, `checkFail`, `checkWarn`, `getState/putState/modifyState`,
  `askEnv/setEnv/modifyEnv`, `liftResult`, `freshMeta`, `freshName`, `bindMeta`,
  `lookupMeta`, `mapMCheck`, `forEachCheck`, `foldCheck`, `whenCheck`.

Workers add functions; they do not change contract types. A worker who needs a contract
change messages the supervisor with the exact diff.

## Function-level interfaces between slices

```
// types.gfx (W3)
zonk(t Type) Check[Type]                                   // apply subst deeply
unify(pos Pos, expected Type, actual Type) Check[Unit]     // "type mismatch: expected T, got U"
typeString(t Type) String                                  // same rendering as Go types.TypeString
kindOf(t Type) Kind
freeMetas(t Type) Check[List[Int]]                         // of the zonked type, ascending, unique
instantiate(sch Scheme) Check[(Type, List[(String, Type)])] // fresh metas; constraints as (class, meta)
generalize(fixed List[Int], constraints List[(String, Type)], t Type) Check[(Scheme, List[(String, Type)])]
     // metas not in `fixed` become tparams (names "t0","t1",… in order of appearance);
     // constraints on them become scheme constraints; the rest are returned unchanged
substTVars(mapping Map[String, Type], t Type) Type          // rigid-var substitution (no Check)
resolveSType(scope Map[String, Type], t SType) Check[Type]  // names→kinds via env.typeKinds; arity/kind errors
defaultMetas(metas List[Type], origins Map[Int, (Pos, String)], constraints List[(String, Type)]) Check[Unit]
     // §5: `metas` are the (zonked, still unresolved) TMeta nodes of the body; `constraints` the
     // unresolved (class, type) obligations. Each meta: arrow kind -> error; appears in a constraint
     // -> error "unresolved class constraint C[?n]" (defaults.go/expr.go wording); else bind to Unit and
     // warn at origins[id] = (pos, what) with "type of <what> defaulted to Unit; add an annotation or
     // use the value"; warnings in pos order. Infer computes `origins` (priorities of
     // defaults.go prepareDefaults) and passes its leftover constraints.

// decls.gfx (W4)
checkProgram(files List[File]) Result[Program]
buildEnv(files List[File]) Check[Unit]
resolveDict(given List[(String, String)], className String, t Type) Check[DictRef]
     // given = (class, rigid tvar name) constraints in scope; t may be unzonked
superclassChain(className String) Check[List[String]]       // [className, super, super's super …]

// infer.gfx (W5)
inferFuncBody(info FuncInfo) Check[Core]
inferInstanceMethod(inst InstanceInfo, method MethodInfo, im InstMethod) Check[Core]

// cg_expr.gfx (W7)
type Gen[A] = State[Int, A]                                 // fresh-name counter
type GenCtx = GenCtx{instances: Map[String, CoreInstance], classes: Map[String, CoreClass],
                     datas: Map[String, CoreData], externs: Map[String, String],
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

// cg_decls.gfx (W6)
generate(prog Program, goPackage String) Result[String]
```

## Go output conventions (from codegen/*.go; keep identical)

- Header `// Code generated by gofx. DO NOT EDIT.`, `package P`, imports `"gofx/rt"` and
  extern aliases `ext_1`, `ext_2`, … in order of first appearance over funcs sorted by name.
- Data type `T` → `type T interface { isT() }`; constructor `C` → `type T_C struct { F0 any; … }`
  with `func (*T_C) isT() {}`. `rt.Shapes` init when both `List` and `Option` exist.
- Class → Go interface (embedding the superclass); methods `Exported(dict_X Class, a0 any, …) any`.
- Instance → struct `Class_TypeCon` embedding `Super_TypeCon` and `dict_P Class` fields;
  methods `func (dictself Class_TypeCon) Exported(dict…, p_ any, …) any`.
- Function `f` → `func f_(dict_P Class, …, x_ any, …) any`; extern body `return ext_N.Name(x_, …)`.
- `func main() { rt.RunMain(main_()) }`; `TupleN` structs for every used arity; `ord_lt/le/gt/ge`
  helpers when any ordering comparison is generated.
- Expressions: locals mangled; constructors `&T_C{F0: …}`; match → `func() any { … }()` with
  type switches / field tests; `+` on Int `rt.AddInt`, String `rt.AddString`; etc. Read
  `codegen/expr.go` for every case.

Byte-identical output with the Go compiler is a stretch goal, not a requirement.
Required: generated Go builds, and each `examples/*/main.gfx` prints the same stdout as
under the Go compiler.

## Verification

- Each slice: a throwaway `selfhost/dev/<slice>_main.gfx` driven by
  `go run ./cmd/gofx run selfhost/base.gfx selfhost/ast.gfx selfhost/ty.gfx selfhost/core.gfx selfhost/check.gfx selfhost/<slice>.gfx selfhost/dev/<slice>_main.gfx`.
- Integration: `selfhost/bootstrap.sh` builds stage1 with the Go compiler, compiles every
  example and the compiler itself, diffs stage2/stage3.

## Known gofx constraints that shape the port

- Keywords cannot be record field names (`class`, `extern`, `deriving` → `className`,
  `externRef`, `derivings`).
- Constructor names share one namespace across the whole program: prefix by family
  (`E*`, `P*`, `ST*`, `T*`, `C*`, `CP*`, `Tk*`, `K*`, `D*`).
- Prelude names cannot be shadowed (`getEnv`, `map`, `filter`, `get`, `put`, …).
- No mutation: counters and substitutions live in `CheckState` / `Gen` state.
- Single-file `package main` programs; the driver accepts several files
  (`gofx run a.gfx b.gfx`) and merges them.
