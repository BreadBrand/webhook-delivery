package db

// event_internal_test.go uses package db (not db_test) so it can access the
// unexported EventStore.db field to insert rows with backdated received_at
// timestamps. This is needed to test that Volume correctly excludes events
// that fall outside the requested window — something that cannot be verified
// through the public Create API because received_at is set by SQLite DEFAULT.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func mustOpenStoreInternal(t *testing.T) *EventStore {
	t.Helper()
	db, err := testDB()
	if err != nil {
		t.Fatalf("testDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &EventStore{db: db}
}

func insertEventAt(t *testing.T, db *sql.DB, id, typ string, receivedAt time.Time) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO events (id, type, source, time, data, received_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, typ, "https://test.example.com",
		receivedAt.UTC().Format("2006-01-02 15:04:05"),
		string(json.RawMessage(`{"key":"value"}`)),
		receivedAt.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		t.Fatalf("insertEventAt %s: %v", id, err)
	}
}

func TestEventVolumeExcludesOutsideWindow(t *testing.T) {
	store := mustOpenStoreInternal(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Two events inside the 30-minute window.
	insertEventAt(t, store.db, "in-1", "order.created", now.Add(-5*time.Minute))
	insertEventAt(t, store.db, "in-2", "order.created", now.Add(-10*time.Minute))
	// One event outside the window — should be excluded.
	insertEventAt(t, store.db, "out-1", "order.created", now.Add(-60*time.Minute))

	pts, err := store.Volume(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("Volume: %v", err)
	}

	counts := map[string]int{}
	for _, p := range pts {
		counts[p.Type] = p.Count
	}

	if counts["order.created"] != 2 {
		t.Errorf("order.created count = %d, want 2 (event outside window must be excluded)", counts["order.created"])
	}
}

func TestEventListOrdering(t *testing.T) {
	store := mustOpenStoreInternal(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Insert with explicit received_at values so ordering is deterministic.
	for i, typ := range []string{"oldest", "middle", "newest"} {
		insertEventAt(t, store.db, fmt.Sprintf("ord-%d", i), typ, now.Add(time.Duration(i)*time.Minute))
	}

	list, err := store.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List len = %d, want 3", len(list))
	}
	// ORDER BY received_at DESC: newest first.
	if list[0].Type != "newest" {
		t.Errorf("list[0].Type = %q, want %q (most recent first)", list[0].Type, "newest")
	}
	if list[2].Type != "oldest" {
		t.Errorf("list[2].Type = %q, want %q (oldest last)", list[2].Type, "oldest")
	}
}
