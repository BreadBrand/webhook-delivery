package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/b2randon/webhook-delivery/internal/crypto"
	"github.com/b2randon/webhook-delivery/internal/models"
)

func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var input struct {
		URL              string `json:"url"`
		CircuitThreshold *int   `json:"circuit_threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	u, err := url.ParseRequestURI(input.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeError(w, http.StatusBadRequest, "url must be http:// or https:// with a non-empty host")
		return
	}

	threshold := 5
	if input.CircuitThreshold != nil {
		if *input.CircuitThreshold < 1 {
			writeError(w, http.StatusBadRequest, "circuit_threshold must be >= 1")
			return
		}
		threshold = *input.CircuitThreshold
	}

	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	plaintext := "sk_" + base64.RawURLEncoding.EncodeToString(raw)
	hint := "sk_…" + plaintext[len(plaintext)-4:]

	encrypted, err := crypto.Encrypt(h.encKey, []byte(plaintext))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	wh, err := h.stores.Webhooks.Create(r.Context(), input.URL, encrypted, hint, threshold)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.broadcaster.Publish("webhook_updated", wh)

	writeJSON(w, http.StatusCreated, struct {
		*models.Webhook
		Secret string `json:"secret"`
	}{Webhook: wh, Secret: plaintext})
}

func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := h.stores.Webhooks.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	wh, err := h.stores.Webhooks.Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && wh.Status == models.StatusDeleted) {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.stores.Webhooks.SoftDelete(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := h.stores.Deliveries.AbortForWebhook(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.broadcaster.Publish("webhook_updated", map[string]string{"id": id, "status": "deleted"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SetCircuit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	wh, err := h.stores.Webhooks.Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && wh.Status == models.StatusDeleted) {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var input struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	switch input.Action {
	case "open":
		if err := h.stores.Webhooks.SetCircuitOpen(ctx, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := h.stores.Deliveries.HoldPendingForWebhook(ctx, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	case "close":
		if err := h.stores.Webhooks.CloseCircuit(ctx, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := h.stores.Deliveries.FlushHeld(ctx, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, `action must be "open" or "close"`)
		return
	}

	updated, err := h.stores.Webhooks.Get(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.broadcaster.Publish("webhook_updated", updated)
	writeJSON(w, http.StatusOK, updated)
}
