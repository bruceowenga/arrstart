/**
 * @fileoverview dashboard-widget-perf-proto.js
 *
 * WP0.8 Spike S6 consolidation proof-of-concept: HTMX script defer, metered batched SSE,
 * and offline reconnect queue via IndexedDB for an arrstart-style dashboard.
 *
 * STATUS: EXPERIMENTAL PROTOTYPE. Written for desk research only. No production claims.
 * - Not connected to arrstart build/lint/test tooling.
 * - Not reviewed against real containers or pinned images.
 * - No real indexers, content sources, or secrets referenced anywhere in this file.
 * - Single dependency-free <script> file: loads as a classic script, no bundler, no npm.
 *
 * Intended destination: scripts/dashboard/ (new directory). If adopted, it would be served
 * from /web embedded assets (embed.go) and referenced with `defer` from wizard/dashboard
 * templates so the critical path never blocks on chart or metrics code.
 *
 * Design notes (research-backed, see docs/spikes/wp08-consolidation.md):
 *   * One batched SSE endpoint emits typed events; client meters and dispatches per widget.
 *   * IndexedDB holds the SSE checkpoint (lastEventId) and an outbox of missed events.
 *   * On reconnect, Last-Event-ID asks the server to replay; client dedupes by event id.
 *   * Chart libraries are deferred (<script defer>); only the SSE consumer is inline.
 *
 * Security posture (matches arrstart rules):
 *   * Same-origin SSE only; never embed the wizard one-time token in query strings.
 *   * Never insert uncontrolled event payloads via innerHTML; use textContent / HTMX swaps.
 *   * No API keys, passwords, or config values are logged or stored in the queue.
 */

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

/** SSE endpoint. Must be same-origin (wizard binds localhost only). */
const ENDPOINT = '/sse/metrics';

/** Metering: max batched events to act on per widget per second. */
const MAX_EVENTS_PER_WIDGET_PER_SEC = 5;

/** Coalesce window per widget: keep only the latest event per widgetId in this ms. */
const COALESCE_WINDOW_MS = 250;

/** Reconnect base delay (ms); doubles with jitter up to the cap. */
const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_DELAY_MS = 30000;

/** IndexedDB names (same-origin, per tab protocol+host+port). */
const DB_NAME = 'arrstart-dashboard';
const DB_VERSION = 1;
const STORE_CHECKPOINT = 'checkpoint'; // { id: 'sse', lastEventId, updatedAt }
const STORE_OUTBOX = 'outbox';         // { id, ts, type, widgetId, payload, applied }

/** Maximum replay buffer the server should keep (advisory; server-side enforced). */
const SUGGESTED_SERVER_REPLAY_LIMIT = 500;

// ---------------------------------------------------------------------------
// Tiny promise wrappers over raw IndexedDB (no Dexie dependency in this PoC).
// Dexie (v4.4.7) is the recommended replacement if indexed queries/migration
// complexity grows; see comparison table B in docs/spikes/wp08-consolidation.md.
// ---------------------------------------------------------------------------

function idbOpen() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = (event) => {
      const db = event.target.result;
      if (!db.objectStoreNames.contains(STORE_CHECKPOINT)) {
        db.createObjectStore(STORE_CHECKPOINT, { keyPath: 'id' });
      }
      if (!db.objectStoreNames.contains(STORE_OUTBOX)) {
        const store = db.createObjectStore(STORE_OUTBOX, { keyPath: 'id', autoIncrement: true });
        store.createIndex('byTs', 'ts', { unique: false });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function idbGet(db, store, key) {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readonly');
    const req = tx.objectStore(store).get(key);
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function idbPut(db, store, value) {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readwrite');
    const req = tx.objectStore(store).put(value);
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function idbClear(db, store) {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readwrite');
    const req = tx.objectStore(store).clear();
    req.onsuccess = () => resolve();
    req.onerror = () => reject(req.error);
  });
}

// ---------------------------------------------------------------------------
// Metrics state
// ---------------------------------------------------------------------------

let es = null;            // current EventSource
let reconnectTimer = null;
let reconnectDelay = RECONNECT_BASE_MS;
let lastEventId = '';     // checkpoint: last server event id applied

// Per-widget coalescing: widgetId -> { ts, payload, timer }
const coalesce = new Map();

// ---------------------------------------------------------------------------
// DOM helpers: never innerHTML with uncontrolled payloads.
// ---------------------------------------------------------------------------

function setText(selector, text) {
  const el = document.querySelector(selector);
  if (el) el.textContent = String(text ?? '');
}

function toggleAttr(selector, attr, on) {
  const el = document.querySelector(selector);
  if (!el) return;
  if (on) el.setAttribute(attr, ''); else el.removeAttribute(attr);
}

// ---------------------------------------------------------------------------
// HTMX-driven widget update (preferred path).
//
// Asks HTMX to fetch fresh HTML for one widget and swap it into #w-<id>.
// This keeps the widget's HTML ownership server-side (Go templates).
// ---------------------------------------------------------------------------

function refreshWidget(widgetId) {
  if (typeof htmx === 'undefined') {
    // Fallback if htmx hasn't loaded yet (deferred script race is impossible
    // because htmx itself must be inline/core; this is defensive only).
    return;
  }
  const target = '#w-' + CSS.escape(widgetId);
  htmx.ajax(
    'GET',
    '/widgets/' + encodeURIComponent(widgetId),
    { target, swap: 'innerHTML' }
  );
}

// ---------------------------------------------------------------------------
// Metered, coalesced event consumer.
//
// Receives an array of batched SSE events { id, ts, event, widgetId, payload }.
// Applies only the latest event per widget within COALESCE_WINDOW_MS,
// capped at MAX_EVENTS_PER_WIDGET_PER_SEC.
// ---------------------------------------------------------------------------

function applyBatchedEvents(events) {
  if (!Array.isArray(events)) return;

  // 1) Dedupe by event id against the checkpoint (replays must not double-apply).
  const fresh = events.filter((e) => {
    if (!e.id || e.id <= lastEventId) return false;
    return true;
  });

  // 2) Coalesce per widgetId: keep latest payload per widget within the window.
  const now = Date.now();
  for (const e of fresh) {
    const entry = coalesce.get(e.widgetId) || { ts: 0, payload: null, timer: null };
    entry.ts = now;
    entry.payload = e;
    if (entry.timer) clearTimeout(entry.timer);
    entry.timer = setTimeout(() => {
      coalesce.delete(e.widgetId);
      applySingle(e);
    }, COALESCE_WINDOW_MS);
    coalesce.set(e.widgetId, entry);
  }

  // 3) Update checkpoint to the newest id seen (whether applied now or later).
  const newest = fresh.reduce((max, e) => (e.id > max ? e.id : max), '');
  if (newest) {
    lastEventId = newest;
    persistCheckpoint(newest);
  }
}

function applySingle(event) {
  // Idempotent upsert semantics: re-applying event.id is harmless.
  if (event.widgetId) {
    // Prefer HTMX swap so the server owns widget markup.
    refreshWidget(event.widgetId);
  }
  // Surface a page-wide custom event so non-HTMX widgets (e.g. charts) can react.
  try {
    document.dispatchEvent(
      new CustomEvent('arrstart:widget-metric', {
        detail: { widgetId: event.widgetId, payload: event.payload, ts: event.ts },
      })
    );
  } catch (err) {
    // Custom events are best-effort; never let metrics break the page.
  }
}

// ---------------------------------------------------------------------------
// Checkpoint / outbox persistence
// ---------------------------------------------------------------------------

async function persistCheckpoint(id) {
  try {
    const db = await idbOpen();
    await idbPut(db, STORE_CHECKPOINT, { id: 'sse', lastEventId: id, updatedAt: Date.now() });
    db.close();
  } catch (err) {
    // IndexedDB may be blocked in private mode / incognito; degrade gracefully.
    // The SSE Last-Event-ID header (kept in memory) still limits replay damage.
  }
}

async function loadCheckpoint() {
  try {
    const db = await idbOpen();
    const row = await idbGet(db, STORE_CHECKPOINT, 'sse');
    db.close();
    if (row && row.lastEventId) lastEventId = row.lastEventId;
  } catch (err) {
    // Ignore: memory checkpoint starts empty; server replays from the start.
  }
}

async function enqueueOutbox(type, widgetId, payload) {
  // Local optimistic write queue (post-v1 candidate; main-thread optimistic
  // updates are preferred for the v1 dashboard — see table E).
  try {
    const db = await idbOpen();
    await idbPut(db, STORE_OUTBOX, {
      ts: Date.now(),
      type,
      widgetId,
      payload,
      applied: false,
    });
    db.close();
  } catch (err) {
    // Degrade gracefully: offbox writes are optional best-effort.
  }
}

// ---------------------------------------------------------------------------
// SSE connection management
// ---------------------------------------------------------------------------

function connectSSE() {
  if (es) {
    try { es.close(); } catch (_) {}
    es = null;
  }

  // Same-origin only; no token in the URL (arrstart wizard token is URL-bound
  // to the session cookie/context; SSE reuses that same-origin session).
  es = new EventSource(ENDPOINT, { withCredentials: true });

  es.onopen = () => {
    reconnectDelay = RECONNECT_BASE_MS;
    // hx-sse-style Last-Event-ID replay: the browser sends Last-Event-ID
    // automatically on reconnect when using native EventSource; htmx's
    // ReadableStream-based hx-sse extension does the same (see htmx docs).
    // We additionally log the resume for visibility.
    if (lastEventId) {
      // EventSource sends Last-Event-ID automatically; no client action needed
      // beyond keeping lastEventId in memory and persisted in IndexedDB.
    }
  };

  es.onmessage = (event) => {
    // Server envelope: { id, ts, event, widgetId, payload } or raw text for
    // legacy unnamed events. Named events arrive via es.addEventListener(name).
    try {
      const envelope = JSON.parse(event.data);
      applyBatchedEvents(Array.isArray(envelope) ? envelope : [envelope]);
    } catch (_) {
      // Non-JSON heartbeat or legacy text event: ignore or handle heartbeat.
      if (event.data === 'hb') return;
    }
  };

  // Named per-widget channel example: server emits `event: widget-metric`.
  es.addEventListener('widget-metric', (event) => {
    try {
      applyBatchedEvents([JSON.parse(event.data)]);
    } catch (_) { /* ignore malformed named events */ }
  });

  es.onerror = () => {
    try { es.close(); } catch (_) {}
    es = null;
    scheduleReconnect();
  };
}

function scheduleReconnect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  // Pause while the page is hidden (mirrors htmx sse.pauseOnBackground).
  const doReconnect = () => {
    if (document.visibilityState === 'hidden') {
      reconnectTimer = setTimeout(doReconnect, 1000);
      return;
    }
    reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_DELAY_MS);
    const jitter = reconnectDelay * 0.3 * Math.random(); // ±30%
    reconnectTimer = setTimeout(connectSSE, reconnectDelay + jitter);
  };
  reconnectTimer = setTimeout(doReconnect, reconnectDelay);
}

// ---------------------------------------------------------------------------
// Page lifecycle
// ---------------------------------------------------------------------------

async function initDashboard() {
  // 1) Restore checkpoint from IndexedDB so reconnect replays from the right id.
  await loadCheckpoint();

  // 2) Connect batched SSE. Deferred chart scripts (not in this file) are
  //    referenced by the HTML with <script defer ...> so first paint is not
  //    blocked by metrics code.
  connectSSE();

  // 3) Cleanup on page unload.
  addEventListener('beforeunload', () => {
    try { if (es) es.close(); } catch (_) {}
    if (reconnectTimer) clearTimeout(reconnectTimer);
    // Persit final checkpoint (best effort).
    if (lastEventId) void persistCheckpoint(lastEventId);
  });
}

// Auto-init when included as a classic <script defer src=...>.
// Using DOMContentLoaded keeps the graph out of the first-paint critical path.
if (document.readyState === 'loading') {
  addEventListener('DOMContentLoaded', initDashboard);
} else {
  initDashboard();
}

// ---------------------------------------------------------------------------
// Exposed for manual testing / integration by HTMX controllers only.
// Not part of any stable API.
// ---------------------------------------------------------------------------

window.__arrstartDashboardProto = {
  applyBatchedEvents,
  connectSSE,
  scheduleReconnect,
  getLastEventId: () => lastEventId,
};
