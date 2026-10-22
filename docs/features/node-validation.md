English | [简体中文](node-validation.zh-CN.md)

# Node Import, Structural Validation, and Subscription Policies

Structural validation checks that a node identity can be extracted and converted to the canonical proxy representation. **It is not a speed, latency, connectivity, IP quality, or unlock test.** A `success` status does not prove that the server or credentials work. Use the [speed test system](speedtest.md) for network measurements.

## Import and compatibility

The node page submits `POST /api/v1/nodes/add-async` (form data). It returns `data.taskId`, `data.task`, and `data.total`; the `node_add` task continues after leaving the page. Supported input includes proxy URIs, WireGuard configuration text, Clash `proxies:` YAML, and remote subscription URLs. Ordinary HTTP/HTTPS **proxy** links remain proxy nodes, not subscription downloads.

The synchronous `POST /api/v1/nodes/add` remains supported and uses the same import engine. Both accept `nameMode` and legacy `NameMode`; `custom` remains an alias for `remark`. The synchronous duplicate-only response keeps `data.skipped: true`, `data.duplicateInfo`, and the additive `data.result`; successful imports return the structured result directly in `data`. Do not assume that every response has the same nesting. The async endpoint splits newline/comma-separated input before invoking the shared engine; do not assume that the legacy synchronous endpoint provides the same batch splitting.

Results distinguish `added`, `skipped`, `invalid`, `failed`, and optional `warnings`. A saved node with an auxiliary failure (for example tag assignment) produces a partial result, not a false total failure. Import may create nodes from a remote subscription; **revalidation never does**.

## Status, invalidation, and safe display

Nodes expose `ValidationStatus`, `ValidationReason`, and `ValidationCheckAt` independently of speed-test fields:

| Status | Meaning |
|:---|:---|
| `untested` | No current structural result; historical empty status is displayed as untested |
| `success` | Structure accepted; for a remote row, fetched entries validated without reported issues |
| `partial` | Remote subscription has usable entries and parsing/validation issues |
| `failed` | Invalid structure, failed subscription fetch, or no valid subscription entries |
| `disabled` | Stored disabled validation state; not a connectivity result |

Actual changes to `Link`, `ClashExtra`, `SkipTLSVerify`, or `DialerProxyName` reset status to `untested` and clear the reason and check time. This applies to normal edits, raw-field edits, and batch update paths. Metadata-only edits and no-op changes retain the result. Reasons in the table and mobile cards are bounded and redacted; do not infer reachability from their wording.

## Revalidate selected nodes or all matching nodes

Use the node page's selected-node validation action, or validate **all nodes matching the current filters**, after reviewing the confirmation count. The all-filtered action is not restricted to the visible page.

`POST /api/v1/nodes/revalidate` requires authentication, is blocked in demo mode, and accepts JSON in exactly one of these forms:

```json
{"ids":[12,18]}
```

```json
{"all":true,"filters":{"validationStatus":"untested","group":"US","countries":["US"]}}
```

- `ids` cannot coexist with `all` or `filters`, even with `all:false`. IDs must be positive and exist; duplicates are deduplicated. Empty selections fail.
- `all:true` requires a `filters` object (`{}` means all nodes). Unknown selection keys, unknown filter keys, and invalid status values fail instead of broadening the selection.
- Filters reuse the node-list decoder, including country/tag/excluded-ID arrays and unlock-rule semantics. Pagination is not a revalidation filter. `validationStatus` is also available on node list, selector, and ID-list queries; accepted values are `untested`, `success`, `partial`, `failed`, `disabled`, or empty/`all` for no restriction. Unknown validation statuses are rejected.
- The response contains `data.taskId`, `data.task`, and `data.total`. The `node_revalidate` task fixes its target snapshots before starting; later additions are not included.

Validation writes only the existing row's validation metadata. Remote rows are safely fetched and parsed, but are not expanded into new stored nodes. A remote fetch is network I/O; no proxy-node dial, speed test, or availability probe is performed. Fetch failure/no valid entries becomes `failed`; mixed valid and invalid entries become `partial`.

Writeback requires the same ID, validation inputs, `UpdatedAt`, and previous `ValidationCheckAt`. Edits (including edit-then-revert), deletion, or an already-applied overlapping validation invalidate the old snapshot. Stale results are **skipped**, not used to overwrite a newer node or recreate a deleted one. Cache refresh uses current persisted data.

The result has `mode: "revalidate"`, `total`, `processed`, `validated`, `partial`, `skipped`, and `failed`. `validated` counts existing rows successfully updated with success/partial results; `partial` is a subset, not an extra count. No nodes are added. Row failures are counted separately. Inspect counters and result `status`, not merely the task's `completed` state.

## Task recovery, cancellation, and bounded details

- Refreshing the page or navigating away does not cancel server work. After authentication, the UI hydrates active tasks from `/api/v1/tasks/running`, then reconciles active task details serially, scheduling the next pass five seconds after the previous pass. SSE provides prompt progress; reconciliation compensates for missed events. Logout/unmount cleans up polling. Terminal notifications are deduplicated with a bounded recent-ID set.
- A server restart marks persisted `pending` and `running` tasks as `error`, records completion time, and refreshes the task cache. **There is no automatic resume, retry, or durable worker queue.** Restart recovery is not the same as browser refresh recovery.
- Stop a task from the global task UI or `POST /api/v1/tasks/{id}/stop`. Cancellation is cooperative. Changes already applied are not rolled back. Cancellation preserves the last processed summary for result review; it does not promise a full per-item cancellation report. In-flight remote cancellation does not write a fabricated validation failure.
- Progress updates retain summaries in memory, without per-item database writes; nil updates do not erase them. Terminal transitions persist them. Terminal persistence failures are surfaced as `error` rather than reported as successful completion. If the database is unavailable, durable persistence cannot be guaranteed.
- Import and revalidation inline results retain at most **200 items and 32 diagnostics**, while preserving full aggregate counts. Check `truncated`, `omittedItems`, and `omittedDiagnostics` (optional when zero). Progress SSE sends summaries, not cumulative item arrays. Task history/details provide result review after transient progress cards disappear. Truncated details are not a complete audit export.

## Subscription validation policy

The subscription form's `ValidationPolicy` is persisted, copied with the subscription, and used by saved/form preview and shared/native/converted output. Missing or empty input defaults to `all`, including legacy rows; unknown policies return HTTP 400 on add/update/form preview. On update, omission also selects `all`: clients retaining a strict policy must send it explicitly.

| Policy | Stored statuses kept |
|:---|:---|
| `all` (default) | All statuses, including historical empty status |
| `exclude_failed` | Everything except `failed`, including `untested`, `partial`, and `disabled` |
| `only_success` | Only `success` |

This is a **stored-row structural-status policy**, not live availability checking or per-expanded-child certification. Policy filtering joins the normal filters before node-filter scripts and is applied again afterwards; chain prerequisites are checked after scripts and before rendering. Existing output scripts and the Sub-Store sidecar remain trusted transformation boundaries, not a sandbox or a guarantee of final-output validation.

Preview returns `ValidationExcludedCount` and `ValidationDependencyExcludedCount`; the preview dialog displays both. A saved preview (`SubscriptionID`) uses the saved policy, whereas an unsaved preview uses the supplied `ValidationPolicy`. Regular rendered responses, including HEAD, expose:

- `X-Subscription-Validation-Excluded`: policy exclusions.
- `X-Subscription-Validation-Dependency-Excluded`: additional chain-dependency exclusions.
- `X-Subscription-Diagnostics` and `X-Subscription-Diagnostics-Count`: merged, safe diagnostics, at most eight entries (not the full failure count).

When a non-`all` policy excludes a chain prerequisite, dependent targets are omitted safely, including transitive stored dialer dependencies. Missing explicit/dynamic/custom-group prerequisites do not cause fallback to direct or lower-priority rules, and excluded nodes are not reintroduced. Unresolved/empty custom chain groups are not emitted in the strict-policy Clash bridge. `all` preserves legacy chain behavior. See [chain proxy](chain-proxy.md).

If every node is excluded, output stays empty of stored nodes: there is **no fallback to all nodes**. Templates may still render an envelope. Existing synthetic invalid/expired share notification nodes are added afterwards and are intentionally exempt. Share tokens and expiration behavior are unchanged; see [subscription sharing](subscription-share.md).

## Remote-fetch safety and proxy behavior

Manual remote import, remote output, airport root subscriptions, and provider fetches share the controlled subscription-fetch boundary. It accepts HTTP/HTTPS only, rejects URL userinfo and private/loopback/reserved destinations, validates DNS/IPs again at dialing and each redirect, and pins a checked numeric destination for dialing. Cross-host redirects clear request headers. Responses must be 2xx; the decoded body (including gzip) is limited to **4 MiB**, with a **30-second** request budget and bounded redirects.

TLS verification is enabled unless the existing `skipTLSVerify` setting explicitly opts out for the relevant fetch. This does not bypass destination restrictions. Existing airport headers, User-Agent, usage metadata, provider expansion, and configured proxy downloads remain supported. A configured proxy initialization failure fails the request rather than silently falling back to direct; the proxy dialer receives the checked numeric target, not an unchecked hostname. There is no production private-network allow switch for this fetch boundary.

For exact request fields and curl examples, see the [API reference](../../skill-sublinkpro/reference/api.md).
