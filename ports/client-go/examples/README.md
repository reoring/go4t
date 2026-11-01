# Controlled native examples

These sources port the shipped client-go v0.37.2 workflows. They are **unverified
implementation sources**, not an acceptance result. Use the module README for
exact commands and full differential-comparison instructions. No command in this
README has been run by the implementation owner.

## Upstream-to-native mapping

| Pinned upstream source | Native entrypoint / implementation |
| --- | --- |
| `examples/in-cluster-client-configuration/main.go` | `in-cluster-client-configuration/main.gfx`; `configurationExample(true)` |
| `examples/out-of-cluster-client-configuration/main.go` | `out-of-cluster-client-configuration/main.gfx`; `configurationExample(false)` |
| `examples/create-update-delete-deployment/main.go` | `create-update-delete-deployment/main.gfx`; `deploymentWorkflow`, `updateDeployment` |
| `examples/dynamic-create-update-delete-deployment/main.go` | `dynamic-create-update-delete-deployment/main.gfx`; `dynamicDeploymentWorkflow`, `updateDynamicDeployment` |
| `examples/fake-client/main_test.go` | `fake-client/main.gfx`; `fakeInformerExample` |
| `examples/workqueue/main.go` | `workqueue/main.gfx`; `Controller`, `newController`, `syncToStdout`, `handleErr`, `processNextItem`, `runWorker`, `runController` |
| `examples/leader-election/main.go` | `leader-election/main.gfx`; `electionExample` |
| Root `example_test.go` | `../example_test.gfx`; config, API error, dynamic list, informer, SSA and election equivalents |

Interactive prompts and infinite signal waits are replaced by bounded controlled
fixtures, not deleted client operations. `deploymentWorkflow` and its dynamic
counterpart accept actual native clients. Their example runners validate the
transmitted create/update replicas, container images, preserved resource version,
foreground delete policy, resource paths, decoded list, and response errors.

## Fixture boundaries

- `environment.gfx`: explicit feature, transport, clock, authentication, limiter,
  and REST composition; no hidden clientset or feature singleton.
- `trace.gfx`: native ordered observation ledger, request capture, strict route
  sequence and consumption checks. The generic host HTTP adapter binds loopback.
- `typed_parity.gfx`, `fake_parity.gfx`, `apply_parity.gfx`: all nine original
  `oracle/expected.json` scenarios. The expected file is never read at runtime.
- `host/trace.go`: external JSON encoding/decoding, URL query decoding, canonical
  external error inspection, and apimachinery unstructured accessor adapters.
  It contains no client-go import or client-go policy.
- `fake_informer.gfx`: establishes the fake tracker watch before creating the
  object, preserving the upstream example's watch-registration barrier.
- `controller.gfx`: real HTTP Pod list/watch into a native shared informer,
  indexed store, deduplicating/rate-limiting queue, and two processing workers.
- `election.gfx`: local Lease protocol storage; native lock/election code acquires,
  calls the controller callback, observes cancellation, and releases the lease.
- `integration/main.gfx`: composes clientset protocol regressions and actual
  remotecommand/port-forward socket fixtures owned by Streaming.

Every HTTP fixture rejects unexpected paths; ordered parity fixtures also reject
extra or unconsumed requests. Request bodies and headers are observed rather than
synthesized. Full JSON comparison is deliberately strict about options, errors,
list resource versions, action copies, and managed-field ownership. A divergence
must be investigated, not normalized away.

All example entrypoints have a bounded parent context. Server cleanup, informer
shutdown, and queue/task joins use native bracket/ensuring scopes. The primary
still needs to exercise success, assertion-failure, and cancellation cleanup.
