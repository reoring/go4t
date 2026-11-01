# Code statistics: client-go v0.37.2 versus the native gofx port

Measured from committed blobs, not working trees. Raw numbers and the
classification rules are in `code-stats.json`.

- Before: `kubernetes/client-go` v0.37.2, revision
  `72c52dd0cc3b9099b36af11290b0304045dd1ada`, all 2,453 `.go` files. Every blob's
  SHA-256 and physical line count matched `upstream-inventory.json`.
- After: this port at gofx commit `d1fc0b5` on `experiment/client-go-gofx`.
- Lines are physical lines, including comments and blank lines. `nonblank` and
  `bytes` are also recorded in the JSON.
- Tests are files named `*_test.go` / `*_test.gfx`. Native verification programs
  such as `regression/main.gfx`, `loopback/*.gfx` and `consumer/main.gfx` stay in
  the non-test rows.
- Generated means a `Generated`/`Code generated` marker in the leading comment
  block; upstream uses the pinned inventory category.
- Excluded on both sides: documentation, JSON inventories, fixtures, `go.mod`,
  `go.sum`, emitted Go under `.gofx-source-*`/`*_gen`, build caches.
- Changes to the gofx compiler itself (`cmd/gofx`, `codegen`) are not counted.

## Native port only

Excludes external dependency bindings, generation tools and the original-Go
oracle. The upstream type schema embedded in
`applyconfigurations/internal/internal.go` lives in the port as
`applyconfigurations/host/schema.yaml` (17,663 lines, 430,734 bytes, byte-equal
to the embedded original); it is counted on both sides so relocation does not
appear as a reduction.

| Metric | Before | After | Change |
|---|---:|---:|---:|
| Files | 2,453 | 1,218 | −50.4% |
| Non-generated, non-test lines | 56,300 | 21,253 | −62.3% |
| Generated lines + type schema | 221,864 | 88,452 | −60.1% |
| `*_test.*` lines | 65,981 | 18,651 | −71.7% |
| Total lines | 344,145 | 128,356 | −62.7% |
| Source bytes | 14,220,375 | 11,848,217 | −16.7% |

After, by language:

| Port | Files | Lines |
|---|---:|---:|
| gofx `.gfx` | 1,119 | 97,784 |
| Go host code and Go tests `.go` | 98 | 12,909 |
| Relocated type schema `.yaml` | 1 | 17,663 |

Go host code covers generic I/O, synchronization, networking, cryptography,
external type conversion, generated type registration and verification helpers.
Client-go policy lives in `.gfx`.

Test file count went from 157 to 310 while test lines fell; this is a file-size
difference, not a test-count or coverage comparison.

## Interoperability and generation overhead

| Part | Files | Lines | Bytes |
|---|---:|---:|---:|
| External Go dependency bindings, gofx side (`deps/**`) | 233 | 44,114 | 7,895,950 |
| External Go dependency bindings, Go side (`host/deps/**`) | 10 | 43,825 | 5,661,666 |
| Generators and inventory tools (`tools/binddeps`, `tools/inventory`, `tools/portgen`, root `tools/portgen`) | 26 | 3,171 | 225,999 |
| Original-Go oracle (`oracle/**`) | 3 | 833 | 42,958 |

Port plus bindings: 1,461 files, 216,295 lines, 25,405,833 bytes (−37.2% lines,
+78.7% bytes versus upstream). All migration source including tools and oracle:
1,490 files, 220,299 lines, 25,674,790 bytes.

## Reading the numbers

- The port body is smaller in lines and bytes.
- Bytes fell much less than lines: average non-blank line length rose from about
  47 bytes to about 128 bytes, because generated gofx files aggregate per group
  and some expressions are written on one line. The line reduction does not
  measure complexity or maintenance effort.
- The interoperability layer is large. With bindings, total bytes exceed
  upstream client-go alone.
