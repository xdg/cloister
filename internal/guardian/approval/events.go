package approval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// EventType represents the type of SSE event.
type EventType string

const (
	// EventRequestAdded is sent when a new request is added to the queue.
	EventRequestAdded EventType = "request-added"
	// EventRequestRemoved is sent when a request is removed (approved/denied/timed out).
	EventRequestRemoved EventType = "request-removed"
	// EventHeartbeat is sent periodically to keep the connection alive.
	EventHeartbeat EventType = "heartbeat"
	// EventDomainRequestAdded is sent when a new domain approval request is added.
	EventDomainRequestAdded EventType = "domain-request-added"
	// EventDomainRequestRemoved is sent when a domain request is removed (approved/denied/timed out).
	EventDomainRequestRemoved EventType = "domain-request-removed"
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
	if ch == nil {
		return
	}

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

// BroadcastPendingRequestAdded broadcasts a request-added event for a PendingRequest.
// This is a convenience method that converts PendingRequest to the template format.
func (h *EventHub) BroadcastPendingRequestAdded(req *PendingRequest) {
	h.BroadcastRequestAdded(templateRequest{
		ID:        req.ID,
		Cloister:  req.Cloister,
		Project:   req.Project,
		Branch:    req.Branch,
		Agent:     req.Agent,
		Cmd:       req.Cmd,
		Timestamp: req.Timestamp.Format(time.RFC3339),
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

// BroadcastDomainRequestAdded broadcasts a domain-request-added event with rendered HTML.
func (h *EventHub) BroadcastDomainRequestAdded(req *DomainRequest) {
	// Render the domain_request template
	var buf bytes.Buffer
	templateReq := domainTemplateRequest{
		ID:        req.ID,
		Domain:    req.Domain,
		Cloister:  req.Cloister,
		Project:   req.Project,
		Timestamp: req.Timestamp.Format(time.RFC3339),
		Wildcard:  domainToWildcard(req.Domain),
	}
	if err := templates.ExecuteTemplate(&buf, "domain_request", templateReq); err != nil {
		// Log error but don't fail - SSE is best-effort
		return
	}

	h.Broadcast(Event{
		Type: EventDomainRequestAdded,
		Data: buf.String(),
	})
}

// countDomainComponents counts the number of domain components (labels) in a domain.
// Examples: "api.example.com" -> 3, "example.com" -> 2, "localhost" -> 1
func countDomainComponents(domain string) int {
	if domain == "" {
		return 0
	}
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return 0
	}
	return strings.Count(domain, ".") + 1
}

// domainToWildcard converts a domain like "api.example.com" to a wildcard
// pattern like "*.example.com". Returns empty string if the domain doesn't
// have at least three components to prevent overly broad patterns like "*.com".
func domainToWildcard(domain string) string {
	// Require at least 3 components to prevent overly broad patterns
	if countDomainComponents(domain) < 3 {
		return ""
	}
	// Find the first dot
	idx := strings.Index(domain, ".")
	if idx == -1 || idx == len(domain)-1 {
		return ""
	}
	// Return *.rest_of_domain
	return "*" + domain[idx:]
}

// BroadcastDomainRequestRemoved broadcasts a domain-request-removed event.
// This is a placeholder for Phase 6.5 - currently just broadcasts the ID.
func (h *EventHub) BroadcastDomainRequestRemoved(id string) {
	data, _ := json.Marshal(RemovedEventData{ID: id})
	h.Broadcast(Event{
		Type: "domain-request-removed",
		Data: string(data),
	})
}

// FormatSSE formats an event as an SSE message.
// For multiline data, each line must be prefixed with "data: " per the SSE spec.
func FormatSSE(event Event) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("event: %s\n", event.Type))

	// Split data into lines and prefix each with "data: "
	lines := bytes.Split([]byte(event.Data), []byte("\n"))
	for _, line := range lines {
		buf.WriteString("data: ")
		buf.Write(line)
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n') // End of message
	return buf.String()
}
