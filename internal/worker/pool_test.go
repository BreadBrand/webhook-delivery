package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/models"
)

func TestPoolDeliverSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 5)
	ev := mustEvent(t, s)

	if err := s.Deliveries.CreateBatch(context.Background(), ev.ID, []models.Webhook{*wh}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool := NewPool(s, encKey, 1)
	pool.pollInterval = 20 * time.Millisecond
	pool.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		list, _ := s.Deliveries.List(context.Background(), 10)
		if len(list) > 0 && list[0].Status == models.DeliverySuccess {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}

	list, _ := s.Deliveries.List(context.Background(), 10)
	if len(list) == 0 {
		t.Fatal("no deliveries found")
	}
	t.Fatalf("delivery status = %q after 3s, want success", list[0].Status)
}

func TestProcessSchedulesRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 10) // high threshold, no circuit trip
	ev := mustEvent(t, s)

	s.Deliveries.CreateBatch(context.Background(), ev.ID, []models.Webhook{*wh})
	list, _ := s.Deliveries.List(context.Background(), 1)
	if len(list) == 0 {
		t.Fatal("no delivery created")
	}
	d := list[0]
	s.Deliveries.MarkInFlight(context.Background(), d.ID)

	pool := NewPool(s, encKey, 1)
	pool.process(context.Background(), d)

	updated, err := s.Deliveries.Get(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("Get delivery: %v", err)
	}
	if updated.Status != models.DeliveryPending {
		t.Errorf("status = %q, want pending", updated.Status)
	}
	if updated.NextAttemptAt == nil {
		t.Fatal("next_attempt_at must be set after first failure")
	}
	if updated.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", updated.Attempt)
	}
	delta := time.Until(*updated.NextAttemptAt)
	if delta < 8*time.Second || delta > 12*time.Second {
		t.Errorf("retry delay = %v, want ~10s", delta)
	}
}

func TestProcessTerminalFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 100)
	ev := mustEvent(t, s)

	s.Deliveries.CreateBatch(context.Background(), ev.ID, []models.Webhook{*wh})
	list, _ := s.Deliveries.List(context.Background(), 1)
	d := list[0]
	d.Attempt = MaxAttempts - 1 // on the last allowed attempt
	s.Deliveries.MarkInFlight(context.Background(), d.ID)

	pool := NewPool(s, encKey, 1)
	pool.process(context.Background(), d)

	updated, _ := s.Deliveries.Get(context.Background(), d.ID)
	if updated.Status != models.DeliveryFailed {
		t.Errorf("status = %q, want failed (terminal)", updated.Status)
	}
	if updated.NextAttemptAt != nil {
		t.Errorf("next_attempt_at should be nil on terminal failure, got %v", *updated.NextAttemptAt)
	}
}

func TestProcessCircuitTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 1) // threshold=1: trips on first failure

	ctx := context.Background()
	ev1 := &models.Event{ID: "e1", Type: "t", Source: "s", Time: time.Now().UTC(), Data: json.RawMessage(`{}`)}
	ev2 := &models.Event{ID: "e2", Type: "t", Source: "s", Time: time.Now().UTC(), Data: json.RawMessage(`{}`)}
	s.Events.Create(ctx, ev1)
	s.Events.Create(ctx, ev2)

	s.Deliveries.CreateBatch(ctx, ev1.ID, []models.Webhook{*wh})
	s.Deliveries.CreateBatch(ctx, ev2.ID, []models.Webhook{*wh})

	all, err := s.Deliveries.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(all))
	}

	d := all[0]
	s.Deliveries.MarkInFlight(ctx, d.ID)

	pool := NewPool(s, encKey, 1)
	pool.process(ctx, d)

	updatedWH, _ := s.Webhooks.Get(ctx, wh.ID)
	if updatedWH.Status != models.StatusCircuitOpen {
		t.Errorf("webhook status = %q, want circuit_open", updatedWH.Status)
	}

	deliveries, _ := s.Deliveries.List(ctx, 10)
	for _, del := range deliveries {
		if del.Status == models.DeliveryPending {
			t.Errorf("delivery %s is still pending after circuit opened", del.ID)
		}
	}
}

func TestCheckProbesRestoresCircuit(t *testing.T) {
	var allowSuccess atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowSuccess.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 1)
	ev := mustEvent(t, s)

	ctx := context.Background()
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{*wh})

	s.Webhooks.SetCircuitOpen(ctx, wh.ID)
	list, _ := s.Deliveries.List(ctx, 10)
	if len(list) == 0 {
		t.Fatal("no deliveries created")
	}
	s.Deliveries.MarkHeld(ctx, list[0].ID)
	s.Webhooks.TriggerProbe(ctx, wh.ID)

	allowSuccess.Store(true)
	pool := NewPool(s, encKey, 1)
	pool.checkProbes(ctx)

	updated, _ := s.Webhooks.Get(ctx, wh.ID)
	if updated.Status != models.StatusActive {
		t.Errorf("webhook status = %q after successful probe, want active", updated.Status)
	}

	final, _ := s.Deliveries.List(ctx, 10)
	if len(final) == 0 {
		t.Fatal("no deliveries found after probe")
	}
	if final[0].Status != models.DeliverySuccess {
		t.Errorf("delivery status = %q after successful probe, want success", final[0].Status)
	}
}

func TestCheckProbesFailedProbeResetsTimer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 1)
	ev := mustEvent(t, s)

	ctx := context.Background()
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{*wh})

	s.Webhooks.SetCircuitOpen(ctx, wh.ID)
	list, _ := s.Deliveries.List(ctx, 10)
	if len(list) == 0 {
		t.Fatal("no deliveries created")
	}
	s.Deliveries.MarkHeld(ctx, list[0].ID)
	s.Webhooks.TriggerProbe(ctx, wh.ID)

	pool := NewPool(s, encKey, 1)
	pool.checkProbes(ctx)

	updated, _ := s.Webhooks.Get(ctx, wh.ID)
	if updated.Status != models.StatusCircuitOpen {
		t.Errorf("webhook status = %q after failed probe, want circuit_open", updated.Status)
	}

	due, _ := s.Webhooks.ListDueForProbe(ctx)
	for _, w := range due {
		if w.ID == wh.ID {
			t.Error("webhook is still immediately due for probe — timer was not reset")
		}
	}

	finalList, _ := s.Deliveries.List(ctx, 10)
	if len(finalList) > 0 && finalList[0].Status != models.DeliveryHeld {
		t.Errorf("delivery status = %q after failed probe, want held", finalList[0].Status)
	}
}

func TestProcessLogsDeliveryAttempt(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(old)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	encKey := make([]byte, 32)
	s := mustStores(t)
	wh := mustWebhook(t, s, srv.URL, []byte("secret"), encKey, 5)
	ev := mustEvent(t, s)
	s.Deliveries.CreateBatch(context.Background(), ev.ID, []models.Webhook{*wh})

	list, _ := s.Deliveries.List(context.Background(), 1)
	d := list[0]
	s.Deliveries.MarkInFlight(context.Background(), d.ID)

	pool := NewPool(s, encKey, 1)
	pool.process(context.Background(), d)

	log := buf.String()
	if !strings.Contains(log, "delivery attempt") {
		t.Errorf("expected 'delivery attempt' in log output, got: %s", log)
	}
	if !strings.Contains(log, ev.ID) {
		t.Errorf("expected event_id %q in log output, got: %s", ev.ID, log)
	}
}
