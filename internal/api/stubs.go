package api

import "net/http"

func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {}
func (h *Handler) Redeliver(w http.ResponseWriter, r *http.Request)      {}
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request)         {}
