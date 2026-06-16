package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfigHandler(t *testing.T) {
	h := configHandler("my-api-key")
	req := httptest.NewRequest("GET", "/config", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["api_key"] != "my-api-key" {
		t.Fatalf("want api_key=my-api-key, got %q", body["api_key"])
	}
}
