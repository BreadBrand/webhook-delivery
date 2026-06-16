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
			nextAt = time.Now().UTC().Format("2006-01-02 15:04:05")
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
	out := make([]models.Delivery, 0)
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
		LIMIT ?`, now.UTC().Format("2006-01-02 15:04:05"), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Delivery, 0)
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
// worker already claimed it (CAS — checks RowsAffected).
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
		nextAt = nextAttemptAt.UTC().Format("2006-01-02 15:04:05")
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

// FlushHeld moves all held deliveries for a webhook to pending (oldest first).
func (s *DeliveryStore) FlushHeld(ctx context.Context, webhookID string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET status = 'pending', next_attempt_at = ?, updated_at = datetime('now')
		WHERE id IN (
			SELECT id FROM deliveries
			WHERE webhook_id = ? AND status = 'held'
			ORDER BY created_at ASC
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
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
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
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
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

func (s *DeliveryStore) HoldPendingForWebhook(ctx context.Context, webhookID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET status = 'held', updated_at = datetime('now')
		WHERE webhook_id = ? AND status = 'pending'`, webhookID)
	return err
}

// MarkProbeInFlight atomically transitions a held delivery to in_flight (for circuit probe).
// Returns false if the delivery is not in held state (CAS — checks RowsAffected).
func (s *DeliveryStore) MarkProbeInFlight(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE deliveries SET status = 'in_flight', updated_at = datetime('now')
		WHERE id = ? AND status = 'held'`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (s *DeliveryStore) CountPending(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deliveries WHERE status = 'pending'`).Scan(&n)
	return n, err
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
		t := parseTime(nextAt.String)
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
	d.CreatedAt = parseTime(createdAt)
	d.UpdatedAt = parseTime(updatedAt)
	return &d, nil
}
