// Package approval provides an in-memory queue for pending command execution
// requests that require human review before proceeding.
package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// DefaultTimeout is the default timeout for pending requests (5 minutes).
const DefaultTimeout = 5 * time.Minute

// Response represents the result of an approval decision.
// This type is defined locally to avoid import cycles with the request package.
// It has the same JSON structure as request.CommandResponse.
type Response struct {
	Status   string `json:"status"`
	Pattern  string `json:"pattern,omitempty"`
	Reason   string `json:"reason,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// PendingRequest represents a command execution request awaiting human approval.
type PendingRequest struct {
	ID        string
	Cloister  string
	Project   string
	Branch    string
	Agent     string
	Cmd       string
	Timestamp time.Time
	Response  chan<- Response // Channel to send result back
}

// Queue manages pending approval requests with thread-safe operations.
type Queue struct {
	mu       sync.RWMutex
	requests map[string]*PendingRequest
	cancels  map[string]context.CancelFunc // Cancel functions for timeout goroutines
	timeout  time.Duration
	events   *EventHub // Optional event hub for SSE broadcasts
}

// NewQueue creates a new empty approval queue with the default timeout.
func NewQueue() *Queue {
	return NewQueueWithTimeout(DefaultTimeout)
}

// NewQueueWithTimeout creates a new empty approval queue with a custom timeout duration.
func NewQueueWithTimeout(timeout time.Duration) *Queue {
	return &Queue{
		requests: make(map[string]*PendingRequest),
		cancels:  make(map[string]context.CancelFunc),
		timeout:  timeout,
	}
}

// SetEventHub sets the event hub for SSE broadcasts.
// When set, the queue will broadcast events when requests are added or time out.
func (q *Queue) SetEventHub(hub *EventHub) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = hub
}

// Add adds a new pending request to the queue and returns its generated ID.
// The ID is generated using crypto/rand (8 bytes = 16 hex characters).
// A timeout goroutine is started that will send a timeout response on the
// request's Response channel if the request is not approved/denied in time.
// If an EventHub is configured, a request-added event is broadcast to SSE clients.
func (q *Queue) Add(req *PendingRequest) (string, error) {
	id, err := generateID()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())

	q.mu.Lock()
	req.ID = id
	q.requests[id] = req
	q.cancels[id] = cancel
	events := q.events // Capture reference while holding lock
	q.mu.Unlock()

	// Broadcast request-added event to SSE clients
	if events != nil {
		events.BroadcastPendingRequestAdded(req)
	}

	// Start timeout goroutine
	go q.handleTimeout(ctx, id, req.Response)

	return id, nil
}

// handleTimeout waits for the timeout duration and sends a timeout response
// if the context has not been canceled (i.e., request not approved/denied).
// If an EventHub is configured, a request-removed event is broadcast to SSE clients.
func (q *Queue) handleTimeout(ctx context.Context, id string, respChan chan<- Response) {
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
		}
		events := q.events // Capture reference while holding lock
		q.mu.Unlock()

		// Only send timeout response if the request was still pending
		if exists {
			// Broadcast request-removed event to SSE clients
			if events != nil {
				events.BroadcastRequestRemoved(id)
			}

			if respChan != nil {
				respChan <- Response{
					Status: "timeout",
					Reason: "Request timed out waiting for approval",
				}
			}
		}
	}
}

// Get retrieves a pending request by ID.
// Returns nil and false if the request is not found.
func (q *Queue) Get(id string) (*PendingRequest, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	req, ok := q.requests[id]
	return req, ok
}

// Remove removes a pending request from the queue by ID and cancels its
// timeout goroutine. This is a no-op if the ID is not found.
func (q *Queue) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if cancel, ok := q.cancels[id]; ok {
		cancel() // Cancel the timeout goroutine
		delete(q.cancels, id)
	}
	delete(q.requests, id)
}

// List returns a copy of all pending requests for the approval UI.
// The returned slice is safe to iterate without holding locks.
// The Response channel is excluded from the returned copies for safety.
func (q *Queue) List() []PendingRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]PendingRequest, 0, len(q.requests))
	for _, req := range q.requests {
		// Copy the request without the Response channel for safety
		result = append(result, PendingRequest{
			ID:        req.ID,
			Cloister:  req.Cloister,
			Project:   req.Project,
			Branch:    req.Branch,
			Agent:     req.Agent,
			Cmd:       req.Cmd,
			Timestamp: req.Timestamp,
			// Response channel intentionally omitted
		})
	}
	return result
}

// Len returns the number of pending requests in the queue.
func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.requests)
}

// generateID creates a cryptographically random ID as 16 hex characters (8 bytes).
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
