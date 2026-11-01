# Native certificate utilities

This domain ports client-go **v0.37.2**, revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`, across `util/cert`,
`util/keyutil`, `util/certificate`, `util/certificate/csr`, and
`util/connrotation`. `coverage.json` maps all 73 exported source declarations
(including receiver methods and context-overload equivalents) to native APIs.
Pinned test files are the behavioral references, not production dependencies.
The Kubernetes Authors' Apache-2.0 notices are retained in the source.

## API and representation

All policy is native `.gt`. Go helpers perform cryptography, typed external
object conversion, filesystem/socket effects, and fixture setup only. There
is no production `k8s.io/client-go` import. Keys, certificates, certificate
requests, TLS identities, IP addresses, file identities, and CSR API objects
reuse the canonical `deps` types. PEM data is an exact byte-bearing `String`.
Fallible operations use `support/outcome.Outcome`; `parseCertsPEM` preserves
successfully parsed certificates when a later certificate fails to parse.

Certificate signing policy is selected by native `cert.signCertificate`:
CA certificates use KeyUsage 37 (digital signature, key encipherment, cert
signing), serving certificates use KeyUsage 5 and ServerAuth, and both mark
basic constraints valid. Its unchanged public signature calls the private
`_signCertificate` effect with explicit canonical key usages and the constraints
flag. The Go signing primitive only converts those supplied fields, signs and
parses; it has no CA/server usage branch or policy defaults.

Public functions use lowerCamel names. Explicit receiver arguments replace
methods. The implicit IO context replaces the `WithContext`/`DialContext`
overloads; no alias APIs are maintained. Duration and wall-clock values are
nanosecond `Int`s. `sync.Clock`, client callbacks, key generation and jitter
are injectable. `newConfig(clock, store)` supplies manager defaults; explicit
`Config` construction retains the upstream validation of mutually exclusive
`usages` and `getUsages`.

* `keyutil`: PKCS#1 RSA, SEC1 EC, and unencrypted PKCS#8 private keys; public
  extraction from RSA/EC public/private/certificate blocks; ignored unrelated
  PEM blocks; key persistence and load-or-generate behavior. Public extraction
  intentionally retains the pinned parser's RSA-only PKCS#8 fallback, while
  private PKCS#8 parsing accepts all algorithms supported by x509.
* `cert`: PEM parsing/encoding, CA creation, self-signed serving chains,
  fixture-file naming/reuse, CSR generation and RSA signature selection,
  pool construction, certificate file IO and pair readability.
* `certificate`: `Store` is a current/update capability record. `FileStore`
  adds `currentPath`. `Manager` exposes `start`, `stop`, `current`,
  `serverHealthy`, `rotateCerts`, and `getClientCertificate`.
* `csr`: `fromClientset(kubernetes.Interface)` adapts real typed v1/v1beta1
  clients. Request creation selects v1 when signer/usages permit, falls back
  on NotFound, and reuses compatible AlreadyExists requests. List/watch waits
  validate UID and Denied/Failed/Approved outcomes, relist after stream
  closure, and release watches on cancellation. Error message prefixes retain
  Kubernetes APIStatus classification. CSR compatibility checks actual
  signatures against the supplied key and rejects expired signed chains.
* `connrotation`: `ConnectionTracker` is the existing Transport
  `ConnectionSet`; `track`, dialers, and `closeAll` reuse that implementation.
  Closing a connection removes it; close-all detaches the old set so new
  concurrent connections belong to the next generation.

## Persistence and lifecycle

Key files are created with mode 0600, certificate-only files with 0644, and
parent directories with 0755. Existing file modes are not silently chmodded.
The store reads `prefix-current.pem`, then explicit cert/key paths, then
`prefix.crt`/`prefix.key`. Missing pairs retain the distinct `NoCertKeyError`
identity through `noCertKeyError`/`isNoCertKeyError`.

Rotation opens/truncates a timestamped 0600 pair file, writes normalized
certificate PEM followed by the first private-key PEM block, fsyncs it, loads
and validates the actual TLS key pair, then updates the current symlink.
Regular files in either reserved symlink position are rejected. The pinned
updated/current symlink recovery and remove/rename order are retained; this
is **not** advertised as a transactional filesystem or a gap-free swap.
Cancellation brackets close acquired file/watch/socket resources.

A manager rejects expired current certificates, falls back to bootstrap
credentials, rotates bootstrap credentials immediately, and uses the pinned
70%-90% lifetime jitter. Renewal retries use 2/4/8/16-second jittered delays,
then 32-second polling. Dynamic template changes cancel superseded waits.
Each attempt reloads its client from the current certificate, while a failed
attempt leaves the last usable certificate intact. `stop` cancels and joins
all manager workers and active request cleanup; no real cluster or credential
access is needed for fixtures.

## Unverified server inspection is not authentication

`getClientCANames(host)`, `getClientCANamesForURL(url)`,
`getServingCertificates(host, serverName)`, and its URL variant intentionally
preserve upstream's **unverified metadata inspection** behavior. They create
an isolated TLS configuration with peer verification disabled, supply no
client credentials, and return the actual handshake's CA-name/certificate
metadata. Returned certificates are **not trusted** merely because inspection
succeeded. This policy does not alter ordinary client transport verification.
The loopback fixture explicitly covers a self-signed, expired peer and checks
that no client certificate was presented.

## Verification status

No build, formatter, linter, test or network probe was executed in this slice.
The following permanent native tests were authored for the supervisor's
integrated verification:

| Directory | Cases |
| --- | --- |
| `util/keyutil` | fixed RSA/EC/PKCS#8 fixtures, malformed PEM, public extraction, ignored parameters, corrupt-file preservation, modes |
| `util/cert` | partial parse errors, header filtering, actual CSR signature/SAN, key mismatch, expiry metadata, CA clock/SAN, certificate modes, controlled loopback inspection |
| `util/certificate` | bootstrap/expiry/template/deadline, real rotation and client reload, retry decisions, shutdown, usage/key errors, file precedence, symlink recovery, regular-file rejection, key mismatch |
| `util/certificate/csr` | condition outcomes, UID/deletion, duration casts, compatibility, APIStatus identity, v1 fallback/reuse, listed issuance, cancellation/watch cleanup |
| `util/connrotation` | real pipe close-all, reuse, shared tracking and individual-close cleanup |

Run `go4t test` for each directory above after the shared dependency/clientset
integration has landed. Those commands are recommendations, not observed
passing results. Test private keys are public, pinned upstream fixtures and
must never be used as credentials.
