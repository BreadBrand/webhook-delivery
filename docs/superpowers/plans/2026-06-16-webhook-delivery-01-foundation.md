# Webhook Delivery — Plan 1: Foundation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Go project scaffold, shared types, crypto helpers, config loader, and all three database stores with full test coverage — giving every subsequent plan a reliable foundation to build on.

**Architecture:** Pure Go (no CGO) with `modernc.org/sqlite`, WAL mode, real SQLite in tests (no mocks). All domain types live in `internal/models`; crypto primitives in `internal/crypto`; secrets persistence in `internal/config`; three repository interfaces + SQLite implementations in `internal/db`.

**Tech Stack:** Go 1.23, `modernc.org/sqlite`, `github.com/google/uuid`, stdlib only for crypto.

---

## File Map

```
/
├── go.mod
├── .gitignore
├── internal/
│   ├── models/
│   │   └── models.go
│   ├── crypto/
│   │   ├── aes.go
│   │   ├── aes_test.go
│   │   ├── hmac.go
│   │   └── hmac_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   └── db/
│       ├── schema.sql
│       ├── schema.go
│       ├── open.go
│       ├── webhook.go
│       ├── webhook_test.go
│       ├── event.go
│       ├── event_test.go
│       ├── delivery.go
│       └── delivery_test.go
├── web/
│   ├── embed.go
│   └── dist/
│       └── .gitkeep
└── cmd/
    └── server/
        └── main.go   (stub only — wired in Plan 3)
```

---

## Task 1: Scaffold

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `internal/db/schema.sql`
- Create: `internal/db/open.go`
- Create: `internal/db/schema.go`
- Create: `web/embed.go`
- Create: `web/dist/.gitkeep`
- Create: `cmd/server/main.go`

- [ ] **Step 1: Init git and directory structure**

```bash
cd /home/bash/Dev/Droplet_Webhook_Deliveries
git init
mkdir -p internal/{models,crypto,config,db} web/dist cmd/server
touch web/dist/.gitkeep
```

- [ ] **Step 2: Create `go.mod` and fetch dependencies**

```bash
go mod init github.com/b2randon/webhook-delivery
go get github.com/go-chi/chi/v5
go get github.com/google/uuid
go get modernc.org/sqlite
go mod tidy
```

- [ ] **Step 3: Create `.gitignore`**

```
data/
dist/webhook-delivery-*
web/dist/*
!web/dist/.gitkeep
*.db
```

- [ ] **Step 4: Create `internal/db/schema.sql`**

```sql
CREATE TABLE IF NOT EXISTS webhooks (
    id                TEXT PRIMARY KEY,
    url               TEXT NOT NULL,
    encrypted_secret  TEXT NOT NULL,
    secret_hint       TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'active',
    failure_streak    INTEGER NOT NULL DEFAULT 0,
    circuit_threshold INTEGER NOT NULL DEFAULT 5,
    next_probe_at     DATETIME,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS events (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    source      TEXT NOT NULL,
    time        DATETIME NOT NULL,
    data        TEXT NOT NULL,
    received_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS deliveries (
    id                 TEXT PRIMARY KEY,
    event_id           TEXT NOT NULL REFERENCES events(id),
    webhook_id         TEXT NOT NULL REFERENCES webhooks(id),
    parent_delivery_id TEXT,
    status             TEXT NOT NULL DEFAULT 'pending',
    attempt            INTEGER NOT NULL DEFAULT 0,
    next_attempt_at    DATETIME,
    last_status_code   INTEGER,
    last_response_ms   INTEGER,
    last_error         TEXT,
    created_at         DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at         DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(event_id, webhook_id) WHERE parent_delivery_id IS NULL
);
```

- [ ] **Step 5: Create `internal/db/schema.go`**

```go
package db

import (
	_ "embed"
	"database/sql"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

func runSchema(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("run schema: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Create `internal/db/open.go`**

```go
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := runSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// testDB opens an in-memory SQLite instance. Only called from _test.go files.
func testDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	if err := runSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
```

- [ ] **Step 7: Create `web/embed.go`**

```go
package web

import "embed"

// FS holds the compiled React app. web/dist must be built before compiling cmd/server.
// A .gitkeep placeholder in web/dist/ keeps the embed valid during Go-only development.
//
//go:embed dist
var FS embed.FS
```

- [ ] **Step 8: Create stub `cmd/server/main.go`**

```go
package main

func main() {}
```

- [ ] **Step 9: Verify project compiles**

```bash
go build ./...
```

Expected: no output, exit 0.

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat: project scaffold, schema, db open, web embed stub"
```

---

## Task 2: Shared Models

**Files:**
- Create: `internal/models/models.go`

- [ ] **Step 1: Create `internal/models/models.go`**

```go
package models

import (
	"encoding/json"
	"time"
)

type WebhookStatus string

const (
	StatusActive      WebhookStatus = "active"
	StatusDegraded    WebhookStatus = "degraded"
	StatusCircuitOpen WebhookStatus = "circuit_open"
	StatusDeleted     WebhookStatus = "deleted"
)

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryInFlight  DeliveryStatus = "in_flight"
	DeliverySuccess   DeliveryStatus = "success"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryHeld      DeliveryStatus = "held"
)

type Webhook struct {
	ID               string        `json:"id"`
	URL              string        `json:"url"`
	EncryptedSecret  string        `json:"-"`
	SecretHint       string        `json:"secret_hint"`
	Status           WebhookStatus `json:"status"`
	FailureStreak    int           `json:"failure_streak"`
	CircuitThreshold int           `json:"circuit_threshold"`
	NextProbeAt      *time.Time    `json:"next_probe_at,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// CloudEvent is the envelope sent on every delivery attempt.
// Fields are declared in this order so json.Marshal output is deterministic —
// the HMAC is computed over the marshalled bytes, so ordering must never change.
type CloudEvent struct {
	SpecVersion string          `json:"specversion"`
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Source      string          `json:"source"`
	Time        time.Time       `json:"time"`
	Data        json.RawMessage `json:"data"`
}

type Event struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Source     string          `json:"source"`
	Time       time.Time       `json:"time"`
	Data       json.RawMessage `json:"data"`
	ReceivedAt time.Time       `json:"received_at"`
}

type Delivery struct {
	ID               string         `json:"id"`
	EventID          string         `json:"event_id"`
	WebhookID        string         `json:"webhook_id"`
	ParentDeliveryID *string        `json:"parent_delivery_id,omitempty"`
	Status           DeliveryStatus `json:"status"`
	Attempt          int            `json:"attempt"`
	NextAttemptAt    *time.Time     `json:"next_attempt_at,omitempty"`
	LastStatusCode   *int           `json:"last_status_code,omitempty"`
	LastResponseMs   *int           `json:"last_response_ms,omitempty"`
	LastError        *string        `json:"last_error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	// Joined display fields populated by List queries.
	EventType  string `json:"event_type,omitempty"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

type VolumePoint struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/models/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/models/models.go
git commit -m "feat: shared domain models"
```

---

## Task 3: Crypto Package

**Files:**
- Create: `internal/crypto/aes.go`
- Create: `internal/crypto/aes_test.go`
- Create: `internal/crypto/hmac.go`
- Create: `internal/crypto/hmac_test.go`

- [ ] **Step 1: Write `internal/crypto/aes_test.go`**

```go
package crypto_test

import (
	"testing"

	"github.com/b2randon/webhook-delivery/internal/crypto"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plaintext := []byte("super secret signing key sk_abc123")

	ciphertext, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	key := make([]byte, 32)
	plaintext := []byte("hello")

	c1, _ := crypto.Encrypt(key, plaintext)
	c2, _ := crypto.Encrypt(key, plaintext)
	if c1 == c2 {
		t.Error("two encryptions of same plaintext should not be identical (nonce must be random)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1

	ct, _ := crypto.Encrypt(key1, []byte("hello"))
	_, err := crypto.Decrypt(key2, ct)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/crypto/... -run TestEncrypt -v
```

Expected: `cannot find package` or compile error — `aes.go` doesn't exist yet.

- [ ] **Step 3: Create `internal/crypto/aes.go`**

```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

func Encrypt(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("random nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func Decrypt(key []byte, encoded string) ([]byte, error) {
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plain, nil
}
```

- [ ] **Step 4: Write `internal/crypto/hmac_test.go`**

```go
package crypto_test

import (
	"strings"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/crypto"
)

func TestSignFormat(t *testing.T) {
	sig := crypto.Sign([]byte("body"), []byte("secret"))
	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("signature must start with 'sha256=', got %q", sig)
	}
	if len(sig) != len("sha256=")+64 {
		t.Errorf("unexpected signature length: %d", len(sig))
	}
}

func TestSignDeterministic(t *testing.T) {
	body := []byte(`{"id":"1","type":"order.created"}`)
	secret := []byte("mysecret")
	if crypto.Sign(body, secret) != crypto.Sign(body, secret) {
		t.Error("same inputs must produce same signature")
	}
}

func TestSignDifferentBodyDifferentSig(t *testing.T) {
	secret := []byte("mysecret")
	s1 := crypto.Sign([]byte("body1"), secret)
	s2 := crypto.Sign([]byte("body2"), secret)
	if s1 == s2 {
		t.Error("different bodies must produce different signatures")
	}
}

func TestSignDifferentKeyDifferentSig(t *testing.T) {
	body := []byte("same body")
	s1 := crypto.Sign(body, []byte("key1"))
	s2 := crypto.Sign(body, []byte("key2"))
	if s1 == s2 {
		t.Error("different keys must produce different signatures")
	}
}
```

- [ ] **Step 5: Create `internal/crypto/hmac.go`**

```go
package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign returns "sha256=<hex>" HMAC-SHA256 of body using secret.
// body must be the exact []byte that will be sent as the HTTP request body —
// never re-marshal the payload after calling Sign.
func Sign(body, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
```

- [ ] **Step 6: Run all crypto tests — expect PASS**

```bash
go test ./internal/crypto/... -v
```

Expected:
```
--- PASS: TestEncryptDecryptRoundtrip
--- PASS: TestEncryptProducesUniqueNonces
--- PASS: TestDecryptWrongKeyFails
--- PASS: TestSignFormat
--- PASS: TestSignDeterministic
--- PASS: TestSignDifferentBodyDifferentSig
--- PASS: TestSignDifferentKeyDifferentSig
PASS
```

- [ ] **Step 7: Commit**

```bash
git add internal/crypto/
git commit -m "feat: AES-256-GCM encrypt/decrypt and HMAC-SHA256 signing"
```

---

## Task 4: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write `internal/config/config_test.go`**

```go
package config_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/config"
)

func TestLoadCreatesSecretsOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("EncryptionKey length = %d, want 32", len(cfg.EncryptionKey))
	}
	if cfg.APIKey == "" {
		t.Error("APIKey must not be empty")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("secrets.json was not created")
	}
}

func TestLoadReadsExistingSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	cfg1, _ := config.Load(path)

	cfg2, err := config.Load(path)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if string(cfg1.EncryptionKey) != string(cfg2.EncryptionKey) {
		t.Error("EncryptionKey must be stable across loads")
	}
	if cfg1.APIKey != cfg2.APIKey {
		t.Error("APIKey must be stable across loads")
	}
}

func TestLoadRejectsShortKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	shortKey := base64.StdEncoding.EncodeToString([]byte("tooshort"))
	content := `{"encryption_key":"` + shortKey + `","api_key":"validkey"}`
	os.WriteFile(path, []byte(content), 0600)

	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for short encryption key")
	}
}

func TestLoadRejectsEmptyAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	content := `{"encryption_key":"` + key + `","api_key":""}`
	os.WriteFile(path, []byte(content), 0600)

	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for empty api_key")
	}
}

func TestDefaultValues(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "secrets.json"))

	if cfg.Port != "8080" {
		t.Errorf("default Port = %q, want 8080", cfg.Port)
	}
	if cfg.WorkerCount != 10 {
		t.Errorf("default WorkerCount = %d, want 10", cfg.WorkerCount)
	}
	if cfg.DBPath != "data/webhooks.db" {
		t.Errorf("default DBPath = %q", cfg.DBPath)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL (package missing)**

```bash
go test ./internal/config/... -v
```

Expected: compile error.

- [ ] **Step 3: Create `internal/config/config.go`**

```go
package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	EncryptionKey []byte
	APIKey        string
	Port          string
	WorkerCount   int
	DBPath        string
	Simulate      bool
	LogFormat     string
}

type secretsFile struct {
	EncryptionKey string `json:"encryption_key"`
	APIKey        string `json:"api_key"`
}

func Load(secretsPath string) (*Config, error) {
	s, err := loadOrCreate(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}

	key, err := base64.StdEncoding.DecodeString(s.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decode encryption_key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption_key must decode to 32 bytes, got %d", len(key))
	}
	if s.APIKey == "" {
		return nil, fmt.Errorf("api_key must not be empty")
	}

	return &Config{
		EncryptionKey: key,
		APIKey:        s.APIKey,
		Port:          env("PORT", "8080"),
		WorkerCount:   envInt("WORKER_COUNT", 10),
		DBPath:        env("DB_PATH", "data/webhooks.db"),
		Simulate:      envBool("SIMULATE", false),
		LogFormat:     env("LOG_FORMAT", "text"),
	}, nil
}

func loadOrCreate(path string) (secretsFile, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var s secretsFile
		if err := json.Unmarshal(data, &s); err != nil {
			return secretsFile{}, fmt.Errorf("parse %s: %w", path, err)
		}
		return s, nil
	}
	if !os.IsNotExist(err) {
		return secretsFile{}, err
	}

	s := secretsFile{
		EncryptionKey: randomBase64(32),
		APIKey:        randomBase64(32),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return secretsFile{}, err
	}
	out, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(path, out, 0600); err != nil {
		return secretsFile{}, err
	}
	return s, nil
}

func randomBase64(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/config/... -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: config loader with auto-generated secrets persistence"
```

---

## Task 5: Webhook Store

**Files:**
- Create: `internal/db/webhook.go`
- Create: `internal/db/webhook_test.go`

- [ ] **Step 1: Write `internal/db/webhook_test.go`**

```go
package db_test

import (
	"context"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
)

func mustOpenDB(t *testing.T) *db.Stores {
	t.Helper()
	stores, err := db.OpenStores(":memory:")
	if err != nil {
		t.Fatalf("OpenStores: %v", err)
	}
	t.Cleanup(func() { stores.Close() })
	return stores
}

func TestWebhookCreateAndGet(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, err := s.Webhooks.Create(ctx, "https://example.com/hook", "enc-secret", "sk_…abcd", 5)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wh.ID == "" {
		t.Error("ID must be set")
	}
	if wh.Status != models.StatusActive {
		t.Errorf("Status = %q, want active", wh.Status)
	}

	got, err := s.Webhooks.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != "https://example.com/hook" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestWebhookList(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	s.Webhooks.Create(ctx, "https://a.com", "enc", "hint", 5)
	s.Webhooks.Create(ctx, "https://b.com", "enc", "hint", 5)

	list, err := s.Webhooks.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
}

func TestWebhookSoftDelete(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 5)
	if err := s.Webhooks.SoftDelete(ctx, wh.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	list, _ := s.Webhooks.List(ctx)
	if len(list) != 0 {
		t.Error("deleted webhook should not appear in List")
	}

	got, _ := s.Webhooks.Get(ctx, wh.ID)
	if got.Status != models.StatusDeleted {
		t.Errorf("Status after delete = %q, want deleted", got.Status)
	}
}

func TestWebhookRecordFailure(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 3)

	streak, status, err := s.Webhooks.RecordFailure(ctx, wh.ID)
	if err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if streak != 1 || status != models.StatusDegraded {
		t.Errorf("after 1 failure: streak=%d status=%q", streak, status)
	}

	s.Webhooks.RecordFailure(ctx, wh.ID)
	streak, status, _ = s.Webhooks.RecordFailure(ctx, wh.ID)
	if streak != 3 || status != models.StatusCircuitOpen {
		t.Errorf("after 3 failures (threshold=3): streak=%d status=%q", streak, status)
	}
}

func TestWebhookRecordSuccess(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 5)
	s.Webhooks.RecordFailure(ctx, wh.ID)
	s.Webhooks.RecordFailure(ctx, wh.ID)

	if err := s.Webhooks.RecordSuccess(ctx, wh.ID); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	got, _ := s.Webhooks.Get(ctx, wh.ID)
	if got.FailureStreak != 0 || got.Status != models.StatusActive {
		t.Errorf("after RecordSuccess: streak=%d status=%q", got.FailureStreak, got.Status)
	}
}

func TestWebhookCloseCircuit(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 1)
	s.Webhooks.RecordFailure(ctx, wh.ID)

	got, _ := s.Webhooks.Get(ctx, wh.ID)
	if got.Status != models.StatusCircuitOpen {
		t.Fatalf("expected circuit_open, got %q", got.Status)
	}

	if err := s.Webhooks.CloseCircuit(ctx, wh.ID); err != nil {
		t.Fatalf("CloseCircuit: %v", err)
	}

	got, _ = s.Webhooks.Get(ctx, wh.ID)
	if got.Status != models.StatusActive || got.FailureStreak != 0 || got.NextProbeAt != nil {
		t.Errorf("after CloseCircuit: status=%q streak=%d probe=%v", got.Status, got.FailureStreak, got.NextProbeAt)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/db/... -run TestWebhook -v
```

Expected: compile error — `db.OpenStores` not defined.

- [ ] **Step 3: Create `internal/db/webhook.go`**

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/b2randon/webhook-delivery/internal/models"
)

type WebhookStore struct{ db *sql.DB }

func (s *WebhookStore) Create(ctx context.Context, url, encryptedSecret, hint string, threshold int) (*models.Webhook, error) {
	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhooks (id, url, encrypted_secret, secret_hint, circuit_threshold)
		VALUES (?, ?, ?, ?, ?)`,
		id, url, encryptedSecret, hint, threshold)
	if err != nil {
		return nil, fmt.Errorf("insert webhook: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *WebhookStore) Get(ctx context.Context, id string) (*models.Webhook, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, url, encrypted_secret, secret_hint, status, failure_streak,
		       circuit_threshold, next_probe_at, created_at, updated_at
		FROM webhooks WHERE id = ?`, id)
	return scanWebhook(row)
}

func (s *WebhookStore) List(ctx context.Context) ([]models.Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, url, encrypted_secret, secret_hint, status, failure_streak,
		       circuit_threshold, next_probe_at, created_at, updated_at
		FROM webhooks WHERE status != 'deleted' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func (s *WebhookStore) SoftDelete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE webhooks SET status = 'deleted', updated_at = datetime('now') WHERE id = ?`, id)
	return err
}

func (s *WebhookStore) RecordFailure(ctx context.Context, id string) (newStreak int, newStatus models.WebhookStatus, err error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE webhooks SET
			failure_streak = failure_streak + 1,
			status = CASE
				WHEN failure_streak + 1 >= circuit_threshold THEN 'circuit_open'
				ELSE 'degraded'
			END,
			next_probe_at = CASE
				WHEN failure_streak + 1 >= circuit_threshold THEN datetime('now', '+5 minutes')
				ELSE next_probe_at
			END,
			updated_at = datetime('now')
		WHERE id = ?
		RETURNING failure_streak, status`, id)
	err = row.Scan(&newStreak, &newStatus)
	return
}

func (s *WebhookStore) RecordSuccess(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE webhooks SET
			failure_streak = 0,
			status = 'active',
			next_probe_at = NULL,
			updated_at = datetime('now')
		WHERE id = ?`, id)
	return err
}

func (s *WebhookStore) CloseCircuit(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE webhooks SET
			status = 'active',
			failure_streak = 0,
			next_probe_at = NULL,
			updated_at = datetime('now')
		WHERE id = ?`, id)
	return err
}

func (s *WebhookStore) SetCircuitOpen(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE webhooks SET
			status = 'circuit_open',
			next_probe_at = datetime('now', '+5 minutes'),
			updated_at = datetime('now')
		WHERE id = ?`, id)
	return err
}

// scanner works for both *sql.Row and *sql.Rows
type rowScanner interface {
	Scan(dest ...any) error
}

func scanWebhook(row rowScanner) (*models.Webhook, error) {
	var w models.Webhook
	var nextProbeAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(
		&w.ID, &w.URL, &w.EncryptedSecret, &w.SecretHint, &w.Status,
		&w.FailureStreak, &w.CircuitThreshold, &nextProbeAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	if nextProbeAt.Valid {
		t, _ := time.Parse(time.RFC3339, nextProbeAt.String)
		w.NextProbeAt = &t
	}
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &w, nil
}
```

- [ ] **Step 4: Create `internal/db/stores.go` (wires all stores together)**

```go
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Stores groups all repository types for convenient passing through the app.
type Stores struct {
	Webhooks  *WebhookStore
	Events    *EventStore
	Deliveries *DeliveryStore
	db        *sql.DB
}

func (s *Stores) Close() error { return s.db.Close() }

// OpenStores opens (or creates) the SQLite database and returns all stores.
// Use path ":memory:" in tests.
func OpenStores(path string) (*Stores, error) {
	var (
		sqldb *sql.DB
		err   error
	)
	if path == ":memory:" {
		sqldb, err = testDB()
	} else {
		sqldb, err = Open(path)
	}
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return &Stores{
		Webhooks:   &WebhookStore{db: sqldb},
		Events:     &EventStore{db: sqldb},
		Deliveries: &DeliveryStore{db: sqldb},
		db:         sqldb,
	}, nil
}
```

- [ ] **Step 5: Create stub `internal/db/event.go` and `internal/db/delivery.go` so it compiles**

```go
// internal/db/event.go
package db

import "database/sql"

type EventStore struct{ db *sql.DB }
```

```go
// internal/db/delivery.go
package db

import "database/sql"

type DeliveryStore struct{ db *sql.DB }
```

- [ ] **Step 6: Run webhook tests — expect PASS**

```bash
go test ./internal/db/... -run TestWebhook -v
```

Expected: all 6 webhook tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/db/
git commit -m "feat: webhook store with circuit breaker state transitions"
```

---

## Task 6: Event Store

**Files:**
- Modify: `internal/db/event.go`
- Create: `internal/db/event_test.go`

- [ ] **Step 1: Write `internal/db/event_test.go`**

```go
package db_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/models"
)

func makeEvent(id, typ string) *models.Event {
	return &models.Event{
		ID:     id,
		Type:   typ,
		Source: "https://test.example.com",
		Time:   time.Now().UTC().Truncate(time.Second),
		Data:   json.RawMessage(`{"key":"value"}`),
	}
}

func TestEventCreateAndGet(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	ev := makeEvent("evt-1", "order.created")
	if err := s.Events.Create(ctx, ev); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Events.Get(ctx, "evt-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Type != "order.created" {
		t.Errorf("Type = %q", got.Type)
	}
}

func TestEventDuplicateIDErrors(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	ev := makeEvent("evt-dup", "order.created")
	s.Events.Create(ctx, ev)
	err := s.Events.Create(ctx, ev)
	if err == nil {
		t.Error("expected error on duplicate event ID")
	}
}

func TestEventList(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	for i, typ := range []string{"a", "b", "c"} {
		s.Events.Create(ctx, makeEvent(fmt.Sprintf("evt-%d", i), typ))
	}

	list, err := s.Events.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List len = %d, want 3", len(list))
	}
}

func TestEventVolume(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	s.Events.Create(ctx, makeEvent("e1", "order.created"))
	s.Events.Create(ctx, makeEvent("e2", "order.created"))
	s.Events.Create(ctx, makeEvent("e3", "payment.failed"))

	pts, err := s.Events.Volume(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("Volume: %v", err)
	}
	counts := map[string]int{}
	for _, p := range pts {
		counts[p.Type] = p.Count
	}
	if counts["order.created"] != 2 {
		t.Errorf("order.created count = %d, want 2", counts["order.created"])
	}
	if counts["payment.failed"] != 1 {
		t.Errorf("payment.failed count = %d, want 1", counts["payment.failed"])
	}
}
```

Add `"fmt"` import to the test file — add it to the import block in `event_test.go`.

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/db/... -run TestEvent -v
```

- [ ] **Step 3: Replace `internal/db/event.go` with full implementation**

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/b2randon/webhook-delivery/internal/models"
)

type EventStore struct{ db *sql.DB }

func (s *EventStore) Create(ctx context.Context, e *models.Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, type, source, time, data)
		VALUES (?, ?, ?, ?, ?)`,
		e.ID, e.Type, e.Source,
		e.Time.UTC().Format(time.RFC3339),
		string(e.Data),
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (s *EventStore) Get(ctx context.Context, id string) (*models.Event, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, type, source, time, data, received_at
		FROM events WHERE id = ?`, id)
	return scanEvent(row)
}

func (s *EventStore) List(ctx context.Context, limit int) ([]models.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, source, time, data, received_at
		FROM events ORDER BY received_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *EventStore) Volume(ctx context.Context, window time.Duration) ([]models.VolumePoint, error) {
	since := time.Now().Add(-window).UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT type, COUNT(*) as count FROM events
		WHERE received_at >= ?
		GROUP BY type ORDER BY count DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.VolumePoint
	for rows.Next() {
		var p models.VolumePoint
		if err := rows.Scan(&p.Type, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanEvent(row rowScanner) (*models.Event, error) {
	var e models.Event
	var t, receivedAt, data string
	if err := row.Scan(&e.ID, &e.Type, &e.Source, &t, &data, &receivedAt); err != nil {
		return nil, err
	}
	e.Time, _ = time.Parse(time.RFC3339, t)
	e.ReceivedAt, _ = time.Parse(time.RFC3339, receivedAt)
	e.Data = []byte(data)
	return &e, nil
}
```

- [ ] **Step 4: Add `"fmt"` to event_test.go imports**

The test file uses `fmt.Sprintf`. Add `"fmt"` to the import block in `internal/db/event_test.go`.

- [ ] **Step 5: Run tests — expect PASS**

```bash
go test ./internal/db/... -run TestEvent -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/db/event.go internal/db/event_test.go
git commit -m "feat: event store with volume aggregation"
```

---

## Task 7: Delivery Store

**Files:**
- Modify: `internal/db/delivery.go`
- Create: `internal/db/delivery_test.go`

- [ ] **Step 1: Write `internal/db/delivery_test.go`**

```go
package db_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
)

func seedWebhookAndEvent(t *testing.T, s *db.Stores) (wh models.Webhook, ev models.Event) {
	t.Helper()
	ctx := context.Background()
	w, err := s.Webhooks.Create(ctx, "https://example.com/hook", "enc", "hint", 5)
	if err != nil {
		t.Fatal(err)
	}
	e := &models.Event{
		ID: "evt-seed", Type: "order.created", Source: "src",
		Time: time.Now().UTC(), Data: json.RawMessage(`{}`),
	}
	if err := s.Events.Create(ctx, e); err != nil {
		t.Fatal(err)
	}
	return *w, *e
}

func TestDeliveryCreateBatchPending(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)

	err := s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	list, err := s.Deliveries.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(list))
	}
	if list[0].Status != models.DeliveryPending {
		t.Errorf("status = %q, want pending", list[0].Status)
	}
}

func TestDeliveryCreateBatchHeldWhenCircuitOpen(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)

	s.Webhooks.SetCircuitOpen(ctx, wh.ID)
	wh.Status = models.StatusCircuitOpen

	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	if list[0].Status != models.DeliveryHeld {
		t.Errorf("status = %q, want held for circuit_open webhook", list[0].Status)
	}
}

func TestDeliveryMarkInFlightCAS(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID

	// Two goroutines race to claim the same delivery — only one should win.
	var wins int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, _ := s.Deliveries.MarkInFlight(ctx, id)
			if claimed {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Errorf("CAS: %d goroutines claimed the same delivery, want exactly 1", wins)
	}
}

func TestDeliveryMarkSuccess(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID
	s.Deliveries.MarkInFlight(ctx, id)

	if err := s.Deliveries.MarkSuccess(ctx, id, 200, 42); err != nil {
		t.Fatalf("MarkSuccess: %v", err)
	}
	d, _ := s.Deliveries.Get(ctx, id)
	if d.Status != models.DeliverySuccess {
		t.Errorf("status = %q, want success", d.Status)
	}
	if *d.LastStatusCode != 200 || *d.LastResponseMs != 42 {
		t.Errorf("unexpected status code or response ms")
	}
}

func TestDeliveryMarkFailedWithRetry(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID
	s.Deliveries.MarkInFlight(ctx, id)

	next := time.Now().Add(10 * time.Second)
	code := 500
	ms := 100
	msg := "server error"
	if err := s.Deliveries.MarkFailed(ctx, id, 1, &code, &ms, &msg, &next); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	d, _ := s.Deliveries.Get(ctx, id)
	if d.Status != models.DeliveryPending {
		t.Errorf("status = %q, want pending (retry scheduled)", d.Status)
	}
	if d.NextAttemptAt == nil {
		t.Error("NextAttemptAt must be set")
	}
}

func TestDeliveryMarkFailedTerminal(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID
	s.Deliveries.MarkInFlight(ctx, id)

	if err := s.Deliveries.MarkFailed(ctx, id, 5, nil, nil, nil, nil); err != nil {
		t.Fatalf("MarkFailed terminal: %v", err)
	}

	d, _ := s.Deliveries.Get(ctx, id)
	if d.Status != models.DeliveryFailed {
		t.Errorf("status = %q, want failed", d.Status)
	}
}

func TestDeliveryResetInFlight(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	s.Deliveries.MarkInFlight(ctx, list[0].ID)

	if err := s.Deliveries.ResetInFlight(ctx); err != nil {
		t.Fatalf("ResetInFlight: %v", err)
	}

	d, _ := s.Deliveries.Get(ctx, list[0].ID)
	if d.Status != models.DeliveryPending {
		t.Errorf("status = %q, want pending after reset", d.Status)
	}
}

func TestDeliveryAbortForWebhook(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	if err := s.Deliveries.AbortForWebhook(ctx, wh.ID); err != nil {
		t.Fatalf("AbortForWebhook: %v", err)
	}

	list, _ := s.Deliveries.List(ctx, 10)
	if list[0].Status != models.DeliveryFailed {
		t.Errorf("status = %q, want failed", list[0].Status)
	}
}

func TestDeliveryFlushHeld(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, _ := seedWebhookAndEvent(t, s)

	s.Webhooks.SetCircuitOpen(ctx, wh.ID)
	wh.Status = models.StatusCircuitOpen

	// Create 15 held deliveries across 15 events.
	for i := 0; i < 15; i++ {
		ev := &models.Event{
			ID: fmt.Sprintf("ev-%d", i), Type: "t", Source: "s",
			Time: time.Now().UTC(), Data: json.RawMessage(`{}`),
		}
		s.Events.Create(ctx, ev)
		s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})
	}

	// Flush should move only 10 to pending.
	if err := s.Deliveries.FlushHeld(ctx, wh.ID); err != nil {
		t.Fatalf("FlushHeld: %v", err)
	}

	pending, _ := s.Deliveries.ClaimPending(ctx, time.Now().Add(time.Minute), 20)
	if len(pending) != 10 {
		t.Errorf("after FlushHeld: %d pending, want 10", len(pending))
	}
}

func TestDeliveryCreateRedelivery(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	original := list[0]

	// Mark as failed (terminal).
	s.Deliveries.MarkInFlight(ctx, original.ID)
	s.Deliveries.MarkFailed(ctx, original.ID, 5, nil, nil, nil, nil)

	redel, err := s.Deliveries.CreateRedelivery(ctx, original.ID)
	if err != nil {
		t.Fatalf("CreateRedelivery: %v", err)
	}
	if redel.ParentDeliveryID == nil || *redel.ParentDeliveryID != original.ID {
		t.Error("ParentDeliveryID not set correctly")
	}
	if redel.Status != models.DeliveryPending {
		t.Errorf("redelivery status = %q, want pending", redel.Status)
	}
	if redel.Attempt != 0 {
		t.Errorf("redelivery attempt = %d, want 0", redel.Attempt)
	}
}

func TestDeliveryHasActiveRedelivery(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	s.Deliveries.MarkInFlight(ctx, list[0].ID)
	s.Deliveries.MarkFailed(ctx, list[0].ID, 5, nil, nil, nil, nil)

	s.Deliveries.CreateRedelivery(ctx, list[0].ID)

	active, err := s.Deliveries.HasActiveRedelivery(ctx, ev.ID, wh.ID)
	if err != nil {
		t.Fatalf("HasActiveRedelivery: %v", err)
	}
	if active == nil {
		t.Error("expected active redelivery to be found")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/db/... -run TestDelivery -v
```

- [ ] **Step 3: Replace `internal/db/delivery.go` with full implementation**

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/b2randon/webhook-delivery/internal/models"
)

type DeliveryStore struct{ db *sql.DB }

func (s *DeliveryStore) CreateBatch(ctx context.Context, eventID string, webhooks []models.Webhook) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, wh := range webhooks {
		if wh.Status == models.StatusDeleted {
			continue
		}
		status := models.DeliveryPending
		var nextAt any
		if wh.Status == models.StatusCircuitOpen {
			status = models.DeliveryHeld
		} else {
			nextAt = time.Now().UTC().Format(time.RFC3339)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO deliveries (id, event_id, webhook_id, status, next_attempt_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(event_id, webhook_id) WHERE parent_delivery_id IS NULL DO NOTHING`,
			uuid.New().String(), eventID, wh.ID, string(status), nextAt)
		if err != nil {
			return fmt.Errorf("insert delivery for webhook %s: %w", wh.ID, err)
		}
	}
	return tx.Commit()
}

func (s *DeliveryStore) Get(ctx context.Context, id string) (*models.Delivery, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT d.id, d.event_id, d.webhook_id, d.parent_delivery_id, d.status, d.attempt,
		       d.next_attempt_at, d.last_status_code, d.last_response_ms, d.last_error,
		       d.created_at, d.updated_at,
		       COALESCE(e.type,'') as event_type, COALESCE(w.url,'') as webhook_url
		FROM deliveries d
		LEFT JOIN events e ON e.id = d.event_id
		LEFT JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.id = ?`, id)
	return scanDelivery(row)
}

func (s *DeliveryStore) List(ctx context.Context, limit int) ([]models.Delivery, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.event_id, d.webhook_id, d.parent_delivery_id, d.status, d.attempt,
		       d.next_attempt_at, d.last_status_code, d.last_response_ms, d.last_error,
		       d.created_at, d.updated_at,
		       COALESCE(e.type,'') as event_type, COALESCE(w.url,'') as webhook_url
		FROM deliveries d
		LEFT JOIN events e ON e.id = d.event_id
		LEFT JOIN webhooks w ON w.id = d.webhook_id
		ORDER BY d.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Delivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (s *DeliveryStore) ClaimPending(ctx context.Context, now time.Time, limit int) ([]models.Delivery, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.event_id, d.webhook_id, d.parent_delivery_id, d.status, d.attempt,
		       d.next_attempt_at, d.last_status_code, d.last_response_ms, d.last_error,
		       d.created_at, d.updated_at,
		       COALESCE(e.type,'') as event_type, COALESCE(w.url,'') as webhook_url
		FROM deliveries d
		LEFT JOIN events e ON e.id = d.event_id
		LEFT JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.status = 'pending' AND d.next_attempt_at <= ?
		LIMIT ?`, now.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Delivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// MarkInFlight atomically claims a pending delivery. Returns false if another
// worker already claimed it (CAS — check RowsAffected).
func (s *DeliveryStore) MarkInFlight(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET status = 'in_flight', updated_at = datetime('now')
		WHERE id = ? AND status = 'pending'`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (s *DeliveryStore) MarkSuccess(ctx context.Context, id string, statusCode, responseMs int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET
			status = 'success',
			last_status_code = ?,
			last_response_ms = ?,
			updated_at = datetime('now')
		WHERE id = ?`, statusCode, responseMs, id)
	return err
}

// MarkFailed sets status to 'pending' with nextAttemptAt if a retry is due,
// or 'failed' if nextAttemptAt is nil (all attempts exhausted).
func (s *DeliveryStore) MarkFailed(ctx context.Context, id string, attempt int, statusCode, responseMs *int, errMsg *string, nextAttemptAt *time.Time) error {
	status := "failed"
	var nextAt any
	if nextAttemptAt != nil {
		status = "pending"
		nextAt = nextAttemptAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET
			status = ?,
			attempt = ?,
			last_status_code = ?,
			last_response_ms = ?,
			last_error = ?,
			next_attempt_at = ?,
			updated_at = datetime('now')
		WHERE id = ?`,
		status, attempt, statusCode, responseMs, errMsg, nextAt, id)
	return err
}

func (s *DeliveryStore) MarkHeld(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE deliveries SET status = 'held', updated_at = datetime('now') WHERE id = ?`, id)
	return err
}

// FlushHeld moves up to 10 held deliveries for a webhook to pending (oldest first).
func (s *DeliveryStore) FlushHeld(ctx context.Context, webhookID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET status = 'pending', next_attempt_at = ?, updated_at = datetime('now')
		WHERE id IN (
			SELECT id FROM deliveries
			WHERE webhook_id = ? AND status = 'held'
			ORDER BY created_at ASC LIMIT 10
		)`, now, webhookID)
	return err
}

func (s *DeliveryStore) AbortForWebhook(ctx context.Context, webhookID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET
			status = 'failed',
			last_error = 'webhook deleted',
			updated_at = datetime('now')
		WHERE webhook_id = ? AND status IN ('pending', 'in_flight', 'held')`, webhookID)
	return err
}

func (s *DeliveryStore) ResetInFlight(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET
			status = 'pending',
			next_attempt_at = ?,
			updated_at = datetime('now')
		WHERE status = 'in_flight'`, now)
	return err
}

func (s *DeliveryStore) OldestHeld(ctx context.Context, webhookID string) (*models.Delivery, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT d.id, d.event_id, d.webhook_id, d.parent_delivery_id, d.status, d.attempt,
		       d.next_attempt_at, d.last_status_code, d.last_response_ms, d.last_error,
		       d.created_at, d.updated_at, '' as event_type, '' as webhook_url
		FROM deliveries d
		WHERE d.webhook_id = ? AND d.status = 'held'
		ORDER BY d.created_at ASC LIMIT 1`, webhookID)
	d, err := scanDelivery(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func (s *DeliveryStore) CreateRedelivery(ctx context.Context, parentID string) (*models.Delivery, error) {
	parent, err := s.Get(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("get parent delivery: %w", err)
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO deliveries (id, event_id, webhook_id, parent_delivery_id, status, attempt, next_attempt_at)
		VALUES (?, ?, ?, ?, 'pending', 0, ?)`,
		id, parent.EventID, parent.WebhookID, parentID, now)
	if err != nil {
		return nil, fmt.Errorf("insert redelivery: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *DeliveryStore) HasActiveRedelivery(ctx context.Context, eventID, webhookID string) (*models.Delivery, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id FROM deliveries
		WHERE event_id = ? AND webhook_id = ?
		  AND parent_delivery_id IS NOT NULL
		  AND status IN ('pending', 'in_flight')
		LIMIT 1`, eventID, webhookID)
	var id string
	if err := row.Scan(&id); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func scanDelivery(row rowScanner) (*models.Delivery, error) {
	var d models.Delivery
	var parentID, nextAt, lastErr sql.NullString
	var statusCode, responseMs sql.NullInt64
	var createdAt, updatedAt string

	err := row.Scan(
		&d.ID, &d.EventID, &d.WebhookID, &parentID, &d.Status, &d.Attempt,
		&nextAt, &statusCode, &responseMs, &lastErr,
		&createdAt, &updatedAt, &d.EventType, &d.WebhookURL,
	)
	if err != nil {
		return nil, err
	}

	if parentID.Valid {
		d.ParentDeliveryID = &parentID.String
	}
	if nextAt.Valid {
		t, _ := time.Parse(time.RFC3339, nextAt.String)
		d.NextAttemptAt = &t
	}
	if statusCode.Valid {
		n := int(statusCode.Int64)
		d.LastStatusCode = &n
	}
	if responseMs.Valid {
		n := int(responseMs.Int64)
		d.LastResponseMs = &n
	}
	if lastErr.Valid {
		d.LastError = &lastErr.String
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	d.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &d, nil
}
```

- [ ] **Step 4: Run all delivery tests — expect PASS**

```bash
go test ./internal/db/... -run TestDelivery -v
```

Expected: all 9 delivery tests PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -v
```

Expected: all tests across crypto, config, and db packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/delivery.go internal/db/delivery_test.go internal/db/stores.go
git commit -m "feat: delivery store — CAS claiming, retry scheduling, circuit flush, redelivery"
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Covered by |
|---|---|
| SQLite WAL + busy_timeout | Task 1 `open.go` DSN |
| AES-256-GCM encrypt/decrypt | Task 3 `aes.go` |
| HMAC-SHA256 sign (bytes invariant) | Task 3 `hmac.go` |
| Secrets persist across restarts | Task 4 `config.go` |
| Auto-generate key+apikey on first run | Task 4 `loadOrCreate` |
| Startup validates key length | Task 4 `Load` |
| Webhook create/list/get/delete | Task 5 `webhook.go` |
| Circuit state machine (RecordFailure/Success/Close) | Task 5 `webhook.go` |
| Event create/list/volume | Task 6 `event.go` |
| Duplicate event ID → error | Task 6, test |
| Delivery batch create (pending vs held) | Task 7 `delivery.go` |
| CAS MarkInFlight (race test) | Task 7, `TestDeliveryMarkInFlightCAS` |
| Retry scheduling vs terminal failure | Task 7 `MarkFailed` |
| Startup in_flight reset | Task 7 `ResetInFlight` |
| Soft-delete aborts deliveries | Task 7 `AbortForWebhook` |
| FlushHeld batches 10 at a time | Task 7 `FlushHeld`, test |
| Re-delivery with parent tracking | Task 7 `CreateRedelivery` |
| Duplicate re-delivery guard | Task 7 `HasActiveRedelivery` |

**Placeholder scan:** None found. All steps contain complete code.

**Type consistency:** `models.StatusCircuitOpen`, `models.DeliveryPending`, `models.DeliveryHeld`, `models.DeliveryFailed` used consistently across stores and tests.

---

**Plan complete and saved to `docs/superpowers/plans/2026-06-16-webhook-delivery-01-foundation.md`.**

**Two execution options:**

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks

**2. Inline Execution** — execute tasks in this session using executing-plans

Which approach?
