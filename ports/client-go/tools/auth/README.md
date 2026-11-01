# Authentication configuration helpers

Native gofx implementation of pinned client-go v0.37.2 `tools/auth` and
`tools/auth/exec`. Copyright 2014–2020 The Kubernetes Authors; Apache-2.0.

`Info` is the legacy file-format record. `loadFromFile(Source, path)` retains
filesystem errors; `decode` accepts the source JSON field names case-insensitively.
`mergeWithConfig` returns a new Rest-owned Config and respects the presence of
`Info.insecure`, including an explicit false. `complete` retains the source
username/certificate/token predicate.

`exec.loadExecCredential(data)` and `exec.loadExecCredentialFromEnv(source)`
return `IO[Outcome[LoadedExecCredential]]`. The result retains the approved
native credential object/version and an anonymous Rest Config; it never
registers a native record as a Go runtime.Object. Input decoding and version
conversion use `pkg/apis/clientauthentication`; cluster-to-Rest conversion uses
Rest's `execClusterToConfig`. An unset/empty environment value and missing
cluster information remain distinct failures. This package does not run the
credential executable; execution policy and caching belong to AuthPlugins.

The shared filesystem/codec interfaces, exact symbol map, and deterministic
regression entry point are documented in `../clientcmd/README.md`. No checks
were run in this assignment.
