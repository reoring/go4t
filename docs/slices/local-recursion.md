# Local recursion — SPEC delta

## Grammar

Add to SPEC §4 and §4.10:

```text
LocalFunc := 'func' Ident '(' [Param {',' Param}] ')' Type Block
Param     := Ident Type
Stmt      := 'let' (Ident | Pattern) [':' Type] '=' Expr | LocalFunc | Expr
DoStmt    := (Ident | Pattern) '<-' Expr
           | 'let' (Ident | Pattern) [':' Type] '=' Expr | LocalFunc | Expr
```

Local function declarations require parameter types and a return type; they have no type parameters or extern form. Enclosing function type parameters may appear in their signatures. `func(...) => ...` remains a lambda expression, including in final-expression position. A local declaration is a statement, not the required final block/do expression. Newlines use the existing layout rule.

## Bindings and typing

Remove §4.2's statement **“Bindings are not recursive”**. Replace it with:

- A bare-name lambda binding (`let f = func(...) => ...`, optionally annotated) binds `f` monomorphically within the lambda before inferring its body. A free occurrence of `f` refers to this new binding, not an outer `f`; lambda parameters, patterns, and nested bindings can shadow it.
- After checking its recursive body, the binding follows the existing let-generalization/value-restriction rules. Unannotated recursive lambdas can therefore be instantiated at several types after their binding, including with generalized class constraints. Recursive calls within the definition must agree on one monotype; polymorphic recursion is not inferred.
- Non-lambda and pattern bindings remain non-recursive. `let x = x + 1` uses an enclosing `x`, or reports `unknown identifier x` when absent. Annotated and pattern lets remain monomorphic.
- A maximal run of consecutive local `func` statements forms one mutually recursive group. All group signatures are available monomorphically while every body is checked, then generalization is performed together against the pre-group environment before publishing the names. Required signatures currently contain only concrete types or enclosing rigid type parameters, so they introduce no fresh generalizable type variables.
- A group sees the enclosing scope and earlier statements, but not later statements or another group separated by a let, expression, or do bind. Every body must match its declared result type. Duplicate names within a group are rejected. Groups can shadow enclosing values; subsequent statements can shadow group names without changing already-created closures.
- In `do`, local declarations and recursive lets are ordinary pure bindings, not monadic steps. They can capture earlier `<-` results. The final expression must still have the do block's monadic type.

## Code generation contract

`ast.FuncDecl` implements both `ast.Stmt` and `ast.DoStmt`; no new declaration representation is needed. `types.Ann.Recursive` marks let-bound lambdas with a free self reference. Self references remain `RefLocal` with no dictionaries, while generalized uses after the binding instantiate dictionaries as before.

Recursive lets emit a typed Go function variable declaration before its closure assignment. A constrained recursive let's existing dictionary factory encloses this monomorphic self binding, so recursive calls capture the factory's dictionaries without rebuilding the function. Local groups declare every function variable before any closure assignment. Lexical scopes preserve captures across shadowing, and `_ = name_` keeps unused helpers valid Go. Source-language function arity and the uniform `any` parameter/result ABI are preserved.

The codegen-quality slice can apply its `namedLambda` lowering at the two lambda-emission sites in `codegen/local_recursion.go`: recursive lets and the synthetic lambdas constructed from local declarations. This slice does not implement tail-call optimization.

## Diagnostics

Existing positioned diagnostics remain in force. New errors:

- `local functions cannot have type parameters`
- `duplicate local function <name>`

Missing parameter/return types use the existing parser's `expected identifier, found ...` diagnostic. Invalid local names, duplicate parameters, unknown later bindings, arity mismatches, and return-type mismatches use their existing checker diagnostics.

## Prelude signatures and removed limitations

No prelude or builtin signatures change. Blocks and do blocks now support recursive lambda lets and mutually recursive named local functions; the previous non-recursive-binding limitation is removed only for bare-name lambda lets.

## Deviations

None.

## Duplicated names

None.

## Evidence

`examples/local-recursion/main.gfx` exercises a recursive `go` loop, mutually recursive parity helpers, a recursive local do helper capturing a bound result, polymorphic use after generalization, and recursive generalized dictionary use. Its README records the command and observed output. Dedicated `local_recursion_test.go` files cover parser syntax/errors, typing/scoping/generalization, and generated Go compilation via the existing codegen `assertSource` harness.

Observed verification (run through the VM serialization wrapper):

- `go test ./parser ./types ./codegen` — all three packages passed.
- `go run ./cmd/gofx run examples/local-recursion/main.gfx` — exited successfully with the README's five output lines.
- `go run ./cmd/gofx check examples/wc/main.gfx` — exited successfully without diagnostics.

No repository-wide tests, separate vet command, commits, or pushes were run. The generated example `main_gen` directory was removed after the execution receipt was collected.
