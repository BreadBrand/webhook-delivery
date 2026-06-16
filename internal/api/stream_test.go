package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/api"
	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/sse"
)

func TestStreamSSE(t *testing.T) {
	stores, err := db.OpenStores(":memory:")
	if err != nil {
		t.Fatalf("OpenStores: %v", err)
	}
	defer stores.Close()

	encKey := make([]byte, 32)
	b := sse.NewBroadcaster()
	h := api.NewHandler(stores, encKey, b)
	router := api.NewRouter(h, testAPIKey, nil)

	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/stream", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()

	time.Sleep(50 * time.Millisecond)
	b.Publish("test", "hello")

	srv.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("stream goroutine did not exit after server close")
	}
}

func TestStreamRequiresAuth(t *testing.T) {
	router, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
