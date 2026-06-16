package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/b2randon/webhook-delivery/internal/crypto"
	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
)

type deliveryResult struct {
	StatusCode int
	ResponseMs int
	Err        *string
	Success    bool
}

func executeDelivery(ctx context.Context, d models.Delivery, stores *db.Stores, encKey []byte, client *http.Client) deliveryResult {
	ev, err := stores.Events.Get(ctx, d.EventID)
	if err != nil {
		msg := fmt.Sprintf("get event: %v", err)
		return deliveryResult{Err: &msg}
	}

	wh, err := stores.Webhooks.Get(ctx, d.WebhookID)
	if err != nil {
		msg := fmt.Sprintf("get webhook: %v", err)
		return deliveryResult{Err: &msg}
	}

	secret, err := crypto.Decrypt(encKey, wh.EncryptedSecret)
	if err != nil {
		msg := fmt.Sprintf("decrypt secret: %v", err)
		return deliveryResult{Err: &msg}
	}

	// Build CloudEvent envelope. Field order is fixed (see models.CloudEvent) for HMAC stability.
	envelope := models.CloudEvent{
		SpecVersion: "1.0",
		ID:          ev.ID,
		Type:        ev.Type,
		Source:      ev.Source,
		Time:        ev.Time,
		Data:        ev.Data,
	}

	// Marshal exactly once — the same bytes are signed and sent as the HTTP body.
	body, err := json.Marshal(envelope)
	if err != nil {
		msg := fmt.Sprintf("marshal envelope: %v", err)
		return deliveryResult{Err: &msg}
	}

	sig := crypto.Sign(body, secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		msg := fmt.Sprintf("build request: %v", err)
		return deliveryResult{Err: &msg}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", sig)
	req.Header.Set("X-Delivery-Id", d.ID)

	start := time.Now()
	resp, err := client.Do(req)
	responseMs := int(time.Since(start).Milliseconds())
	if err != nil {
		msg := fmt.Sprintf("http do: %v", err)
		return deliveryResult{ResponseMs: responseMs, Err: &msg}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return deliveryResult{
			StatusCode: resp.StatusCode,
			ResponseMs: responseMs,
			Success:    true,
		}
	}

	msg := fmt.Sprintf("non-2xx status %d", resp.StatusCode)
	return deliveryResult{
		StatusCode: resp.StatusCode,
		ResponseMs: responseMs,
		Err:        &msg,
	}
}
