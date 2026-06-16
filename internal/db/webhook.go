package db

import (
	"context"
	"database/sql"
	"fmt"

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

// rowScanner works for both *sql.Row and *sql.Rows.
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
		t := parseTime(nextProbeAt.String)
		w.NextProbeAt = &t
	}
	w.CreatedAt = parseTime(createdAt)
	w.UpdatedAt = parseTime(updatedAt)
	return &w, nil
}
