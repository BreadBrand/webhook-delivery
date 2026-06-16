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
    updated_at         DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Partial unique index: only one root delivery (no parent) per event+webhook pair.
CREATE UNIQUE INDEX IF NOT EXISTS uidx_deliveries_root
    ON deliveries(event_id, webhook_id)
    WHERE parent_delivery_id IS NULL;
