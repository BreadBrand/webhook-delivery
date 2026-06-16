package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
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
