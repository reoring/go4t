# Native client configuration

This directory and `../auth` port the client configuration surface from client-go
v0.37.2, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. The implementation
is native go4t, not Go-source-compatible client-go bindings. Upstream copyright
notices are retained; the implementation is licensed under Apache-2.0.

## API and ownership

`api/types.gt` owns the immutable internal kubeconfig records. `api/wire.gt`
owns the v1 wire records, exposed as aliases by `api/v1`. Native record updates
are the getters/setters: `cluster.server` and `cluster{server: value}`. Source
pointer and collection presence is represented with `Option`; in particular,
extensions are `Option[Map[String, Option[runtime.Object]]]`. An absent map,
an empty map, an absent entry and an entry containing a nil interface remain
separate values. Exact bytes are `Option[String]`, never floating-point arrays.

The file `SYMBOLS.json` maps the production exports in every owned upstream
source file to their native functions, records, callback fields, or canonical
error operations. It also lists the upstream test sources consulted. Native
`DeepCopyInto` functions return the replacement destination. Immutable children
are shared; only canonical mutable external objects require effectful copying.

Kubeconfig records are **not** registered as external Go `runtime.Object`s.
The native `Scheme` stores known versions, conversion callbacks and defaulting
callbacks. `api/v1.addToScheme` installs the v1 type, conversions and defaults.
`latest.newCodec(scheme)` uses the supplied registry; `latest.codec()` constructs
the standard internal/v1 registry. Arbitrary extension values remain canonical
external Objects/RawExtensions through their external codecs.

Rest owns `rest.Config` and `AuthProviderConfigPersister`. Credential input uses
`pkg/apis/clientauthentication`'s native Scheme and codec. Exec plugin policy
validation is consumed from the AuthPlugins owner's implementation; there is no
second policy implementation in clientcmd.

### Collection conversion destinations

The eight `api.convertNamed{Clusters,AuthInfos,Contexts,Extensions}{To,From}Internal`
operations are also exposed by `api/v1`. They take `(input, destination)` and
return `IO[outcome.Outcome[destination]]`. Callers must retain both `.value`
and `.error`: a conversion failure does not erase earlier successful entries.
There are no fresh-output compatibility overloads.

List-to-map conversion leaves the destination's presence unchanged for nil or
empty input. Each element is converted before checking its destination name.
Only an existing non-nil value collides; a present nil entry is replaceable.
Map-to-list conversion appends keys in ascending order after the existing
destination slice, preserving nil versus present-empty when no entry is added.
Duplicate errors include the complete original input list, not the remaining
tail. Primitive bytes and canonical RawExtensions use dependency formatting;
native records assemble the diagnostic in wire-field order.
Nonnil nested exec/auth-provider records render their actual native values as
`&{...}` rather than synthesizing Go's nondeterministic pointer addresses.
This is a native-API representation difference: the error category/prefix,
quoted duplicate key, complete input order and field context are retained.
Ordinary and nil-pointer cases retain the source diagnostic text.
Nil typed Cluster/AuthInfo/Context entries in the reverse direction retain the
source nil-dereference panic; they are not successful empty objects or invented
conversion errors. Nil runtime.Object extension values remain valid.

`convertConfigToInternal`, `convertConfigFromInternal`, and the nested
Preferences/Cluster/AuthInfo/Context converters use the same explicit
destination and Outcome contract. Full Config conversion stops on the first
error in this order: preferences, clusters, auth infos, contexts, current-context
assignment, extensions. Unvisited destination fields and kind/apiVersion are
unchanged. `Scheme.Conversion` callbacks have those same destination-aware
signatures. Serialization's `convert`/codec boundary raises the original
canonical error; consumers needing partial values call the Outcome boundary.


## Effect boundary

`api.Source` is the injectable filesystem/environment capability. It contains
`read`, `write`, `stat`, `mkdirAll`, `remove`, `createExclusive`, `getenv`, `cwd`,
and `canRead` callbacks. `api.systemSource()` supplies the real implementation.
`canRead` opens/closes a file rather than allocating its entire contents merely
to validate a path. Path joining, absolute/relative conversion, OS metadata,
JSON tokenization, YAML conversion, base64 and standard-library parsing are
narrow host effects. No Go helper performs kubeconfig merge, override,
validation, redaction, persistence, or fallback policy; none imports client-go.

Fallible domain calls preserve the shared `support/outcome.Outcome[A]` with a
canonical Error. Cancellation, deadlines and panics stay in IO. Configuration
loading preserves a partially merged configuration alongside aggregate errors.

The main entry points are:

- `load(data)` and `loadFromFile(source, filename)`:
  `IO[Outcome[Option[api.Config]]]`.
- `newDefaultClientConfigLoadingRules(source)` and
  `loadRules(source, rules)`: explicit paths, ordered multi-file loading,
  migration, missing-file warnings and origin-relative paths.
- `newNonInteractiveClientConfig(source, config, contextName, overrides, access)`
  and the interactive/default variants: `IO[DirectClientConfig]`.
- `clientConfig(direct)`, `deferredClientConfig(deferred)`,
  `restConfigFromKubeConfig(source, bytes)`, and the flag/getter builders:
  `IO[Outcome[Option[rest.Config]]]`.
- `asClientConfig` / `asOverridingClientConfig`: callback capabilities for
  consumers. `ClientConfigLoader` and `InClusterConfig` are injectable, allowing
  deterministic fallback without cluster access.
- `modifyConfig(source, access, config, relativizePaths, useLock)`:
  `IO[Option[Error]]`. The final boolean explicitly replaces the upstream mutable
  global `UseModifyConfigLock`; `useModifyConfigLock()` returns its default.

`ClientConfigLoadingRules.defaultClientConfig` holds the optional default
client capability. Its comparison uses Rest's `configEqual`, including source
DeepEqual's treatment of non-nil functions and canonical external values.
Deferred loading caches only a successfully loaded direct client under the
shared mutex/ref primitives; failed loads can be retried by the caller.

## Preserved policies

- Earlier files own entire map entries, including intentionally empty fields;
  later entries never fill in fields of an already selected user/cluster/context.
  Non-map zero values do not overwrite earlier nonzero values.
- Context-name, context-field, user and cluster overrides follow the source
  order. CA/insecure overrides clear conflicting CA values; server overrides
  clear an inherited TLS server name unless one was explicitly supplied.
- Origins are never serialized. Relative credential files and exec commands
  containing a path separator resolve against their own stanza's origin.
  PATH-based commands are left unchanged.
- Credentials/TLS are applied only to a secure endpoint. Token-file precedence,
  impersonation, auth provider persistence, per-cluster exec extension data,
  timeout parsing and proxy validation retain separate policy steps.
- Empty configurations can fall back to an injected in-cluster capability.
  Nonempty configuration failures do not. Explicit namespace overrides and
  explicit `default` namespaces take precedence over in-cluster namespaces.
- Persistence targets each stanza's origin, defaults new stanzas to the selected
  destination, preserves unrelated stanzas, and removes deleted stanzas from
  their originating files. Lock files are acquired in sorted order and released
  through the shared cancellation-safe bracket primitive.
- Redaction, shortening, flattening and normal persistence are distinct
  operations. Normal persistence does not silently redact credentials.

Filesystem absence uses `api.isNotExist`, backed by `os.IsNotExist` on the
original canonical error. It is intentionally distinct from
`deps/support.errorIsNotFound`, which recognizes Kubernetes API status errors.
Loading, migration, persistence, and legacy authentication use the filesystem
predicate; neither predicate changes the other's meaning.

Configuration errors expose `isEmptyConfig`, `isContextNotFound`,
`isConfigurationInvalid`, and `configurationErrors`. In particular, these
source diagnostics remain distinct:

- `no context chosen`
- `context was not found for specified context: <name>`
- `no configuration has been provided, try setting KUBERNETES_MASTER environment variable`
- `cluster has no server defined`
- `cannot have values for both path and contents`

## Deterministic regression program

`regression/main.gt` executes the native loading, overriding, validation,
conversion, persistence and exec-input paths against an immutable in-memory
filesystem behind the same Source capability. It covers the assignment's
multi-file precedence, relative origins, missing/malformed configuration,
context/user/cluster overrides, in-cluster fallback, origin-aware persistence,
lock cleanup, secret redaction, version defaulting/conversion and exec input.
It also exercises legacy auth input, flattening and repeated native flag values.
It makes no network or live-cluster calls.

The supervisor should run, from the worktree root:

```sh
go run ./cmd/go4t run ports/client-go/tools/clientcmd/regression
```

The isolated public conversion consumer is prepared separately:

```sh
go run ./cmd/go4t run ports/client-go/tools/clientcmd/conversion_regression
```

It exercises all eight `api/v1` collection operations with existing
destinations, nil/present-empty collections, sorted appends, nil-entry
replacement, middle duplicates, nested conversion failure precedence, complete
diagnostics, partial full-Config results, and nil typed-entry panics.

The supervisor-released focused validation ran both programs sequentially with
the current `go run ./cmd/go4t` compiler through `serialize-vm-tests --class short`
and a 180-second bound. Both exited successfully. This observed acceptance is
limited to these two programs; no formatter, linter, full core suite, or full
upstream differential suite ran. The primary retains integration acceptance for
the combined Rest/Transport/AuthPlugins surface and full upstream parity.
