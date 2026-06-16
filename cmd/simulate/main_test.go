package main

import (
	"fmt"
	"net/http"
	"testing"
)

func TestStartReceiverFail(t *testing.T) {
	srv, port, err := startReceiver(true)
	if err != nil {
		t.Fatalf("startReceiver: %v", err)
	}
	defer srv.Close()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestStartReceiverSuccess(t *testing.T) {
	srv, port, err := startReceiver(false)
	if err != nil {
		t.Fatalf("startReceiver: %v", err)
	}
	defer srv.Close()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestEventTypeRotation(t *testing.T) {
	seen := map[string]bool{}
	for i := range 20 {
		seen[eventTypes[i%len(eventTypes)]] = true
	}
	if len(seen) < 4 {
		t.Fatalf("expected ≥4 distinct event types, got %d", len(seen))
	}
}
