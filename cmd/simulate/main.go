package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/b2randon/webhook-delivery/internal/config"
	"github.com/b2randon/webhook-delivery/internal/simulate"
)

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

	if err := simulate.Run(ctx, simulate.Config{
		Receivers:   *nReceivers,
		FailureRate: *failureRate,
		EventRate:   *eventRate,
		ServerURL:   *serverURL,
		APIKey:      cfg.APIKey,
	}); err != nil {
		log.Fatalf("simulator: %v", err)
	}
}
