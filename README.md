# Webhook Delivery Service

A self-contained webhook delivery system with circuit breaker, retry scheduling, SSE-driven live dashboard, and a simulation tool for generating realistic traffic.

## Quick start (Docker)

```bash
docker compose up --build
```

Open http://localhost:8080 — the dashboard loads automatically.

## Quick start (local)

**Prerequisites:** Go 1.23+, Node 22+

```bash
# Build the React app and server binary
make build

# Start the server (generates data/secrets.json on first run)
./bin/webhook-server
```

In a second terminal, start the simulator:

```bash
./bin/simulate --receivers 5 --failure-rate 0.3 --event-rate 2
```

The dashboard at http://localhost:8080 populates within seconds.

## Development (React hot reload)

```bash
# Terminal 1: Go server
./bin/webhook-server

# Terminal 2: Vite dev server (proxies API calls to :8080)
cd web && npm run dev
```

Open http://localhost:5173.

## Testing

```bash
make test
```

## Simulation flags

| Flag | Default | Description |
|------|---------|-------------|
| `--receivers N` | 5 | Number of mock receiver HTTP servers |
| `--failure-rate F` | 0.3 | Fraction of receivers that always return 500 |
| `--event-rate R` | 2.0 | CloudEvents fired per second |
| `--server URL` | http://localhost:8080 | Delivery service base URL |
| `--secrets path` | data/secrets.json | Path to secrets file |

## Architecture

```
cmd/server        → HTTP server (chi router), graceful shutdown
cmd/simulate      → Self-contained traffic generator
internal/api      → REST + SSE handlers, /config endpoint
internal/worker   → Delivery pool, retry scheduling, circuit probe
internal/db       → SQLite stores (modernc.org/sqlite, pure Go, no CGO)
internal/sse      → SSE broadcaster (sync.Map of buffered channels)
internal/config   → Secrets management (auto-generates on first run)
internal/crypto   → AES-256-GCM secret encryption
web/              → React 18 + Vite + TypeScript dashboard (embedded in binary)
```

## Cross-platform binaries

```bash
make build-all
# Outputs to dist/:
#   webhook-server-linux-amd64
#   webhook-server-linux-arm64
#   webhook-server-darwin-amd64
#   webhook-server-darwin-arm64
#   webhook-server-windows-amd64.exe
#   simulate-linux-amd64
#   simulate-darwin-arm64
#   simulate-windows-amd64.exe
```

## Configuration

The server auto-generates `data/secrets.json` on first run containing a random AES-256 encryption key and API key. Override behaviour with environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP listen port |
| `DB_PATH` | data/webhooks.db | SQLite database path |
| `WORKER_COUNT` | 10 | Delivery worker goroutines |
