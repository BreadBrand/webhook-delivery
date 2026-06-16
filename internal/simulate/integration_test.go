package simulate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/api"
	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/simulate"
	"github.com/b2randon/webhook-delivery/internal/sse"
)

func TestRunAgainstLiveServer(t *testing.T) {
	stores, err := db.OpenStores(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer stores.Close()

	encKey := make([]byte, 32)
	broadcaster := sse.NewBroadcaster()
	h := api.NewHandler(stores, encKey, broadcaster)
	router := api.NewRouter(h, "test-key", nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = simulate.Run(ctx, simulate.Config{
		Receivers:   2,
		FailureRate: 0,
		EventRate:   5.0,
		ServerURL:   srv.URL,
		APIKey:      "test-key",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	events, err := stores.Events.List(context.Background(), 100)
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected at least one event to be ingested")
	}

	webhooks, err := stores.Webhooks.List(context.Background())
	if err != nil {
		t.Fatalf("List webhooks: %v", err)
	}
	if len(webhooks) != 0 {
		t.Errorf("expected all webhooks deregistered on shutdown, got %d remaining", len(webhooks))
	}
}

func TestRunReturnsErrorOnUnreachableServer(t *testing.T) {
	// Use a closed server's URL to guarantee connection refused without relying on a fixed port.
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := simulate.Run(ctx, simulate.Config{
		Receivers:   1,
		FailureRate: 0,
		EventRate:   1.0,
		ServerURL:   closedURL,
		APIKey:      "any",
	})
	if err == nil {
		t.Error("expected error when no server is reachable, got nil")
	}
}
