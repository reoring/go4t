# Security policy

## Supported versions

go4t is pre-1.0. Only the latest tagged release and `main` receive fixes.

## Reporting a vulnerability

Report privately through GitHub's
[private vulnerability reporting](https://github.com/reoring/go4t/security/advisories/new)
for this repository. Do not open a public issue for a suspected vulnerability.
Include the go4t version or commit, `go version`, a minimal reproducer, and
what an attacker gains.

## What is in scope

- **The compiler executes the Go toolchain.** `go4t build`, `run`, `test`,
  `bind` and `get` invoke `go` on generated sources and on the Go packages a
  program imports through `import "go:…"`. Building an untrusted go4t program
  therefore has the same exposure as running `go build` on untrusted Go code.
  A way to make go4t run something *other* than that toolchain, or write
  outside its documented output and cache directories, is a vulnerability.
- **Generated code.** go4t source that compiles to Go with behaviour the
  source cannot express — memory unsafety, an effect performed by a function
  whose type is pure without a trusted `extern` — is a vulnerability.
- **The runtime (`rt/`) and standard library (`std/`)**, including the HTTP
  server and client and JSON decoding.

## What is a bug, not a vulnerability

- **Law soundness.** A false law that `go4t check` accepts is a serious
  correctness bug and is fixed with priority, but laws are a development-time
  check: they generate no runtime code and protect no trust boundary. Report it
  as an ordinary issue.
- **Trusted externs.** An `extern` declared with a pure type whose Go
  implementation performs effects breaks purity by construction; the checker
  cannot inspect foreign code (SPEC §6.1, §9).
- **Resource exhaustion in your own program**, including stack exhaustion
  from deep non-tail recursion (SPEC §9).
