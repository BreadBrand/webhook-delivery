package db_test

import (
	"context"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
)

func mustOpenDB(t *testing.T) *db.Stores {
	t.Helper()
	stores, err := db.OpenStores(":memory:")
	if err != nil {
		t.Fatalf("OpenStores: %v", err)
	}
	t.Cleanup(func() { stores.Close() })
	return stores
}

func TestWebhookCreateAndGet(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, err := s.Webhooks.Create(ctx, "https://example.com/hook", "enc-secret", "sk_…abcd", 5)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wh.ID == "" {
		t.Error("ID must be set")
	}
	if wh.Status != models.StatusActive {
		t.Errorf("Status = %q, want active", wh.Status)
	}

	got, err := s.Webhooks.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != "https://example.com/hook" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestWebhookList(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	s.Webhooks.Create(ctx, "https://a.com", "enc", "hint", 5)
	s.Webhooks.Create(ctx, "https://b.com", "enc", "hint", 5)

	list, err := s.Webhooks.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
}

func TestWebhookSoftDelete(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 5)
	if err := s.Webhooks.SoftDelete(ctx, wh.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	list, _ := s.Webhooks.List(ctx)
	if len(list) != 0 {
		t.Error("deleted webhook should not appear in List")
	}

	got, _ := s.Webhooks.Get(ctx, wh.ID)
	if got.Status != models.StatusDeleted {
		t.Errorf("Status after delete = %q, want deleted", got.Status)
	}
}

func TestWebhookRecordFailure(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 3)

	streak, status, err := s.Webhooks.RecordFailure(ctx, wh.ID)
	if err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if streak != 1 || status != models.StatusDegraded {
		t.Errorf("after 1 failure: streak=%d status=%q", streak, status)
	}

	s.Webhooks.RecordFailure(ctx, wh.ID)
	streak, status, _ = s.Webhooks.RecordFailure(ctx, wh.ID)
	if streak != 3 || status != models.StatusCircuitOpen {
		t.Errorf("after 3 failures (threshold=3): streak=%d status=%q", streak, status)
	}
	w, _ := s.Webhooks.Get(ctx, wh.ID)
	if w.NextProbeAt == nil {
		t.Error("NextProbeAt must be set when circuit trips")
	}
}

func TestWebhookRecordSuccess(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 5)
	s.Webhooks.RecordFailure(ctx, wh.ID)
	s.Webhooks.RecordFailure(ctx, wh.ID)

	if err := s.Webhooks.RecordSuccess(ctx, wh.ID); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	got, _ := s.Webhooks.Get(ctx, wh.ID)
	if got.FailureStreak != 0 || got.Status != models.StatusActive {
		t.Errorf("after RecordSuccess: streak=%d status=%q", got.FailureStreak, got.Status)
	}
}

func TestWebhookCloseCircuit(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 1)
	s.Webhooks.RecordFailure(ctx, wh.ID)

	got, _ := s.Webhooks.Get(ctx, wh.ID)
	if got.Status != models.StatusCircuitOpen {
		t.Fatalf("expected circuit_open, got %q", got.Status)
	}

	if err := s.Webhooks.CloseCircuit(ctx, wh.ID); err != nil {
		t.Fatalf("CloseCircuit: %v", err)
	}

	got, _ = s.Webhooks.Get(ctx, wh.ID)
	if got.Status != models.StatusActive || got.FailureStreak != 0 || got.NextProbeAt != nil {
		t.Errorf("after CloseCircuit: status=%q streak=%d probe=%v", got.Status, got.FailureStreak, got.NextProbeAt)
	}
}

func TestWebhookSetCircuitOpen(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, _ := s.Webhooks.Create(ctx, "https://example.com", "enc", "hint", 5)

	if err := s.Webhooks.SetCircuitOpen(ctx, wh.ID); err != nil {
		t.Fatalf("SetCircuitOpen: %v", err)
	}

	got, _ := s.Webhooks.Get(ctx, wh.ID)
	if got.Status != models.StatusCircuitOpen {
		t.Errorf("Status = %q, want circuit_open", got.Status)
	}
	if got.NextProbeAt == nil {
		t.Error("NextProbeAt must be set after SetCircuitOpen")
	}
}

func TestListDueForProbe(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	// wh1: circuit_open, probe due now (via TriggerProbe)
	wh1, err := s.Webhooks.Create(ctx, "https://a.com", "enc", "hint", 1)
	if err != nil {
		t.Fatal(err)
	}
	s.Webhooks.SetCircuitOpen(ctx, wh1.ID)
	if err := s.Webhooks.TriggerProbe(ctx, wh1.ID); err != nil {
		t.Fatalf("TriggerProbe: %v", err)
	}

	// wh2: circuit_open but NOT due (SetCircuitOpen sets next_probe_at = now+5min)
	wh2, err := s.Webhooks.Create(ctx, "https://b.com", "enc", "hint", 1)
	if err != nil {
		t.Fatal(err)
	}
	s.Webhooks.SetCircuitOpen(ctx, wh2.ID)

	// wh3: active — must never appear
	s.Webhooks.Create(ctx, "https://c.com", "enc", "hint", 5)

	due, err := s.Webhooks.ListDueForProbe(ctx)
	if err != nil {
		t.Fatalf("ListDueForProbe: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("got %d due for probe, want 1", len(due))
	}
	if due[0].ID != wh1.ID {
		t.Errorf("due[0].ID = %q, want %q", due[0].ID, wh1.ID)
	}
}

func TestTriggerProbe(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	wh, err := s.Webhooks.Create(ctx, "https://x.com", "enc", "hint", 1)
	if err != nil {
		t.Fatal(err)
	}
	s.Webhooks.SetCircuitOpen(ctx, wh.ID)

	// Before TriggerProbe: not due (probe is +5 min in the future)
	before, _ := s.Webhooks.ListDueForProbe(ctx)
	for _, w := range before {
		if w.ID == wh.ID {
			t.Error("webhook should not be due before TriggerProbe")
		}
	}

	if err := s.Webhooks.TriggerProbe(ctx, wh.ID); err != nil {
		t.Fatalf("TriggerProbe: %v", err)
	}

	after, err := s.Webhooks.ListDueForProbe(ctx)
	if err != nil {
		t.Fatalf("ListDueForProbe: %v", err)
	}
	found := false
	for _, w := range after {
		if w.ID == wh.ID {
			found = true
		}
	}
	if !found {
		t.Error("webhook not found in due-for-probe list after TriggerProbe")
	}
}
