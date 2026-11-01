# Native version and client authentication APIs

Source oracle: client-go v0.37.2, revision 72c52dd0cc3b9099b36af11290b0304045dd1ada. Policy lives in go4t. `version.get()` returns canonical apimachinery version.Info; `getWith(BuildInfo)` is the native build-injection equivalent of upstream ldflags, preserving all fallback strings and runtime compiler/platform/version fields.

## Client authentication

`apis/clientauthentication` owns immutable ExecCredential, ExecCredentialSpec, ExecCredentialStatus and Cluster records. Both versioned modules export aliases to the common immutable wire shape, while the enclosing CredentialObject retains the API version. All fields use lowerCamel spelling. Status, Cluster and ExpirationTimestamp preserve pointer presence. CertificateAuthorityData is Option[String] containing exact bytes, not Unicode code points or base64 text; None and Some("") are distinct. Token/certificate/private-key data remain strings. Time, TypeMeta, Object, RawExtension, GroupVersion, GroupKind and GroupResource use canonical dependency types, never duplicate nominal external records.

Every generated Convert_* capability maps to the corresponding `convert{Cluster,ExecCredential,ExecCredentialSpec,ExecCredentialStatus}{To,From}Internal` in v1/v1beta1. External RawExtension/Object conversions call apimachinery's own conversion routines. Generated credential conversion omits TypeMeta; deep copy retains it. DeepCopy methods become `deepCopy<Type>`, DeepCopyInto returns a replacement destination, and DeepCopyObject returns the same typed native credential capability. Immutable byte strings need no extra copying; external object trees use DeepCopyObject/RawExtension.DeepCopy and timestamps use Time.DeepCopy. Nullable native callers retain None around these non-null record operations.

`newScheme`, module `addToScheme`, and `install.install` supply typed native construction, recognition, conversion and defaulting. `schemeBuilder()` returns registration actions applied by `applySchemeBuilder`. `groupName()`, Kind/Resource functions and `schemeGroupVersion()` retain their canonical identities. Both pinned `zz_generated.defaults.go:31-33` implementations return without registering field defaulters. Native `registerDefaults` records that identity policy independently, without adding known types or replacing conversion callbacks. Model-name functions retain exact io.k8s.client-go names.

This is a native credential scheme, not a Go runtime.Scheme populated with Go client-go record types. Consumers must use CredentialObject and native conversion entry points. Generic external runtime.Object cannot contain a native credential without a separate explicitly agreed interoperability adapter; do not substitute JSON/dyn for the typed surface. The versioned aliases require callers to retain their enclosing API version when passing through the generic native scheme.

`encode(scheme, object, targetVersion)` and `decode(scheme, text)` implement the native JSON wire path; `decodeToVersion` additionally converts to an explicitly requested version. Required `interactive` and `server` fields are emitted, optional empty scalars omitted, pointer fields retain presence, CA bytes use base64, and timestamps use canonical metav1.Time JSON semantics. Arbitrary plugin config stays raw JSON through canonical RawExtension, including integers beyond 2^53; it never passes through Float or dyn. JSON tokenization and external value codecs are narrow host effects; credential field selection and assembly are native go4t.

Behavioral program: `pkg/apis/clientauthentication/regression`, including wire/version/base64/timestamp/large-config-integer cases. The assigned isolated invocation passed after correcting a mismatched closing parenthesis in the DecodeRaw host adapter. Evidence and exact controlled-environment command: `local://clientgo-featuresversion-auth-regression.txt`. The prior parser-only check covered all 17 owned `.gt` files; no broad suite was run. Integrated plugin/clientcmd verification remains under the primary's matrix.

External object and RawExtension copies use the shared canonical
`deps/objects.objectDeepCopy` and `rawExtensionDeepCopy` adapters.
The object-copy result remains optional, including custom objects whose
DeepCopyObject returns nil.
