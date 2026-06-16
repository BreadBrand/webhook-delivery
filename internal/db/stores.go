package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/b2randon/webhook-delivery/internal/models"
)

// Stores groups all repository types for convenient passing through the app.
type Stores struct {
	Webhooks   *WebhookStore
	Events     *EventStore
	Deliveries *DeliveryStore
	db         *sql.DB
}

func (s *Stores) Close() error { return s.db.Close() }

func (s *Stores) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// CreateEventWithDeliveries inserts ev and its delivery rows atomically.
// Returns ErrConflict if ev.ID already exists.
func (s *Stores) CreateEventWithDeliveries(ctx context.Context, ev *models.Event, webhooks []models.Webhook) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO events (id, type, source, time, data) VALUES (?, ?, ?, ?, ?)`,
		ev.ID, ev.Type, ev.Source,
		ev.Time.UTC().Format("2006-01-02 15:04:05"),
		string(ev.Data),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrConflict
		}
		return fmt.Errorf("insert event: %w", err)
	}

	for _, wh := range webhooks {
		if wh.Status == models.StatusDeleted {
			continue
		}
		status := models.DeliveryPending
		var nextAt any
		if wh.Status == models.StatusCircuitOpen {
			status = models.DeliveryHeld
		} else {
			nextAt = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO deliveries (id, event_id, webhook_id, status, next_attempt_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(event_id, webhook_id) WHERE parent_delivery_id IS NULL DO NOTHING`,
			uuid.New().String(), ev.ID, wh.ID, string(status), nextAt,
		); err != nil {
			return fmt.Errorf("insert delivery for webhook %s: %w", wh.ID, err)
		}
	}

	return tx.Commit()
}

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
