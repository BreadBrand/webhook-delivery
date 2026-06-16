# Droplet Webhook Delivery Service

A self-contained webhook delivery system with circuit breaker, retry scheduling, SSE-driven live dashboard, and a simulation tool for generating realistic traffic. Ships as a single binary — no Node, no external database.

---

## Design thoughts
I built a design doc and a technical requirements doc those can be found at docs/specs/ some of my design choices I made just to try a new piece of software I haven't used before like zustand and others I made a decision to use because I'm confortable with it like React and Go. I want to use multi-client keys with webhook package signing to similulate a more real world choice. And I though it would be nice to add some interactivability to the dashboard. However the simulator just simulates a percentage failure rate and no latency so I would have liked to add that to the simulation so you could see the latency reporting but I ran out of time. 

I made the design docs in concert with claude code using a skill I developed called feature-spec. This skill takes a prompt from me about the new feature or project and then asks me questions to narrow the scope and figure out what are table stakes and what is out of scope. Then we collaborate on what could be potential edge cases and missed features and we iterate on the document. Once that is done we use the document to make the technical requirements where I make desicions about what technology I'll use and what design patterns will help with future development and expansion. Then we talk about engineering guidlines and make a guidline doc for the agents that will coding the work. Once that is done the work gets split into phases so the output token limit doesn't get hit and we code in TDD way so that the agents can catch bugs as we develope and I run manual tests accross the feature prompt to fix things. I did pull request reviews on all the code written and made sure I could understand it before pushing.

## Running it

### Option 1 — Pre-built binary (no Go or Node required)

Download `webhook-delivery-release.zip` from the [latest release](https://github.com/BreadBrand/webhook-delivery/releases).

| Platform | Steps |
|---|---|
| **macOS (Apple Silicon)** | Open Terminal in the unzipped folder and run: `xattr -d com.apple.quarantine webhook-server-darwin-arm64 && ./webhook-server-darwin-arm64 --simulate` |
| **macOS (Intel)** | Double-click `open-macos-amd64.command` |
| **Windows** | Double-click `webhook-server-windows-amd64.exe` |
| **Linux** | `./run-linux.sh` |

The server starts, opens **http://localhost:8080** in your browser, and begins generating live traffic automatically. The dashboard populates within seconds.

> **macOS Apple Silicon — Gatekeeper blocks unsigned binaries with no GUI workaround on Ventura/Sonoma+.** To run: right-click the unzipped folder in Finder → **Services** → **New Terminal at Folder**, then paste the command above.
>
> **macOS Intel — "cannot be opened because the developer cannot be verified":** Right-click the `.command` file → **Open**, then click **Open** again. One-time only.
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

Open **http://localhost:8080**. The container generates secrets automatically on first run and persists them in `./data/secrets.json`. The `./data/` directory (secrets and database) persists across restarts via the volume mount — delete the entire `./data/` folder to reset to a clean state. The simulator starts automatically — the dashboard populates with live data within seconds.

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
