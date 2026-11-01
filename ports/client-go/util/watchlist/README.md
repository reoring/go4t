# Watch-list option negotiation

Native port of client-go v0.37.2 `util/watchlist/watch_list.go` and its companion tests (Apache-2.0).

- `PrepareWatchListOptionsFromListOptions` → `prepareWatchListOptionsFromListOptions(enabled,validation,options) IO[Outcome[(meta.ListOptions,Bool)]]`.
- `DoesClientNotSupportWatchListSemantics` → `doesClientNotSupportWatchListSemantics(Option[func()Bool]) Bool`. Absence is not rejection; an explicit true rejects.
- `canonicalValidation()` supplies external Scheme conversion plus `ValidateListOptions(...,true)`. No validation policy is copied into host code.

Feature disabled, external validation rejection, nonlegacy positive Limit and Exact RV match produce empty options/false without an API error. Conversion errors remain errors. Successful negotiation clears Limit, sets NotOlderThan, Watch, AllowWatchBookmarks and Some(true) SendInitialEvents, preserving all other fields. RV=0 positive Limit is the legacy exception. Validation runs both before and after transformation, so continuation/RV combinations retain upstream distinctions.

`watch_list_test.gt` covers optional capability presence, feature disabled without invoking validation, legacy Limit, Exact rejection, output flags, and conversion-error versus validation rejection. These programs have not been executed. Upstream `TestPrepareWatchListOptionsFromListOptions` and `TestDoesClientNotSupportWatchListSemantics` remain the full matrix authority.
