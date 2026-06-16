package api

import (
	"net/http"
	"time"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pending, err := h.stores.Deliveries.CountPending(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	dbOK := h.stores.Ping(ctx) == nil

	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"uptime_s":           time.Since(h.startedAt).Seconds(),
		"pending_deliveries": pending,
		"db_ok":              dbOK,
		"worker_count":       h.workerCount,
	})
}
