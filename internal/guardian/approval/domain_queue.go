// Package approval provides an in-memory queue for pending domain approval
// requests that require human review before proceeding.
//
// Note: This file shares package-level imports with queue.go.
package approval

import (
	"context"
	"sync"
	"time"

	"github.com/xdg/cloister/internal/audit"
)

// DomainResponse represents the result of a domain approval decision.
type DomainResponse struct {
	Status           string `json:"status"`                      // "approved", "denied", or "timeout"
	Scope            string `json:"scope"`                       // "session", "project", or "global" (only for approved)
	Reason           string `json:"reason,omitempty"`            // Reason for denial (only for denied)
	Pattern          string `json:"pattern,omitempty"`           // Wildcard pattern if approved with wildcard (e.g., "*.example.com")
	PersistenceError string `json:"persistence_error,omitempty"` // Error message if config persistence failed (domain still approved for session)
}

// DomainRequest represents a domain approval request awaiting human decision.
type DomainRequest struct {
	ID        string
	Cloister  string
	Project   string
	Domain    string
	Token     string // Token that made the request (used for deduplication)
	Timestamp time.Time
	ExpiresAt time.Time
	// Responses holds channels to send result back to all waiters (for coalesced requests).
	// IMPORTANT: These channels MUST be buffered (buffer size >= 1) to prevent goroutine leaks.
	// The timeout handler uses a non-blocking send, but callers should still use buffered
	// channels to ensure reliable delivery of approval/denial responses.
	Responses []chan<- DomainResponse
}

// DomainQueue manages pending domain approval requests with thread-safe operations.
type DomainQueue struct {
	mu          sync.RWMutex
	requests    map[string]*DomainRequest
	cancels     map[string]context.CancelFunc // Cancel functions for timeout goroutines
	pending     map[string]string             // "token:domain" -> requestID for deduplication
	timeout     time.Duration
	events      *EventHub     // Optional event hub for SSE broadcasts
	auditLogger *audit.Logger // Optional audit logger for domain events
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
		pending:  make(map[string]string),
		timeout:  timeout,
	}
}

// pendingKey generates the deduplication key for a token and domain combination.
func pendingKey(token, domain string) string {
	return token + ":" + domain
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

// SetAuditLogger sets the audit logger for domain events.
// When set, the queue will log domain request and timeout events.
//
// IMPORTANT: This should be called before any requests are added to the queue.
// Audit events for requests added before SetAuditLogger is called will not be logged.
func (dq *DomainQueue) SetAuditLogger(logger *audit.Logger) {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	dq.auditLogger = logger
}

// Add adds a new pending domain request to the queue and returns its generated ID.
// The ID is generated using crypto/rand (8 bytes = 16 hex characters).
// A timeout goroutine is started that will send a timeout response on the
// request's Responses channels if the request is not approved/denied in time.
// If an EventHub is configured, a domain-request-added event is broadcast to SSE clients.
//
// Request Deduplication:
// If a request with the same token+domain already exists, the new response channel
// is added to the existing request's Responses slice, and the existing request's ID
// is returned. This ensures multiple requesters for the same domain receive the same
// approval decision without creating duplicate queue entries.
func (dq *DomainQueue) Add(req *DomainRequest) (string, error) {
	dq.mu.Lock()

	// Check for existing request with same token+domain (deduplication)
	key := pendingKey(req.Token, req.Domain)
	if existingID, exists := dq.pending[key]; exists {
		if existingReq, ok := dq.requests[existingID]; ok {
			// Add new response channel to existing request
			if len(req.Responses) > 0 {
				existingReq.Responses = append(existingReq.Responses, req.Responses...)
			}
			dq.mu.Unlock()
			return existingID, nil
		}
		// Stale entry in pending map, clean it up
		delete(dq.pending, key)
	}

	// Generate new ID for this request
	id, err := generateID()
	if err != nil {
		dq.mu.Unlock()
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())

	req.ID = id
	req.ExpiresAt = time.Now().Add(dq.timeout)
	dq.requests[id] = req
	dq.cancels[id] = cancel
	dq.pending[key] = id
	events := dq.events           // Capture reference while holding lock
	auditLogger := dq.auditLogger // Capture reference while holding lock
	dq.mu.Unlock()

	// Log domain request event
	if auditLogger != nil {
		_ = auditLogger.LogDomainRequest(req.Project, req.Cloister, req.Domain)
	}

	// Broadcast domain-request-added event to SSE clients
	if events != nil {
		events.BroadcastDomainRequestAdded(req)
	}

	// Start timeout goroutine
	go dq.handleTimeout(ctx, id)

	return id, nil
}

// handleTimeout waits for the timeout duration and sends a timeout response
// if the context has not been canceled (i.e., request not approved/denied).
// If an EventHub is configured, a domain-request-removed event is broadcast to SSE clients.
// The timeout response is broadcast to all response channels in the request's Responses slice.
func (dq *DomainQueue) handleTimeout(ctx context.Context, id string) {
	select {
	case <-ctx.Done():
		// Request was approved/denied before timeout, do nothing
		return
	case <-time.After(dq.timeout):
		// Timeout reached, send timeout response
		dq.mu.Lock()
		req, exists := dq.requests[id]
		if exists {
			delete(dq.requests, id)
			delete(dq.cancels, id)
			// Clean up pending map entry
			key := pendingKey(req.Token, req.Domain)
			delete(dq.pending, key)
		}
		events := dq.events           // Capture reference while holding lock
		auditLogger := dq.auditLogger // Capture reference while holding lock
		dq.mu.Unlock()

		// Only send timeout response if the request was still pending
		if exists {
			// Log domain timeout event
			if auditLogger != nil {
				_ = auditLogger.LogDomainTimeout(req.Project, req.Cloister, req.Domain)
			}

			// Broadcast domain-request-removed event to SSE clients
			if events != nil {
				events.BroadcastDomainRequestRemoved(id)
			}

			// Broadcast timeout response to all waiting channels
			timeoutResp := DomainResponse{
				Status: "timeout",
				Reason: "Request timed out waiting for approval",
			}
			for _, respChan := range req.Responses {
				if respChan != nil {
					// Use non-blocking send to prevent goroutine leak if caller provided unbuffered channel
					select {
					case respChan <- timeoutResp:
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
	// Clean up pending map entry
	if req, ok := dq.requests[id]; ok {
		key := pendingKey(req.Token, req.Domain)
		delete(dq.pending, key)
	}
	delete(dq.requests, id)
}

// List returns a copy of all pending domain requests for the approval UI.
// The returned slice is safe to iterate without holding locks.
// The Responses channels are excluded from the returned copies for safety.
func (dq *DomainQueue) List() []DomainRequest {
	dq.mu.RLock()
	defer dq.mu.RUnlock()

	result := make([]DomainRequest, 0, len(dq.requests))
	for _, req := range dq.requests {
		// Copy the request without the Responses channels for safety
		result = append(result, DomainRequest{
			ID:        req.ID,
			Cloister:  req.Cloister,
			Project:   req.Project,
			Domain:    req.Domain,
			Token:     req.Token,
			Timestamp: req.Timestamp,
			ExpiresAt: req.ExpiresAt,
			// Responses channels intentionally omitted
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
