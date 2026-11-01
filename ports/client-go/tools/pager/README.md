# Native pager

Pinned policy and tests: client-go v0.37.2 `tools/pager/pager.go` and `pager_test.go` (Apache-2.0).

| Upstream exported surface | Native surface |
| --- | --- |
| `ListPageFunc` | `ListPageFunc[P] = func(meta.ListOptions) IO[Outcome[Option[P]]]` |
| `SimplePageFunc` | `simplePageFunc`; callbacks use the current IO environment, with explicit contexts applied by `sync.withContext` |
| `ListPager` | `ListPager[P,A] {pageSize,pageFn,fullListIfExpired,pageBufferSize,access}` |
| `New` | `new(pageFn,PageAccess[P,A])`; defaults 500, true, 10 |
| `List` | `list(pager,options)` |
| `ListWithAlloc` | `listWithAlloc(pager,options)` |
| explicit-context native equivalent | `listWithContext(pager,options,allocate)` under `sync.withContext` |
| `EachListItem` | `eachListItem(pager,options,callback)` |
| `EachListItemWithAlloc` | `eachListItemWithAlloc(pager,options,callback)` |

`PageAccess` has effectful metadata extraction, item extraction (allocation flag), and assembly callbacks plus an expiration predicate. They adapt representation only; paging, retry, buffering and ordering policy remain in `pager.gt`. `objectPageAccess()` provides the canonical runtime.Object specialization with `A=Option[runtime.Object]`, preserving nil elements. It uses external `meta.ExtractList`/`ExtractListWithAlloc` through the canonical dependency boundary, not JSON or custom object types.

`List` preserves selectors and uses PageSize only for zero Limit. Subsequent requests carry the server continuation and clear RV/RVM. Page-one metadata supplies assembled RV/SelfLink. A single complete page is returned unchanged. Resource-expiration fallback occurs only after a continuation and only when enabled: it clears Limit/Continue and restores the original RV/RVM. The paginated flag stays true. Initial expiration and non-expiration errors do not trigger fallback. Allocation mode shallow-copies non-pointer list items through the external extraction operation; pointer elements retain identity.

Streaming each-item calls intentionally differ: continuations do not clear RV/RVM, and expiration is returned instead of retrying as a full list. The producer has exactly `pageBufferSize` queued pages plus its one in-flight page. Negative sizes fail before a request with `ListPager.PageBufferSize must be >= 0, got N`. Consumer errors win; producer cancellation and actual join happen before return. Callback processing finishes the current chunk even if the original context is canceled, matching the source's context check between chunks. Both normal and cancellation paths retain canonical errors rather than flattening them to text.

Regression programs in `pager_test.gt` cover restored fallback options, first-page expiration, negative buffer rejection and a rendezvous-buffer consumer-error path. They are unexecuted source in this assignment. Upstream `TestListPager_List`, `TestListPager_EachListItem` and `TestListPager_eachListPageBuffered` remain the parity authority, including empty/single/multiple pages, option precedence, cancellation and buffer sizes.
