package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Config holds simulation parameters.
type Config struct {
	Receivers   int
	FailureRate float64
	EventRate   float64
	ServerURL   string
	APIKey      string
}

var eventTypes = []string{
	"order.created",
	"payment.failed",
	"user.signup",
	"inventory.updated",
}

// Run registers N mock receiver webhooks and fires CloudEvents until ctx is cancelled.
// Returns an error if cfg.EventRate is non-positive or if no receivers could be registered.
func Run(ctx context.Context, cfg Config) error {
	if cfg.EventRate <= 0 {
		return fmt.Errorf("EventRate must be positive, got %g", cfg.EventRate)
	}

	nFail := int(float64(cfg.Receivers) * cfg.FailureRate)

	type entry struct {
		srv *http.Server
		id  string
	}
	var entries []entry

	for i := range cfg.Receivers {
		fail := i < nFail
		srv, port, err := startReceiver(fail)
		if err != nil {
			slog.Error("start receiver", "i", i, "err", err)
			continue
		}
		id, err := registerWebhook(ctx, cfg.ServerURL, cfg.APIKey,
			fmt.Sprintf("http://127.0.0.1:%d", port))
		if err != nil {
			slog.Info("register webhook skipped", "i", i, "err", err)
			srv.Close()
			continue
		}
		entries = append(entries, entry{srv: srv, id: id})
		slog.Info("simulator: registered receiver", "id", id, "port", port, "fail", fail)
	}

	if len(entries) == 0 {
		return fmt.Errorf("no receivers registered — is the server running?")
	}

	interval := time.Duration(float64(time.Second) / cfg.EventRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	seq := 0
	for {
		select {
		case <-ctx.Done():
			slog.Info("simulator: shutting down")
			for _, e := range entries {
				deleteWebhook(cfg.ServerURL, cfg.APIKey, e.id)
				e.srv.Close()
			}
			slog.Info("simulator: done")
			return nil
		case <-ticker.C:
			evType := eventTypes[seq%len(eventTypes)]
			id := fmt.Sprintf("sim-%d-%d", time.Now().UnixNano(), seq)
			seq++
			if err := fireEvent(ctx, cfg.ServerURL, cfg.APIKey, id, evType); err != nil {
				slog.Info("simulator: fire event failed", "err", err)
				continue
			}
			slog.Info("simulator: fired event", "id", id, "type", evType, "webhooks", len(entries))
		}
	}
}

func startReceiver(fail bool) (*http.Server, int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "simulated failure", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln) //nolint:errcheck
	return srv, port, nil
}

func registerWebhook(ctx context.Context, baseURL, apiKey, url string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"url":               url,
		"circuit_threshold": 3,
	})
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/webhooks",
		bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var wh struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wh); err != nil {
		return "", err
	}
	return wh.ID, nil
}

func deleteWebhook(baseURL, apiKey, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+"/webhooks/"+id, nil)
	if err != nil {
		slog.Error("simulator: build delete request", "id", id, "err", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("simulator: delete webhook", "id", id, "err", err)
		return
	}
	resp.Body.Close()
}

func fireEvent(ctx context.Context, baseURL, apiKey, id, evType string) error {
	payload := map[string]any{
		"specversion":     "1.0",
		"id":              id,
		"type":            evType,
		"source":          "https://sim.local",
		"time":            time.Now().UTC().Format(time.RFC3339Nano),
		"datacontenttype": "application/json",
		"data":            map[string]any{"seq": id},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/events",
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("ingest returned %d", resp.StatusCode)
	}
	return nil
}
