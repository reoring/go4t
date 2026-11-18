---
name: go4t-dev
description: Write, check, and embed go4t (`.gt`) code — the language at github.com/reoring/go4t that compiles to Go and machine-checks `law`s. Use when editing or creating `.gt` files, writing a pure `decide` for a Go program or Kubernetes controller, stating or fixing laws/proofs, reading `go4t check` diagnostics, or generating Go with `--emit-go`. Not for changing the go4t compiler itself (follow CONTRIBUTING.md).
---

# go4t development

go4t compiles to Go. Its value is in three compile-time guarantees for a pure
function: every `match` is exhaustive, a function without `IO` in its type
cannot perform I/O, and a `law` is verified for **all** inputs, not for chosen
test cases.

Source of truth: the go4t repository this file ships in. Read before guessing
syntax: `docs/tutorial.md`, `docs/laws.md`, `docs/go-interop.md`,
`docs/cli.md`, `SPEC.md` (§8.3 bindings, §8.7 exported Go API), and
`docs/stdlib.md` for library names. Every example under `examples/` compiles.
Where this file and `SPEC.md` differ, `SPEC.md` wins.

## Default shape: decide in go4t, read and write in Go

Use this unless the user asks otherwise.

| Part | Language | Why |
| --- | --- | --- |
| `decide(observed) action` and its laws | go4t | exhaustiveness, purity and laws all apply here |
| observe (read APIs), apply (write), watches, client setup | Go | no law reaches I/O; unstructured Kubernetes objects in go4t become nested `dyn.Map(map.mapFromList([...]))`, which is harder to read than Go |
| simulation or envtest of the loop | Go tests | checks what laws cannot see: what the world actually does |

Templates: `examples/go-decision-template` (smallest),
`examples/node-drain-controller`, `examples/attachment-repair-controller`.
Writing observe/apply in go4t is supported (`examples/approval-controller`,
`examples/node-drain-shell`); choose it only when asked. `docs/go-interop.md`
compares the three shapes.

## Workflow

1. **Model the observation as sums, not a value plus a flag.** One constructor
   per situation the decision distinguishes. Use named payloads
   (`Held{owner: String}`), which become named Go fields; positional payloads
   become `F0`, `F1`.
2. **Return exactly one action per step.** A level-triggered controller keeps no
   state: it resumes from the observation alone. Make `Wait` name what it waits
   for (`Wait{on: Blocker}`).
3. **Write `decide` as one `match`** with the terminal and irreversible states
   first.
4. **State the laws** (see below), run `go4t check`, add proofs where `refl` is
   not enough.
5. **Emit Go and call it** from the shell; write the Go tests.
6. **Mutate.** Break `decide` in a temp copy, one edit per invariant, and confirm
   a law rejects each. A mutation nothing rejects is a missing law.

## Commands

In a Go module that requires go4t, run the compiler with
`go run github.com/reoring/go4t/cmd/go4t`; in a checkout of this repository,
`go build ./cmd/go4t`. Below, `go4t` stands for either.

```sh
go4t check DIR                                   # types, exhaustiveness, laws
go4t fmt -w DIR                                  # canonical format
go4t build --emit-go _gen --package NAME DIR     # Go sources for a library
go4t eval 'debug(decide("a", Observed(true, NoLease, 0)))' DIR   # prints Acquire; needs deriving Debug
```

- **Only `check` verifies laws.** `build`, `run`, `test` and `eval` compile a
  file whose laws fail and exit 0. Always run `check` before `build`, and gate
  CI on both: `go4t check DIR` then `go generate ./... && git diff --exit-code`.
- A library package needs no `main`. `run`, `build -o` and `--emit-go` with the
  default package `main` do.
- A directory is one package; pass the directory, not one file, when it has
  several `.gt` files.
- In a Go module, pin the compiler through `go.mod` and generate with
  `//go:generate go run github.com/reoring/go4t/cmd/go4t build --emit-go _gen --package NAME DIR`.
- `go:` imports need the build module's `go.mod` to require the Go package.
- Emission is pruned to what the program reaches and is deterministic.

## A complete decision file

This file passes `go4t check` and is in `go4t fmt` form.

```go4t
package gate

// What one reconcile saw. Sums, not a value plus a flag.
type Lease = NoLease | Held{owner: String, expiresAt: Int} deriving Eq, Debug

type Observed = Observed{wanted: Bool, lease: Lease, now: Int} deriving Eq, Debug

type Blocker = OtherOwner deriving Eq, Debug

type Action = Acquire | Renew | Release | Wait{on: Blocker} | Done deriving Eq, Debug

func decide(self String, o Observed) Action {
  match o.lease {
    | NoLease => if o.wanted {
      Acquire
    } else {
      Done
    }
    | Held(owner, expiresAt) if owner != self => if o.wanted && o.now >= expiresAt {
      Acquire
    } else if o.wanted {
      Wait(OtherOwner)
    } else {
      Done
    }
    | Held(_, _) => if o.wanted {
      Renew
    } else {
      Release
    }
  }
}

func _takes(a Action) Bool {
  match a {
    Acquire => true
    Renew | Release | Wait(_) | Done => false
  }
}

// Never take a lease another owner still holds.
law neverStealsLiveLease(self String, other String, exp Int, o Observed)
  requires other != self
  requires o.now < exp
{
  _takes(decide(self, o{lease: Held(other, exp)})) == false
} proof {
  if o.wanted {
    refl
  } else {
    refl
  }
}

// Not wanted: never acquire or renew, whatever else is observed.
law unwantedNeverHolds(self String, o Observed) {
  match decide(self, o{wanted: false}) {
    Acquire | Renew => false
    _ => true
  }
} proof {
  match o.lease {
    NoLease => refl
    | Held(owner, expiresAt) => if owner != self {
      refl
    } else {
      refl
    }
  }
}
```

Syntax shown and verified: sum and record types with `deriving`; a named-field
constructor is built and matched positionally (`Held(owner, expiresAt)`,
`Wait(OtherOwner)`) and read with `.field`; match guards (`if`), alternatives
(`A | B =>`) and `_`; record update `o{field: v}`; a `_`-prefixed function is
private (not exported to Go); a law body that is a Bool expression states that
it is `true`; `requires` clauses, one per line; inline `} proof { … }`.
Time, randomness and API results arrive as values (`now: Int`); `decide` never
reads a clock.

## Writing laws

- **Quantify over the whole observation and pin only the relevant field:**
  `decide(o{record: Decided(d)})` says "whatever else is observed".
- **Write one general contract law plus focused laws.** The general law says
  every chosen action is permitted in that observation (a private
  `_permitted(o, a) Bool`); focused laws pin one invariant each ("decided never
  aborts", "a lagging stream blocks the decision"). When a refactor breaks a
  proof, the focused laws tell a real violation from a proof that only needs
  repair.
- **Irreversibility, ordering, idempotence, progress** are the properties worth
  a law in a controller: once committed never roll back; effect B only after
  acknowledgement A; never repeat an effect already observed; `Wait` only when
  a named external prerequisite is unmet.
- A law is not a test of I/O. Whether an abort really released a hold, or a
  router really loaded a revision, belongs to the Go tests.
- Keep law quantities in `Int`. Float expressions are excluded from proofs.
- `Int` wraps: `a + 1 > a` is false at the maximum. State the range with
  `requires` instead of hiding it.

## Reading `go4t check` failures

| Output | Meaning | Action |
| --- | --- | --- |
| `non-exhaustive match; missing: (…)` | a case is unhandled | add the case; do not add `_ =>` to silence it in `decide` |
| `stuck normal forms differ` with `counterexample: x = …` | the law is false for that input (evaluated, real) | fix `decide` or the law |
| `… no counterexample in 128 candidates: the law may hold; this proof needs a case split, rewrite or induction` | **neither proved nor refuted** | read `stuck on:`; split on the condition or constructor that blocks reduction |
| `no proof was written for this law; it was checked by refl` | the automatic attempt failed | add `} proof { … }` |
| `requires can never hold` / `no arguments satisfying requires were found` | the precondition is unsatisfiable or untested | fix `requires`; the law may say nothing |
| `law body must be an equality` / `expected expression, found "if"` | unparenthesized `if` beside `==` | write `(if c { a } else { b }) == x` |

Proof tactics: `refl`, `match e { A => refl | B(x) => … }` (constructor split),
`if cond { … } else { … }` (guard split), `absurd` for a case the assumptions
rule out, `rewrite`, `induction` (see `docs/laws.md`, tutorial §13). Split on
what `stuck on:` shows first: usually the same field or condition `decide`
branches on, in the same order. Proofs follow the branch structure of the code,
so restructuring `decide` can require rewriting a proof whose law still holds.
Budget for it: the contract and its proofs usually take longer than `decide`.

Never weaken or delete a law to get `check` green. If it cannot be proved,
report the exact output.

## The Go side

Mapping (SPEC §8.7; confirm in the emitted `go4t_export.go`, whose comments
show how to construct each type):

| go4t | Go |
| --- | --- |
| exported `func decide` | `Decide`; a function colliding with a type name gets `Fn` |
| `_helper` | not exported |
| sum type `Action` | interface `Action` + one struct per constructor: `ActionWait{On: …}` |
| sum with only nullary constructors | constants: `BlockerOtherOwner` |
| named payload / positional payload | named field / `F0`, `F1` |
| `Int`, `String`, `Bool` | `int64`, `string`, `bool` |
| `IO[T]` | `func(context.Context) (T, error)` |

```go
switch a := gate.Decide(self, observed).(type) {
case gate.ActionAcquire:
    // perform the effect in Go
case gate.ActionWait:
    _ = a.On
}
```

Keep the projection from Go's nil/zero conventions into the sum in one Go
function, and test it in Go. Never pass a sentinel (`-1`, `""`) across the
boundary; use `Option` or a constructor.

Go tests to pair with the laws: the happy-path action trace; crash at every
step and resume from observation; a world-level safety oracle asserted at
every step; a fixed-seed randomized schedule. The laws prove the decision; the
oracle checks the system property (for example "one writer") that the decision
alone does not establish.

## Report

State separately: `go4t check` result (laws proved, hand-written proofs),
emitted package builds, Go tests, mutation results, and what the model does not
cover. A passing `check` proves the decision, not the system.
