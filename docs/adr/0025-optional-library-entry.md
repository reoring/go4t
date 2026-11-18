# ADR 0025: Only executable output requires `main`

Status: Accepted.

The former entry-module rule required `func main() IO[Unit]` even for `check`
and importable Go output. The controller PoC had to supply an entry function
although its Go shell calls only typed exports. A better missing-entry error
explains the workaround but does not remove it; synthesizing an empty entry
would advertise an action the library never declared.

Check the type of a declared entry, but require its presence only to build or
run an executable (including emitting sources for package `main`). An embedded
package gets `Main() rt.IO` only for a declared entry. `check` and the LSP use
the same entry-optional checker. `test` and `eval` still replace the source
entry before checking: they execute a generated test or expression harness,
not the user's action. No source-package-name convention is needed.
