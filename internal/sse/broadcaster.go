package sse

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Broadcaster fan-outs SSE messages to all connected clients.
// Each client gets a buffered channel; sends are non-blocking.
type Broadcaster struct {
	clients sync.Map // id string → chan string
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{}
}

func (b *Broadcaster) Subscribe(id string) chan string {
	ch := make(chan string, 64)
	b.clients.Store(id, ch)
	return ch
}

func (b *Broadcaster) Unsubscribe(id string) {
	b.clients.Delete(id)
}

// Publish sends an SSE-formatted message to every subscriber.
// Slow clients whose channel is full are silently skipped.
func (b *Broadcaster) Publish(eventType string, payload any) {
	data, _ := json.Marshal(payload)
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
	b.clients.Range(func(_, v any) bool {
		ch := v.(chan string)
		select {
		case ch <- msg:
		default:
		}
		return true
	})
}
