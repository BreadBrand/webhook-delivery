# Webhook Delivery Service — Technical Requirements Document
**Date:** 2026-06-15
**Status:** Draft
**Design doc:** `2026-06-15-webhook-delivery-design.md`

---

## Functional Requirements

### FR1 — Webhook Registration

| ID | Requirement |
|---|---|
| FR1.1 | `POST /webhooks` accepts `{ url, circuit_threshold? }` and registers a new webhook endpoint |
| FR1.2 | `url` must parse as `http://` or `https://` scheme with a non-empty host; any other value → 400 with field-level error |
| FR1.3 | `circuit_threshold` must be an integer ≥ 1 when provided; defaults to 5 if absent; invalid value → 400 |
| FR1.4 | System generates a unique 32-byte signing secret per registration using `crypto/rand`, formatted as `sk_<base64url>` |
| FR1.5 | Signing secret is returned **once** in the registration response body and never again |
| FR1.6 | Secret is stored encrypted (AES-256-GCM, server-side key); only a masked hint (last 4 chars, e.g. `sk_…a1b2`) is stored for display |
| FR1.7 | `GET /webhooks` returns all non-deleted webhooks with id, url, status, failure_streak, circuit_threshold, secret_hint, created_at |
| FR1.8 | `DELETE /webhooks/:id` soft-deletes the webhook (sets `status='deleted'`) |
| FR1.9 | On soft-delete, all `pending`, `in_flight`, and `held` deliveries for that webhook are immediately set to `status='failed'` with `last_error='webhook deleted'` |
| FR1.10 | Deleted webhooks do not appear in `GET /webhooks` but their delivery rows are preserved for audit |

**Acceptance criteria — FR1:**
- [ ] Registering with `ftp://bad-url` returns 400
- [ ] Registering with `circuit_threshold=0` returns 400
- [ ] Registration response contains the full `sk_…` secret and a `secret_hint`
- [ ] A second `GET /webhooks` does not reveal the full secret
- [ ] Deleting a webhook with a pending delivery causes that delivery to appear as `failed` within one poll cycle

---

### FR2 — Event Ingestion

| ID | Requirement |
|---|---|
| FR2.1 | `POST /events` accepts a CloudEvents 1.0 JSON payload |
| FR2.2 | Required fields: `specversion`, `id`, `type`, `source`, `time`, `data`; any missing field → 400 naming the missing field(s) |
| FR2.3 | `specversion` must equal `"1.0"`; any other value → 400 |
| FR2.4 | `data` must be a valid JSON object or array; `null` or scalar values → 400 |
| FR2.5 | Request body exceeding 1MB → 413 |
| FR2.6 | Duplicate `id` (event already stored) → 409 with the existing event's `id` and `received_at` in the response |
| FR2.7 | Valid event → stored in `events` table → 202 Accepted |
| FR2.8 | For each non-deleted webhook, a delivery row is created atomically with the event insert |
| FR2.9 | Delivery rows for `circuit_open` webhooks are created with `status='held'`; all others with `status='pending'` and `next_attempt_at=now()` |
| FR2.10 | If no webhooks are registered, event is stored and 202 is returned; no delivery rows created |

**Acceptance criteria — FR2:**
- [ ] Missing `type` field → 400 with `"missing field: type"` (or equivalent)
- [ ] `specversion: "2.0"` → 400
- [ ] `data: null` → 400
- [ ] 1.1MB payload → 413
- [ ] Re-ingesting same `id` → 409
- [ ] Ingesting to a `circuit_open` webhook creates a `held` delivery, not `pending`
- [ ] Ingesting with no webhooks → 202, `SELECT count(*) FROM deliveries` = 0

---

### FR3 — Delivery

| ID | Requirement |
|---|---|
| FR3.1 | A configurable worker pool of goroutines delivers pending events asynchronously |
| FR3.2 | Workers poll `deliveries WHERE status='pending' AND next_attempt_at <= now()` every 500ms |
| FR3.3 | Workers claim rows atomically: `UPDATE deliveries SET status='in_flight' WHERE id=? AND status='pending'`; skip if `rows_affected == 0` |
| FR3.4 | Before attempting delivery, workers re-check webhook status: `circuit_open` → set delivery `held`; `deleted` → set delivery `failed` |
| FR3.5 | Every delivery attempt sends these headers: `X-Webhook-Event-ID`, `X-Webhook-Delivery-Attempt` (1-indexed), `X-Webhook-Signature`, `Content-Type: application/json` |
| FR3.6 | `X-Webhook-Signature` value is `sha256=<hex(HMAC-SHA256(rawBodyBytes, secret))`; the exact same `[]byte` is used for both signing and the HTTP body — no re-serialization |
| FR3.7 | Success = any 2xx HTTP response received within 10 seconds |
| FR3.8 | Failure = non-2xx response, connection error, or timeout |
| FR3.9 | On success: delivery `status='success'`; webhook `failure_streak=0`, `status='active'` |
| FR3.10 | On failure: `attempt` incremented; retry scheduled per backoff table; `failure_streak` incremented |
| FR3.11 | After 5 failed attempts: delivery `status='failed'`, no further automatic retries |
| FR3.12 | Retry backoff: attempt 1→2: 10s; 2→3: 30s; 3→4: 2m; 4→5: 10m; 5th failure: terminal |
| FR3.13 | On server startup, all `in_flight` deliveries are reset to `status='pending'` with `next_attempt_at=now()` before the worker pool starts |
| FR3.14 | After the HTTP call returns, workers re-fetch the webhook status; if `deleted`, the result is discarded (delivery row already set to `failed` by the delete handler) |
| FR3.15 | Each worker goroutine wraps its loop body in `defer recover()`; a panic logs the stack trace, re-queues the delivery (`pending`, `next_attempt_at=now()+10s`), and the goroutine is restarted |

**Acceptance criteria — FR3:**
- [ ] Delivery to a healthy endpoint arrives with all three `X-Webhook-*` headers
- [ ] Receiver can verify `X-Webhook-Signature` using the secret from registration
- [ ] Simulating a 500 response produces retries at approximately 10s, 30s, 2m, 10m intervals
- [ ] After 5 failures, delivery `status='failed'`, no further retries observed
- [ ] Killing and restarting the server with an `in_flight` delivery results in that delivery being retried
- [ ] Two workers started simultaneously cannot double-deliver the same event (verified via receiver log — at most one delivery per attempt number)

---

### FR4 — Circuit Breaker

| ID | Requirement |
|---|---|
| FR4.1 | `failure_streak` increments by 1 on each failed delivery attempt to a webhook |
| FR4.2 | `failure_streak` resets to 0 on any successful delivery |
| FR4.3 | When `failure_streak >= circuit_threshold`: webhook `status='circuit_open'`, `next_probe_at=now()+5min` |
| FR4.4 | While `circuit_open`: delivery rows for new events are created with `status='held'`; worker skips `held` rows |
| FR4.5 | When `now() >= next_probe_at`: worker selects the oldest `held` delivery and attempts it as a probe; attempt counter is not incremented |
| FR4.6 | Probe success: circuit closes — webhook `status='active'`, `failure_streak=0`, `next_probe_at=NULL`; all `held` deliveries → `pending` in batches of 10 |
| FR4.7 | Probe failure: webhook stays `circuit_open`; `next_probe_at = now()+5min` |
| FR4.8 | `POST /webhooks/:id/circuit` with `{ "action": "open" }` immediately sets `status='circuit_open'`; new events → `held` |
| FR4.9 | `POST /webhooks/:id/circuit` with `{ "action": "close" }` triggers the same circuit-close flow as a successful probe, regardless of `failure_streak` |
| FR4.10 | Manual override does not reset `failure_streak` |
| FR4.11 | Webhook `status` transitions through: `active` → `degraded` (failure_streak > 0 but < threshold) → `circuit_open` |

**Acceptance criteria — FR4:**
- [ ] 5 consecutive failures to a webhook → `status='circuit_open'` in dashboard
- [ ] Events ingested while circuit is open appear as `held` in delivery log
- [ ] After simulated recovery, probe fires within ~5min and closes circuit
- [ ] All held deliveries appear as `pending` (then `success`) after circuit closes
- [ ] Manual open via dashboard → subsequent events go to `held`
- [ ] Manual close via dashboard → held events begin delivering immediately

---

### FR5 — Re-delivery

| ID | Requirement |
|---|---|
| FR5.1 | `POST /deliveries/:id/redeliver` is only valid when the target delivery has `status='failed'`; other statuses → 409 |
| FR5.2 | Before creating a re-delivery, system checks for an existing delivery with `status IN ('pending','in_flight')` for the same `(event_id, webhook_id)` with a non-null `parent_delivery_id`; if found → 409 with the existing re-delivery ID |
| FR5.3 | Creates a new delivery row: `attempt=0`, `next_attempt_at=now()`, `parent_delivery_id=<original id>` |
| FR5.4 | Original failed delivery row is not modified |
| FR5.5 | Returns 202 with the new delivery ID |

**Acceptance criteria — FR5:**
- [ ] Re-delivering a `success` delivery → 409
- [ ] Re-delivering a `failed` delivery → 202, new delivery row visible in log
- [ ] Clicking "Re-deliver Now" twice in rapid succession → second click returns 409 with existing re-delivery ID
- [ ] Re-delivery sends all standard headers including `X-Webhook-Delivery-Attempt: 1`

---

### FR6 — Dashboard

| ID | Requirement |
|---|---|
| FR6.1 | `GET /stream` serves an SSE stream; connection kept alive until client disconnects |
| FR6.2 | SSE events emitted: `webhook_updated`, `event_ingested`, `delivery_updated` |
| FR6.3 | Each SSE client gets a buffered channel (size 64); sends are non-blocking — events are dropped for stalled clients rather than blocking the broadcaster |
| FR6.4 | `GET /events?limit=N` returns the N most recent events (default 50) |
| FR6.5 | `GET /deliveries?limit=N` returns the N most recent delivery rows (default 100), joined with event type and webhook URL |
| FR6.6 | `GET /events/volume?window=<5m\|30m\|1h\|24h>` returns event counts grouped by `type` over the specified window; default `30m` |
| FR6.7 | Dashboard hydrates from FR6.4–FR6.6 on mount and on every SSE reconnect before re-opening the stream |
| FR6.8 | Webhook Registry panel shows: URL, status badge, masked secret hint, failure streak, circuit breaker toggle |
| FR6.9 | Recent Events panel shows: scrolling feed of type, source, time, truncated payload preview |
| FR6.10 | Delivery Log panel shows: event ID, webhook URL, attempt #, status, HTTP status code, latency ms, timestamp; "Re-deliver Now" button on `failed` rows |
| FR6.11 | Endpoint Health panel shows per-webhook: success rate %, failure streak, avg response time ms, circuit state, last delivery time |
| FR6.12 | Event Volume panel shows bar or pie chart by `type`; window dropdown (5m, 30m, 1h, 24h); updates in real-time via SSE |
| FR6.13 | In development, React runs on Vite `:5173` with a proxy to Go `:8080`; in production, built assets are embedded in the Go binary via `embed.FS` |

**Acceptance criteria — FR6:**
- [ ] Opening the dashboard with no events shows empty states (not errors)
- [ ] Ingesting an event causes it to appear in the Recent Events feed within 2s without a page refresh
- [ ] Circuit breaker badge updates in real-time when circuit trips
- [ ] Closing and reopening the browser tab shows correct current state (hydration works)
- [ ] "Re-deliver Now" button absent on `success` rows, present on `failed` rows
- [ ] Event volume chart updates in real-time as events arrive
- [ ] Switching window dropdown to `1h` updates the chart immediately

---

### FR7 — Simulation Tool

| ID | Requirement |
|---|---|
| FR7.1 | `go run ./cmd/simulate` starts a self-contained, continuous simulation |
| FR7.2 | Flags: `--receivers N` (default 5), `--failure-rate F` (default 0.3), `--event-rate R` events/sec (default 2), `--server URL` (default `http://localhost:8080`) |
| FR7.3 | Starts N local HTTP servers on random available ports and registers them as webhooks |
| FR7.4 | Fires CloudEvents continuously with at least 4 distinct `type` values (e.g. `order.created`, `payment.failed`, `user.signup`, `inventory.updated`) |
| FR7.5 | A configurable fraction of receivers return HTTP 500 on receipt to drive retry and circuit breaker behavior |
| FR7.6 | Logs a summary line per event: event ID, type, number of webhooks targeted |
| FR7.7 | On Ctrl+C: deregisters all registered webhooks, shuts down mock receivers, exits cleanly |

**Acceptance criteria — FR7:**
- [ ] Running `go run ./cmd/simulate` with the server running populates all 5 dashboard panels with data within 60 seconds
- [ ] Dashboard shows at least one `circuit_open` webhook within ~2 minutes of simulator start
- [ ] Ctrl+C produces clean shutdown with no registered webhooks remaining

---

### FR8 — Authentication

| ID | Requirement |
|---|---|
| FR8.1 | All API endpoints (including `GET /stream`) require `Authorization: Bearer <API_KEY>` |
| FR8.2 | Missing or incorrect key → 401 Unauthorized |
| FR8.3 | `GET /health` is exempt from authentication |
| FR8.4 | `API_KEY` is read from the environment at startup; server refuses to start if absent or empty |

**Acceptance criteria — FR8:**
- [ ] `curl /events` without auth header → 401
- [ ] `curl /events` with wrong key → 401
- [ ] `curl /health` without auth header → 200
- [ ] Starting server without `API_KEY` set → process exits with a clear fatal error message

---

## Non-Functional Requirements

### NFR1 — Startup

| ID | Requirement |
|---|---|
| NFR1.1 | `WEBHOOK_ENCRYPTION_KEY` must be present and decode to exactly 32 bytes; server exits fatally if not |
| NFR1.2 | `API_KEY` must be present and non-empty; server exits fatally if not |
| NFR1.3 | SQLite connection must open successfully with WAL mode confirmed; server exits fatally if not |
| NFR1.4 | `in_flight` delivery sweep (FR3.13) must complete before the HTTP server begins accepting requests |

### NFR2 — Performance

| ID | Requirement |
|---|---|
| NFR2.1 | Webhook delivery attempts must timeout after 10 seconds |
| NFR2.2 | Worker pool must poll at most every 500ms |
| NFR2.3 | Circuit close flush processes held deliveries in batches of 10 to prevent thundering herd |
| NFR2.4 | SSE broadcaster must not block on slow clients (non-blocking send, drop-on-full) |
| NFR2.5 | SQLite connection string must include `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` |

### NFR3 — Security

| ID | Requirement |
|---|---|
| NFR3.1 | Signing secrets are encrypted at rest with AES-256-GCM using a server-side key |
| NFR3.2 | The raw signing secret is never stored, logged, or returned after the initial registration response |
| NFR3.3 | There is no endpoint to retrieve or reveal a stored secret |
| NFR3.4 | The SQLite database file must be created with permissions `0600` |
| NFR3.5 | `WEBHOOK_ENCRYPTION_KEY` must never be logged |

### NFR4 — Reliability

| ID | Requirement |
|---|---|
| NFR4.1 | The system provides at-least-once delivery; duplicate delivery is possible and expected |
| NFR4.2 | Every delivery worker goroutine must recover from panics without crashing the process |
| NFR4.3 | A recovered goroutine must re-queue the in-progress delivery before exiting |
| NFR4.4 | A supervisor must restart panicked worker goroutines |

### NFR5 — Observability

| ID | Requirement |
|---|---|
| NFR5.1 | Structured log line on every delivery attempt: event_id, webhook_id, attempt, http_status, latency_ms, error (if any) |
| NFR5.2 | `GET /health` returns 200 with JSON: server uptime, DB connectivity status, worker pool goroutine count, pending delivery count |

---

## API Contract Summary

### Endpoints

| Method | Path | Auth | Request | Success | Error |
|---|---|---|---|---|---|
| `POST` | `/webhooks` | ✓ | `{url, circuit_threshold?}` | 201 + webhook + secret | 400, 401 |
| `GET` | `/webhooks` | ✓ | — | 200 + array | 401 |
| `DELETE` | `/webhooks/:id` | ✓ | — | 204 | 401, 404 |
| `POST` | `/webhooks/:id/circuit` | ✓ | `{action: "open"\|"close"}` | 200 + webhook | 400, 401, 404 |
| `POST` | `/events` | ✓ | CloudEvents JSON | 202 + event | 400, 401, 409, 413 |
| `GET` | `/events` | ✓ | `?limit=N` | 200 + array | 401 |
| `GET` | `/events/volume` | ✓ | `?window=5m\|30m\|1h\|24h` | 200 + `{type: count}` | 401 |
| `GET` | `/deliveries` | ✓ | `?limit=N` | 200 + array | 401 |
| `POST` | `/deliveries/:id/redeliver` | ✓ | — | 202 + new delivery | 401, 404, 409 |
| `GET` | `/stream` | ✓ | — | SSE stream | 401 |
| `GET` | `/health` | — | — | 200 + status JSON | — |

### Delivery Payload Shape

```json
{
  "specversion": "1.0",
  "id": "01J8K2M3N4P5Q6R7S8T9U0V1W2",
  "type": "order.created",
  "source": "https://example.com/orders",
  "time": "2026-06-15T14:30:00Z",
  "data": { "order_id": 42, "total": 99.99 }
}
```

### Delivery Headers (sent on every attempt)

```
Content-Type: application/json
X-Webhook-Event-ID: 01J8K2M3N4P5Q6R7S8T9U0V1W2
X-Webhook-Delivery-Attempt: 2
X-Webhook-Signature: sha256=3b4c5d6e7f8a9b0c...
```

### Webhook Status Values

| Status | Meaning |
|---|---|
| `active` | Delivering normally, `failure_streak=0` |
| `degraded` | Recent failures but below `circuit_threshold` |
| `circuit_open` | Auto or manually tripped; deliveries held |
| `deleted` | Soft-deleted; excluded from registry; deliveries aborted |

### Delivery Status Values

| Status | Meaning |
|---|---|
| `pending` | Waiting to be picked up by a worker |
| `in_flight` | Claimed by a worker, HTTP call in progress |
| `success` | 2xx received within 10s |
| `failed` | All attempts exhausted, or webhook deleted |
| `held` | Circuit is open; waiting for circuit to close |

---

## Explicit Out-of-Scope

All of the following are confirmed out of scope:

| Item |
|---|
| Multi-tenancy / per-client API key isolation |
| Horizontal scaling / clustering |
| Guaranteed delivery ordering within a webhook's queue (FIFO best-effort only) |
| Webhook URL migration / retargeting |
| Bulk event replay (single Re-deliver Now only) |
| Event filtering or routing rules (all events delivered to all active webhooks) |

---

## Data Model Summary

### `webhooks`
```
id TEXT PK, url TEXT, encrypted_secret TEXT, secret_hint TEXT,
status TEXT, failure_streak INT, circuit_threshold INT,
next_probe_at DATETIME, created_at DATETIME, updated_at DATETIME
```

### `events`
```
id TEXT PK, type TEXT, source TEXT, time DATETIME,
data TEXT, received_at DATETIME
```

### `deliveries`
```
id TEXT PK, event_id TEXT FK, webhook_id TEXT FK,
parent_delivery_id TEXT NULLABLE,
status TEXT, attempt INT, next_attempt_at DATETIME,
last_status_code INT, last_response_ms INT, last_error TEXT,
created_at DATETIME, updated_at DATETIME

UNIQUE(event_id, webhook_id) WHERE parent_delivery_id IS NULL
```
