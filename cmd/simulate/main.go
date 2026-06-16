package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/b2randon/webhook-delivery/internal/config"
)

var eventTypes = []string{
	"order.created",
	"payment.failed",
	"user.signup",
	"inventory.updated",
}

func main() {
	nReceivers := flag.Int("receivers", 5, "number of mock receiver servers")
	failureRate := flag.Float64("failure-rate", 0.3, "fraction of receivers that return HTTP 500")
	eventRate := flag.Float64("event-rate", 2.0, "events per second to fire")
	serverURL := flag.String("server", "http://localhost:8080", "base URL of the webhook delivery service")
	secretsPath := flag.String("secrets", "data/secrets.json", "path to secrets.json")
	flag.Parse()

	if *eventRate <= 0 {
		log.Fatal("--event-rate must be positive")
	}

	cfg, err := config.Load(*secretsPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	nFail := int(float64(*nReceivers) * *failureRate)

	type entry struct {
		srv *http.Server
		id  string
	}
	var entries []entry

	for i := range *nReceivers {
		fail := i < nFail
		srv, port, err := startReceiver(fail)
		if err != nil {
			log.Fatalf("start receiver %d: %v", i, err)
		}
		id, err := registerWebhook(ctx, *serverURL, cfg.APIKey,
			fmt.Sprintf("http://127.0.0.1:%d", port))
		if err != nil {
			log.Printf("register webhook %d: %v (skipping)", i, err)
			srv.Close()
			continue
		}
		entries = append(entries, entry{srv: srv, id: id})
		log.Printf("registered %s → :%d (fail=%v)", id, port, fail)
	}

	if len(entries) == 0 {
		log.Fatal("no receivers registered — is the server running?")
	}

	interval := time.Duration(float64(time.Second) / *eventRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	seq := 0
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down…")
			for _, e := range entries {
				deleteWebhook(*serverURL, cfg.APIKey, e.id)
				e.srv.Close()
			}
			log.Println("done")
			return
		case <-ticker.C:
			evType := eventTypes[seq%len(eventTypes)]
			id := fmt.Sprintf("sim-%d-%d", time.Now().UnixNano(), seq)
			seq++
			if err := fireEvent(ctx, *serverURL, cfg.APIKey, id, evType); err != nil {
				log.Printf("fire event: %v", err)
				continue
			}
			log.Printf("→ %s [%s] → %d webhook(s)", id, evType, len(entries))
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
	body, _ := json.Marshal(map[string]any{
		"url":               url,
		"circuit_threshold": 3,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/webhooks",
		bytes.NewReader(body))
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
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/webhooks/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("delete webhook %s: %v", id, err)
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
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/events",
		bytes.NewReader(body))
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
