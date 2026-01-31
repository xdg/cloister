package approval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
)

// EventType represents the type of SSE event.
type EventType string

const (
	// EventRequestAdded is sent when a new request is added to the queue.
	EventRequestAdded EventType = "request-added"
	// EventRequestRemoved is sent when a request is removed (approved/denied/timed out).
	EventRequestRemoved EventType = "request-removed"
)

// Event represents an SSE event to be broadcast to clients.
type Event struct {
	Type EventType
	Data string
}

// RemovedEventData is the JSON payload for request-removed events.
type RemovedEventData struct {
	ID string `json:"id"`
}

// EventHub manages SSE client connections and broadcasts events.
// It is safe for concurrent use.
type EventHub struct {
	mu       sync.RWMutex
	clients  map[chan Event]struct{}
	bufSize  int
	shutdown bool
}

// NewEventHub creates a new event hub for managing SSE connections.
func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[chan Event]struct{}),
		bufSize: 16,
	}
}

// Subscribe registers a new client to receive events.
// Returns a channel that will receive events. The caller must call
// Unsubscribe when done to prevent resource leaks.
func (h *EventHub) Subscribe() chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.shutdown {
		return nil
	}

	ch := make(chan Event, h.bufSize)
	h.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a client from receiving events and closes its channel.
func (h *EventHub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
}

// Broadcast sends an event to all connected clients.
// Clients that have full buffers will have the event dropped.
func (h *EventHub) Broadcast(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- event:
			// Event sent successfully
		default:
			// Client buffer is full, drop the event
		}
	}
}

// Close shuts down the event hub and closes all client channels.
func (h *EventHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.shutdown = true
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
}

// ClientCount returns the number of connected clients.
func (h *EventHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// BroadcastRequestAdded broadcasts a request-added event with rendered HTML.
func (h *EventHub) BroadcastRequestAdded(req templateRequest) {
	// Render the request template
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "request", req); err != nil {
		// Log error but don't fail - SSE is best-effort
		return
	}

	h.Broadcast(Event{
		Type: EventRequestAdded,
		Data: buf.String(),
	})
}

// BroadcastRequestRemoved broadcasts a request-removed event with the request ID.
func (h *EventHub) BroadcastRequestRemoved(id string) {
	data, _ := json.Marshal(RemovedEventData{ID: id})
	h.Broadcast(Event{
		Type: EventRequestRemoved,
		Data: string(data),
	})
}

// FormatSSE formats an event as an SSE message.
func FormatSSE(event Event) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, event.Data)
}
