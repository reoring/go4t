# Native clientset integration scenarios

Pinned sources are client-go v0.37.2 `kubernetes_test/clientset_test.go`,
`fake_client_test.go`, and `timeout_test.go`. This directory contains source-level
ports of their consumer-visible assertions. **No checks have been run by this
owner.**

Prepared command, from `ports/client-go`:

```sh
gofx test kubernetes_test
```

| Original behavior | Native scenario |
| --- | --- |
| Real client supports WatchList; fake client explicitly does not | `testWatchListCapabilityAndFactories` |
| WatchList bookmark completes real informer cache sync | `examples.controllerExample`, called by the capability test; real loopback API emits ADDED and initial-events-end BOOKMARK |
| Fake informer uses legacy LIST/WATCH and reaches sync | `examples.fakeInformerExample`, called by the capability test; waits for tracker watch before mutation |
| Default/custom User-Agent shared by generated clients | `testClientUserAgent`, two actual resource requests for each configuration |
| List and Watch send `timeout=21s` | `testListAndWatchTimeout`; both actual routes return structured NotFound |
| Fake Get/List and real typed decoding omit GVK | `testFakeAndRealGVK` |
| Canceled request reaches underlying HTTP handler | `testCancellationReachesHTTP`; explicit start and cancellation observation channels, task joined |

The full differential program additionally covers fake action snapshots, label
filtering, real/fake extension distinctions and managed-field conflicts. The
controlled integration entrypoint calls the protocol checks together with the
native workflows and Streaming's socket-backed fixtures:

```sh
gofx run examples/integration/main.gfx
```

No cluster or credential is contacted. The capability-only client uses loopback
port 1 but performs no API operation; all requests use newly allocated httptest
listeners. The module/root entrypoints provide finite parent deadlines; fixture
cleanup uses native bracket/ensuring and task joins. Acceptance must still verify
all timeout, error, and cleanup paths rather than infer success from these files.
