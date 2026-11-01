# Data consistency detector

Native port of client-go v0.37.2 `util/consistencydetector/data_consistency_detector.go` and companion tests (Apache-2.0).

| Export | Native equivalent |
| --- | --- |
| `IsDataConsistencyDetectionForWatchListEnabled` | `isDataConsistencyDetectionForWatchListEnabled(Detector)` |
| `SetDataConsistencyDetectionForWatchListEnabledForTest` | `setDataConsistencyDetectionForWatchListEnabledForTest(Detector,Bool) IO[func()IO[Unit]]` |
| `RetrieveItemsFunc[U]` | `RetrieveItemsFunc[A]` |
| `ListFunc[T]` | `ListFunc[P]` |
| `TransformFunc` | typed `TransformFunc[A]` |
| `CheckDataConsistency` | `checkDataConsistency(identity,rv,listFn,transform,options,retrieve,Access[P,A],Clock)` |

`newDetector` explicitly captures `KUBE_WATCHLIST_INCONSISTENCY_DETECTOR` once when constructing an application-shared detector. Its state uses support/sync instead of a hidden package global. Test override returns an effectful restoration callback.

The additional request is skipped for nonempty Continue or RV=0. Empty RV is allowed, exactly as upstream. The legacy Limit is cleared only when original RV=0, Limit>0, and the retrieved count exceeds Limit. The new request sets the last synced RV and Exact RVM, preserving other options.

Retrieval, external metadata access, optional transformation, stable UID sorting and deep equality are separate typed capabilities. `canonicalAccess` uses canonical external objects, extracting with allocation and retaining nil elements for explicit metadata rejection. Deep equality uses the original objects, not projected metadata alone. Diagnostic difference formatting is supplied as a typed callback, since it is an external representation operation rather than consistency policy.

List errors log and retry every integer one-second interval until original context cancellation. Cancellation logs the stop outcome and returns normally. Metadata/extraction/transform failures panic, and a mismatch logs the difference then raises `data inconsistency detected for IDENTITY, panicking!`. No mismatch is downgraded to a warning or a fallback.

`data_consistency_detector_test.gt` has unexecuted regressions for UID order, Exact request options, legacy Limit, mismatch panic, skipped invalid request and retry cancellation. Upstream `TestDataConsistencyChecker` and `TestDataConsistencyCheckerRetry` are the parity authority, including canonical typed and unstructured objects.
