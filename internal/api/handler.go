package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/sse"
)

type Handler struct {
	stores      *db.Stores
	encKey      []byte
	broadcaster *sse.Broadcaster
	startedAt   time.Time
	workerCount int
}

func NewHandler(stores *db.Stores, encKey []byte, broadcaster *sse.Broadcaster) *Handler {
	return &Handler{
		stores:      stores,
		encKey:      encKey,
		broadcaster: broadcaster,
		startedAt:   time.Now(),
	}
}

func (h *Handler) SetWorkerCount(n int) { h.workerCount = n }

func NewRouter(h *Handler, apiKey string, staticFS http.FileSystem) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)

	r.Get("/health", h.Health)
	r.Get("/config", configHandler(apiKey))

	r.Group(func(r chi.Router) {
		r.Use(authMiddleware(apiKey))

		r.Post("/webhooks", h.CreateWebhook)
		r.Get("/webhooks", h.ListWebhooks)
		r.Delete("/webhooks/{id}", h.DeleteWebhook)
		r.Post("/webhooks/{id}/circuit", h.SetCircuit)

		r.Post("/events", h.IngestEvent)
		r.Get("/events", h.ListEvents)
		r.Get("/events/volume", h.EventVolume)

		r.Get("/deliveries", h.ListDeliveries)
		r.Post("/deliveries/{id}/redeliver", h.Redeliver)

		r.Get("/stream", h.Stream)
	})

	if staticFS != nil {
		r.NotFound(http.FileServer(staticFS).ServeHTTP)
	}

	return r
}
