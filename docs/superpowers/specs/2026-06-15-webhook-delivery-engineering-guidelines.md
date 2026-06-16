# Webhook Delivery Service — Engineering Guidelines
**Date:** 2026-06-15
**Design doc:** `2026-06-15-webhook-delivery-design.md`
**TRD:** `2026-06-15-webhook-delivery-trd.md`

---

## Project Structure

```
/
├── cmd/
│   ├── server/         # main.go — wires dependencies, starts HTTP server + worker pool
│   └── simulate/       # main.go — continuous simulation tool
├── internal/
│   ├── api/            # HTTP handlers, middleware, SSE handler
│   ├── db/             # schema, repository interfaces + SQLite implementations
│   ├── delivery/       # worker pool, backoff, signing, HTTP dispatch
│   ├── circuit/        # circuit breaker state machine logic
│   ├── sse/            # SSE broadcaster
│   └── crypto/         # AES-GCM encrypt/decrypt, HMAC signing helpers
├── web/                # React + Vite frontend
│   ├── src/
│   └── dist/           # built output (gitignored; embedded at build time)
├── data/               # SQLite file lives here (gitignored)
├── docs/
└── Makefile
```

Keep `internal/` packages narrowly scoped. `api/` imports `db/`, `delivery/`, and `sse/` — no other cross-package imports except through explicit constructor injection. Nothing in `internal/` imports from `cmd/`.

---

## Libraries

### Go

| Library | Purpose | Why |
|---|---|---|
| `github.com/go-chi/chi/v5` | HTTP router | Idiomatic `net/http` compatible, lightweight, good middleware support (`chi.Use`). Avoid `gin`/`echo` — they introduce non-standard context patterns that complicate handler testing. |
| `modernc.org/sqlite` | SQLite driver | Pure Go — no CGO, no C toolchain required. Simpler CI and cross-compilation. Avoid `github.com/mattn/go-sqlite3` for this reason. |
| `github.com/google/uuid` | UUID generation | Standard, well-tested. Use `uuid.New().String()` everywhere an ID is needed. |
| `log/slog` | Structured logging | Stdlib in Go 1.21+. Use `slog.Default()` with a JSON handler in production, text handler in dev. No third-party logger needed at this scope. |
| `crypto/aes`, `crypto/cipher`, `crypto/hmac`, `crypto/sha256`, `crypto/rand` | Cryptography | All stdlib. Never import third-party crypto primitives. |
| `database/sql` | DB access | Stdlib. Use directly with `modernc.org/sqlite` driver — no ORM. |
| `embed` | Frontend assets | Stdlib. Embed `web/dist` into the binary for production serving. |

### Frontend (React)

| Library | Purpose | Why |
|---|---|---|
| `vite` | Build tool | Fast HMR, simple config, native ESM. |
| `react` + `react-dom` | UI | Already decided. Use hooks throughout — no class components. |
| `typescript` | Type safety | Catches shape mismatches between API responses and component state. |
| `recharts` | Charts | React-native composable chart library. Fits the SSE-driven update model well — just pass new data as props and it re-renders. Avoid `chart.js` (imperative, awkward in React). |
| `zustand` | State management | Lightweight, no boilerplate. One store for the dashboard state updated by SSE events. Avoid Redux — overkill for a single-page dashboard. |
| `@tanstack/react-query` | REST hydration | Handles loading/error states, refetch-on-reconnect, and cache invalidation cleanly. Use for the four hydration endpoints; SSE updates go directly into Zustand. |

---

## Patterns

### 1. Repository Pattern (db layer)

Define interfaces in `internal/db/`; SQLite implementations live in the same package. Handlers and workers depend on the interface, not the SQLite type — makes unit testing handlers with a fake repo straightforward.

```go
// internal/db/webhook.go
type WebhookRepo interface {
    Create(ctx context.Context, url, encryptedSecret, hint string, threshold int) (*Webhook, error)
    List(ctx context.Context) ([]Webhook, error)
    SoftDelete(ctx context.Context, id string) error
    SetCircuit(ctx context.Context, id, action string) error
    IncrementFailureStreak(ctx context.Context, id string) (newStreak int, err error)
    ResetFailureStreak(ctx context.Context, id string) error
}
```

Never write raw SQL in handlers or workers. All queries live in repository methods.

### 2. Worker Pool with Supervisor (delivery layer)

The pool is a fixed number of goroutines started at server boot. Each goroutine runs inside a `supervise` wrapper that recovers panics, re-queues the in-progress delivery, and restarts the loop. The pool is stopped via context cancellation on server shutdown.

```go
// internal/delivery/pool.go
func StartPool(ctx context.Context, n int, deps WorkerDeps) {
    for range n {
        go supervise(ctx, func() { runWorker(ctx, deps) })
    }
}

func supervise(ctx context.Context, fn func()) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            func() {
                defer func() {
                    if r := recover(); r != nil {
                        slog.Error("worker panic", "stack", string(debug.Stack()))
                        // re-queue is handled inside runWorker's deferred cleanup
                    }
                }()
                fn()
            }()
        }
    }
}
```

`runWorker` polls on a ticker, not a sleep loop. Use `time.NewTicker(500 * time.Millisecond)`.

### 3. CAS Delivery Claim (delivery layer)

Never SELECT then UPDATE as separate statements — two workers will race. Claim with a single UPDATE and check rows affected:

```go
// internal/delivery/worker.go
result, err := db.ExecContext(ctx,
    `UPDATE deliveries SET status='in_flight', updated_at=? WHERE id=? AND status='pending'`,
    time.Now(), id)
if err != nil {
    return err
}
if n, _ := result.RowsAffected(); n == 0 {
    return nil // another worker claimed it
}
```

Run this inside a `BEGIN IMMEDIATE` transaction to prevent read-then-write races under WAL mode.

### 4. Sign-Then-Send (delivery layer)

Serialize once. Sign the bytes. Send the bytes. Never touch the struct again after serialization.

```go
// internal/delivery/sign.go
func buildRequest(ctx context.Context, event Event, webhook Webhook, attempt int) (*http.Request, error) {
    body, err := json.Marshal(event)
    if err != nil {
        return nil, err
    }

    mac := hmac.New(sha256.New, webhook.RawSecret) // decrypted at call site
    mac.Write(body)
    sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Webhook-Event-ID", event.ID)
    req.Header.Set("X-Webhook-Delivery-Attempt", strconv.Itoa(attempt))
    req.Header.Set("X-Webhook-Signature", sig)
    return req, nil
}
```

The `body` variable is the single source of truth. Do not re-marshal `event` anywhere downstream.

### 5. Circuit Breaker as DB State Machine (circuit layer)

The circuit breaker lives entirely in the database — there is no in-memory state. This means it survives restarts and is correct across all goroutines without locking. The worker reads webhook status from the DB on every delivery pickup.

Transitions are implemented as targeted UPDATE statements, not general-purpose status setters:

```go
// internal/circuit/breaker.go

// Called after each failure
func RecordFailure(ctx context.Context, db *sql.DB, webhookID string, threshold int) error {
    _, err := db.ExecContext(ctx, `
        UPDATE webhooks SET
            failure_streak = failure_streak + 1,
            status = CASE
                WHEN failure_streak + 1 >= ? THEN 'circuit_open'
                ELSE 'degraded'
            END,
            next_probe_at = CASE
                WHEN failure_streak + 1 >= ? THEN datetime('now', '+5 minutes')
                ELSE next_probe_at
            END,
            updated_at = datetime('now')
        WHERE id = ?`, threshold, threshold, webhookID)
    return err
}

// Called after probe or manual close
func CloseCircuit(ctx context.Context, db *sql.DB, webhookID string) error {
    tx, _ := db.BeginTx(ctx, nil)
    tx.ExecContext(ctx, `UPDATE webhooks SET status='active', failure_streak=0, next_probe_at=NULL WHERE id=?`, webhookID)
    tx.ExecContext(ctx, `UPDATE deliveries SET status='pending', next_attempt_at=datetime('now')
        WHERE webhook_id=? AND status='held'
        ORDER BY created_at ASC LIMIT 10`, webhookID)
    return tx.Commit()
}
```

The LIMIT 10 in `CloseCircuit` is intentional — flush in batches, let the worker's normal poll rate handle the rest.

### 6. SSE Broadcaster (sse layer)

One broadcaster instance, shared across all handlers. Each connected client gets a buffered channel. Sends are non-blocking.

```go
// internal/sse/broadcaster.go
type Broadcaster struct {
    clients sync.Map // id string → chan string
}

func (b *Broadcaster) Subscribe(id string) chan string {
    ch := make(chan string, 64)
    b.clients.Store(id, ch)
    return ch
}

func (b *Broadcaster) Unsubscribe(id string) {
    b.clients.Delete(id)
}

func (b *Broadcaster) Publish(eventType string, payload any) {
    data, _ := json.Marshal(payload)
    msg := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
    b.clients.Range(func(_, v any) bool {
        ch := v.(chan string)
        select {
        case ch <- msg:
        default: // client too slow — drop
        }
        return true
    })
}
```

The SSE handler writes from the channel until the client disconnects:

```go
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    id := uuid.New().String()
    ch := h.broadcaster.Subscribe(id)
    defer h.broadcaster.Unsubscribe(id)

    flusher := w.(http.Flusher)
    for {
        select {
        case <-r.Context().Done():
            return
        case msg := <-ch:
            fmt.Fprint(w, msg)
            flusher.Flush()
        }
    }
}
```

### 7. AES-GCM Secret Encryption (crypto layer)

The key comes from `WEBHOOK_ENCRYPTION_KEY` (base64-encoded 32 bytes). Encrypt on registration, decrypt before each delivery signing.

```go
// internal/crypto/secret.go
func Encrypt(key, plaintext []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    sealed := gcm.Seal(nonce, nonce, plaintext, nil)
    return base64.StdEncoding.EncodeToString(sealed), nil
}

func Decrypt(key []byte, encoded string) ([]byte, error) {
    sealed, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        return nil, err
    }
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
    return gcm.Open(nil, nonce, ciphertext, nil)
}
```

---

## SQLite Conventions

**DSN:**
```
file:data/webhooks.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)
```

**Schema migrations:** Use a single `schema.sql` file embedded via `embed.FS`. On startup, run `CREATE TABLE IF NOT EXISTS` for each table. No migration framework needed at this scope.

**Transactions:** Use `db.BeginTx` anywhere multiple writes must be atomic (e.g., ingest + create deliveries, circuit close + flush held). Never rely on autocommit for multi-step operations.

**Query style:** Write queries as raw SQL strings in repository methods. Use named constants for repeated queries. No query builder.

**File permissions:** Create the `data/` directory with `os.MkdirAll("data", 0700)` before opening the database.

---

## Frontend Conventions

**State architecture:** One Zustand store holds all dashboard state. SSE events are dispatched into it directly. React Query manages the hydration fetches — on success it seeds the Zustand store, then SSE takes over.

```ts
// src/store.ts
interface DashboardStore {
  webhooks: Webhook[]
  events: Event[]
  deliveries: Delivery[]
  volumeData: VolumePoint[]
  volumeWindow: '5m' | '30m' | '1h' | '24h'
  applySSEEvent: (type: string, data: unknown) => void
}
```

**SSE reconnect:** Wrap `EventSource` in a custom hook that re-fires React Query's `refetchAll` on the `error` event (connection dropped), then re-opens the stream.

```ts
// src/hooks/useSSE.ts
useEffect(() => {
  const es = new EventSource('/stream', { withCredentials: false })
  es.onerror = () => {
    queryClient.refetchQueries() // re-hydrate on reconnect
    es.close()
    // hook re-runs on next render and opens a new EventSource
  }
  es.addEventListener('webhook_updated', e => store.applySSEEvent('webhook_updated', JSON.parse(e.data)))
  // ...
  return () => es.close()
}, [])
```

**Component structure:**
```
src/
├── components/
│   ├── WebhookRegistry.tsx
│   ├── EventFeed.tsx
│   ├── DeliveryLog.tsx
│   ├── EndpointHealth.tsx
│   └── VolumeChart.tsx
├── hooks/
│   ├── useSSE.ts
│   └── useHydration.ts
├── store.ts
├── api.ts         # typed fetch wrappers for all REST endpoints
└── App.tsx
```

Keep components as pure display components — no fetching inside them. All data flows in from the store.

---

## Testing Requirements

**Approach:** Integration tests against a real SQLite `:memory:` database. Do not mock the database — the test suite must exercise real SQL. The spec says we got burned by mocked tests masking real issues.

**What to test:**

| Area | What |
|---|---|
| Repository layer | CRUD operations, constraint violations (duplicate event id, duplicate delivery), soft-delete cascade |
| Delivery worker | CAS claim (two goroutines, one delivery → only one claims), startup recovery sweep, backoff scheduling |
| Circuit breaker | All state transitions: active→degraded→circuit_open→half-open→active; manual open/close |
| Signing | `buildRequest` output: correct headers, signature verifies against body bytes |
| Ingestion handler | Valid CloudEvents → 202; missing fields → 400; duplicate id → 409; oversized → 413; specversion wrong → 400 |
| Re-delivery | Only allowed on `failed`; 409 on duplicate in-flight re-delivery |
| SSE broadcaster | Non-blocking send drops event when channel full; unsubscribe cleans up client |

**Test file layout:** Co-locate tests with the package they test (`internal/delivery/worker_test.go`). No separate `test/` directory.

**Running:** `go test ./...` from root. All tests must pass with no external dependencies (SQLite is in-process).

---

## Deployment and Build

### Reviewer path A (primary) — Standalone executable, no install required

Unzip, double-click, browser opens. No Docker, no Go, no Node, no terminal commands.

This works because `modernc.org/sqlite` is pure Go (no CGO) — the only thing that normally blocks cross-compiling a single binary for every OS from one dev machine. With CGO out of the picture, `embed.FS` bundles the built React app and the SQLite driver into one self-contained binary per platform.

**What ships in the zip:**
```
dist/
├── webhook-delivery-windows.exe
├── webhook-delivery-macos          # raw binary, do not double-click directly
├── webhook-delivery-macos.command  # double-click this on macOS
├── webhook-delivery-linux
└── run-linux.sh                    # `./run-linux.sh` from terminal
```

**Build command (run once, on any one machine, before zipping for submission):**
```bash
cd web && npm run build && cd ..
GOOS=windows GOARCH=amd64 go build -o dist/webhook-delivery-windows.exe ./cmd/server
GOOS=darwin  GOARCH=arm64 go build -o dist/webhook-delivery-macos       ./cmd/server
GOOS=linux   GOARCH=amd64 go build -o dist/webhook-delivery-linux      ./cmd/server
chmod +x dist/webhook-delivery-macos dist/webhook-delivery-linux
```

**`dist/webhook-delivery-macos.command`** (double-click target on macOS — opens Terminal, runs the binary, shows logs, auto-opens browser):
```bash
#!/bin/bash
cd "$(dirname "$0")"
./webhook-delivery-macos
```
A bare Mach-O binary double-clicked in Finder runs invisibly with no console and no way to stop it — the `.command` wrapper is what makes it behave like a normal double-clickable app.

**`dist/run-linux.sh`:**
```bash
#!/bin/bash
cd "$(dirname "$0")"
./webhook-delivery-linux
```

**Windows `.exe`** needs no wrapper — double-clicking opens a console window directly showing logs, which is the desired behavior (transparent, stoppable with Ctrl+C or closing the window).

**In-process responsibilities the binary takes on (none of this exists in the Docker path):**

1. **Auto-open the browser on startup** — after the HTTP server starts listening, launch the default browser to `http://localhost:8080`:
   ```go
   func openBrowser(url string) {
       var cmd *exec.Cmd
       switch runtime.GOOS {
       case "darwin":
           cmd = exec.Command("open", url)
       case "windows":
           cmd = exec.Command("cmd", "/c", "start", url)
       default:
           cmd = exec.Command("xdg-open", url)
       }
       cmd.Start()
   }
   ```
2. **Persist secrets across runs** — there's no `.env` to hand-edit. On first launch, if `data/secrets.json` doesn't exist, generate `WEBHOOK_ENCRYPTION_KEY` and `API_KEY`, write them to that file, and load them on every subsequent launch. This is a correctness requirement, not just convenience: if the encryption key changed on every run, webhook signing secrets encrypted in a previous session would become permanently undecryptable.
   ```go
   type secrets struct {
       EncryptionKey string `json:"encryption_key"`
       APIKey        string `json:"api_key"`
   }

   func loadOrCreateSecrets(path string) (secrets, error) {
       if data, err := os.ReadFile(path); err == nil {
           var s secrets
           return s, json.Unmarshal(data, &s)
       }
       s := secrets{
           EncryptionKey: randomBase64(32),
           APIKey:        randomBase64(32),
       }
       data, _ := json.MarshalIndent(s, "", "  ")
       os.MkdirAll(filepath.Dir(path), 0700)
       return s, os.WriteFile(path, data, 0600)
   }
   ```
3. **Data lives next to the binary** — `data/webhooks.db` and `data/secrets.json` are created relative to the executable's working directory, keeping the whole thing portable: delete the folder, everything resets.
4. **`--simulate` defaults to on** for these builds, so the dashboard has live data the moment the browser opens — a reviewer shouldn't have to know to register a webhook manually.

**Caveats to put in the README, stated plainly so reviewers aren't alarmed:**

- **macOS Gatekeeper** will block first launch with *"cannot be opened because the developer cannot be verified"* — this happens to any unsigned binary downloaded via email/zip (it gets a quarantine flag). Fix: right-click the `.command` file → **Open** (only needed once), or System Settings → Privacy & Security → **Open Anyway**. There is no way around this without a paid Apple Developer ID and notarization.
- **Windows SmartScreen** may show *"Windows protected your PC."* Fix: click **More info** → **Run anyway**. Same root cause (unsigned binary), same one-click bypass.

---

### Reviewer path B (fallback) — Docker Compose

If the executable doesn't run for some reason (architecture mismatch, antivirus quarantine, corporate device restrictions), Docker Compose is the fallback:

```bash
docker compose up
```

Open `http://localhost:8080`. Ctrl+C to stop.

No Go, no Node, no npm, no manual secret generation. The multi-stage Docker build handles everything internally:

1. **Stage 1 (Node)** — `npm ci && npm run build` compiles the React app into `web/dist`
2. **Stage 2 (Go)** — embeds `web/dist` via `embed.FS` and compiles the server binary
3. **Stage 3 (alpine)** — ships only the final binary (~20MB image)

Node never touches the reviewer's machine.

**`Dockerfile`:**
```dockerfile
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS backend
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN go build -o bin/server ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=backend /app/bin/server ./server
RUN mkdir -p data
EXPOSE 8080
CMD ["./server", "--simulate"]
```

**`docker-compose.yml`:**
```yaml
services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      SIMULATE: "true"
      WEBHOOK_ENCRYPTION_KEY: ${WEBHOOK_ENCRYPTION_KEY:-}
      API_KEY: ${API_KEY:-}
    volumes:
      - ./data:/app/data
```

Secrets are auto-generated in the container entrypoint if `WEBHOOK_ENCRYPTION_KEY` or `API_KEY` are absent — reviewers never need to touch `.env`. Use a small `entrypoint.sh`:

```bash
#!/bin/sh
export WEBHOOK_ENCRYPTION_KEY=${WEBHOOK_ENCRYPTION_KEY:-$(openssl rand -base64 32)}
export API_KEY=${API_KEY:-$(openssl rand -base64 32)}
exec "$@"
```

```dockerfile
# add to final stage:
COPY entrypoint.sh ./
RUN chmod +x entrypoint.sh
ENTRYPOINT ["./entrypoint.sh"]
CMD ["./server", "--simulate"]
```

---

### `--simulate` flag

The `--simulate` flag (or `SIMULATE=true` env var) starts the simulation tool as in-process goroutines alongside the server. Mock webhook receivers register themselves, fire CloudEvents continuously, and intentionally fail at a configurable rate — the dashboard has live data from the moment the container starts.

---

### Frontend serving

The Go server serves the embedded React build at `/` and all API routes at their normal paths. One port, one process, no separate frontend server needed.

---

### Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `WEBHOOK_ENCRYPTION_KEY` | Yes | Auto-generated in Docker | Base64-encoded 32-byte AES key |
| `API_KEY` | Yes | Auto-generated in Docker | Bearer token for all API requests |
| `PORT` | No | `8080` | HTTP listen port |
| `WORKER_COUNT` | No | `10` | Delivery worker goroutines |
| `DB_PATH` | No | `data/webhooks.db` | SQLite file path |
| `SIMULATE` | No | `false` | `true` to start simulator inline |
| `LOG_FORMAT` | No | `text` | `json` or `text` |

---

### Local development (Go + Node required)

For iterating on the frontend or backend without rebuilding Docker:

```bash
# Terminal 1 — Go server
go run ./cmd/server

# Terminal 2 — Vite dev server (proxies /api and /stream to :8080)
cd web && npm run dev
```

Generate secrets once for local dev:
```bash
export WEBHOOK_ENCRYPTION_KEY=$(openssl rand -base64 32)
export API_KEY=$(openssl rand -base64 32)
```

---

### README must include

1. **Download the right file for your OS and double-click it** as the very first instruction, with the macOS Gatekeeper / Windows SmartScreen one-click bypass noted right there (not buried) so reviewers aren't alarmed
2. **"Docker not working? Run `docker compose up` instead"** as the clearly-labeled fallback immediately after
3. The dashboard URL (`http://localhost:8080`) and a note that it opens automatically
4. A note that the simulator starts automatically and the dashboard will have live data within seconds
5. Local development instructions (`go run` + `npm run dev`) in a separate section below, clearly marked as optional and for code review only
