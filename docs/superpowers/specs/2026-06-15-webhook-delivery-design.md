# Webhook Delivery Service — Design Doc
**Date:** 2026-06-15
**Status:** Draft

---

## Problem Statement

Build a webhook delivery service that ingests CloudEvents and fans them out to registered subscriber endpoints. The system must demonstrate real production concerns: at-least-once delivery, retry with exponential backoff, per-endpoint circuit breaking, HMAC payload signing, and a live observability dashboard.

This is a Droplet take-home assignment. The goal is to show engineering taste, production awareness, and ability to build a complete, polished system within a 2–6 hour budget.

---

## Goals

1. Register webhook endpoint URLs, each with a unique per-endpoint HMAC signing secret
2. Ingest CloudEvents-format events via HTTP with validation
3. Deliver events asynchronously to all active registered endpoints
4. Guarantee at-least-once delivery with exponential backoff (up to 5 attempts)
5. Implement a circuit breaker per endpoint (auto-trip + manual toggle) that holds queued events
6. Include idempotency and signing headers on every delivery attempt so receivers can safely deduplicate retries
7. Serve a live React dashboard via SSE: webhook registry, event feed, delivery log, endpoint health cards, event volume chart
8. Ship a self-contained continuous simulation tool that keeps the dashboard populated with live data

---

## Non-Goals

- Multi-tenancy / per-client isolation (single-tenant, one API key)
- Guaranteed delivery ordering across endpoints or within a webhook's queue (FIFO best-effort)
- Horizontal scaling / clustering (single Go process, single SQLite file)
- Webhook URL migration or retargeting
- Event replay beyond the "Re-deliver Now" single-delivery trigger
- Event filtering or routing rules (all events → all active webhooks)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Go HTTP Server (:8080)                                      │
│                                                              │
│  POST   /events                 ← ingest CloudEvent          │
│  POST   /webhooks               ← register endpoint          │
│  GET    /webhooks               ← list all endpoints         │
│  DELETE /webhooks/:id           ← remove endpoint            │
│  POST   /webhooks/:id/circuit   ← manual open/close         │
│  POST   /deliveries/:id/redeliver ← manual re-delivery      │
│  GET    /stream                 ← SSE for dashboard          │
│  GET    /health                 ← system health check        │
│                                                              │
│  Static file handler for embedded React build (production)   │
└──────────────────────┬──────────────────────────────────────┘
                       │ read/write
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  SQLite (WAL mode, single file: data/webhooks.db)           │
│  DSN: file:data/webhooks.db                                  │
│    ?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)     │
│  tables: events, webhooks, deliveries                        │
└──────────────────────┬──────────────────────────────────────┘
                       │ polls
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Delivery Worker Pool (N goroutines, N configurable)        │
│  - Poll deliveries WHERE status=pending AND next_attempt ≤ now │
│  - Check circuit breaker state per webhook                  │
│  - Send HTTP POST with signing + idempotency headers        │
│  - Update delivery row on success or failure                │
│  - Update webhook failure_streak and circuit state          │
│  - Half-open probe scheduling                               │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  SSE Broadcaster                                            │
│  - In-process pub/sub (sync.Map of buffered channels)       │
│  - Non-blocking send: drop event if client channel is full  │
│  - Publishes: webhook_updated, event_ingested, delivery_updated │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  React Dashboard (Vite :5173 in dev, embedded in prod)      │
│  - EventSource('/stream') for live updates                  │
│  - 5 panels: Webhooks, Events, Deliveries, Health, Volume   │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Simulation Tool (go run ./cmd/simulate)                    │
│  - Starts N mock HTTP receivers (configurable failure rate) │
│  - Registers them as webhooks                               │
│  - Fires CloudEvents continuously (varied types)            │
│  - Runs indefinitely; Ctrl+C to stop                        │
└─────────────────────────────────────────────────────────────┘
```

---

## Data Models

### `webhooks` table

| Column             | Type    | Notes |
|--------------------|---------|-------|
| `id`               | TEXT PK | UUID v4 |
| `url`              | TEXT    | Target endpoint URL |
| `encrypted_secret` | TEXT    | AES-GCM encrypted signing secret (base64) |
| `secret_hint`      | TEXT    | Last 4 chars of raw secret, e.g. `sk_…a1b2` |
| `status`           | TEXT    | `active` \| `degraded` \| `circuit_open` |
| `failure_streak`   | INT     | Consecutive delivery failures |
| `circuit_threshold`| INT     | Failures before auto-trip (default: 5) |
| `next_probe_at`    | DATETIME| When to run the half-open probe (NULL if not tripped) |
| `created_at`       | DATETIME| |
| `updated_at`       | DATETIME| |

### `events` table

| Column       | Type    | Notes |
|--------------|---------|-------|
| `id`         | TEXT PK | CloudEvents `id` (client-supplied or server-generated UUID) |
| `type`       | TEXT    | CloudEvents `type` (used for volume chart) |
| `source`     | TEXT    | CloudEvents `source` |
| `time`       | DATETIME| CloudEvents `time` |
| `data`       | TEXT    | JSON blob (CloudEvents `data`) |
| `received_at`| DATETIME| Server ingest timestamp |

### `deliveries` table

| Column            | Type    | Notes |
|-------------------|---------|-------|
| `id`              | TEXT PK | UUID v4 |
| `event_id`        | TEXT FK | → `events.id` |
| `webhook_id`      | TEXT FK | → `webhooks.id` |
| `status`          | TEXT    | `pending` \| `in_flight` \| `success` \| `failed` \| `held` |
| `attempt`         | INT     | 0-indexed, increments per attempt |
| `next_attempt_at` | DATETIME| When worker should next pick this up |
| `last_status_code`| INT     | HTTP status from last attempt (NULL if no attempt yet) |
| `last_response_ms`| INT     | Response latency in ms (NULL if no attempt yet) |
| `last_error`      | TEXT    | Error message if non-HTTP failure (timeout, DNS, etc.) |
| `created_at`      | DATETIME| |
| `updated_at`      | DATETIME| |

**Constraint:** `UNIQUE(event_id, webhook_id)` — prevents duplicate delivery rows from concurrent ingest of the same event ID.

---

## Startup Validation

On server start, before accepting any requests:

1. **`WEBHOOK_ENCRYPTION_KEY`** — must be set and exactly 32 bytes when base64-decoded. If missing or wrong length, log a fatal error and refuse to start. Never fall back to a zero key or a derived default.
2. **`API_KEY`** — must be set and non-empty. Same fatal failure if absent.
3. **SQLite WAL pragma** — open the database connection and verify the journal_mode returned is `wal`. Log a warning if not (e.g., read-only filesystem).

---

## Authentication

All API endpoints require:
```
Authorization: Bearer <API_KEY>
```
Requests missing the header or with an incorrect key receive `401 Unauthorized`. The dashboard Vite proxy forwards the key automatically in dev; in production the embedded frontend reads it from a `/config` endpoint served by Go at startup.

---

## Signing Secrets

Each webhook registration generates a random 32-byte secret via `crypto/rand`, formatted as `sk_<base64url>`.

**Storage:** The raw secret is encrypted with AES-256-GCM using a server-side key (`WEBHOOK_ENCRYPTION_KEY` env var, 32 bytes). The ciphertext (base64) is stored in `encrypted_secret`. This is retrievable at delivery time by decrypting with the same key.

**Why not hash?** Signing requires computing `HMAC-SHA256(payload, raw_secret)` at delivery time, which requires the raw secret. A one-way hash is not reversible. AES-GCM encryption is the correct primitive here — it provides confidentiality at rest while allowing retrieval.

**Dashboard display:** Shows `sk_…a1b2` (masked hint). The full secret is shown **once** in the registration response. There is no "reveal" endpoint; if lost, the user regenerates a new secret.

**Signature correctness invariant:** The HMAC must be computed over the exact raw bytes sent as the HTTP body — the same `[]byte` value, not a re-serialization of the same struct. JSON marshaling is not guaranteed to produce identical byte sequences across calls (map key ordering, float formatting). Serialize once, sign that, send that. Any divergence between what is signed and what is sent will cause receiver verification to fail silently.

---

## Delivery Flow

### Ingestion

```
POST /events
  Body: { specversion, id, type, source, time, data, ... }

1. Validate required CloudEvents fields present: specversion, id, type, source, time, data
2. Enforce specversion == "1.0" → 400 if any other value (future versions handled explicitly)
3. Validate data is valid JSON object or array; reject null or scalar → 400
4. Reject body > 1MB → 413
5. INSERT INTO events (conflict on id → 409 with existing event id in response)
6. For each webhook WHERE status NOT IN ('deleted'):
     status == 'circuit_open' → INSERT with status='held'
     otherwise              → INSERT with status='pending', next_attempt_at=now()
     ON CONFLICT (event_id, webhook_id) DO NOTHING
7. SSE broadcast: event_ingested
8. Return 202 Accepted

POST /webhooks
  Body: { url, circuit_threshold? }

1. Validate url: must parse as http:// or https:// scheme, non-empty host → 400 otherwise
2. Validate circuit_threshold: if provided, must be integer >= 1 → 400 otherwise
```

### Startup Recovery

On server start, before the worker pool begins polling:
```
UPDATE deliveries SET status='pending', next_attempt_at=now()
WHERE status = 'in_flight'
```
Any deliveries stuck `in_flight` from a prior crash are immediately re-queued. This is safe because at-least-once delivery allows re-sending; receivers deduplicate via `X-Webhook-Event-ID`.

### Worker Delivery Loop

Each worker goroutine wraps its entire loop body in `defer recover()`. A panic logs the stack trace and re-queues the delivery (`status='pending'`, `next_attempt_at=now()+10s`) before the goroutine exits and is restarted by a supervisor.

```
Every 500ms, worker pool polls:
  BEGIN IMMEDIATE
  SELECT * FROM deliveries
  WHERE status = 'pending'
    AND next_attempt_at <= now()
  LIMIT 50

  For each candidate row:
    -- CAS claim: only one worker wins per row
    UPDATE deliveries SET status='in_flight'
    WHERE id = ? AND status = 'pending'
    -- if rows_affected == 0: another worker claimed it, skip
  COMMIT

For each claimed delivery:
  1. Fetch webhook; check status:
       circuit_open → UPDATE delivery SET status='held'; continue
       deleted      → UPDATE delivery SET status='failed', last_error='webhook deleted'; continue
  2. Build HTTP request:
       a. Serialize the CloudEvents payload to []byte ONCE (json.Marshal)
       b. Compute signature over those exact bytes: HMAC-SHA256(rawBytes, secret)
          — never re-serialize or reconstruct from a struct after this point
       c. Build request:
            POST <webhook.url>
            Content-Type: application/json
            X-Webhook-Event-ID: <event.id>
            X-Webhook-Delivery-Attempt: <attempt + 1>   (1-indexed for humans)
            X-Webhook-Signature: sha256=<hex(hmac)>
            Body: rawBytes (the exact same []byte used for signing)
  4. Send with 10s timeout
  5. After HTTP call returns, re-fetch webhook status:
       if webhook.status == 'deleted': discard result, do nothing (deletion already set delivery to failed)
  5a. Success (2xx):
       UPDATE delivery SET status='success', last_status_code, last_response_ms
       UPDATE webhook SET failure_streak=0, status='active'
       SSE broadcast: delivery_updated
  5b. Failure (non-2xx, timeout, connection error):
       attempt++
       UPDATE webhook SET failure_streak++
       IF failure_streak >= circuit_threshold:
         UPDATE webhook SET status='circuit_open', next_probe_at=now()+5min
       ELSE IF failure_streak > 0:
         UPDATE webhook SET status='degraded'
       IF attempt >= 5:
         UPDATE delivery SET status='failed'
       ELSE:
         UPDATE delivery SET status='pending', next_attempt_at=now()+backoff(attempt)
       SSE broadcast: delivery_updated, webhook_updated
```

### Backoff Schedule

| Attempt | Delay before retry |
|---------|--------------------|
| 1 → 2   | 10 seconds |
| 2 → 3   | 30 seconds |
| 3 → 4   | 2 minutes |
| 4 → 5   | 10 minutes |
| 5th fails | status=`failed`, no more retries |

---

## Circuit Breaker

### State Machine

```
         [failure_streak >= threshold]
  active ──────────────────────────────► circuit_open
    ▲                                         │
    │  [probe succeeds]              [cooldown 5min → probe]
    │                                         ▼
    └─────────────────────────────── half-open (one delivery attempt)
                                              │
                                    [probe fails]
                                              │
                                              ▼
                                    circuit_open (reset next_probe_at)
```

### Probe Mechanics

- When `now() >= next_probe_at` and webhook is `circuit_open`:
  - Select the **oldest `held` delivery** for this webhook
  - Attempt delivery (does not consume a retry slot — attempt counter is not incremented for the probe)
  - On success: close circuit (see below)
  - On failure: stay `circuit_open`, set `next_probe_at = now() + 5min`

### Closing the Circuit

When circuit closes (probe success or manual close via dashboard):
1. `UPDATE webhook SET status='active', failure_streak=0, next_probe_at=NULL`
2. Find all `held` deliveries for this webhook, ordered by `created_at ASC`
3. `UPDATE SET status='pending', next_attempt_at=now()` in batches of 10
4. Worker picks them up at its normal poll rate — no special flush logic needed; the batch size prevents thundering herd

### Manual Override

`POST /webhooks/:id/circuit` with body `{ "action": "open" | "close" }`
- `open`: immediately sets `status=circuit_open`, all new deliveries go to `held`
- `close`: triggers the same circuit-close flow above, regardless of `failure_streak`
- Manual override does not reset `failure_streak` — the streak reflects reality even if the operator intervened

---

## Idempotency Headers

Every delivery attempt sends:

```
X-Webhook-Event-ID: 01J8K2M3N4P5Q6R7S8T9     ← CloudEvents id, constant across retries
X-Webhook-Delivery-Attempt: 2                  ← 1-indexed, increments per retry
X-Webhook-Signature: sha256=abc123def456...    ← HMAC-SHA256(body, raw_secret)
Content-Type: application/json
```

`X-Webhook-Event-ID` is the stable deduplication key. Receivers store it and discard any delivery where the ID was already processed. `X-Webhook-Delivery-Attempt` lets them log which attempt arrived, without needing to compare payloads.

---

## Re-deliver Now

`POST /deliveries/:id/redeliver`

- Only valid on deliveries with `status='failed'`
- Before creating a new row, check for an existing non-failed re-delivery for the same `(event_id, webhook_id)`: if one exists with `status IN ('pending','in_flight')`, return 409 with the existing re-delivery ID — do not create a duplicate
- Creates a **new delivery row** (new UUID, `attempt=0`, `next_attempt_at=now()`, `parent_delivery_id=<original id>`) for the same `(event_id, webhook_id)` pair
- The original failed delivery row is preserved for audit history
- Returns 202 with the new delivery ID

> **Implementation note:** The UNIQUE constraint `UNIQUE(event_id, webhook_id) WHERE parent_delivery_id IS NULL` applies only to original deliveries. Re-deliveries carry a non-null `parent_delivery_id` and are exempt, but the 409 guard above prevents duplicate in-flight re-deliveries at the application level.

---

## Dashboard (React + SSE)

### Initial State Hydration

SSE only delivers deltas after connection. On mount, the dashboard fetches current state via REST before opening the SSE stream:

| Endpoint | Used by |
|---|---|
| `GET /webhooks` | Webhook registry panel, endpoint health cards |
| `GET /events?limit=50` | Recent events feed |
| `GET /deliveries?limit=100` | Delivery log |
| `GET /events/volume?window=30m` | Event volume chart (aggregated counts by type) |

After hydration, the SSE stream keeps state current. On reconnect, the dashboard re-fetches all four endpoints to fill any gap, then re-opens the stream.

### SSE Event Types

The server sends newline-delimited SSE from `GET /stream`:

```
event: webhook_updated
data: { "id": "...", "status": "circuit_open", "failure_streak": 5, ... }

event: event_ingested
data: { "id": "...", "type": "order.created", "source": "...", "time": "..." }

event: delivery_updated
data: { "id": "...", "status": "failed", "webhook_id": "...", "attempt": 3, ... }
```

React uses `new EventSource('/stream')` and dispatches updates into local state (no Redux; `useReducer` or Zustand is sufficient).

Each client connection gets a **buffered channel (size 64)**. The broadcaster uses non-blocking sends — if a client's channel is full (slow/stalled browser), the event is dropped for that client. The client will resync on the next reconnect via REST hydration. This prevents one dead tab from back-pressuring delivery status updates for all other clients.

### Dashboard Panels

| Panel | Content |
|---|---|
| **Webhook Registry** | URL, status badge (active/degraded/circuit_open), masked secret hint, failure streak, circuit breaker toggle button |
| **Recent Events** | Scrolling feed of ingested CloudEvents: type, source, time, payload preview (truncated JSON) |
| **Delivery Log** | Table: event ID, webhook URL, attempt #, status, HTTP code, latency ms, timestamp; "Re-deliver Now" button on `failed` rows |
| **Endpoint Health** | Per-webhook card: success rate %, failure streak, avg response time ms, circuit state, last delivery time |
| **Event Volume** | Bar or pie chart of event count by `type` over the last N minutes (configurable window) |

---

## Simulation Tool

`go run ./cmd/simulate [flags]`

**Flags:**
- `--receivers N` — number of mock HTTP receiver servers to start (default: 5)
- `--failure-rate F` — fraction of receivers that fail randomly (default: 0.3)
- `--event-rate R` — events per second to fire (default: 2)
- `--server URL` — webhook delivery service base URL (default: http://localhost:8080)

**Behavior:**
1. Starts N local HTTP servers on random ports
2. Registers them as webhooks via `POST /webhooks`
3. Loops indefinitely, firing CloudEvents with varied `type` values (e.g., `order.created`, `payment.failed`, `user.signup`, `inventory.updated`)
4. Receiver servers that are in the "failing" pool return 500 randomly
5. Logs a summary line per event: `→ event_id [type] delivered to N webhooks`
6. Ctrl+C deregisters webhooks and shuts down receivers cleanly

---

## Use Cases and Edge Cases

### Happy Path

1. Register webhook → get secret in response, masked hint stored
2. Ingest event → 202, delivery rows created, worker delivers within 1s, dashboard updates live

### Retry Path

1. Endpoint returns 500 → delivery retries at 10s, 30s, 2m, 10m, 30m
2. After 5 failures → delivery `status=failed`, "Re-deliver Now" button appears in dashboard

### Circuit Breaker Path

1. 5 consecutive failures → circuit trips, new events go to `held`
2. Dashboard shows circuit_open badge + toggle
3. After 5min cooldown → probe fires (oldest held delivery)
4. Probe succeeds → circuit closes, all held events flush to `pending`
5. Probe fails → cooldown resets, try again in 5min
6. Manual toggle → same close/open flow, bypasses cooldown

### Idempotency Path

1. Delivery succeeds but the response times out on our end → we retry anyway
2. Receiver gets same event twice with same `X-Webhook-Event-ID`
3. Receiver deduplicates, processes only once

### Ingest Edge Cases

- Duplicate CloudEvents `id` → 409 Conflict with existing event details
- Missing required fields → 400 with field-level errors
- Payload > 1MB → 413
- No registered webhooks → 202, event stored, no deliveries created

### Circuit Breaker Edge Cases

- Circuit trips mid-retry (delivery at attempt 2, waiting for `next_attempt_at`) → when worker picks it up, circuit is open → transitions to `held`
- Manual close while probe is pending → cancel pending probe, flush queue
- All webhooks circuit-open when event ingested → create delivery rows with `status=held` immediately (skip `pending`)

---

## Resolved Decisions

1. **`status=held` at ingest time:** When a webhook is `circuit_open` at event ingestion, delivery rows are created with `status=held` directly — the worker is never involved. This avoids unnecessary poll cycles and makes circuit state the authoritative gate at ingest time.

2. **Event volume chart window:** Default to 30-minute rolling window, updating in real-time via SSE. A dropdown in the UI allows switching to 5min, 30min, 1hr, 24hr windows.

3. **Webhook deletion semantics:** Soft-delete only. On `DELETE /webhooks/:id`:
   - Set `webhooks.status = 'deleted'`
   - Any `pending` or `in_flight` deliveries for this webhook are immediately aborted: `UPDATE deliveries SET status='failed', last_error='webhook deleted' WHERE webhook_id = :id AND status IN ('pending','in_flight','held')`
   - Historical delivery rows are preserved for audit
   - Deleted webhooks do not appear in the registry panel but are visible in the delivery log for traceability

---

## Stress Test: School Internet Outage

*Real example used to validate this design before presenting.*

**Setup:** `wh_001` registered at `https://school.example.com/hooks`, `circuit_threshold=5`. Simulator fires ~6 events/min for 8 minutes. Internet goes down at t=30s, recovers at t=6m.

**Walk-through:**

- **t=0–30s:** 3 events arrive, all delivered successfully. `failure_streak=0`, `status=active`.
- **t=30s:** Event 4 arrives. Delivery attempt fails (internet down). `failure_streak=1`, `status=degraded`. Retry in 10s.
- **t=40s:** Retry of event 4 fails. `failure_streak=2`. Retry in 30s.
- **t=1m10s:** Retry fails. `failure_streak=3`. Retry in 2min.
- Meanwhile events 5–6 arrive, fail immediately, streak → 5 on event 6's first attempt. **Circuit trips: `status=circuit_open`, `next_probe_at = now()+5min`.**
- **t=1m10s–6m:** 25 more events arrive. Delivery rows created as `held` directly (circuit open). Dashboard shows circuit_open badge.
- **t=6m10s (5min after trip):** Worker checks `next_probe_at <= now()`. Selects oldest `held` delivery (event 4's pending retry). Attempts delivery.
- **t=6m10s:** Probe succeeds (internet back). Circuit closes. All ~25 `held` deliveries → `pending` in batches of 10. Worker processes them at normal rate.

**What the stress test caught and fixed:**

| Issue | Fix Applied |
|---|---|
| "Store hashed" is incompatible with signing | Changed to AES-GCM encrypted storage |
| Which delivery is the probe? | Oldest `held` delivery; probe doesn't consume retry slot |
| Race on duplicate event IDs | Added `UNIQUE(event_id, webhook_id) WHERE parent_delivery_id IS NULL` |
| Thundering herd on circuit close | Batch of 10 per flush, worker rate naturally throttles |
| Circuit trips mid-retry-cycle | Worker checks circuit state on every pickup, transitions to `held` |
| `in_flight` rows orphaned on crash | Startup sweep resets all `in_flight` → `pending` |
| Two workers claiming same delivery | CAS `UPDATE … WHERE id=? AND status='pending'`; check rows_affected |
| Worker panic kills process | `defer recover()` per goroutine; re-queues delivery, goroutine restarted |
| Goroutine completes after webhook deleted | Re-fetch webhook status post-HTTP-call; discard result if `deleted` |
| Rapid re-delivery creates duplicates | 409 if an active re-delivery already exists for same `(event_id, webhook_id)` |
| SSE reconnect loses events | REST hydration on mount and reconnect before re-opening stream |
| Slow client blocks broadcaster | Buffered channels (64), non-blocking send, drop-on-full per client |
| `WEBHOOK_ENCRYPTION_KEY` missing | Fatal startup validation; server refuses to start |
| API key header undefined | Specified: `Authorization: Bearer <API_KEY>` |
| Invalid webhook URL registered | Validate `http/https` scheme + non-empty host at `POST /webhooks` |
| `specversion` not enforced | Enforce `"1.0"` at ingest; 400 on unknown versions |
