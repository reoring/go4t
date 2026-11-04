# Todo: CLI, HTTP and file persistence

An ordinary application written in go4t, with a pure core and an IO entrypoint:

- `todo.gt`: command parsing, todo records, state transitions, rendering and
  three proved laws.
- `main.gt`: argv via `std/io`, the `TODO_FILE` environment variable, JSON file
  persistence, timestamps and `std/http` routes.
- Derived `json.Encode` / `json.Decode` instances encode `Todo` and `Store`;
  the HTTP request and error records derive the codecs they need.

Commands run from the repository root with `go4t` on `PATH`. Alternatively,
replace `go4t` with `go run ./cmd/go4t`. Use the **directory** as the input so
both `main.gt` and `todo.gt` are loaded, as in the modules and sales-report
examples; an explicit `main.gt` input does not load sibling source files.

## CLI

The store is `$TODO_FILE`, or `./todo.json` relative to the process's working
directory when the variable is unset. A missing file starts an empty store;
mutations save the whole store as pretty-printed JSON. To try it with a fresh
file:

```sh
export TODO_FILE="$(mktemp -d)/todo.json"
go4t check examples/todo
go4t run examples/todo -- list
go4t run examples/todo -- add buy milk
go4t run examples/todo -- list
go4t run examples/todo -- done 1
go4t run examples/todo -- list --all
go4t run examples/todo -- rm 1
```

The first three runs print, respectively:

```text
(nothing to do)
added 1
[ ]   1  buy milk
```

`list` shows open todos; `list --all` also shows completed todos with `[x]`.
`help` prints the command list. Invalid commands exit 2; completing or removing
an unknown ID exits 1. `args.txt` supplies `list` for CI, which prints
`(nothing to do)` without creating a store when no file exists.

## HTTP server

The server uses the same store and transitions as the CLI. In one terminal:

```sh
export TODO_FILE="$(mktemp -d)/todo.json"
go4t run examples/todo -- serve 8080
```

In another terminal:

```sh
curl -sS http://127.0.0.1:8080/todos
curl -sS -X POST http://127.0.0.1:8080/todos \
  -H 'Content-Type: application/json' -d '{"text":"buy milk"}'
curl -sS -X POST http://127.0.0.1:8080/todos/1/done
curl -sS -X DELETE http://127.0.0.1:8080/todos/1
```

| Route | Result |
| --- | --- |
| `GET /todos` | 200, all todos, including completed ones. |
| `POST /todos` with `{"text":"..."}` | 201, the new todo. |
| `POST /todos/<id>/done` | 200, the updated list. |
| `DELETE /todos/<id>` | 200, the updated list. |

Invalid JSON or non-numeric IDs return 400; missing IDs and unknown routes
return 404, with an `{"error":"..."}` body. Stop the server with Ctrl-C.
This is a single-writer file-store example, not a concurrent database: writes
have no locking or atomic replacement, and the HTTP routes have no authentication.

## Checked laws

`go4t check examples/todo` verifies all three laws; `run` and `build` do not
verify proofs.

| Law | States |
| --- | --- |
| `markDoneIdempotent` | Marking the same ID done twice is the same as once. |
| `removeForgets` | Looking up an ID after removing it returns `None`. |
| `markDoneKeepsLength` | Marking done preserves the number of todos. |

The last law uses the directly recursive `count`, not `std/list`'s `length`.
`count(Cons(x, rest))` reduces to `1 + count(rest)`, matching the induction
hypothesis on `rest`. `list.length` uses an accumulator fold: its recursive
step changes the accumulator, so a hypothesis about the initial accumulator
alone does not apply. It would need a stronger accumulator lemma instead
([SPEC: laws and proofs](../../SPEC.md#65-laws-and-proofs)).

## Build a binary

```sh
go4t build examples/todo && go build -o todo ./examples/todo/_gen
./todo list
./todo add buy milk
./todo serve 8080
```

`go4t build` emits Go sources in `examples/todo/_gen`; `go build` makes the
executable. The binary uses the same `TODO_FILE` environment variable and CLI
arguments, without the `go4t run` separator.
