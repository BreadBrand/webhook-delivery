package db_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
)

func seedWebhookAndEvent(t *testing.T, s *db.Stores) (wh models.Webhook, ev models.Event) {
	t.Helper()
	ctx := context.Background()
	w, err := s.Webhooks.Create(ctx, "https://example.com/hook", "enc", "hint", 5)
	if err != nil {
		t.Fatal(err)
	}
	e := &models.Event{
		ID: "evt-seed", Type: "order.created", Source: "src",
		Time: time.Now().UTC(), Data: json.RawMessage(`{}`),
	}
	if err := s.Events.Create(ctx, e); err != nil {
		t.Fatal(err)
	}
	return *w, *e
}

func TestDeliveryCreateBatchPending(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)

	err := s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	list, err := s.Deliveries.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(list))
	}
	if list[0].Status != models.DeliveryPending {
		t.Errorf("status = %q, want pending", list[0].Status)
	}
}

func TestDeliveryCreateBatchHeldWhenCircuitOpen(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)

	s.Webhooks.SetCircuitOpen(ctx, wh.ID)
	wh.Status = models.StatusCircuitOpen

	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	if list[0].Status != models.DeliveryHeld {
		t.Errorf("status = %q, want held for circuit_open webhook", list[0].Status)
	}
}

func TestDeliveryMarkInFlightCAS(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID

	// Two goroutines race to claim the same delivery — only one should win.
	var wins int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, _ := s.Deliveries.MarkInFlight(ctx, id)
			if claimed {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Errorf("CAS: %d goroutines claimed the same delivery, want exactly 1", wins)
	}
}

func TestDeliveryMarkSuccess(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID
	s.Deliveries.MarkInFlight(ctx, id)

	if err := s.Deliveries.MarkSuccess(ctx, id, 200, 42); err != nil {
		t.Fatalf("MarkSuccess: %v", err)
	}
	d, _ := s.Deliveries.Get(ctx, id)
	if d.Status != models.DeliverySuccess {
		t.Errorf("status = %q, want success", d.Status)
	}
	if *d.LastStatusCode != 200 || *d.LastResponseMs != 42 {
		t.Errorf("unexpected status code or response ms")
	}
}

func TestDeliveryMarkFailedWithRetry(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID
	s.Deliveries.MarkInFlight(ctx, id)

	next := time.Now().Add(10 * time.Second)
	code := 500
	ms := 100
	msg := "server error"
	if err := s.Deliveries.MarkFailed(ctx, id, 1, &code, &ms, &msg, &next); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	d, _ := s.Deliveries.Get(ctx, id)
	if d.Status != models.DeliveryPending {
		t.Errorf("status = %q, want pending (retry scheduled)", d.Status)
	}
	if d.NextAttemptAt == nil {
		t.Error("NextAttemptAt must be set")
	}
}

func TestDeliveryMarkFailedTerminal(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	id := list[0].ID
	s.Deliveries.MarkInFlight(ctx, id)

	if err := s.Deliveries.MarkFailed(ctx, id, 5, nil, nil, nil, nil); err != nil {
		t.Fatalf("MarkFailed terminal: %v", err)
	}

	d, _ := s.Deliveries.Get(ctx, id)
	if d.Status != models.DeliveryFailed {
		t.Errorf("status = %q, want failed", d.Status)
	}
}

func TestDeliveryResetInFlight(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	s.Deliveries.MarkInFlight(ctx, list[0].ID)

	if err := s.Deliveries.ResetInFlight(ctx); err != nil {
		t.Fatalf("ResetInFlight: %v", err)
	}

	d, _ := s.Deliveries.Get(ctx, list[0].ID)
	if d.Status != models.DeliveryPending {
		t.Errorf("status = %q, want pending after reset", d.Status)
	}
}

func TestDeliveryAbortForWebhook(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	if err := s.Deliveries.AbortForWebhook(ctx, wh.ID); err != nil {
		t.Fatalf("AbortForWebhook: %v", err)
	}

	list, _ := s.Deliveries.List(ctx, 10)
	if list[0].Status != models.DeliveryFailed {
		t.Errorf("status = %q, want failed", list[0].Status)
	}
}

func TestDeliveryFlushHeld(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, _ := seedWebhookAndEvent(t, s)

	s.Webhooks.SetCircuitOpen(ctx, wh.ID)
	wh.Status = models.StatusCircuitOpen

	// Create 15 held deliveries across 15 events.
	for i := 0; i < 15; i++ {
		ev := &models.Event{
			ID: fmt.Sprintf("ev-%d", i), Type: "t", Source: "s",
			Time: time.Now().UTC(), Data: json.RawMessage(`{}`),
		}
		s.Events.Create(ctx, ev)
		s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})
	}

	// Flush should move only 10 to pending.
	if err := s.Deliveries.FlushHeld(ctx, wh.ID); err != nil {
		t.Fatalf("FlushHeld: %v", err)
	}

	pending, _ := s.Deliveries.ClaimPending(ctx, time.Now().Add(time.Minute), 20)
	if len(pending) != 10 {
		t.Errorf("after FlushHeld: %d pending, want 10", len(pending))
	}
}

func TestDeliveryCreateRedelivery(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	original := list[0]

	// Mark as failed (terminal).
	s.Deliveries.MarkInFlight(ctx, original.ID)
	s.Deliveries.MarkFailed(ctx, original.ID, 5, nil, nil, nil, nil)

	redel, err := s.Deliveries.CreateRedelivery(ctx, original.ID)
	if err != nil {
		t.Fatalf("CreateRedelivery: %v", err)
	}
	if redel.ParentDeliveryID == nil || *redel.ParentDeliveryID != original.ID {
		t.Error("ParentDeliveryID not set correctly")
	}
	if redel.Status != models.DeliveryPending {
		t.Errorf("redelivery status = %q, want pending", redel.Status)
	}
	if redel.Attempt != 0 {
		t.Errorf("redelivery attempt = %d, want 0", redel.Attempt)
	}
}

func TestDeliveryHasActiveRedelivery(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()
	wh, ev := seedWebhookAndEvent(t, s)
	s.Deliveries.CreateBatch(ctx, ev.ID, []models.Webhook{wh})

	list, _ := s.Deliveries.List(ctx, 10)
	s.Deliveries.MarkInFlight(ctx, list[0].ID)
	s.Deliveries.MarkFailed(ctx, list[0].ID, 5, nil, nil, nil, nil)

	s.Deliveries.CreateRedelivery(ctx, list[0].ID)

	active, err := s.Deliveries.HasActiveRedelivery(ctx, ev.ID, wh.ID)
	if err != nil {
		t.Fatalf("HasActiveRedelivery: %v", err)
	}
	if active == nil {
		t.Error("expected active redelivery to be found")
	}
}
