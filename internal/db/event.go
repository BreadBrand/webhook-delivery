package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/b2randon/webhook-delivery/internal/models"
)

// ErrConflict is returned when an INSERT violates a UNIQUE constraint.
var ErrConflict = errors.New("conflict")

type EventStore struct{ db *sql.DB }

func (s *EventStore) Create(ctx context.Context, e *models.Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, type, source, time, data)
		VALUES (?, ?, ?, ?, ?)`,
		e.ID, e.Type, e.Source,
		e.Time.UTC().Format("2006-01-02 15:04:05"),
		string(e.Data),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrConflict
		}
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
	out := make([]models.Event, 0)
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
	since := time.Now().Add(-window).UTC().Format("2006-01-02 15:04:05")
	rows, err := s.db.QueryContext(ctx, `
		SELECT type, COUNT(*) as count FROM events
		WHERE received_at >= ?
		GROUP BY type ORDER BY count DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.VolumePoint, 0)
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
	e.Time = parseTime(t)
	e.ReceivedAt = parseTime(receivedAt)
	e.Data = []byte(data)
	return &e, nil
}
