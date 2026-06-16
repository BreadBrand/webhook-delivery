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
