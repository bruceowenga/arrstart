# WP0.8 Consolidation — HTMX + SSE dashboard performance strategies

**WP:** WP0.8 (BUILD-PLAN Phase 0: "Spike S6: HTMX + SSE progress screen - prove the live progress screen pattern")
**Date:** 2026-11-23
**Status:** Investigation report + proof-of-concept spec. Experimental, not production code. No commits, no Docker verification.
**Related:** docs/build-log.md (newest entry), ADR-0002 (wizard UI stack: HTMX + Go templates + SSE), PRD §10.2 W5/W8, BUILD-PLAN §6 Phase 0.

*This spike is intentionally desk research plus a vanilla-JavaScript prototype sketch. Per the task constraints: no Docker containers were started, no pinned images were pulled, and no real-container API behavior was verified.*

---

## 1. Repo recon findings

### 1.1 Docs/PERF-ENGINEERING.md

The file `docs/PERF-ENGINEERING.md` does **not exist** in this repo. Its §5 "area-specific themes" referenced in the spike brief is therefore unavailable, and the areas named in the task (frontend dashboard: SSE metrics first paint <3s; SSE bus; API layer: POR timing instrumentation) are taken from the task context, not from a checked-in spec.

**Recommendation:** author `docs/PERF-ENGINEERING.md` as a follow-up (after WP3.13 scoping), seeding it with the findings and comparison tables below.

### 1.2 Directory inventory

| Path | Exists? | Notes |
|---|---|---|
| `/web` | No | PRD §18 layout plans `web/` with `embed.go` exposing an `embed.FS`; not yet created (WP1.x wizard work) |
| `/ui` | No | No UI package of any kind exists |
| `/scripts` | No | Created for this spike at `scripts/dashboard/dashboard-widget-perf-proto.js` |
| `/bench` | No | Created for this spike at `bench/cache-comparison-harness.json` |
| `package.json` | No | Repo is Go-only, single binary `cmd/arrstart/main.go`; PRD §12.1 mandates "No Node build step" |
| `cmd/` | No | Only `internal/placeholder/placeholder.go` exists (WP0.2 CI scaffolding artifact) |

### 1.3 Existing HTMX / SSE code

None. No wizard templates, SSE handlers, or HTMX attributes exist yet. ADR-0002 has already decided the stack: HTMX + Go templates + SSE, localhost-only wizard bind, random one-time token in URL, `--rm` container. WP0.8's job is to prove the live progress screen pattern; this consolidation extends that proof to the WP3.13 dashboard metrics area (SSE first paint under 3s, batching, offline queueing, virtualized charts).

### 1.4 Relevant conventions this spike respects

- Go module `github.com/bruceowenga/arrstart`; all UI embedded via `go:embed` in `/web`, `/templates`, `/presets` (each with its own `embed.go`) once created.
- No Node build step anywhere (PRD §12.1) → the benchmark harness is JSON, not a JS test runner; the PoC is a single dependency-free script.
- No real indexers / content sources anywhere; no secrets; `.env`/compose output never committed.
- Conventional Commits with `Refs:` footer when this work lands in a PR.

---

## 2. Web research findings

Sources are listed in §8. Where a page could not be fetched (403 or no results), the entry is marked "search result only" and treated as low-confidence.

### 2.1 graphql-hooks CacheProvider context-bound cache (React)

Package `graphql-hooks` (unpkg README, version unconfirmed — npm page returned 403):

- API: `new GraphQLClient({ url, cache })` where `cache` implements `get/set/delete/clear/keys/getInitialState` (opaque key/value, **not normalized**). `memcache` package available.
- Hooks: `useQuery(query, { variables, useCache, skipCache, updateData })`; `useMutation` (not cached); `useSubscription` (needs `subscriptionClient`).
- Invalidation: `client.invalidateQuery(query)` (delete + refetch + store), `client.setQueryData(query, updater)` (override without refetch), `refetch()` from `useQuery` presets `skipCache=true`. No documented `cache.reset`/`evict`.
- Context: hooks read the client from `ClientContext` or an explicit `client` option — client-bound rather than a true per-subtree cache; no documented nested `CacheProvider` for context-bound invalidation domains.
- Maintenance: unclear from README alone (badges present, no date); signal is low compared with Apollo/urql.

**Assessment:** not a fit for arrstart's HTMX-first dashboard, and arguably not a fit for any new GraphQL integration given uncertain upkeep. Listed for completeness only.

### 2.2 Dexie IndexedDB storage (current version: 4.4.7)

- `new Dexie('name'); db.version(1).stores({ widgets: 'id,++ts,area', metrics: 'ts,widgetId' })` — versioned schema, indexed tables with compound indexes (e.g. `'id,+timestamp'`), transactions, `bulkPut`/`bulkAdd`, `where`/`filter`/Collection API, lifecycle hooks, `liveQuery` observables (v4).
- vs `localStorage`: Dexie is async, structured values (no string coercion), no 5 MB string quota, indexed queries, multi-tab safe; localStorage is synchronous, main-thread blocking, string-only, same-origin per tab protocol+host+port.
- vs `sessionStorage`: same storage limits as localStorage but tab-scoped (dies when tab closes) — useful for ephemeral per-session widget draft state, not layout persistence.
- vs `idb-keyval`/`localforage`: key/value only; Dexie adds real indexes and schema migrations.
- Persistence: IndexedDB quota managed by browser (Storage Estimation API); survives restarts; cleared only by user/site data clearing. No built-in sync between tabs (use `BroadcastChannel` if needed).

**Assessment:** best choice for persisted widget layout/preferences in the dashboard; localStorage only for tiny primitive prefs (e.g. collapsed sidebar); sessionStorage for tab-ephemeral drafts.

### 2.3 HTMX script defer + hx-sse for dashboard metrics SSE; batching streams

htmx `hx-sse` extension (htmx.org / four.htmx.org, v4 behavior):

- `hx-sse:connect="/sse/metrics"` opens a persistent SSE connection (GET); `hx-sse:close` ends it.
- htmx 4 implements the stream with `fetch()` + `ReadableStream` (not `EventSource`); reconnect defaults on: first delay 500 ms, doubling, capped by `sse.reconnectMaxDelay`; `sse.reconnectMaxAttempts` (default unlimited); jitter ±30%; `sse.connectTimeout`; `sse.pauseOnBackground` suspends streams on hidden tabs.
- Last-Event-ID: client tracks the current event ID and sends `Last-Event-ID: <id>` on reconnect so the server can replay missed messages; the client does **not** deduplicate replays (idempotence is the app's job).
- Unnamed events: `data` swapped into `hx-target` (default `innerHTML`). Named events dispatch DOM events handled by `hx-on`/`hx-trigger`; `hx-swap-oob` and `<hx-partial>` support extra swaps.
- Multiple listeners can attach to one endpoint; the server differentiates channels by SSE `event:` type and client dispatches by event name.

**Batching recommendation:** one batched SSE endpoint emitting envelope `{ ts, widgetId, seq, payload }` events, client-side meter (max N events/s, coalesce per widget ID keeping latest), each widget dispatches a targeted HTMX swap (`#w-<id>`). One TCP connection per tab instead of N per-widget streams; avoids connection limits (~6/host) and SSE proxy buffering issues.

**Script defer:** inline the critical SSE consumer + skeleton markup inline in the HTML; `defer` the charting library and heavy widgets so first paint isn't blocked.

### 2.4 Offline SSE queueing + sync-on-reconnect (service worker + IndexedDB patterns)

SSE protocol basics (search results on Last-Event-ID / retry):

- Browser `EventSource` auto-reconnects and sends `Last-Event-ID`; server `retry:` field hints reconnect delay.
- Events **can be lost** while disconnected unless the server keeps a replay buffer keyed by event ID. A bounded replay buffer (size + TTL) is required for exactly-once semantics; clients must deduplicate by event ID because replays may redeliver.
- Offline writes: queue mutations in IndexedDB (`outbox` {id, ts, type, payload}) and a `checkpoint` {lastEventId}; on reconnect, send `Last-Event-ID`, drain/apply the queue, dedupe by ID. Service workers can't receive SSE (no push-to-ServiceWorker for SSE), so queueing lives in the page; a service worker can only serve the offline shell.

**Assessment:** for arrstart, keep it simple: IndexedDB outbox + checkpoint in the dashboard page, replay via `Last-Event-ID`, idempotent widget updates (upsert by widgetId). No service worker complexity in v1.

### 2.5 Virtualized metrics line charts (1k+ time-series points)

Chart.js (canvas-only; no SVG, no WebGL in core) performance guidance:

- Canvas Path2D line caches when `animation: false`; decimation plugin reduces points before rendering; `parsing: false`, `normalized: true` (unique/sorted/consistent indices), explicit scale `min`/`max`, `ticks.sampleSize`, `pointRadius: 0`, Bézier curves off.
- Recharts: SVG DOM — each point is DOM-adjacent elements; cost grows linearly with point count; no built-in virtualization; good for <~500–1k points, poor beyond without downsampling.
- WebGL options (plotly.js-gl, TradingView lightweight-charts, LightningChart): 10k–100k+ points at 60 fps, but heavy bundles (hundreds of KB–MB) and GPU/shader complexity; overkill for a dashboard widget.
- Virtualized canvas (offscreen canvas + viewport windowing + requestAnimationFrame) is the middle ground for >10k points with pan/zoom.

**Assessment:** arrstart dashboard metrics first paint <3s with ~1k points/widget → canvas with decimation + `animation: false` is the sweet spot. Recharts SVG fine below ~500 points per series. WebGL / virtualized canvas only if the harness (bench) shows >10k point needs. All unverified until the harness runs in a real browser.

### 2.6 sql.js / WASM SQLite in a Web Worker (client-side mutation queue)

sql.js (npm, v4 line at sql.js.org, version unconfirmed on the fetched page):

- Builds: `sql-wasm.js` (needs `locateFile` for the `.wasm`), `sql-wasm-inline.js` (embedded WASM, no `locateFile`), `worker.sql-wasm-inline.js` for single-file Web Workers.
- `initSqlJs(config)` is async; `new SQL.Database()`; `exec`/`run`/`prepare`/`batch` exist per general API knowledge (signatures not confirmed by fetch).
- **No built-in persistence**: `db.export()` produces a byte array you must copy to IndexedDB/OPFS yourself; @sqlite.org/sqlite-wasm adds OPFS persistence.
- Single `Database` instance is not thread-safe; worker queue pattern: main thread posts `{sql, args}`, worker executes sequentially, posts results back.

**Assessment:** main-thread optimistic updates are enough for dashboard widget config; a Web Worker WASM SQLite queue is justified only for offline mutation queues with DB-level concurrency simulation (>thousands of queued ops, conflict resolution). Post-v1 option for arrstart; do not add to v1 dashboard.

---

## 3. Comparison tables

### Table A — GraphQL client cache for context-bound invalidation

| Criterion | graphql-hooks CacheProvider | Apollo InMemoryCache | urql document cache + graphcache + dedupExchange |
|---|---|---|---|
| Cache model | Opaque key/value `get/set/delete` (not normalized) | Normalized, entity-based by `__typename` + ID | Document cache keyed by query+variables; graphcache = normalized fields |
| Context binding | `ClientContext` client + explicit `client` hook option; no nested per-subtree cache documented | `ApolloProvider` context; one cache per client instance | One cache per `Client`; exchanges compose per client |
| Invalidation API | `client.invalidateQuery(query)`, `setQueryData(query, updater)`, `refetch()` (skipCache) | `cache.modify({id, fields})` with `INVALIDATE`, `cache.evict({id})`, `cache.gc()`, `cache.reset()`, `readField`/`writeQuery` | graphcache: field resolvers, `updateQuery`, optimistic updates; manual targeted invalidation is limited (urql discussions #998/#1425); `cache-reset` on logout |
| In-flight dedup | No | No | `dedupExchange` dedupes identical in-flight requests |
| Bundle / complexity | Small, low signal / maintenance unclear | Large, mature | Moderate; graphcache has learning curve |
| Fit for arrstart dashboard widgets | Poor (no GraphQL planned; unmaintained signal) | Poor (heavy; no GraphQL planned) | Poor today; hold as option if a React/GraphQL dashboard is ever added |

**Recommendation:** do not adopt any of the three for the v1 HTMX dashboard. arrstart's dashboard is server-driven HTML fragments over SSE; client cache should be DOM-targeted swaps plus a small persisted state store (Dexie), not a GraphQL client. Revisit urql graphcache only if a React dashboard is built later (see open items).

### Table B — Browser persisted widget state

| Criterion | Dexie indexed tables (v4.4.7) | localStorage | sessionStorage |
|---|---|---|---|
| API | Promise-based, DB/version/stores/indexes, transactions, bulk ops | Sync `getItem`/`setItem`, string only | Sync, tab-scoped, string only |
| Quota | Browser IndexedDB quota (Storage Estimation API), typically hundreds of MB | ~5 MB, string UTF-16 cost | ~5 MB per tab |
| Indexed queries | Yes, compound indexes | No (full scan manual) | No |
| Persistence | Survives browser restarts; cleared by site-data clear | Survives restarts, same origin | Dies when tab closes |
| Main-thread cost | Async; bulk ops off main thread | Sync; blocks on read/write | Sync; blocks on read/write |
| Multi-tab | Shared DB; use BroadcastChannel for sync | Shared across tabs | Isolated per tab |
| Best for arrstart | Widget layout, column order, widget preferences, SSE checkpoint | Tiny primitives (collapsed state, theme) | Tab-ephemeral widget drafts |

**Recommendation:** Dexie for widget layout/preferences and the SSE checkpoint/outbox; localStorage only for ≤2 tiny primitives; sessionStorage for per-tab draft state.

### Table C — SSE delivery for dashboard metrics

| Criterion | Per-widget SSE stream (N endpoints) | Single batched SSE endpoint | HTMX `defer` + batched SSE | `hx-trigger` polling fallback |
|---|---|---|---|---|
| Connections per tab | N (hits ~6/host limit) | 1 | 1 | 0 (uses `hx-get` XHR/fetch) |
| First paint impact | High (N connection handshakes) | Low (one stream, deferred chart libs) | Low | Low initial, recurring cost |
| Widget targeting | Natural (one stream per widget) | Envelope `{widgetId}` + client dispatcher | Same as batched | Full-page or per-widget HTML swap |
| Offline/replay | N `Last-Event-ID` states | One `Last-Event-ID` + bounded server replay buffer | Same as batched | Re-request on next poll (gaps possible) |
| Proxy friendliness | Poor (many long-lived connections) | Good (one `text/event-stream`) | Good | Good (short requests) |
| Implementation complexity | N × server handlers | 1 handler + client meter/dispatcher | + inline critical script, `defer` libs | `hx-trigger="every 30s"` |

**Recommendation:** single batched SSE endpoint `/sse/metrics` with event typing per widget, client-side metering (max ~5 events/s/widget, coalesce latest per widgetId within 250 ms), `hx-sse:connect` with default reconnect backoff (500 ms start, doubling, jitter), `Last-Event-ID` replay, plus `hx-trigger="every 45s"` polling fallback only when SSE fails (corporate proxies may block long-lived streams). Defer chart libraries with `<script defer>`; inline critical SSE consumer.

### Table D — Metrics chart rendering (1k+ time-series points)

| Criterion | SVG DOM (Recharts) | Virtualized canvas | WebGL (plotly-gl / lightweight-charts) |
|---|---|---|---|
| 1k points/series cost | ~linear DOM/symbol cost; noticeable at >1k | Low (bitmap, offscreen + viewport windowing) | Very low (GPU) |
| Bundle size | Moderate (React + Recharts) | Small–moderate (vanilla or tiny lib) | Large (hundreds of KB–MB) |
| Interactivity (zoom/pan) | Re-render cost grows | Virtualized viewport handles it | Native GPU handling |
| First paint <3s suitability | OK below ~500 pts/series | Good | Overkill; heavy parse cost hurts first paint |
| arrstart fit | Widgets under ~500 pts | Default for metrics widgets | Defer; bench-gated |

**Recommendation:** canvas rendering with decimation (`animation: false`, `parsing: false`, `pointRadius: 0`) as the default; Recharts SVG only for small summary sparklines (<500 pts); WebGL/virtualized canvas only if the harness shows sustained >10k point/series requirements.

### Table E — Mutation queue: main-thread optimistic vs Web Worker WASM SQLite (sql.js)

| Criterion | Main-thread optimistic updates | Web Worker sql.js (WASM) queue |
|---|---|---|
| State size suited to | <~MB widget config; tens of ops | 10k+ queued mutations; DB-style concurrency |
| Offline persistence | IndexedDB outbox manually | `db.export()` → IndexedDB/OPFS manually |
| Main-thread blocking | Minimal for widget toggles/reorders | Worker keeps UI thread free |
| Complexity | Low | High (worker marshaling, WASM load, conflict resolution) |
| Bundle | None | sql.js WASM inline (~MB range) |
| arrstart v1 fit | Yes — widget config via HTMX swaps + Dexie checkpoint | No — post-v1 only |

**Recommendation:** optimistic main-thread updates with IndexedDB checkpoint for the v1 dashboard. Document sql.js worker queue as a post-v1 option for offline-first widget editing.

---

## 4. Recommendations mapped to WP3.13 area trust boundaries

WP3.13 areas from the task context (note: `docs/PERF-ENGINEERING.md` does not exist, so §5 themes could not be read):

### 4.1 Frontend dashboard — SSE metrics first paint <3s

Trust boundary: browser tab; only localhost-origin SSE in v1 (wizard binds localhost, token in URL). No credentials in query strings; SSE is same-origin only.

| Element | Recommendation |
|---|---|
| HTML/critical path | Inline critical CSS + inline SSE consumer script; skeletons for widget placeholders; `<script defer src="...chart...">` for chart libs |
| SSE | `hx-sse:connect="/sse/metrics"` on load, single batched endpoint, event-typed per widget (`event: widget-metric`) |
| Client meter | Coalesce events per `widgetId` (keep latest within 250 ms), cap ~5 events/s/widget |
| DOM updates | Targeted HTMX swaps into `#w-<id>`; `hx-swap="innerHTML"`; `hx-on:arrstart:widget-metric` or a tiny controller calling `htmx.ajax` |
| Persisted state | Dexie: widget layout/preferences, checkpoint `{lastEventId}`, outbox |
| Offline | IndexedDB outbox + checkpoint; on reconnect send `Last-Event-ID`; dedupe replayed events by `event.id`; idempotent upsert by `widgetId` |
| Fallback | `hx-trigger="every 45s"` on `#w-<id>` when SSE unavailable |

### 4.2 SSE bus

Trust boundary: wizard → browser, same-origin, localhost only; one long-lived stream per tab.

| Element | Recommendation |
|---|---|
| Endpoint | `GET /sse/metrics`, `Content-Type: text/event-stream`, envelope `{ id, ts, event, widgetId, seq, payload }` |
| Replay buffer | Bounded (e.g. last 500 events, TTL 5 min) keyed by event `id`; replay on `Last-Event-ID` |
| Reconnect | 500 ms base, exponential backoff with jitter, cap attempts; honor `sse.pauseOnBackground` |
| Concurrency | One stream per tab; reject or queue duplicate connections from same token session |
| Metrics | Bus-level: connect rate, reconnect rate, events/s, replay buffer hit/miss, average event lag |

### 4.3 API layer — POR timing instrumentation

"POR" (point-of-request / per-operation-request timing, to be confirmed — see open items).

| Element | Recommendation |
|---|---|
| Instrumentation | Per-endpoint histograms (p50/p95/p99) at API edge middleware; correlation ID per request propagated to SSE events where useful |
| What to time | Wiring steps (`WaitHealthy/Configure/Verify`), app API calls, compose generation; **not** every widget query on the bus (the bus aggregates) |
| Privacy/security | Never log API keys, tokens, or full request bodies; redact in diagnostic output (doctor already redacts reports) |
| Exposure | Timing summary available via `arrstack status` / doctor output, not over the wizard HTTP surface to unauthenticated token holders beyond what the token allows |

---

## 5. Deliverables written

| File | Purpose |
|---|---|
| `docs/spikes/wp08-consolidation.md` | This investigation report (findings, tables A–E, area mapping, open items) |
| `scripts/dashboard/dashboard-widget-perf-proto.js` | Vanilla-JS PoC: script defer + metered batched SSE + IndexedDB offline reconnect queue; explicitly experimental |
| `bench/cache-comparison-harness.json` | Benchmark spec: scenarios, metrics, payload sizes, SLO gates for comparing caching/SSE/chart strategies (JSON — no Node build step, honoring PRD §12.1) |

---

## 6. Open items / questions blocking WP3.13

1. **`docs/PERF-ENGINEERING.md` is absent.** §5 area-specific themes could not be read; this report's §4 maps recommendations to the three task-context areas as a placeholder until the spec is authored.
2. **No UI package exists yet** (`/web`, `/ui` absent; `cmd/` absent). HTMX + SSE patterns here are unproven against this repo's actual templates; real proof lands in WP1.8 (deploy + live progress screen).
3. **POR timing instrumentation definition unconfirmed.** Confirm what "POR" covers before adding API middleware (per wiring step? per app API call? per endpoint?).
4. **graphql-hooks maintenance status** unverified (npm page 403'd; README only). Low confidence — treat as "do not use".
5. **Dexie real-world numbers** (bundle size gz, quota on target browsers) unverified numerically.
6. **SSE behind corporate proxies / WSL2 Windows networking** and `ReadableStream` vs `EventSource` reconnect behavior in htmx 4 needs lab/CI e2e verification (`make e2e` against real containers) — out of scope for this desk spike.
7. **Benchmark harness not yet executed** (no browser benchmark runner in repo; JSON spec only). Execution requires a headless Chrome environment.

---

## 7. Sources

- PRD.md (`docs/PRD.md`) — §10.2 W5/W6/W8, §10.4, §10.5b X3, §12.1, §13 Phase 4
- BUILD-PLAN.md (`docs/BUILD-PLAN.md`) — §6 WP0.8, §7 session protocol, §8 ADRs
- CLAUDE.md / CONTEXT.md — repo rules, glossary, ADR-0002 HTMX + Go templates + SSE decision
- graphql-hooks README via unpkg: https://unpkg.com/graphql-hooks/README.md (npm page 403)
- Dexie npm: https://www.npmjs.com/package/dexie (403; version confirmed via unpkg package.json: 4.4.7); docs: https://dexie.org/docs/index
- htmx hx-sse extension: https://four.htmx.org/extensions/hx-sse.md ; https://htmx.org/extensions/sse/ ; https://v1.htmx.org/attributes/hx-sse/
- SSE Last-Event-ID / retry: https://semitexa.com/blog/eventsource-reconnect ; https://adhdecode.com/api-architecture/server-sent-events-and-streaming/implementing-sse-retry-last-event-id/
- urql document caching / graphcache: https://urql.dev/docs/basics/document-caching/ ; https://urql.dev/docs/api/graphcache/ (search results only; discussions #998/#1425 on manual invalidation)
- Apollo InMemoryCache: https://www.apollographql.com/docs/react/api/cache/InMemoryCache (search results only)
- Chart.js performance: https://www.chartjs.org/docs/latest/general/performance.html ; SVG vs canvas: https://apexcharts.com/blog/svg-vs-canvas-charts/ (search results only)
- sql.js: https://sql.js.org/documentation/ ; sqlite-wasm: https://sqlite.org/wasm (search results only)
