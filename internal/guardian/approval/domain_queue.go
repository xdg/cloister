// Package approval provides an in-memory queue for pending command execution
// requests that require human review before proceeding.
package approval

import (
	"context"
	"sync"
	"time"
)

// DefaultDomainTimeout is the default timeout for pending domain requests (60 seconds).
// This is shorter than command approval because proxy connections shouldn't wait too long.
const DefaultDomainTimeout = 60 * time.Second

// DomainScope represents the persistence scope for approved domains.
type DomainScope string

const (
	// ScopeSession means the domain is approved for this session only (memory only).
	// The approval is tied to a specific token and cleared when the cloister stops.
	ScopeSession DomainScope = "session"
	// ScopeProject means the domain is approved for the project and persisted to project config.
	ScopeProject DomainScope = "project"
	// ScopeGlobal means the domain is approved globally and persisted to global config.
	ScopeGlobal DomainScope = "global"
)

// DomainResponse represents the result of a domain approval decision.
type DomainResponse struct {
	Status string      `json:"status"`           // "approved", "denied", "timeout"
	Scope  DomainScope `json:"scope,omitempty"`  // only for approved
	Reason string      `json:"reason,omitempty"` // for denied/timeout
}

// DomainRequest represents a pending domain approval request.
type DomainRequest struct {
	ID        string
	Token     string    // The token that made the request (for session scope)
	Cloister  string    // Name of the cloister container
	Project   string    // Project name
	Domain    string    // The hostname being requested (without port)
	Timestamp time.Time // When the request was created
	Expires   time.Time // When this request times out
	Response  chan<- DomainResponse
}

// DomainQueue manages pending domain approval requests with thread-safe operations.
type DomainQueue struct {
	mu       sync.RWMutex
	requests map[string]*DomainRequest
	cancels  map[string]context.CancelFunc // Cancel functions for timeout goroutines
	// pending tracks domains with pending requests to coalesce duplicates.
	// Key is "token:domain" to allow different tokens to request same domain.
	pending map[string]string // "token:domain" -> request ID
	timeout time.Duration
	events  *EventHub // Optional event hub for SSE broadcasts
}

// NewDomainQueue creates a new empty domain approval queue with the default timeout.
func NewDomainQueue() *DomainQueue {
	return NewDomainQueueWithTimeout(DefaultDomainTimeout)
}

// NewDomainQueueWithTimeout creates a new empty domain approval queue with a custom timeout duration.
func NewDomainQueueWithTimeout(timeout time.Duration) *DomainQueue {
	return &DomainQueue{
		requests: make(map[string]*DomainRequest),
		cancels:  make(map[string]context.CancelFunc),
		pending:  make(map[string]string),
		timeout:  timeout,
	}
}

// SetEventHub sets the event hub for SSE broadcasts.
// When set, the queue will broadcast events when requests are added or time out.
func (q *DomainQueue) SetEventHub(hub *EventHub) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = hub
}

// pendingKey creates a key for the pending map from token and domain.
func pendingKey(token, domain string) string {
	return token + ":" + domain
}

// Add adds a new pending domain request to the queue and returns its generated ID.
// If a request for the same token+domain is already pending, returns the existing ID.
// A timeout goroutine is started that will send a timeout response on the
// request's Response channel if the request is not approved/denied in time.
// If an EventHub is configured, a domain-added event is broadcast to SSE clients.
func (q *DomainQueue) Add(req *DomainRequest) (string, error) {
	q.mu.Lock()

	// Check for existing pending request for same token+domain
	key := pendingKey(req.Token, req.Domain)
	if existingID, ok := q.pending[key]; ok {
		q.mu.Unlock()
		return existingID, nil
	}

	id, err := generateID()
	if err != nil {
		q.mu.Unlock()
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())

	req.ID = id
	req.Expires = time.Now().Add(q.timeout)
	q.requests[id] = req
	q.cancels[id] = cancel
	q.pending[key] = id
	events := q.events // Capture reference while holding lock
	q.mu.Unlock()

	// Broadcast domain-added event to SSE clients
	if events != nil {
		events.BroadcastDomainRequestAdded(req)
	}

	// Start timeout goroutine
	go q.handleTimeout(ctx, id, req.Token, req.Domain, req.Response)

	return id, nil
}

// handleTimeout waits for the timeout duration and sends a timeout response
// if the context has not been canceled (i.e., request not approved/denied).
// If an EventHub is configured, a domain-removed event is broadcast to SSE clients.
func (q *DomainQueue) handleTimeout(ctx context.Context, id, token, domain string, respChan chan<- DomainResponse) {
	select {
	case <-ctx.Done():
		// Request was approved/denied before timeout, do nothing
		return
	case <-time.After(q.timeout):
		// Timeout reached, send timeout response
		q.mu.Lock()
		_, exists := q.requests[id]
		if exists {
			delete(q.requests, id)
			delete(q.cancels, id)
			delete(q.pending, pendingKey(token, domain))
		}
		events := q.events // Capture reference while holding lock
		q.mu.Unlock()

		// Only send timeout response if the request was still pending
		if exists {
			// Broadcast domain-removed event to SSE clients
			if events != nil {
				events.BroadcastDomainRequestRemoved(id)
			}

			if respChan != nil {
				respChan <- DomainResponse{
					Status: "timeout",
					Reason: "Request timed out waiting for approval",
				}
			}
		}
	}
}

// Get retrieves a pending domain request by ID.
// Returns nil and false if the request is not found.
func (q *DomainQueue) Get(id string) (*DomainRequest, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	req, ok := q.requests[id]
	return req, ok
}

// Remove removes a pending domain request from the queue by ID and cancels its
// timeout goroutine. This is a no-op if the ID is not found.
func (q *DomainQueue) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	req, exists := q.requests[id]
	if !exists {
		return
	}

	if cancel, ok := q.cancels[id]; ok {
		cancel() // Cancel the timeout goroutine
		delete(q.cancels, id)
	}
	delete(q.requests, id)
	delete(q.pending, pendingKey(req.Token, req.Domain))
}

// List returns a copy of all pending domain requests for the approval UI.
// The returned slice is safe to iterate without holding locks.
// The Response channel is excluded from the returned copies for safety.
func (q *DomainQueue) List() []DomainRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]DomainRequest, 0, len(q.requests))
	for _, req := range q.requests {
		// Copy the request without the Response channel for safety
		result = append(result, DomainRequest{
			ID:        req.ID,
			Token:     req.Token,
			Cloister:  req.Cloister,
			Project:   req.Project,
			Domain:    req.Domain,
			Timestamp: req.Timestamp,
			Expires:   req.Expires,
			// Response channel intentionally omitted
		})
	}
	return result
}

// Len returns the number of pending domain requests in the queue.
func (q *DomainQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.requests)
}
