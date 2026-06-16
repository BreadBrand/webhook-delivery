package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/models"
)

func validEventBody() string {
	return `{
		"specversion": "1.0",
		"id": "evt-001",
		"type": "order.created",
		"source": "https://shop.example.com",
		"time": "2026-01-01T00:00:00Z",
		"data": {"amount": 42}
	}`
}

func TestIngestEvent_Valid(t *testing.T) {
	router, stores := testServer(t)
	seedWebhook(t, router, "https://recv.example.com")

	w := do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", w.Code, w.Body)
	}

	events, _ := stores.Events.List(context.Background(), 10)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	deliveries, _ := stores.Deliveries.List(context.Background(), 10)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].Status != models.DeliveryPending {
		t.Errorf("delivery status = %q, want pending", deliveries[0].Status)
	}
}

func TestIngestEvent_NoWebhooks(t *testing.T) {
	router, stores := testServer(t)
	w := do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	deliveries, _ := stores.Deliveries.List(context.Background(), 10)
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries when no webhooks registered, got %d", len(deliveries))
	}
}

func TestIngestEvent_MissingField(t *testing.T) {
	router, _ := testServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"missing id", `{"specversion":"1.0","type":"t","source":"s","time":"2026-01-01T00:00:00Z","data":{}}`},
		{"missing type", `{"specversion":"1.0","id":"x","source":"s","time":"2026-01-01T00:00:00Z","data":{}}`},
		{"missing source", `{"specversion":"1.0","id":"x","type":"t","time":"2026-01-01T00:00:00Z","data":{}}`},
		{"missing time", `{"specversion":"1.0","id":"x","type":"t","source":"s","data":{}}`},
		{"missing data", `{"specversion":"1.0","id":"x","type":"t","source":"s","time":"2026-01-01T00:00:00Z"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, router, authReq(t, http.MethodPost, "/events", tc.body))
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestIngestEvent_BadSpecVersion(t *testing.T) {
	router, _ := testServer(t)
	body := `{"specversion":"2.0","id":"x","type":"t","source":"s","time":"2026-01-01T00:00:00Z","data":{}}`
	w := do(t, router, authReq(t, http.MethodPost, "/events", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestIngestEvent_NullData(t *testing.T) {
	router, _ := testServer(t)
	body := `{"specversion":"1.0","id":"x","type":"t","source":"s","time":"2026-01-01T00:00:00Z","data":null}`
	w := do(t, router, authReq(t, http.MethodPost, "/events", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for null data", w.Code)
	}
}

func TestIngestEvent_ScalarData(t *testing.T) {
	router, _ := testServer(t)
	body := `{"specversion":"1.0","id":"x","type":"t","source":"s","time":"2026-01-01T00:00:00Z","data":42}`
	w := do(t, router, authReq(t, http.MethodPost, "/events", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for scalar data", w.Code)
	}
}

func TestIngestEvent_TooLarge(t *testing.T) {
	router, _ := testServer(t)
	big := `{"specversion":"1.0","id":"x","type":"t","source":"s","time":"2026-01-01T00:00:00Z","data":{"x":"` +
		strings.Repeat("a", 1<<20+1) + `"}}`
	w := do(t, router, authReq(t, http.MethodPost, "/events", big))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
}

func TestIngestEvent_Duplicate(t *testing.T) {
	router, _ := testServer(t)
	body := validEventBody()
	do(t, router, authReq(t, http.MethodPost, "/events", body))
	w := do(t, router, authReq(t, http.MethodPost, "/events", body))
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 on duplicate id", w.Code)
	}
	var resp struct {
		ID         string `json:"id"`
		ReceivedAt string `json:"received_at"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.ID != "evt-001" {
		t.Errorf("duplicate response id = %q, want evt-001", resp.ID)
	}
}

func TestIngestEvent_CircuitOpenWebhook(t *testing.T) {
	router, stores := testServer(t)
	wh := seedWebhook(t, router, "https://recv.example.com")

	do(t, router, authReq(t, http.MethodPost, "/webhooks/"+wh.ID+"/circuit", `{"action":"open"}`))

	w := do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}

	deliveries, _ := stores.Deliveries.List(context.Background(), 10)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].Status != models.DeliveryHeld {
		t.Errorf("delivery status = %q, want held for circuit_open webhook", deliveries[0].Status)
	}
}

func TestListEvents(t *testing.T) {
	router, _ := testServer(t)
	do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))

	w := do(t, router, authReq(t, http.MethodGet, "/events", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var list []models.Event
	decodeJSON(t, w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("got %d events, want 1", len(list))
	}
}

func TestEventVolume(t *testing.T) {
	router, _ := testServer(t)
	do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))

	w := do(t, router, authReq(t, http.MethodGet, "/events/volume?window=30m", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var vol []models.VolumePoint
	decodeJSON(t, w.Body.Bytes(), &vol)
	if len(vol) == 0 {
		t.Error("expected at least one volume point")
	}
}

func TestEventVolume_InvalidWindow(t *testing.T) {
	router, _ := testServer(t)
	w := do(t, router, authReq(t, http.MethodGet, "/events/volume?window=99h", ""))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

var _ = time.Now
