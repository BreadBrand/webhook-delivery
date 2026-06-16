package api

import "net/http"

func (h *Handler) IngestEvent(w http.ResponseWriter, r *http.Request)    {}
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request)     {}
func (h *Handler) EventVolume(w http.ResponseWriter, r *http.Request)    {}
func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {}
func (h *Handler) Redeliver(w http.ResponseWriter, r *http.Request)      {}
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request)         {}
