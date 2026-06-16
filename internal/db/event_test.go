package db_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/models"
)

func makeEvent(id, typ string) *models.Event {
	return &models.Event{
		ID:     id,
		Type:   typ,
		Source: "https://test.example.com",
		Time:   time.Now().UTC().Truncate(time.Second),
		Data:   json.RawMessage(`{"key":"value"}`),
	}
}

func TestEventCreateAndGet(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	ev := makeEvent("evt-1", "order.created")
	if err := s.Events.Create(ctx, ev); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Events.Get(ctx, "evt-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Type != "order.created" {
		t.Errorf("Type = %q", got.Type)
	}
}

func TestEventDuplicateIDErrors(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	ev := makeEvent("evt-dup", "order.created")
	s.Events.Create(ctx, ev)
	err := s.Events.Create(ctx, ev)
	if err == nil {
		t.Error("expected error on duplicate event ID")
	}
}

func TestEventList(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	for i, typ := range []string{"a", "b", "c"} {
		s.Events.Create(ctx, makeEvent(fmt.Sprintf("evt-%d", i), typ))
	}

	list, err := s.Events.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List len = %d, want 3", len(list))
	}
}

func TestEventVolume(t *testing.T) {
	s := mustOpenDB(t)
	ctx := context.Background()

	s.Events.Create(ctx, makeEvent("e1", "order.created"))
	s.Events.Create(ctx, makeEvent("e2", "order.created"))
	s.Events.Create(ctx, makeEvent("e3", "payment.failed"))

	pts, err := s.Events.Volume(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("Volume: %v", err)
	}
	counts := map[string]int{}
	for _, p := range pts {
		counts[p.Type] = p.Count
	}
	if counts["order.created"] != 2 {
		t.Errorf("order.created count = %d, want 2", counts["order.created"])
	}
	if counts["payment.failed"] != 1 {
		t.Errorf("payment.failed count = %d, want 1", counts["payment.failed"])
	}
}
