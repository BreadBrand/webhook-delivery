package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateWebhook_ValidURL(t *testing.T) {
	router, _ := testServer(t)
	req := authReq(t, http.MethodPost, "/webhooks", `{"url":"https://example.com/hook"}`)
	w := do(t, router, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body)
	}

	var resp struct {
		ID         string `json:"id"`
		URL        string `json:"url"`
		Secret     string `json:"secret"`
		SecretHint string `json:"secret_hint"`
		Status     string `json:"status"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Error("id must be set")
	}
	if !strings.HasPrefix(resp.Secret, "sk_") {
		t.Errorf("secret = %q, want sk_... prefix", resp.Secret)
	}
	if resp.SecretHint == "" {
		t.Error("secret_hint must be set")
	}
	if resp.Status != "active" {
		t.Errorf("status = %q, want active", resp.Status)
	}
	listW := do(t, router, authReq(t, http.MethodGet, "/webhooks", ""))
	if strings.Contains(listW.Body.String(), resp.Secret) {
		t.Error("secret must not appear in GET /webhooks response")
	}
}

func TestCreateWebhook_InvalidURL(t *testing.T) {
	router, _ := testServer(t)
	for _, bad := range []string{
		`{"url":"ftp://bad.com"}`,
		`{"url":"not-a-url"}`,
		`{"url":""}`,
	} {
		w := do(t, router, authReq(t, http.MethodPost, "/webhooks", bad))
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", bad, w.Code)
		}
	}
}

func TestCreateWebhook_InvalidThreshold(t *testing.T) {
	router, _ := testServer(t)
	w := do(t, router, authReq(t, http.MethodPost, "/webhooks",
		`{"url":"https://example.com","circuit_threshold":0}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListWebhooks(t *testing.T) {
	router, _ := testServer(t)
	seedWebhook(t, router, "https://a.com")
	seedWebhook(t, router, "https://b.com")

	w := do(t, router, authReq(t, http.MethodGet, "/webhooks", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var list []map[string]any
	decodeJSON(t, w.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Errorf("got %d webhooks, want 2", len(list))
	}
}

func TestDeleteWebhook(t *testing.T) {
	router, _ := testServer(t)
	wh := seedWebhook(t, router, "https://x.com")

	w := do(t, router, authReq(t, http.MethodDelete, "/webhooks/"+wh.ID, ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	list := do(t, router, authReq(t, http.MethodGet, "/webhooks", ""))
	if strings.Contains(list.Body.String(), wh.ID) {
		t.Error("deleted webhook must not appear in GET /webhooks")
	}
}

func TestDeleteWebhook_NotFound(t *testing.T) {
	router, _ := testServer(t)
	w := do(t, router, authReq(t, http.MethodDelete, "/webhooks/no-such-id", ""))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestSetCircuit(t *testing.T) {
	router, _ := testServer(t)
	wh := seedWebhook(t, router, "https://cb.com")

	w := do(t, router, authReq(t, http.MethodPost, "/webhooks/"+wh.ID+"/circuit",
		`{"action":"open"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("open: status = %d, want 200; body = %s", w.Code, w.Body)
	}
	var updated struct{ Status string `json:"status"` }
	decodeJSON(t, w.Body.Bytes(), &updated)
	if updated.Status != "circuit_open" {
		t.Errorf("status = %q, want circuit_open", updated.Status)
	}

	w = do(t, router, authReq(t, http.MethodPost, "/webhooks/"+wh.ID+"/circuit",
		`{"action":"close"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("close: status = %d, want 200", w.Code)
	}
	decodeJSON(t, w.Body.Bytes(), &updated)
	if updated.Status != "active" {
		t.Errorf("status = %q, want active", updated.Status)
	}
}

func TestAuthRequired(t *testing.T) {
	router, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	w := do(t, router, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHealthNoAuth(t *testing.T) {
	router, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := do(t, router, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
