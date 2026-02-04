// Package approval provides an in-memory queue for pending domain approval
// requests that require human review before proceeding.
//
// Note: This file shares package-level imports with queue.go.
package approval

import (
	"context"
	"sync"
	"time"
)

// DomainResponse represents the result of a domain approval decision.
type DomainResponse struct {
	Status string `json:"status"` // "approved", "denied", or "timeout"
	Scope  string `json:"scope"`  // "session", "project", or "global" (only for approved)
	Reason string `json:"reason,omitempty"`
}

// DomainRequest represents a domain approval request awaiting human decision.
type DomainRequest struct {
	ID        string
	Cloister  string
	Project   string
	Domain    string
	Timestamp time.Time
	ExpiresAt time.Time
	// Response is the channel to send result back.
	// IMPORTANT: This channel MUST be buffered (buffer size >= 1) to prevent goroutine leaks.
	// The timeout handler uses a non-blocking send, but callers should still use buffered
	// channels to ensure reliable delivery of approval/denial responses.
	Response chan<- DomainResponse
}

// DomainQueue manages pending domain approval requests with thread-safe operations.
type DomainQueue struct {
	mu       sync.RWMutex
	requests map[string]*DomainRequest
	cancels  map[string]context.CancelFunc // Cancel functions for timeout goroutines
	timeout  time.Duration
	events   *EventHub // Optional event hub for SSE broadcasts
}

// NewDomainQueue creates a new empty domain approval queue with the default timeout.
func NewDomainQueue() *DomainQueue {
	return NewDomainQueueWithTimeout(DefaultTimeout)
}

// NewDomainQueueWithTimeout creates a new empty domain approval queue with a custom timeout duration.
func NewDomainQueueWithTimeout(timeout time.Duration) *DomainQueue {
	return &DomainQueue{
		requests: make(map[string]*DomainRequest),
		cancels:  make(map[string]context.CancelFunc),
		timeout:  timeout,
	}
}

// SetEventHub sets the event hub for SSE broadcasts.
// When set, the queue will broadcast events when requests are added or time out.
//
// IMPORTANT: This should be called before any requests are added to the queue.
// Events for requests added before SetEventHub is called will not be broadcast.
func (dq *DomainQueue) SetEventHub(hub *EventHub) {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	dq.events = hub
}

// Add adds a new pending domain request to the queue and returns its generated ID.
// The ID is generated using crypto/rand (8 bytes = 16 hex characters).
// A timeout goroutine is started that will send a timeout response on the
// request's Response channel if the request is not approved/denied in time.
// If an EventHub is configured, a domain-request-added event is broadcast to SSE clients.
func (dq *DomainQueue) Add(req *DomainRequest) (string, error) {
	id, err := generateID()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())

	dq.mu.Lock()
	req.ID = id
	req.ExpiresAt = time.Now().Add(dq.timeout)
	dq.requests[id] = req
	dq.cancels[id] = cancel
	events := dq.events // Capture reference while holding lock
	dq.mu.Unlock()

	// Broadcast domain-request-added event to SSE clients
	if events != nil {
		events.BroadcastDomainRequestAdded(req)
	}

	// Start timeout goroutine
	go dq.handleTimeout(ctx, id, req.Response)

	return id, nil
}

// handleTimeout waits for the timeout duration and sends a timeout response
// if the context has not been canceled (i.e., request not approved/denied).
// If an EventHub is configured, a domain-request-removed event is broadcast to SSE clients.
func (dq *DomainQueue) handleTimeout(ctx context.Context, id string, respChan chan<- DomainResponse) {
	select {
	case <-ctx.Done():
		// Request was approved/denied before timeout, do nothing
		return
	case <-time.After(dq.timeout):
		// Timeout reached, send timeout response
		dq.mu.Lock()
		_, exists := dq.requests[id]
		if exists {
			delete(dq.requests, id)
			delete(dq.cancels, id)
		}
		events := dq.events // Capture reference while holding lock
		dq.mu.Unlock()

		// Only send timeout response if the request was still pending
		if exists {
			// Broadcast domain-request-removed event to SSE clients
			if events != nil {
				events.BroadcastDomainRequestRemoved(id)
			}

			// Use non-blocking send to prevent goroutine leak if caller provided unbuffered channel
			if respChan != nil {
				select {
				case respChan <- DomainResponse{
					Status: "timeout",
					Reason: "Request timed out waiting for approval",
				}:
					// Response sent successfully
				default:
					// Channel full or unbuffered - drop the response
					// This should not happen if callers follow the documented requirement
					// to use buffered channels, but we handle it defensively
				}
			}
		}
	}
}

// Get retrieves a pending domain request by ID.
// Returns nil and false if the request is not found.
func (dq *DomainQueue) Get(id string) (*DomainRequest, bool) {
	dq.mu.RLock()
	defer dq.mu.RUnlock()

	req, ok := dq.requests[id]
	return req, ok
}

// Remove removes a pending domain request from the queue by ID and cancels its
// timeout goroutine. This is a no-op if the ID is not found.
func (dq *DomainQueue) Remove(id string) {
	dq.mu.Lock()
	defer dq.mu.Unlock()

	if cancel, ok := dq.cancels[id]; ok {
		cancel() // Cancel the timeout goroutine
		delete(dq.cancels, id)
	}
	delete(dq.requests, id)
}

// List returns a copy of all pending domain requests for the approval UI.
// The returned slice is safe to iterate without holding locks.
// The Response channel is excluded from the returned copies for safety.
func (dq *DomainQueue) List() []DomainRequest {
	dq.mu.RLock()
	defer dq.mu.RUnlock()

	result := make([]DomainRequest, 0, len(dq.requests))
	for _, req := range dq.requests {
		// Copy the request without the Response channel for safety
		result = append(result, DomainRequest{
			ID:        req.ID,
			Cloister:  req.Cloister,
			Project:   req.Project,
			Domain:    req.Domain,
			Timestamp: req.Timestamp,
			ExpiresAt: req.ExpiresAt,
			// Response channel intentionally omitted
		})
	}
	return result
}

// Len returns the number of pending domain requests in the queue.
func (dq *DomainQueue) Len() int {
	dq.mu.RLock()
	defer dq.mu.RUnlock()

	return len(dq.requests)
}
