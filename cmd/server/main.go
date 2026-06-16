package main

import (
	"context"
	"flag"
	stdfs "io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/b2randon/webhook-delivery/internal/api"
	"github.com/b2randon/webhook-delivery/internal/browser"
	"github.com/b2randon/webhook-delivery/internal/config"
	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/simulate"
	"github.com/b2randon/webhook-delivery/internal/sse"
	"github.com/b2randon/webhook-delivery/internal/worker"
	"github.com/b2randon/webhook-delivery/web"
)

func main() {
	runSimulateFlag := flag.Bool("simulate", false, "start simulator inline (no separate terminal needed)")
	flag.Parse()

	if os.Getenv("LOG_FORMAT") == "json" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	}

	cfg, err := config.Load("data/secrets.json")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	runSimulate := *runSimulateFlag || cfg.Simulate

	stores, err := db.OpenStores(cfg.DBPath)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer stores.Close()

	broadcaster := sse.NewBroadcaster()

	pool := worker.NewPool(stores, cfg.EncryptionKey, cfg.WorkerCount)
	pool.SetBroadcaster(broadcaster)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool.Start(ctx)

	h := api.NewHandler(stores, cfg.EncryptionKey, broadcaster)
	h.SetWorkerCount(cfg.WorkerCount)
	webSub, err := stdfs.Sub(web.FS, "dist")
	if err != nil {
		slog.Error("web FS", "err", err)
		os.Exit(1)
	}
	router := api.NewRouter(h, cfg.APIKey, http.FS(webSub))

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	if runSimulate {
		baseURL := "http://localhost:" + cfg.Port
		// Both goroutines sleep 500ms so the server socket is bound before use.
		go func() {
			time.Sleep(500 * time.Millisecond)
			browser.Open(baseURL)
		}()
		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := simulate.Run(ctx, simulate.Config{
				Receivers:   5,
				FailureRate: 0.3,
				EventRate:   2.0,
				ServerURL:   baseURL,
				APIKey:      cfg.APIKey,
			}); err != nil && ctx.Err() == nil {
				slog.Error("simulator failed", "err", err)
			}
		}()
	}

	slog.Info("server listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
	pool.Wait()
}
