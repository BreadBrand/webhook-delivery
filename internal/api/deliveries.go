package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/b2randon/webhook-delivery/internal/models"
)

func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 100)
	list, err := h.stores.Deliveries.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) Redeliver(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	d, err := h.stores.Deliveries.Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if d.Status != models.DeliveryFailed {
		writeError(w, http.StatusConflict, "redeliver only allowed on failed deliveries")
		return
	}

	existing, err := h.stores.Deliveries.HasActiveRedelivery(ctx, d.EventID, d.WebhookID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":         "active redelivery already exists",
			"redelivery_id": existing.ID,
		})
		return
	}

	newD, err := h.stores.Deliveries.CreateRedelivery(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.broadcaster.Publish("delivery_updated", newD)
	writeJSON(w, http.StatusAccepted, newD)
}
