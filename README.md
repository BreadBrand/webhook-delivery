# Droplet Webhook Delivery Service

A self-contained webhook delivery system with circuit breaker, retry scheduling, SSE-driven live dashboard, and a simulation tool for generating realistic traffic. Ships as a single binary — no Node, no external database.

---

## Running it

### Option 1 — Pre-built binary (no Go or Node required)

Download `webhook-delivery-release.zip` from the [latest release](https://github.com/BreadBrand/webhook-delivery/releases).

| Platform | File to run |
|---|---|
| **macOS (Apple Silicon, 2020+)** | Double-click `open-macos-arm64.command` |
| **macOS (Intel)** | Double-click `open-macos-amd64.command` |
| **Windows** | Double-click `webhook-server-windows-amd64.exe` |
| **Linux** | `./run-linux.sh` |

The server starts, opens **http://localhost:8080** in your browser, and begins generating live traffic automatically. The dashboard populates within seconds.

> **macOS — "cannot be opened because the developer cannot be verified":** Right-click the `.command` file → **Open**, then click **Open** again. One-time only.
>
> **Windows — SmartScreen:** Click **More info** → **Run anyway**. One-time only.

### Option 2 — Build from source (Go 1.23 + Node 22)

```bash
make build
./bin/webhook-server --simulate   # opens browser + starts simulator automatically
```

On first run the server writes `data/secrets.json` (API key + AES encryption key). This file persists across restarts; delete it to reset credentials.

### Option 3 — Docker (no Go or Node required)

```bash
docker compose up --build
```

Open **http://localhost:8080**. The container generates secrets automatically on first run and persists them in `./data/secrets.json`. The simulator starts automatically — the dashboard populates with live data within seconds.

---

## Generating traffic with the simulator

The simulator registers mock webhook receiver servers, then continuously fires CloudEvents at the delivery service. Start it after the server is running:

```bash
make simulate               # build bin/simulate (first time only)
./bin/simulate
```

The dashboard populates within seconds. Default: 5 receivers, 30% failure rate, 2 events/sec.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--receivers N` | 5 | Number of mock receiver HTTP servers |
| `--failure-rate F` | 0.3 | Fraction of receivers that always return 500 |
| `--event-rate R` | 2.0 | Events per second to fire |
| `--server URL` | http://localhost:8080 | Delivery service base URL |
| `--secrets path` | data/secrets.json | Path to secrets file |

To trigger circuit breaker activity: use `--failure-rate 0.7` and a low `--receivers 3`. Circuits will open within seconds.

---

## Development

### React hot-reload + Go server

Two terminals:

```bash
# Terminal 1 — Go server (restart manually when Go files change)
./bin/webhook-server

# Terminal 2 — Vite dev server with HMR at http://localhost:5173
cd web && npm run dev
```

Vite proxies all API paths (`/webhooks`, `/events`, `/deliveries`, `/stream`, `/health`, `/config`) to the Go server at `:8080`. You edit React components and see changes instantly; the Go server handles all data. When you change Go code, `Ctrl+C` and re-run `./bin/webhook-server` (or use a file watcher like `air`).

The React build does **not** need to be rebuilt during frontend dev — the Go server is only used for API calls, not asset serving, when Vite is running.

### Building just the Go server (skip React rebuild)

```bash
go build -o bin/webhook-server ./cmd/server
```

Use this when you're only changing Go code and the frontend is already built.

---

## Testing

### Run everything

```bash
make test
```

This runs `go test ./...` followed by `cd web && npm test`.

### Go tests

```bash
go test ./...
```

All Go tests run against a real SQLite `:memory:` database. There are no mocks for the database layer — queries run against real SQL so constraint violations, transactions, and schema behavior are all exercised.

**What's covered:**

| Package | What the tests verify |
|---|---|
| `internal/db` | CRUD for webhooks, events, deliveries; idempotency (duplicate event ID → `ErrConflict`); delivery status transitions; `ClaimPending` CAS (two goroutines, one row → only one claims); `FlushHeld` limit; `ResetInFlight` sweep on startup |
| `internal/worker` | Backoff scheduling (`NextAttemptAt`); delivery execution (mock HTTP server); circuit open/close transitions via `RecordFailure` / `RecordSuccess` |
| `internal/api` | All REST endpoints via `httptest`: valid request → correct status + body; missing fields → 400; duplicate event → 409; oversized body → 413; unauthorized → 401; SSE stream produces events after a webhook action |
| `internal/sse` | Non-blocking publish drops when channel full; unsubscribe removes client |
| `internal/config` | Secrets auto-generation and round-trip loading |
| `internal/crypto` | AES-GCM encrypt/decrypt round-trip; HMAC signature output |
| `cmd/simulate` | Integration test: starts a real simulate run against a live test server, verifies events are ingested and webhooks are registered/deregistered cleanly |

Run a single package verbosely:

```bash
go test -v ./internal/api/...
go test -v ./internal/worker/...
```

Run a single test by name:

```bash
go test -v -run TestIngestEvent ./internal/api/...
go test -v -run TestClaimPending ./internal/db/...
```

### Frontend tests

```bash
cd web && npm test
```

Uses Vitest + jsdom. Tests cover all five dashboard components (WebhookRegistry, EventFeed, DeliveryLog, EndpointHealth, VolumeChart) and the Zustand store's `applySSEEvent` reducer.

Run in watch mode during development:

```bash
cd web && npm run test:watch    # re-runs affected tests on save
```

Run with coverage:

```bash
cd web && npm run coverage
```

---

## Architecture

```
cmd/server/         → wires dependencies, HTTP server, graceful shutdown
cmd/simulate/       → standalone traffic generator (registers receivers, fires events)
internal/api/       → REST handlers (chi router), SSE /stream, /config bootstrap endpoint
internal/worker/    → delivery pool, retry backoff, circuit probe goroutine
internal/db/        → SQLite repositories (modernc.org/sqlite — pure Go, no CGO)
internal/sse/       → SSE broadcaster (sync.Map of buffered channels, non-blocking send)
internal/config/    → secrets management (auto-generates data/secrets.json on first run)
internal/crypto/    → AES-256-GCM encryption, HMAC-SHA256 request signing
web/                → React 18 + Vite + TypeScript dashboard (embedded into Go binary)
```

**Data flow:** Incoming CloudEvent → `POST /events` validates and persists → delivery rows created per registered webhook in the same transaction → worker pool polls pending rows → CAS claim → HTTP dispatch with HMAC signature → circuit breaker updates webhook status → SSE broadcast updates dashboard in real time.

**Circuit breaker** lives entirely in the database (no in-memory state). A per-webhook failure streak column transitions `active → degraded → circuit_open`. A background probe goroutine retries the oldest held delivery every 30 seconds; success closes the circuit and flushes held deliveries back to pending in batches of 100.

**Frontend** fetches initial state via five React Query queries on mount, then stays live via a single `EventSource` connection. SSE events (`webhook_updated`, `event_ingested`, `delivery_updated`) are applied directly to the Zustand store; React Query handles reconnect re-hydration.

---

## Configuration

The server reads environment variables (and falls back to auto-generated `data/secrets.json` for key material):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DB_PATH` | `data/webhooks.db` | SQLite file path |
| `WORKER_COUNT` | `10` | Delivery worker goroutines |

Secrets (`API_KEY`, `WEBHOOK_ENCRYPTION_KEY`) are read from `data/secrets.json` if present. Set `--secrets` to point at a different path. The file is created automatically on first run.

---

## Cross-platform release builds

```bash
make build-all
```

Outputs to `dist/` — five server binaries and three simulate binaries covering Linux amd64/arm64, macOS amd64/arm64, and Windows amd64. CGO is disabled; `modernc.org/sqlite` is pure Go so cross-compilation works from any single platform without a C toolchain.
