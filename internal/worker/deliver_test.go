package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/crypto"
	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
)

func mustStores(t *testing.T) *db.Stores {
	t.Helper()
	s, err := db.OpenStores(":memory:")
	if err != nil {
		t.Fatalf("OpenStores: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustWebhook(t *testing.T, s *db.Stores, url string, secret, encKey []byte, threshold int) *models.Webhook {
	t.Helper()
	enc, err := crypto.Encrypt(encKey, secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	wh, err := s.Webhooks.Create(context.Background(), url, enc, "hint", threshold)
	if err != nil {
		t.Fatalf("Create webhook: %v", err)
	}
	return wh
}

func mustEvent(t *testing.T, s *db.Stores) *models.Event {
	t.Helper()
	ev := &models.Event{
		ID:     "evt-1",
		Type:   "order.created",
		Source: "test/source",
		Time:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Data:   json.RawMessage(`{"amount":42}`),
	}
	if err := s.Events.Create(context.Background(), ev); err != nil {
		t.Fatalf("Create event: %v", err)
	}
	return ev
}

func testClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func TestExecuteDeliverySuccess(t *testing.T) {
	var (
		gotBody    []byte
		gotSig     string
		gotCT      string
		gotEventID string
		gotAttempt string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-Webhook-Signature")
		gotCT = r.Header.Get("Content-Type")
		gotEventID = r.Header.Get("X-Webhook-Event-ID")
		gotAttempt = r.Header.Get("X-Webhook-Delivery-Attempt")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	secret := []byte("test-signing-secret")
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, secret, encKey, 5)
	ev := mustEvent(t, s)

	d := models.Delivery{ID: "del-1", EventID: ev.ID, WebhookID: wh.ID}
	result := executeDelivery(context.Background(), d, s, encKey, testClient())

	if !result.Success {
		errStr := ""
		if result.Err != nil {
			errStr = *result.Err
		}
		t.Fatalf("expected success, got failure: %s", errStr)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if result.ResponseMs < 0 {
		t.Errorf("ResponseMs = %d, want >= 0", result.ResponseMs)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotEventID != ev.ID {
		t.Errorf("X-Webhook-Event-ID = %q, want %q", gotEventID, ev.ID)
	}
	if gotAttempt != "1" {
		t.Errorf("X-Webhook-Delivery-Attempt = %q, want 1", gotAttempt)
	}
	wantSig := crypto.Sign(gotBody, secret)
	if gotSig != wantSig {
		t.Errorf("X-Webhook-Signature = %q, want %q", gotSig, wantSig)
	}
	if !strings.HasPrefix(gotSig, "sha256=") {
		t.Errorf("signature format wrong: %q", gotSig)
	}
	var envelope models.CloudEvent
	if err := json.Unmarshal(gotBody, &envelope); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if envelope.SpecVersion != "1.0" {
		t.Errorf("specversion = %q, want 1.0", envelope.SpecVersion)
	}
	if envelope.ID != ev.ID {
		t.Errorf("id = %q, want %q", envelope.ID, ev.ID)
	}
	if envelope.Type != ev.Type {
		t.Errorf("type = %q, want %q", envelope.Type, ev.Type)
	}
}

func TestExecuteDelivery5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 5)
	ev := mustEvent(t, s)

	d := models.Delivery{ID: "del-2", EventID: ev.ID, WebhookID: wh.ID}
	result := executeDelivery(context.Background(), d, s, encKey, testClient())

	if result.Success {
		t.Fatal("expected failure for 500 response")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
	if result.Err == nil {
		t.Error("Err must be set on non-2xx response")
	}
	if result.ResponseMs < 0 {
		t.Errorf("ResponseMs = %d, want >= 0", result.ResponseMs)
	}
}

func TestExecuteDeliveryTimeout(t *testing.T) {
	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
	}))
	defer func() {
		close(unblock)
		srv.Close()
	}()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 5)
	ev := mustEvent(t, s)

	d := models.Delivery{ID: "del-3", EventID: ev.ID, WebhookID: wh.ID}
	client := &http.Client{Timeout: 50 * time.Millisecond}
	result := executeDelivery(context.Background(), d, s, encKey, client)

	if result.Success {
		t.Fatal("expected failure for timed-out request")
	}
	if result.Err == nil {
		t.Error("Err must be set on timeout")
	}
}
