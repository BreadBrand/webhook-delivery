package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
)

func (h *Handler) IngestEvent(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var input struct {
		SpecVersion string          `json:"specversion"`
		ID          string          `json:"id"`
		Type        string          `json:"type"`
		Source      string          `json:"source"`
		Time        *time.Time      `json:"time"`
		Data        json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	var missing []string
	if input.ID == "" {
		missing = append(missing, "id")
	}
	if input.Type == "" {
		missing = append(missing, "type")
	}
	if input.Source == "" {
		missing = append(missing, "source")
	}
	if input.Time == nil {
		missing = append(missing, "time")
	}
	if len(input.Data) == 0 {
		missing = append(missing, "data")
	}
	if len(missing) > 0 {
		writeError(w, http.StatusBadRequest, "missing fields: "+joinStrings(missing))
		return
	}
	if input.SpecVersion != "1.0" {
		writeError(w, http.StatusBadRequest, "specversion must be 1.0")
		return
	}
	if input.Data[0] != '{' && input.Data[0] != '[' {
		writeError(w, http.StatusBadRequest, "data must be a JSON object or array")
		return
	}

	ctx := r.Context()
	ev := &models.Event{
		ID:     input.ID,
		Type:   input.Type,
		Source: input.Source,
		Time:   input.Time.UTC(),
		Data:   input.Data,
	}
	if err := h.stores.Events.Create(ctx, ev); err != nil {
		if errors.Is(err, db.ErrConflict) {
			existing, _ := h.stores.Events.Get(ctx, input.ID)
			writeJSON(w, http.StatusConflict, existing)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	webhooks, err := h.stores.Webhooks.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(webhooks) > 0 {
		if err := h.stores.Deliveries.CreateBatch(ctx, ev.ID, webhooks); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	h.broadcaster.Publish("event_ingested", ev)
	writeJSON(w, http.StatusAccepted, ev)
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	list, err := h.stores.Events.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) EventVolume(w http.ResponseWriter, r *http.Request) {
	windows := map[string]time.Duration{
		"5m":  5 * time.Minute,
		"30m": 30 * time.Minute,
		"1h":  time.Hour,
		"24h": 24 * time.Hour,
	}
	win := r.URL.Query().Get("window")
	if win == "" {
		win = "30m"
	}
	d, ok := windows[win]
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be 5m, 30m, 1h, or 24h")
		return
	}
	vol, err := h.stores.Events.Volume(r.Context(), d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vol)
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
