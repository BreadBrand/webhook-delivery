package main

import (
	"context"
	stdfs "io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/b2randon/webhook-delivery/internal/api"
	"github.com/b2randon/webhook-delivery/internal/config"
	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/sse"
	"github.com/b2randon/webhook-delivery/internal/worker"
	"github.com/b2randon/webhook-delivery/web"
)

func main() {
	cfg, err := config.Load("data/secrets.json")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

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

	slog.Info("server listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
