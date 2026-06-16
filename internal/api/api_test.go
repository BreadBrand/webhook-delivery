package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/api"
	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
	"github.com/b2randon/webhook-delivery/internal/sse"
)

const testAPIKey = "test-key"

func testServer(t *testing.T) (http.Handler, *db.Stores) {
	t.Helper()
	stores, err := db.OpenStores(":memory:")
	if err != nil {
		t.Fatalf("OpenStores: %v", err)
	}
	t.Cleanup(func() { stores.Close() })
	encKey := make([]byte, 32)
	b := sse.NewBroadcaster()
	h := api.NewHandler(stores, encKey, b)
	return api.NewRouter(h, testAPIKey), stores
}

func authReq(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func do(t *testing.T, router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decodeJSON: %v (body=%q)", err, body)
	}
}

func seedWebhook(t *testing.T, router http.Handler, url string) models.Webhook {
	t.Helper()
	req := authReq(t, http.MethodPost, "/webhooks",
		`{"url":"`+url+`","circuit_threshold":5}`)
	w := do(t, router, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("seedWebhook: status %d, body %s", w.Code, w.Body)
	}
	var wh models.Webhook
	decodeJSON(t, w.Body.Bytes(), &wh)
	return wh
}
