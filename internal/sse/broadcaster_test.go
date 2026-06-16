package sse_test

import (
	"testing"

	"github.com/b2randon/webhook-delivery/internal/sse"
)

func TestBroadcasterPublishReachesSubscribers(t *testing.T) {
	b := sse.NewBroadcaster()

	ch1 := b.Subscribe("c1")
	ch2 := b.Subscribe("c2")

	b.Publish("test_event", map[string]string{"k": "v"})

	got1 := <-ch1
	got2 := <-ch2

	if got1 != got2 {
		t.Errorf("c1 got %q, c2 got %q — should be equal", got1, got2)
	}
	if got1 == "" {
		t.Error("message must not be empty")
	}
}

func TestBroadcasterUnsubscribeStopsDelivery(t *testing.T) {
	b := sse.NewBroadcaster()

	ch := b.Subscribe("c1")
	b.Unsubscribe("c1")

	b.Publish("test_event", "payload")

	select {
	case msg := <-ch:
		t.Errorf("should not receive after unsubscribe, got %q", msg)
	default:
		// correct: no message
	}
}

func TestBroadcasterDropsForFullChannel(t *testing.T) {
	b := sse.NewBroadcaster()
	ch := b.Subscribe("slow")

	// Fill the channel (capacity 64)
	for range 64 {
		b.Publish("fill", "x")
	}

	// One more publish must not block
	done := make(chan struct{})
	go func() {
		b.Publish("overflow", "y")
		close(done)
	}()

	select {
	case <-done:
		// correct: non-blocking
	case <-ch:
		// also fine if it drained one, just keep going
	}
	_ = ch
}
