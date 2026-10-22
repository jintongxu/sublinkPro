# SublinkPro API Reference

Complete, verified endpoint catalog for the SublinkPro REST API. Base URL: `http://<host>:8000` (default port).

Every command below was cross-checked against the backend handlers (`api/*.go`, `routers/*.go`) **and** the live frontend calls (`webs/src/api/*.js`), which are the authoritative source for paths, methods, content types, and exact field names.

All routes under `/api/v1/...` require authentication via the `X-API-Key` header. The subscription-consume path `/c/` uses a share token instead (see Shares).

---

## How to Call (curl)

The primary, dependency-free way to call the API is **curl**, which ships on virtually every macOS, Windows 10+, and Linux system. (A Python convenience wrapper also exists — see "Optional helper script" below — but it is not required.)

Set these once per session:

```bash
export SUBLINK_BASE_URL=http://localhost:8000   # your instance address
export SUBLINK_API_KEY=prefix_xxx_yyy           # your API key
```

Four call patterns cover every endpoint:

```bash
# 1. GET (query params go in the URL)
curl -s -H "X-API-Key: $SUBLINK_API_KEY" \
  "$SUBLINK_BASE_URL/api/v1/nodes/get?page=1&pageSize=50"

# 2. POST form-encoded  (endpoints marked "form" below)
curl -s -H "X-API-Key: $SUBLINK_API_KEY" \
  --data-urlencode 'link=vless://...' \
  --data-urlencode 'group=US' \
  "$SUBLINK_BASE_URL/api/v1/nodes/add"

# 3. POST/PUT JSON  (endpoints marked "JSON" below)
curl -s -H "X-API-Key: $SUBLINK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"MyAirport","url":"https://...","cronExpr":"0 */6 * * *","enabled":true}' \
  "$SUBLINK_BASE_URL/api/v1/airports"

# 4. DELETE (often a query param)
curl -s -X DELETE -H "X-API-Key: $SUBLINK_API_KEY" \
  "$SUBLINK_BASE_URL/api/v1/nodes/delete?id=123"
```

**Critical — judge success by the body's `code`, not the HTTP status.** A failure commonly comes back as **HTTP 200 with `code:500`** in the body. Always parse the JSON and check `.code` (200 = success; 400/403/404/500 = failure, read `.msg`). For example: `... | python3 -c 'import sys,json;d=json.load(sys.stdin);sys.exit(0 if d.get("code")==200 else 1)'`, or just read the `code` field directly from the response.

For non-envelope responses (subscription output at `/c/`, SSE streams) there is no `{code,msg,data}` wrapper — read the raw body as-is.

Before calling, make sure `$SUBLINK_BASE_URL` and `$SUBLINK_API_KEY` are set; if not, guide the user to set them (see SKILL.md "Error Handling Playbook") rather than sending a broken request.

### Optional helper script

If Python 3 is available, `scripts/sublink.py` wraps the four patterns above (auth header, form-vs-JSON routing, and the `code` check with a non-zero exit on failure):

```bash
python scripts/sublink.py GET  /api/v1/nodes/get --query page=1
python scripts/sublink.py POST /api/v1/nodes/add --form link='vless://...' --form group='US'
python scripts/sublink.py POST /api/v1/airports --json name='MyAirport' --json enabled=true
```

It is a convenience only — everything it does can be done with curl. Use whichever is available.

---

## Response Envelope

All JSON endpoints return:
```json
{
  "code": 200,        // 200=success, 500=error, 400/403/404=client error
  "msg": "...",       // human-readable message
  "data": <any>,      // actual response data
  "i18nKey": "...",   // optional i18n key
  "i18nParams": {}     // optional i18n params
}
```
`code` lives in the body and is what matters — a failure can come back as HTTP 200 with `code:500`. Check `.code`, not the HTTP status.

## Content-Type Trap (read this — it's the #1 cause of "参数错误" failures)

Endpoints are NOT consistent. Sending the wrong body shape produces `参数错误` / validation errors. The rule, verified per-endpoint below:

- **Form-encoded** (`--form`): auth login, node add/update, subscription add/update, template add/update/delete. These read `c.PostForm(...)`.
- **JSON** (`--json`): access keys, **node batch operations**, **subscription preview**, subscription sort/batch-sort, chain rules, airports, tags, scripts, hosts, geoip, group-sort, node-check, settings, AI template endpoints. These use `c.ShouldBindJSON(...)`.
- **Query string** (`--query`): all GET lists/filters, plus several deletes and a few POSTs that take their args in the query (`copy`, `refresh`, `trigger`).

When a call fails with a parameter error, the most likely cause is form-vs-JSON mismatch or a wrong field name — re-check against the exact entry below before retrying.

Field-name casing is inconsistent across modules (e.g. node filters are lowerCamel query params, but subscription form fields and the preview JSON body are UpperCamel). Use the exact casing shown.

---

## Authentication & Access Keys

### Login (JWT) — usually not needed; prefer X-API-Key
**POST** `/api/v1/auth/login` — **form-encoded**
- Fields: `username`, `password`, `captchaKey`, `captchaCode`, `rememberMe`, optional `turnstileToken`
- Returns a JWT (14-day expiry) or an MFA challenge. Captcha is on by default, which is why headless login is impractical — use an API key instead.

### Get Captcha
**GET** `/api/v1/auth/captcha`

### Create API Key
**POST** `/api/v1/accesskey/add` — **JSON** (demo-restricted)
```json
{
  "username": "admin",                    // required — an existing username
  "expiredAt": "2025-12-31T23:59:59Z",    // optional, RFC3339
  "description": "AI skill"               // optional
}
```
Response data: `{"accessKey": "prefix_xxx_yyy"}` — **shown only once**.

### List API Keys
**GET** `/api/v1/accesskey/get/{userId}` (query: `?page=1&pageSize=20`)

### Delete API Key
**DELETE** `/api/v1/accesskey/delete/{accessKeyId}` (demo-restricted)

---

## Users

Base: `/api/v1/users`

- **GET** `/users/me` — current user info
- **POST** `/users/update` — **form** `username`, `password` (updates login credentials)
- **POST** `/users/change-password` — **JSON** (change password with old-password check)
- **POST** `/users/update-profile` — **JSON** (username / nickname)
- **GET** `/users/mfa` — TOTP status
- **POST** `/users/mfa/totp/begin` — **JSON**
- **POST** `/users/mfa/totp/confirm` — **JSON**
- **POST** `/users/mfa/totp/disable` — **JSON**
- **POST** `/users/mfa/recovery-codes/regenerate` — **JSON**

---

## Nodes

Base: `/api/v1/nodes`

### Add Node
**POST** `/nodes/add-async` — **form**, preferred UI/API path
- Creates a persisted `node_add` background task and returns immediately as `{code,msg,data:{taskId,task,total}}`. Track it through the task SSE stream or `GET /api/v1/tasks/:id`; users may leave the node page while it runs. The final task `result` uses the structured node-add result described below.
- **Required:** `link` (one or more newline/comma-separated proxy URIs, WireGuard config text, or Clash YAML containing `proxies:`)
- Optional: `name`, `nameMode` (`link` = use upstream name, `remark` = use custom `name`; `custom` is accepted as an alias of `remark`), `remark` (independent note for the whole node, such as source/purpose), `dialerProxyName`, `group`, `skipTLSVerify` (boolean; only affects HTTPS remote subscription links embedded as nodes; use only when the endpoint certificate cannot be verified), `tags`.
- The final task result includes `mode`, `status` (`success|partial|skipped|invalid|failed`), counts (`added`, `skipped`, `invalid`, `failed`, optional `warnings`), safe per-item `items`, and bounded `diagnostics`. A warning means the node was saved but an auxiliary operation such as tag assignment failed, so the overall status is `partial`. Item statuses are `added|skipped|invalid|failed`; reason codes identify malformed input, unsupported protocol, network/TLS, parsing, duplicate, or persistence failures. Credential-bearing URLs and raw secrets are never returned.
- Inline terminal results retain at most **200 items and 32 diagnostics**, with full aggregate counts and optional `truncated`, `omittedItems`, and `omittedDiagnostics`. Progress SSE carries summary counters, not cumulative detail arrays. These limits do not cap the number of nodes processed.
- **POST** `/nodes/add` remains available as the backward-compatible synchronous endpoint using the same form decoder/import engine. Both accept legacy `NameMode` when `nameMode` is absent. Successful sync imports return the structured result in `data`; duplicate-only sync responses preserve `data:{skipped:true,duplicateInfo,result}`. The async handler splits newline/comma batches before invoking the shared engine; do not assume identical batch splitting in the legacy sync handler.

### Update Node
**POST** `/nodes/update` — **form**
- Located by `oldlink` (and/or `oldname`), NOT by id.
- Fields: `oldname`, `oldlink`, `link` (required, the new/confirmed link), `name`, `nameMode`, `remark` (independent whole-node note), `dialerProxyName`, `group`, `skipTLSVerify` (boolean; only affects HTTPS remote subscription links), `tags`.
- Nodes expose additive validation fields: `ValidationStatus` (`untested|success|partial|failed|disabled`), `ValidationReason` (bounded safe text), and `ValidationCheckAt`. These describe structural validation and are separate from speed, delay, quality, and unlock checks. Actual changes to `Link`, `ClashExtra`, `SkipTLSVerify`, or `DialerProxyName` reset status to `untested` and clear reason/time across normal, raw, and batch updates. Metadata-only/no-op updates retain it.

### Delete Node
**DELETE** `/nodes/delete` (query: `?id=123`)

### Structural Revalidation (not a speed test)
**POST** `/api/v1/nodes/revalidate` — **JSON**, authenticated, demo-restricted.

Choose exactly one selection shape:
```json
{"ids":[12,18]}
```
```json
{"all":true,"filters":{"validationStatus":"untested","group":"US","countries":["US"]}}
```

`ids` is mutually exclusive with the presence of either `all` or `filters` (even `all:false`). IDs must be positive and exist; duplicates are deduplicated. `all:true` requires a `filters` object; `{}` selects all. Empty selections, unknown selection/filter keys, and invalid status filters are rejected through the normal failure envelope. Filters reuse list semantics but do not accept pagination. Supported keys: `search`, `group`, `source`, `protocol`, `speedStatus`, `delayStatus`, `sortBy`, `sortOrder`, `maxDelay`, `minSpeed`, `maxFraudScore`, `countries`, `tags`, `residentialType`, `ipType`, `qualityStatus`, `validationStatus`, `unlockRuleMode`, `unlockRules`, `unlockProvider`, `unlockStatus`, `unlockKeyword`, `excludeIds`, `onlyResidential`, `onlyNative`. Country/tag/excluded-ID arrays also accept their `[]` key aliases. `unlockRules` accepts a rule array or its serialized JSON string.

Response: `{code:200,msg,data:{taskId,task,total}}`. It creates a `node_revalidate` task with fixed snapshots of existing targets before execution. Result fields: `mode:"revalidate"`, `status`, `total`, `processed`, `validated`, `partial`, `skipped`, `failed`, optional `items`, `diagnostics`, `truncated`, `omittedItems`, `omittedDiagnostics`. `validated` counts existing rows updated with success/partial results; `partial` is a subset. Inline limits are 200 items/32 diagnostics with complete counters. Per-item validation statuses can be `success|partial|failed|skipped`; do not constrain these to import-only `added` statuses.

Validation checks identity extraction and canonical conversion without proxy dialing, persistence in the validator, or speed/availability tests. Remote subscription rows are fetched safely and parsed: usable-only becomes `success`, mixed entries `partial`, fetch failure/no valid entries `failed`. Only existing-row validation metadata is written; remote rows are not expanded/inserted. Conditional writeback checks ID, Link, ClashExtra, SkipTLSVerify, DialerProxyName, UpdatedAt, and previous ValidationCheckAt. Editing (including edit/revert), deletion, or an already-applied overlapping result makes the stale snapshot skip. No node is recreated; cache refresh uses persisted truth.

```bash
curl -s -H "X-API-Key: $SUBLINK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"all":true,"filters":{"validationStatus":"untested"}}' \
  "$SUBLINK_BASE_URL/api/v1/nodes/revalidate"
# Check the envelope code, retain data.taskId, then GET /api/v1/tasks/{id}.
```

### Validation and subscription diagnostics
Remote HTTPS subscription fetches during import/revalidation/output honor the relevant `skipTLSVerify` setting (not a destination-safety bypass). Manual import, output, airport roots, and providers share public-destination validation at DNS lookup/dial/each redirect, numeric-IP dialing, cross-host header clearing, a 30-second request budget, 2xx-only responses, and a 4 MiB decoded-body limit (including gzip). Private/loopback/reserved destinations and URL userinfo are rejected; there is no private-network opt-out. Ordinary HTTP/HTTPS proxy links retain their classification. Configured airport proxy downloads remain proxied; proxy initialization failure does not silently fall back to direct, and the dialer receives a checked numeric target.

Partial remote output failures use bounded, safe `X-Subscription-Diagnostics` and `X-Subscription-Diagnostics-Count` headers. Policy/dependency diagnostics merge into the same maximum of eight entries; Count describes emitted diagnostics, not the full failure count. Valid output remains available to clients that ignore diagnostics. See the [feature guide](../../docs/features/node-validation.md).

### Batch Delete
**DELETE** `/nodes/batch-delete` — **JSON** `{"ids": [1,2,3]}`

### Batch Update Group / Dialer-Proxy / Source / Country (all JSON)
- **POST** `/nodes/batch-update-group` — `{"ids":[...], "group":"US"}`
- **POST** `/nodes/batch-update-dialer-proxy` — `{"ids":[...], "dialerProxyName":"..."}`
- **POST** `/nodes/batch-update-source` — `{"ids":[...], "source":"..."}`
- **POST** `/nodes/batch-update-country` — `{"ids":[...], "country":"US"}` manually assigns the same country code to selected nodes. Empty `country` clears the country marker.
- **POST** `/nodes/batch-fill-country` — **JSON** `{"airportId":123, "onlyEmpty":true}` fills node countries from enabled country rules by matching each node name. `airportId` is optional; when omitted, it scans all nodes. This is rule-based filling of empty/missing country values, not landing-IP detection. Use `/nodes/batch-update-country` when you already know the exact country code to set.

### List / Get Nodes
**GET** `/nodes/get` — query filters (all lowerCamel): `search`, `group`, `source`, `protocol`, `validationStatus`, `maxDelay`, `minSpeed`, `sortBy`, `sortOrder`, `page`, `pageSize`, array filters `countries[]`, `tags[]`. Returns `{items, total, page, pageSize, totalPages}` when paginated.

`validationStatus` accepts `untested|success|partial|failed|disabled`; empty/`all` means unrestricted. Unknown values fail on list, selector, and ID-list queries. Historical empty status is treated as untested. Validation-status sorting uses `sortBy=validation`.

### Node Selector (compact picker list)
**GET** `/nodes/selector` — same query filters as `/nodes/get`; returns `{items:[{ID, Name, Group, Source, LinkCountry, ...}], total, ...}`. Use this to present nodes as a numbered pick list.

### Node Selector by IDs
**GET** `/nodes/selector/by-ids` (query: `?ids=1,2,3`)

### Get Node IDs (for "select all matching")
**GET** `/nodes/ids` — same filters as `/nodes/get`; returns the matching id array.

### Discovery lists (for building choices)
- **GET** `/nodes/groups` — all group names
- **GET** `/nodes/sources` — all sources
- **GET** `/nodes/countries` — all country codes present
- **GET** `/nodes/protocols` — all protocols in use
- **GET** `/nodes/group-stats` — per-group counts

### Other
- **GET** `/nodes/ip-info` (query: `?ip=1.2.3.4`)
- **GET** `/nodes/ip-cache/stats`
- **DELETE** `/nodes/ip-cache`
- **GET** `/nodes/protocol-ui-meta`
- **GET** `/nodes/parse-link` (query: `?link=vless://...`)
- **GET** `/nodes/raw-info` (query: `?id=123`)
- **POST** `/nodes/update-raw` — **JSON** `{"nodeId":123, "fields":{...}}` (demo-restricted)

---

## Subscriptions

Base: `/api/v1/subcription` (note the spelling — missing the second "s").

### Add Subscription
**POST** `/subcription/add` — **form**
- **Required:** `name` AND at least one of `nodeIds` (comma-separated node IDs), `groups` (comma-separated group names), or `airports` (comma-separated airport IDs).
- Output template: `config` = a template **filename** (the `file` value from `GET /template/get`, e.g. `clash.yaml`); empty = raw node list.
- Other optional form fields (UpperCamel unless noted): `airports` (comma-separated airport IDs), `scripts` (comma-separated script IDs), `IPWhitelist`, `IPBlacklist`, `DelayTime`, `MinSpeed`, `CountryWhitelist`, `CountryBlacklist`, `NodeNameRule`, `NodeNamePreprocess`, `NodeNameWhitelist`, `NodeNameBlacklist`, `TagWhitelist`, `TagBlacklist`, `ProtocolWhitelist`, `ProtocolBlacklist`, `DeduplicationRule`, `RefreshUsageOnRequest`, `UpdateInterval`, `MaxFraudScore`, `OnlyResidential`, `OnlyNative`, `ResidentialType`, `IPType`, `QualityStatus`, `UnlockProvider`, `UnlockStatus`, `UnlockKeyword`, `UnlockRules`, `UnlockRuleMode`.
- `nodeTemplateAssignments` (optional JSON object): maps selected node IDs to one or more proxy-group names in the selected Clash template, for example `{"12":["🇭🇰 香港节点","🚀 节点选择"],"18":["🇯🇵 日本节点"]}`. For backward compatibility, a single string value is also accepted. In the UI, selecting nodes or enabling custom assignments does not automatically assign nodes to any group. Group membership must be selected explicitly; toggling this option preserves existing assignments. When `config.nodeTemplateAssignmentsEnabled` is `true`, node membership is materialized from assignments without dynamic `include-all`/provider collection, preventing duplicate entries; existing references to other proxy groups are preserved. With the option disabled and assignments cleared, the template's legacy membership behavior is preserved. Invalid or missing group names are ignored during output generation. On update, send `{}` to explicitly clear all assignments; omitting the field preserves the existing assignments.
- `explicitNodeIds` (optional comma-separated node IDs): records which nodes the user selected directly before front/dialer dependencies were expanded. It preserves explicit root selection when editing subscriptions that contain nodes also used as front proxies. The `config` JSON may include `nodeTemplateAssignmentsEnabled` (boolean); new subscriptions default to `false`.
- Name must be unique (duplicate → `订阅名称不能重复`).

### Validation Policy
Form field **`ValidationPolicy`** (exact casing) is accepted by add/update and returned on subscriptions.

| Value | Effect on stored rows |
|:---|:---|
| `all` (default) | Keep every status, including legacy empty |
| `exclude_failed` | Exclude only `failed`; retain untested/partial/disabled |
| `only_success` | Keep only `success` |

Missing/empty input normalizes to `all`, including on update (send the current strict value explicitly to preserve it). Unknown values return HTTP 400 with body `code:400`. Legacy NULL/empty database values migrate to `all` idempotently; copy and cache retain explicit policies.

Policy runs with ordinary filters before node-filter scripts and again after scripts. Shared preparation covers saved/form preview, native output, Sub-Store conversion, and share consumption. It consumes persisted row status, not live reachability or validation of every expanded remote child; output scripts/sidecar remain trusted transformations. Under non-`all` policies, excluded/unresolved chain prerequisites cause dependent targets to be omitted (including transitive dialers), not downgraded to direct/lower-priority rules and not reintroduced. `all` preserves legacy chain behavior. All-excluded output does not fall back to all nodes, though template envelopes may remain. Existing synthetic invalid/expired share notice nodes are added afterwards and exempt.

Regular rendered responses (including HEAD) include `X-Subscription-Validation-Excluded` and `X-Subscription-Validation-Dependency-Excluded`. Preview exposes the corresponding `ValidationExcludedCount` and `ValidationDependencyExcludedCount`. Safe policy/dependency diagnostics join the bounded diagnostic headers described above.

### Update Subscription
**POST** `/subcription/update` — **form**, located by `oldname` (current name). Same field set as add, plus `oldname`.

### Get Subscriptions
**GET** `/subcription/get` (query: `?page=1&pageSize=20`). Paginated → `{items, total, ...}`; without paging → array. Each subscription includes ordered `Nodes`, `Groups`, `Airports`, and `Scripts` relation arrays when present. `NodeTemplateAssignments` contains the persisted node-ID to Clash proxy-group mapping when configured.

### Delete Subscription
**DELETE** `/subcription/delete` (query: `?id=123`)

### Copy Subscription
**POST** `/subcription/copy` (query: `?id=123`)

### Sort Nodes
**POST** `/subcription/sort` — **JSON** `{"ID":123, "NodeSort":[{"ID":1,"Name":"...","Sort":0,"IsGroup":false,"IsAirport":false}, ...]}`. Airport sort items use the airport ID and `"IsAirport":true`; group sort items use `Name` and `"IsGroup":true`.

### Batch Sort
**POST** `/subcription/batch-sort` — **JSON** `{"ID":123, "sortBy":"delay", "sortOrder":"asc"}` (`sortBy`: source|name|protocol|delay|speed|country)

### Preview Nodes (which nodes survive the filters)
**POST** `/subcription/preview` — **JSON** (NOT form — UpperCamel field names):
```json
{
  "SubscriptionID": 123,            // either preview a saved sub by id...
  "NodeIDs": [1,2,3], "NodeSorts": [0,1,2],   // ...or preview an unsaved form
  "Groups": ["US"], "GroupSorts": [0],
  "AirportIDs": [10], "AirportSorts": [2],
  "Scripts": [1],
  "DelayTime": 300, "MinSpeed": 5,
  "ValidationPolicy": "only_success",
  "CountryWhitelist": "", "CountryBlacklist": "",
  "TagWhitelist": "", "TagBlacklist": "",
  "ProtocolWhitelist": "", "ProtocolBlacklist": "",
  "NodeNameWhitelist": "", "NodeNameBlacklist": "",
  "MaxFraudScore": 0, "OnlyResidential": false, "OnlyNative": false,
  "ResidentialType": "", "IPType": "", "QualityStatus": "",
  "UnlockProvider": "", "UnlockStatus": "", "UnlockKeyword": "",
  "UnlockRules": "", "UnlockRuleMode": ""
}
```

For saved preview, send `{"SubscriptionID":123}`; it uses the saved policy, not a form override. For an unsaved preview, omit `SubscriptionID` and send form fields including `ValidationPolicy`. Missing/empty form policy defaults to `all`; unknown enum returns HTTP 400. Response data includes `ValidationExcludedCount` and `ValidationDependencyExcludedCount` in addition to existing preview counts/items.

### Metadata helpers
- **GET** `/subcription/protocol-meta`
- **GET** `/subcription/node-fields-meta`

### NodeNameRule variables
`NodeNameRule` is evaluated when subscription output names are rendered. Country-related variables are based on the node's stored country code:

- `$LinkCountry` — country code, such as `HK` or `US`; empty values render as `未知`.
- `$LinkCountryName` — country name looked up from Country Rules by `$LinkCountry`; falls back to the code when no matching country-rule name exists.
- `$Flag` — flag emoji generated from the country code.
- `$DuplicateIndex` — empty for the first duplicate-name occurrence, then `1`, `2`, `3`, and so on.

Other supported variables include `$Name`, `$LinkName`, `$Group`, `$Source`, `$Protocol`, `$Index`, `$Tags`, `$TagGroup(name)`, speed/delay variables, IP quality variables, and unlock variables such as `$Unlock` and `$Unlock(netflix)`.

### Chain Proxy Rules (all under a subscription id; bodies are JSON)
- **GET** `/subcription/{id}/chain-rules` — list
- **POST** `/subcription/{id}/chain-rules` — **JSON** create
- **PUT** `/subcription/{id}/chain-rules/sort` — **JSON** `{"ruleIds":[...]}` (must be defined before the `:ruleId` route)
- **PUT** `/subcription/{id}/chain-rules/{ruleId}` — **JSON** update
- **DELETE** `/subcription/{id}/chain-rules/{ruleId}`
- **PUT** `/subcription/{id}/chain-rules/{ruleId}/toggle`
- **GET** `/subcription/{id}/chain-options`
- **GET** `/subcription/{id}/chain-rules/preview`

---

## Shares

Base: `/api/v1/shares`

### Get Shares (for a subscription)
**GET** `/shares/get` (query: `?subId=123`) — `subId` is **required**. Optional paginated filters: `page`, `pageSize`, `keyword`, `ipFilter`, `sortBy`, `sortOrder`. `sortBy` accepts `access_count` or natural numeric-aware `name`; `sortOrder` accepts `asc` or `desc`.

The subscription-management UI's first action copies an auto-detect `/c/?token=...` link without `client`. It prefers an enabled, unexpired default (`is_legacy`) share, otherwise the first available share; no available share prompts share management and does not create one. The configured system domain takes precedence over the production page origin. This reuses existing share/settings endpoints; no API contract changes.

### Add Share
**POST** `/shares/add` — **JSON**
```json
{
  "subscription_id": 123,                  // required
  "name": "Public Link",                   // optional
  "token": "",                             // optional, auto-generated if empty
  "expire_type": 0,                        // 0=never, 1=after N days, 2=specific datetime
  "expire_days": 30,                       // when expire_type=1
  "expire_at": "2025-12-31T23:59:59Z"      // when expire_type=2 (RFC3339)
}
```

### Update Share
**POST** `/shares/update` — **JSON** (same fields as add + `id` + `enabled`)

### Delete Share
**DELETE** `/shares/delete` (query: `?id=123`)

### Refresh Share Token
**POST** `/shares/refresh` (query: `?id=123`)

### Get Share Logs
**GET** `/shares/logs` (query: `?shareId=123`)

### Consume Subscription (the actual output a client fetches)
**GET** `/c/` (query: `?token=<shareToken>&client=<idx>`)
- Use `--raw` (this is NOT a JSON envelope — it's the rendered clash/v2ray/surge output).
- `token` is the **share token** (from `/shares/get` or `/shares/add`), NOT the API key.
- Client type auto-detected from User-Agent, or force with `client`.

---

## Templates

Base: `/api/v1/template`

### Get Templates
**GET** `/template/get` (query: `?page=1&pageSize=20`) — returns each template's `file`, `text`, `category` (clash|surge), `ruleSource`, etc. The `file` value is what a subscription's `config` field references.

### Add Template
**POST** `/template/add` — **form**: `filename`, `text`, `category`, `ruleSource`, `useProxy` (`true`/`false`), `proxyLink`, `enableIncludeAll` (`true`/`false`)

### Update Template
**POST** `/template/update` — **form**: `filename`, `oldname`, `text`, `category`, `ruleSource`, `useProxy`, `proxyLink`, `enableIncludeAll`

### Delete Template
**POST** `/template/delete` — **form**: `filename`

### Template Usage
**GET** `/template/usage` (query: `?filename=...`)

### ACL4SSR Presets
**GET** `/template/presets`

### Convert Rules
**POST** `/template/convert` — **JSON**

### AI Template Editing (edit sessions, all JSON; require the server's AI assistant to be configured in the web UI)

The current Template AI contract is an edit-session preview flow. The model must return structured operations, not a full replacement template. The server applies those operations to the current template, validates the candidate preview, then the client can accept the preview into the editor. Accepting does **not** save the template. Save through `/template/update` after the candidate is copied into the editor.

Short-lived session limits:

- Session TTL: `15m`
- Cleanup interval: `5m`
- Max active sessions per authenticated user: `10`
- Sessions are in memory only and are not persistent edit history

#### Start Edit Session Stream

**POST** `/api/v1/template/ai/edit-sessions/stream`, **SSE** (use `--raw`)

Request body:

```json
{
  "filename": "clash.yaml",
  "category": "clash",
  "currentText": "mixed-port: 7890\n...",
  "userPrompt": "Add a Hong Kong auto select policy group",
  "ruleSource": "",
  "useProxy": false,
  "proxyLink": "",
  "enableIncludeAll": false
}
```

SSE event names:

- `template.edit.session.created`
- `template.edit.model.delta`
- `template.edit.operations.ready`
- `template.edit.preview.validating`
- `template.edit.preview.ready`
- `template.edit.warning`
- `template.edit.error`
- `template.edit.completed`

Final preview payload fields include:

```json
{
  "sessionId": "unguessable-id",
  "status": "preview_ready",
  "baseHash": "sha256-or-revision-hash",
  "candidateHash": "sha256-or-revision-hash",
  "candidateText": "mixed-port: 7890\n...",
  "operations": [
    {
      "op": "insert",
      "anchor": "proxy-groups:\n",
      "position": "after",
      "newString": "  - name: HK Auto\n",
      "description": "Add Hong Kong auto group"
    }
  ],
  "validation": {
    "errors": [],
    "warnings": [],
    "detectedType": "clash",
    "protectedTokensFound": []
  },
  "warningFingerprint": "",
  "expiresAt": "2026-06-24T12:15:00Z"
}
```

`candidateText` here is the server-materialized preview after operations were applied and validated. It is not AI model output.

#### Get Edit Session

**GET** `/api/v1/template/ai/edit-sessions/:sessionId`

Returns the current session preview state, including `sessionId`, `status`, `filename`, `category`, `baseHash`, `candidateText` when available, `operations`, `validation`, `warningFingerprint`, `createdAt`, `expiresAt`, and `lastError`.

#### Accept Edit Session

**POST** `/api/v1/template/ai/edit-sessions/:sessionId/accept`, **JSON**

Request body:

```json
{
  "currentText": "mixed-port: 7890\n..."
}
```

`currentText` is optional, but clients should send the editor's current text when accepting a preview, especially after a previous accepted preview has not been saved yet. It is used only as editor base proof: when its revision hash matches the session `baseHash`, accept can proceed even if the template file on disk still contains an earlier saved version. It is not the candidate, is never persisted, and does not replace the server-built `candidateText` returned by accept.

If `currentText` is absent or doesn't match the session base, the server falls back to the saved template file freshness check. If the saved file changed since the session was created, accept fails with `AI_EDIT_STALE_BASE`. Validation errors still block acceptance. Validation warnings remain in the session payload as review metadata, but clients don't send warning-related request fields when accepting.

Response data:

```json
{
  "sessionId": "unguessable-id",
  "candidateText": "mixed-port: 7890\n...",
  "candidateHash": "sha256-or-revision-hash",
  "validation": {
    "errors": [],
    "warnings": []
  }
}
```

Accept verifies the owner, session expiry, `preview_ready` status, base hash freshness through either matching `currentText` or saved file content, and that validation has no errors. It returns the candidate text for the editor. It does not persist the template.

#### Discard Edit Session

**POST** `/api/v1/template/ai/edit-sessions/:sessionId/discard`

Marks the session discarded. It does not change the template or editor content.

#### Operation Schema

AI output must be an operation array. Supported v1 operations are exact-match only. `match` is optional and defaults to `unique` when omitted:

```json
[
  {
    "op": "replace",
    "oldString": "old exact text",
    "newString": "new text",
    "match": "unique",
    "description": "optional human summary"
  },
  {
    "op": "insert",
    "anchor": "exact anchor text",
    "position": "before",
    "newString": "inserted text",
    "description": "optional human summary"
  },
  {
    "op": "delete",
    "oldString": "exact text to remove",
    "match": "all",
    "description": "optional human summary"
  }
]
```

Rules:

- `replace` requires non-empty `oldString`, allows any `newString`, allows optional `match` of `unique` or `all` (empty is treated as the default unique mode), and forbids `anchor` and `position`.
- `insert` requires non-empty `anchor`, non-empty `newString`, and `position` of `before` or `after`, and forbids `oldString` and any explicit non-empty `match`. Anchors must stay exact and unique.
- `delete` requires non-empty `oldString`, allows optional `match` of `unique` or `all` (empty is treated as the default unique mode), and forbids `newString`, `anchor`, and `position`.
- In default `unique` mode, every target or anchor must match exactly once after line endings are normalized for matching.
- In explicit `all` mode, `replace` and `delete` apply to every exact occurrence of `oldString`; zero occurrences still fail with `PATCH_NO_MATCH`.
- `all` mode is intended only when the user clearly asks for all/every/全部/所有 exact occurrences. It does not enable fuzzy, regex, YAML-path, AST, or line-number matching.
- Operation application is atomic. If one operation fails, no preview is ready.

#### Error Codes

Common edit-session error codes:

- `AI_EDIT_INVALID_OPERATION`: operation type or fields are invalid
- `AI_EDIT_EMPTY_OPERATIONS`: model returned no operations
- `AI_EDIT_MALFORMED_OUTPUT`: model output was not valid contract JSON
- `AI_EDIT_LEGACY_OUTPUT`: model tried to return legacy full-template output
- `AI_EDIT_SESSION_LIMIT`: user has too many active sessions
- `AI_EDIT_SESSION_NOT_FOUND`: session id doesn't exist
- `AI_EDIT_SESSION_EXPIRED`: session expired, generate a new preview
- `AI_EDIT_STALE_BASE`: template base changed since the session was created, or the supplied `currentText` doesn't prove the editor still matches the session base while the saved file has also changed
- `AI_EDIT_VALIDATION_FAILED`: validation errors block acceptance
- `PATCH_NO_MATCH`: operation target or anchor was not found
- `PATCH_AMBIGUOUS_MATCH`: operation target or anchor matched more than once in default `unique` mode
- `PATCH_EMPTY_TARGET`: operation target was empty
- `PATCH_NOOP`: operation would not change content
- `PATCH_INVALID_INSERT_POSITION`: insert position was not `before` or `after`
- `PATCH_INVALID_MATCH_MODE`: operation used an unsupported `match` value or requested `match: "all"` for an insert at patch-application time

#### Breaking Change From Legacy Full-Template Generation

The old full-template AI endpoints are removed or return `AI_EDIT_LEGACY_REMOVED`:

- `POST /api/v1/template/ai/generate`
- `POST /api/v1/template/ai/generate-stream`
- `POST /api/v1/template/ai/validate`
- `POST /api/v1/template/ai/apply`

There is no supported fallback that accepts AI-generated full-template `candidateText` as the source of truth. Clients should use `/api/v1/template/ai/edit-sessions/*` and save accepted text through the normal template save flow.

> If `/api/v1/template/ai/edit-sessions/stream` returns an upstream or credential error, the server's AI assistant isn't configured, or its provider failed. The `/api/v1/settings/ai-assistant*` config endpoints reject API-key auth (login-session only), so this must be set up in the web UI.

---

## Airports

Base: `/api/v1/airports` (all write bodies are JSON)

- **GET** `/airports` (query: `?page=1&pageSize=20`) — list
- **GET** `/airports/{id}` — one
- **POST** `/airports` — **JSON** create. Key fields (see `dto.AirportRequest`): `name`(required), `url`(required, valid URL), `websiteUrl`(optional, valid URL; official airport website shown as a clickable link in the web UI), `cronExpr`(required), `enabled`, `group`, `downloadWithProxy`, `proxyLink`, `userAgent`, `fetchUsageInfo`, `skipTLSVerify`, plus filter/rename/dedup fields (`nodeNameWhitelist`, `nodeNameBlacklist`, `protocolWhitelist`, `protocolBlacklist`, `nodeNamePreprocess`, `deduplicationRule`, `nodeNameUniquify`, ...), and country-fill fields (`autoFillCountry`, `backfillExistingCountry`).
- **PUT** `/airports/{id}` — **JSON** update (same schema)
- **POST** `/airports/batch-update` — **JSON** `{ids:[...], applyGroup, group, applySchedule, cronExpr}`
- **DELETE** `/airports/{id}` (query: `?deleteNodes=false`)
- **POST** `/airports/pull-all` — pull all enabled airports now
- **POST** `/airports/{id}/pull` — pull one now
- **POST** `/airports/{id}/refresh-usage`

Country-fill fields apply during airport pulls and use Country Rules against upstream node names:

- `autoFillCountry`: fills the country code for newly imported nodes whose country is empty.
- `backfillExistingCountry`: fills the country code for every existing node from the same airport whose country is still empty, including nodes that otherwise do not need a name/link/order update. Saving an airport with `backfillExistingCountry:true` also persists `autoFillCountry:true`.

Neither field overwrites an existing country code, and neither performs landing-IP detection.

---

## Country Rules

Base: `/api/v1/country-rules`

Country Rules define how SublinkPro parses country codes from node names and how it resolves country names for naming variables. Enabled rules are regex-based, sorted by `priority` descending, and matching stops at the first rule that matches the node name.

Fields:

| Field | Meaning |
|:---|:---|
| `countryCode` | Country code stored on the node, usually ISO-style codes such as `HK`, `US`, `JP` |
| `countryName` | Display name used by `$LinkCountryName` and country-related UI |
| `pattern` | Regular expression matched against the node name; use `(?i)` for case-insensitive patterns |
| `priority` | Integer priority from `0` to `1000`; higher numbers match first |
| `enabled` | Whether the rule participates in matching |

Endpoints:

- **GET** `/country-rules` — list rules. Query supports `page`, `pageSize`, and `keyword`.
- **POST** `/country-rules` — **JSON** create, for example `{"countryCode":"HK","countryName":"香港","pattern":"(?i)香港|HK|Hong Kong|🇭🇰","priority":100,"enabled":true}`.
- **PUT** `/country-rules/{id}` — **JSON** update.
- **DELETE** `/country-rules/{id}` — delete.
- **POST** `/country-rules/test` — **JSON** test one pattern against one node name, for example `{"pattern":"(?i)香港|HK","testName":"HK Premium 01"}`.
- **POST** `/country-rules/batch-test` — **JSON** test enabled rules against multiple node names.
- **POST** `/country-rules/batch` — **JSON** import or replace rule lists.
- **GET** `/country-rules/export` — export all country rules as text.
- **POST** `/country-rules/sync` — **JSON** full sync from text, `{"text":"..."}`.

Text export/sync format is one rule per line:

```text
countryCode countryName priority enabled pattern
HK 香港 100 true (?i)香港|HK|Hong Kong|🇭🇰
```

Use Country Rules together with airport `autoFillCountry` / `backfillExistingCountry`, `/nodes/batch-fill-country`, and subscription `NodeNameRule` variables such as `$LinkCountryName`.

---

## Tags

Base: `/api/v1/tags`

- **GET** `/tags/list` — all tags
- **POST** `/tags/add` — **JSON**
- **POST** `/tags/update` — **JSON**
- **DELETE** `/tags/delete` (query: `?name=<tagName>`) — note: deletes by **name**, not id
- **GET** `/tags/groups` — tag groups
- **GET** `/tags/rules` — rule list
- **POST** `/tags/rules/add` — **JSON**
- **POST** `/tags/rules/update` — **JSON**
- **DELETE** `/tags/rules/delete` (query: `?id=<ruleId>`)
- **POST** `/tags/rules/trigger` (query: `?id=<ruleId>`) — apply a rule
- **POST** `/tags/node/add` — **JSON** add a tag to a node
- **POST** `/tags/node/remove` — **JSON**
- **POST** `/tags/node/batch-add` — **JSON**
- **POST** `/tags/node/batch-set` — **JSON** (overwrite mode)
- **POST** `/tags/node/batch-remove` — **JSON**
- **GET** `/tags/node/tags` (query: `?nodeId=123`)

---

## Scripts

Base: `/api/v1/script`

- **GET** `/script/list` (query: `?page=1&pageSize=20`)
- **POST** `/script/add` — **JSON** (demo-restricted)
- **POST** `/script/update` — **JSON** (demo-restricted)
- **DELETE** `/script/delete` — **JSON** (demo-restricted)
- **GET** `/script/usage` (query: `?...`)

---

## Tasks

Base: `/api/v1/tasks`

- **GET** `/tasks` (query: `?page=...&pageSize=...`) — list (path is exactly `/api/v1/tasks`, no trailing segment)
- **GET** `/tasks/stats`
- **GET** `/tasks/running`
- **GET** `/tasks/{id}`
- **GET** `/tasks/{id}/traffic` (query: filters/paging)
- **POST** `/tasks/{id}/stop`
- **DELETE** `/tasks` — **JSON** body for clearing history

---

Task types include `node_add` and `node_revalidate`. Task details/list include `total`, `result`, `message`, and `completedAt` with current progress; persisted `result` is JSON text, whereas SSE may carry an object. Parse defensively. A task reaching `completed` does not mean every row succeeded: inspect result counters/status.

Browser refresh/navigation does not cancel server work. After authentication the web UI hydrates `/tasks/running`, reconciles known active IDs via `/tasks/{id}`, and schedules a single next poll five seconds after the previous round completes; SSE shares the same normalization and bounded terminal deduplication. Recently completed tasks may no longer appear in `/running`; use task history/details for results.

On server startup, persisted pending/running tasks become `error` with completion timestamp and refreshed cache; they do **not** resume or retry automatically. `/tasks/{id}/stop` is cooperative cancellation without rollback. The last processed summary is retained in memory and persisted on cancellation for partial-result review, without per-item database writes; full item arrays are not promised on cancellation. Nil progress summaries do not erase existing summaries. Terminal persistence failure is surfaced as error; database outage cannot guarantee durability. Progress SSE sends summaries; terminal inline details are bounded to 200 items/32 diagnostics with complete counts and truncation metadata.

## Node Check (speed test / unlock check)

Base: `/api/v1/node-check`

- **GET** `/node-check/meta`
- **GET** `/node-check/profiles` — list profiles
- **GET** `/node-check/profiles/{id}`
- **POST** `/node-check/profiles` — **JSON** create (demo-restricted)
- **PUT** `/node-check/profiles/{id}` — **JSON** (demo-restricted)
- **DELETE** `/node-check/profiles/{id}` (demo-restricted)
- **POST** `/node-check/profiles/{id}/run` — run a profile (demo-restricted)
- **POST** `/node-check/run` — **JSON** `{"profileId":0, "nodeIds":[...]}` — general run entry (demo-restricted)

---

## Hosts

Base: `/api/v1/hosts`

- **GET** `/hosts/list`
- **POST** `/hosts/add` — **JSON**
- **POST** `/hosts/update` — **JSON**
- **DELETE** `/hosts/delete` (query: `?id=123`)
- **DELETE** `/hosts/batch-delete` — **JSON** `{"ids":[...]}`
- **GET** `/hosts/export` — hosts as text
- **POST** `/hosts/sync` — **JSON** `{"text":"..."}`
- **GET** `/hosts/settings`
- **POST** `/hosts/settings` — **JSON**
- **POST** `/hosts/pin` — **JSON** `{"id":123, "pinned":true}`

---

## GeoIP

Base: `/api/v1/geoip`

- **GET** `/geoip/config`
- **PUT** `/geoip/config` — **JSON**
- **GET** `/geoip/status`
- **POST** `/geoip/download`
- **POST** `/geoip/stop`

---

## Group Sort

Base: `/api/v1/group-sort`

- **GET** `/group-sort/groups`
- **GET** `/group-sort/detail` (query: `?group=<name>`)
- **POST** `/group-sort/save` — **JSON** `{"groupName":"...", "airportSorts":[...]}`

---

## Settings

Base: `/api/v1/settings` (write bodies are JSON unless noted; most writes are demo-restricted)

**Webhooks:** **GET** `/settings/webhooks` · **POST** `/settings/webhooks` · **PUT** `/settings/webhooks/{id}` · **DELETE** `/settings/webhooks/{id}` · **POST** `/settings/webhooks/{id}/test`

**Base templates:** **GET** `/settings/base-templates` · **POST** `/settings/base-templates` (`{category, content}`)

**System domain:** **GET** `/settings/system-domain` · **POST** `/settings/system-domain`

**Node dedup:** **GET** `/settings/node-dedup` · **POST** `/settings/node-dedup`

**Global node processing:** **GET** `/settings/global-node-processing` · **POST** `/settings/global-node-processing` (`{nodeNameWhitelist, nodeNameBlacklist, protocolWhitelist, protocolBlacklist, nodeNamePreprocess}`)

**Telegram:** **GET** `/settings/telegram` · **POST** `/settings/telegram` · **POST** `/settings/telegram/test` · **GET** `/settings/telegram/status` · **POST** `/settings/telegram/reconnect`

**Cloudflared:** **GET** `/settings/cloudflared` · **POST** `/settings/cloudflared` · **POST** `/settings/cloudflared/start` · **POST** `/settings/cloudflared/stop` · **DELETE** `/settings/cloudflared/token`

**Sub-Store:** **GET** `/settings/substore` · **POST** `/settings/substore` · **POST** `/settings/substore/test`

### SOCKS5 gateway

All SOCKS5 endpoints require an authenticated administrator. Destructive `POST`/`DELETE` operations are restricted in demo mode.

- **GET** `/api/v1/settings/socks5` — read public gateway settings. The plaintext password is never returned.
- **POST** `/api/v1/settings/socks5` — save and apply JSON settings:
  `enabled`, `listenAddress`, `port`, `username`, optional `password`, `clearPassword`, `nodeId`,
  `selection` (`best`, `random`, `round_robin`, `smart`, or `specific`), `requireAuth`, `maxAttempts` (1-5),
  `dialTimeoutSeconds` (1-120), `failureCooldownSeconds` (0-3600), `specificFallback`,
  `maxConnections` (1-10000), `maxConnectionsPerClient` (1..maxConnections),
  `idleTimeoutSeconds` (0-86400), `maxConnectionDurationSeconds` (0-604800), `healthCheckEnabled`,
  `healthCheckIntervalSeconds` (10-3600), `healthCheckTimeoutSeconds` (1-30), and candidate-pool arrays
  `candidateGroups`, `candidateSources`, `candidateProtocols`, `candidateCountries`, `stickySessionEnabled`,
  `stickySessionMode` (`client_ip` or `username`), and `stickySessionTtlSeconds` (60-604800).
  Candidate-pool fields combine with AND; values inside one field combine with OR. Empty arrays do not restrict the pool.
  Optional fields may be omitted to preserve saved values for legacy clients. `0` disables either connection timeout.
- **POST** `/api/v1/settings/socks5/stop` — stop the listener without changing saved settings.
- **GET** `/api/v1/settings/socks5/status` — return `data.config`, `data.listeners`, aggregate `data.stats`, and `data.health`.
- **GET/POST** `/api/v1/settings/socks5/listeners` — list or create listener endpoints (`id`, `enabled`, `listenAddress`, `port`, `defaultProfileId`, nullable `requireAuth`).
- **PUT/DELETE** `/api/v1/settings/socks5/listeners/:id` — replace or delete a listener endpoint.
- **GET/POST** `/api/v1/settings/socks5/accounts` — list or create independent credentials bound to routing profiles. Plaintext passwords are never returned.
- **PUT/DELETE** `/api/v1/settings/socks5/accounts/:id` — replace or delete an independent account; an omitted/empty update password keeps the saved password.
- **GET** `/api/v1/settings/socks5/profiles` — list the virtual `default` routing profile and saved custom profiles.
- **POST** `/api/v1/settings/socks5/profiles` — create a custom routing profile with an independent candidate pool and routing settings.
- **PUT** `/api/v1/settings/socks5/profiles/:id` — replace a custom profile.
- **DELETE** `/api/v1/settings/socks5/profiles/:id` — delete a custom profile; the reserved `default` profile cannot be deleted.
- **GET** `/api/v1/settings/socks5/routing` — return a server-paginated candidate-node runtime page. Pass `profileId` to inspect a specific profile. Supports `keyword`, `status`, `sortBy`, `sortOrder`, `page`, and `pageSize` (maximum `100`). Items include health state, latency source, active/success/failure counters, smart score, cooldown, and last-selection time.
- **POST** `/api/v1/settings/socks5/routing/reset` — reset per-node historical counters without disconnecting active sessions.
  Stats include `activeConnections`, `totalConnections`, `successfulConnections`, `failedConnections`, `uploadBytes`, and `downloadBytes`.
  Health includes sweep timing/counts and per-node status, latency, consecutive failures, cooldown, and a bounded error message.
- **POST** `/api/v1/settings/socks5/health/probe` — trigger an immediate administrator-only health sweep. Duplicate concurrent sweeps are not started.
- **GET** `/api/v1/settings/socks5/connections` — return the current active connection array. Each item includes
  `id`, `clientAddress`, `target`, `nodeName`, `profileId`, `profileName`, optional `account`, optional `listenerId`, `phase`, `startedAt`, `lastActivity`, `uploadBytes`, and `downloadBytes`.
- **DELETE** `/api/v1/settings/socks5/connections/:id` — disconnect one active connection by ID.
- **DELETE** `/api/v1/settings/socks5/connections` — disconnect all active connections.

`data.config` uses the same public settings shape as the settings endpoint, including `hasPassword`, optional `maskedPassword`, `running`, and `boundAddress`; it never includes plaintext `password`. Omitting `password` preserves the saved password, while `clearPassword: true` clears it. Status counters, active connections, and in-memory sticky leases belong to the current gateway server lifetime and reset after stop/reapply. TCP CONNECT supports multiple listeners, independent profile-bound accounts, adapter reuse, pre-reply candidate retry, exponential failure cooldown, smart P2C load balancing, sticky sessions, connection limits, idle/duration timeouts, active health probing, and live monitoring. UDP ASSOCIATE and BIND are not implemented.

**Database migration:** **POST** `/settings/database-migration/import` — **multipart/form-data** (upload a backup.zip / .db)

**AI assistant (login-session only — reject API key with 403 by design):**
- **GET** `/settings/ai-assistant`
- **POST** `/settings/ai-assistant` (`{baseUrl, model, requestType, apiKey, temperature, maxTokens, extraHeaders}`; `requestType` is `responses` or `chat_completions`)
- **POST** `/settings/ai-assistant/models`
- **POST** `/settings/ai-assistant/test` (accepts the same provider fields and uses `requestType` to test `/responses` or streamed `/chat/completions`)

> These four configure the stored AI provider key, so they only work in a logged-in web session. An API-key call always gets 403 here — that's expected, not a bug. Configure the AI assistant once in the web UI; afterwards `/template/ai/*` works with API-key auth.

---

## Dashboard / Stats

Base: `/api/v1/total`

- **GET** `/total/sub` — subscription count
- **GET** `/total/node` — node count
- **GET** `/total/fastest-speed` — fastest node
- **GET** `/total/lowest-delay` — lowest-delay node
- **GET** `/total/country-stats`
- **GET** `/total/dashboard-country-stats`
- **GET** `/total/protocol-stats`
- **GET** `/total/system-stats`
- **GET** `/total/tag-stats`
- **GET** `/total/group-stats`
- **GET** `/total/source-stats`
- **GET** `/total/dashboard-grouped-stats`
- **GET** `/total/quality-stats`

---

## Other Endpoints

### Version (public — no API key needed; ideal health check)
**GET** `/api/v1/version`

### Backup and WebDAV
All endpoints below require an administrator account and are disabled in demo mode.

- **GET** `/api/v1/backup/download` — download the current system backup ZIP.
- **GET** `/api/v1/backup/webdav` — get WebDAV settings. The password is never returned; use `hasPassword` and `maskedPassword`.
- **POST** `/api/v1/backup/webdav` — save JSON settings: `baseUrl`, `username`, optional `password` (empty preserves the saved password), `clearPassword` (explicitly removes it), `remotePath`, `timeoutSeconds`, `allowInsecureHttp`, `allowPrivateNetwork`, `scheduleEnabled`, and `cronExpr` (standard 5-field cron expression, required when scheduling is enabled). Saving reloads the schedule without a restart.
- **POST** `/api/v1/backup/webdav/test` — test the submitted JSON settings without saving them.
- **POST** `/api/v1/backup/webdav/upload` — generate the current backup ZIP and upload it using the saved settings.
- **GET** `/api/v1/backup/webdav/files` — list up to 200 remote ZIP backups, newest first.
- **POST** `/api/v1/backup/webdav/restore` — download and restore a remote ZIP. JSON fields: `filename`, `includeAccessKeys` (default `true`), `includeSubLogs` (default `false`). Returns `taskId`; follow the existing task APIs for progress.

WebDAV restore reuses the database migration workflow and overwrites current business data. The current instance's JWT, encryption key, Cloudflare Tunnel, WebDAV settings, and SOCKS5 gateway configuration (accounts, listeners, routing profiles) are preserved. Restoring disconnects active SOCKS5 connections and reloads the gateway against the restored nodes. HTTPS is required unless `allowInsecureHttp` is explicitly enabled. Private, loopback, and reserved targets require `allowPrivateNetwork`. WebDAV backup creation is currently SQLite-only. Uploads and downloads are limited to 1 GiB.

### Server-Sent Events
**GET** `/api/se` (query: `?token=<jwt>`) — SSE stream (auth-protected).

---

## Notes

- **Demo mode:** write endpoints marked demo-restricted are blocked when the instance runs in demo mode.
- **Pagination:** most list endpoints accept `?page=&pageSize=` and then return `{items, total, page, pageSize, totalPages}`.
- **Content type is per-endpoint** — see the Content-Type Trap section above. When in doubt, match the exact entry here rather than guessing.
