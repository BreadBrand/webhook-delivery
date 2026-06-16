package worker

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/b2randon/webhook-delivery/internal/db"
	"github.com/b2randon/webhook-delivery/internal/models"
	"github.com/b2randon/webhook-delivery/internal/sse"
)

// Pool manages worker goroutines that deliver pending webhook payloads.
type Pool struct {
	stores        *db.Stores
	encKey        []byte
	workerN       int
	httpClient    *http.Client
	pollInterval  time.Duration
	probeInterval time.Duration
	broadcaster   *sse.Broadcaster
	wg            sync.WaitGroup
}

// NewPool returns a Pool ready to Start. workerN goroutines poll for pending deliveries.
func NewPool(stores *db.Stores, encKey []byte, workerN int) *Pool {
	return &Pool{
		stores:        stores,
		encKey:        encKey,
		workerN:       workerN,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		pollInterval:  500 * time.Millisecond,
		probeInterval: 30 * time.Second,
	}
}

// SetBroadcaster wires an SSE broadcaster for delivery_updated events. Optional.
func (p *Pool) SetBroadcaster(b *sse.Broadcaster) { p.broadcaster = b }

// Start resets orphaned in-flight deliveries, then launches worker and probe goroutines.
// Returns immediately; goroutines run until ctx is cancelled. Call Wait to block until they exit.
func (p *Pool) Start(ctx context.Context) {
	if err := p.stores.Deliveries.ResetInFlight(ctx); err != nil {
		slog.Error("startup recovery failed", "err", err)
	}
	for range p.workerN {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			supervise(ctx, func() { p.runWorker(ctx) })
		}()
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		supervise(ctx, func() { p.runProbe(ctx) })
	}()
}

// Wait blocks until all worker and probe goroutines have exited.
func (p *Pool) Wait() { p.wg.Wait() }

// supervise runs fn in a loop, catching panics. Exits when ctx is cancelled or fn returns normally.
func supervise(ctx context.Context, fn func()) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("worker panicked", "recover", r, "stack", string(debug.Stack()))
						time.Sleep(time.Second)
					}
				}()
				fn()
			}()
		}
	}
}

func (p *Pool) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batch, err := p.stores.Deliveries.ClaimPending(ctx, time.Now(), 10)
		if err != nil {
			slog.Error("claim pending", "err", err)
			sleep(ctx, p.pollInterval)
			continue
		}

		if len(batch) == 0 {
			sleep(ctx, p.pollInterval)
			continue
		}

		for _, d := range batch {
			claimed, err := p.stores.Deliveries.MarkInFlight(ctx, d.ID)
			if err != nil {
				slog.Error("mark in_flight", "id", d.ID, "err", err)
				continue
			}
			if !claimed {
				continue // another worker claimed it first
			}
			p.processWithRecovery(ctx, d)
		}
	}
}

func (p *Pool) process(ctx context.Context, d models.Delivery) {
	result := executeDelivery(ctx, d, p.stores, p.encKey, p.httpClient)

	// NFR5.1: structured log on every delivery attempt.
	logAttrs := []any{
		"event_id", d.EventID,
		"webhook_id", d.WebhookID,
		"attempt", d.Attempt + 1,
		"latency_ms", result.ResponseMs,
	}
	if result.StatusCode > 0 {
		logAttrs = append(logAttrs, "http_status", result.StatusCode)
	}
	if result.Err != nil {
		logAttrs = append(logAttrs, "error", *result.Err)
		slog.Info("delivery attempt failed", logAttrs...)
	} else {
		slog.Info("delivery attempt", logAttrs...)
	}

	newAttempt := d.Attempt + 1

	if result.Success {
		if err := p.stores.Deliveries.MarkSuccess(ctx, d.ID, result.StatusCode, result.ResponseMs); err != nil {
			slog.Error("mark success", "id", d.ID, "err", err)
		}
		if err := p.stores.Webhooks.RecordSuccess(ctx, d.WebhookID); err != nil {
			slog.Error("record success", "webhook_id", d.WebhookID, "err", err)
		}
		p.publishDelivery(ctx, d.ID)
		return
	}

	_, newStatus, err := p.stores.Webhooks.RecordFailure(ctx, d.WebhookID)
	if err != nil {
		slog.Error("record failure", "webhook_id", d.WebhookID, "err", err)
		// Fall through to MarkFailed so the delivery doesn't stay in_flight.
	}

	if newStatus == models.StatusCircuitOpen {
		// Hold the delivery that tripped the circuit, then drain any remaining pending ones.
		if err := p.stores.Deliveries.MarkHeld(ctx, d.ID); err != nil {
			slog.Error("mark held", "id", d.ID, "err", err)
		}
		if err := p.stores.Deliveries.HoldPendingForWebhook(ctx, d.WebhookID); err != nil {
			slog.Error("hold pending for webhook", "webhook_id", d.WebhookID, "err", err)
		}
		p.publishDelivery(ctx, d.ID)
		p.publishWebhook(ctx, d.WebhookID)
		return
	}

	nextAt := NextAttemptAt(newAttempt)
	var sc *int
	if result.StatusCode > 0 {
		sc = &result.StatusCode
	}
	var ms *int
	if result.ResponseMs > 0 {
		ms = &result.ResponseMs
	}
	if err := p.stores.Deliveries.MarkFailed(ctx, d.ID, newAttempt, sc, ms, result.Err, nextAt); err != nil {
		slog.Error("mark failed", "id", d.ID, "err", err)
	}
	p.publishDelivery(ctx, d.ID)
	p.publishWebhook(ctx, d.WebhookID)
}

// processWithRecovery calls process and, if it panics, re-queues the delivery
// as pending with a 10-second delay so it is not stuck in_flight.
func (p *Pool) processWithRecovery(ctx context.Context, d models.Delivery) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("worker panicked during delivery",
				"delivery_id", d.ID,
				"event_id", d.EventID,
				"recover", r,
				"stack", string(debug.Stack()),
			)
			nextAt := time.Now().Add(10 * time.Second)
			if err := p.stores.Deliveries.MarkFailed(ctx, d.ID, d.Attempt, nil, nil, nil, &nextAt); err != nil {
				slog.Error("re-queue after panic failed", "delivery_id", d.ID, "err", err)
			}
		}
	}()
	p.process(ctx, d)
}

func (p *Pool) runProbe(ctx context.Context) {
	ticker := time.NewTicker(p.probeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkProbes(ctx)
		}
	}
}

func (p *Pool) checkProbes(ctx context.Context) {
	webhooks, err := p.stores.Webhooks.ListDueForProbe(ctx)
	if err != nil {
		slog.Error("list due for probe", "err", err)
		return
	}
	for _, wh := range webhooks {
		d, err := p.stores.Deliveries.OldestHeld(ctx, wh.ID)
		if err != nil || d == nil {
			continue
		}
		claimed, err := p.stores.Deliveries.MarkProbeInFlight(ctx, d.ID)
		if err != nil || !claimed {
			continue
		}
		result := executeDelivery(ctx, *d, p.stores, p.encKey, p.httpClient)
		if result.Success {
			if err := p.stores.Deliveries.MarkSuccess(ctx, d.ID, result.StatusCode, result.ResponseMs); err != nil {
				slog.Error("probe: mark success", "id", d.ID, "err", err)
			}
			if err := p.stores.Webhooks.CloseCircuit(ctx, wh.ID); err != nil {
				slog.Error("probe: close circuit", "webhook_id", wh.ID, "err", err)
			}
			if err := p.stores.Deliveries.FlushHeld(ctx, wh.ID); err != nil {
				slog.Error("probe: flush held", "webhook_id", wh.ID, "err", err)
			}
		} else {
			// Probe failed — restore the delivery to held and reset the probe timer.
			if err := p.stores.Deliveries.MarkHeld(ctx, d.ID); err != nil {
				slog.Error("probe: restore held", "id", d.ID, "err", err)
			}
			if err := p.stores.Webhooks.SetCircuitOpen(ctx, wh.ID); err != nil {
				slog.Error("probe: reset timer", "webhook_id", wh.ID, "err", err)
			}
		}
	}
}

func (p *Pool) publishDelivery(ctx context.Context, id string) {
	if p.broadcaster == nil {
		return
	}
	d, err := p.stores.Deliveries.Get(ctx, id)
	if err == nil && d != nil {
		p.broadcaster.Publish("delivery_updated", d)
	}
}

func (p *Pool) publishWebhook(ctx context.Context, id string) {
	if p.broadcaster == nil {
		return
	}
	wh, err := p.stores.Webhooks.Get(ctx, id)
	if err == nil && wh != nil {
		p.broadcaster.Publish("webhook_updated", wh)
	}
}

// sleep waits for d or until ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
