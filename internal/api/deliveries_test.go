package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/models"
)

func TestListDeliveries(t *testing.T) {
	router, _ := testServer(t)
	seedWebhook(t, router, "https://recv.example.com")
	do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))

	w := do(t, router, authReq(t, http.MethodGet, "/deliveries", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var list []models.Delivery
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("got %d deliveries, want 1", len(list))
	}
}

func TestRedeliver_OnFailed(t *testing.T) {
	router, stores := testServer(t)
	seedWebhook(t, router, "https://recv.example.com")
	do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))

	ctx := context.Background()

	deliveries, _ := stores.Deliveries.List(ctx, 1)
	if len(deliveries) == 0 {
		t.Fatal("no delivery created")
	}
	d := deliveries[0]
	stores.Deliveries.MarkInFlight(ctx, d.ID)
	errMsg := "forced"
	stores.Deliveries.MarkFailed(ctx, d.ID, 5, nil, nil, &errMsg, nil)

	w := do(t, router, authReq(t, http.MethodPost, "/deliveries/"+d.ID+"/redeliver", ""))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", w.Code, w.Body)
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == "" || resp.ID == d.ID {
		t.Errorf("redeliver ID = %q, want a new non-empty id", resp.ID)
	}
}

func TestRedeliver_OnPending_Returns409(t *testing.T) {
	router, stores := testServer(t)
	seedWebhook(t, router, "https://recv.example.com")
	do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))

	deliveries, _ := stores.Deliveries.List(context.Background(), 1)
	if len(deliveries) == 0 {
		t.Fatal("no delivery")
	}
	w := do(t, router, authReq(t, http.MethodPost, "/deliveries/"+deliveries[0].ID+"/redeliver", ""))
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for non-failed delivery", w.Code)
	}
}

func TestRedeliver_NotFound(t *testing.T) {
	router, _ := testServer(t)
	w := do(t, router, authReq(t, http.MethodPost, "/deliveries/no-such-id/redeliver", ""))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestRedeliver_DuplicateReturns409WithExistingID(t *testing.T) {
	router, stores := testServer(t)
	seedWebhook(t, router, "https://recv.example.com")
	do(t, router, authReq(t, http.MethodPost, "/events", validEventBody()))

	ctx := context.Background()
	deliveries, _ := stores.Deliveries.List(ctx, 1)
	if len(deliveries) == 0 {
		t.Fatal("no delivery created")
	}
	d := deliveries[0]

	// Mark the original delivery as failed so redeliver is allowed (FR5.1).
	stores.Deliveries.MarkInFlight(ctx, d.ID)
	errMsg := "forced"
	stores.Deliveries.MarkFailed(ctx, d.ID, 5, nil, nil, &errMsg, nil)

	// First redeliver — must succeed with 202 and return a new delivery ID.
	w := do(t, router, authReq(t, http.MethodPost, "/deliveries/"+d.ID+"/redeliver", ""))
	if w.Code != http.StatusAccepted {
		t.Fatalf("first redeliver: status = %d, want 202; body = %s", w.Code, w.Body)
	}
	var first models.Delivery
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first redeliver: %v", err)
	}
	if first.ID == "" || first.ID == d.ID {
		t.Fatalf("first redeliver ID = %q, want a new non-empty id", first.ID)
	}

	// Second redeliver — must return 409 with the first re-delivery's ID in the body.
	w = do(t, router, authReq(t, http.MethodPost, "/deliveries/"+d.ID+"/redeliver", ""))
	if w.Code != http.StatusConflict {
		t.Fatalf("second redeliver: status = %d, want 409; body = %s", w.Code, w.Body)
	}
	var conflict struct {
		Error        string `json:"error"`
		RedeliveryID string `json:"redelivery_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &conflict); err != nil {
		t.Fatalf("decode 409 body: %v", err)
	}
	if conflict.RedeliveryID != first.ID {
		t.Errorf("redelivery_id = %q, want %q (the first re-delivery)", conflict.RedeliveryID, first.ID)
	}
}
