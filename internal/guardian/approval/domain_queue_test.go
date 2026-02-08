package approval

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewDomainQueueWithTimeout(t *testing.T) {
	timeout := 30 * time.Second
	q := NewDomainQueueWithTimeout(timeout)
	if q == nil {
		t.Fatal("NewDomainQueueWithTimeout() returned nil")
	}
	if q.timeout != timeout {
		t.Errorf("timeout = %v, want %v", q.timeout, timeout)
	}
	if q.Len() != 0 {
		t.Errorf("new queue should be empty, got len=%d", q.Len())
	}
}

func TestDomainQueue_TimeoutSendsResponse(t *testing.T) {
	// Use a very short timeout for testing
	timeout := 50 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Wait for timeout response
	select {
	case resp := <-respChan:
		if resp.Status != "timeout" {
			t.Errorf("Status = %q, want %q", resp.Status, "timeout")
		}
		if resp.Reason == "" {
			t.Error("Reason should not be empty for timeout response")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected timeout response, but none received")
	}

	// Request should be removed from queue
	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after timeout", q.Len())
	}
}

func TestDomainQueue_ApproveBeforeTimeout(t *testing.T) {
	// Use a longer timeout to ensure we approve before it fires
	timeout := 500 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Approve immediately (simulate approval before timeout)
	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}

	// Send approval response and remove from queue
	approvalResp := DomainResponse{
		Status: "approved",
		Scope:  "session",
	}
	for _, ch := range got.Responses {
		ch <- approvalResp
	}
	q.Remove(id)

	// Receive the approval response
	select {
	case resp := <-respChan:
		if resp.Status != "approved" {
			t.Errorf("Status = %q, want %q", resp.Status, "approved")
		}
		if resp.Scope != "session" {
			t.Errorf("Scope = %q, want %q", resp.Scope, "session")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected approval response")
	}

	// Wait past the original timeout to ensure no timeout response is sent
	time.Sleep(600 * time.Millisecond)

	// Channel should be empty (no timeout response)
	select {
	case resp := <-respChan:
		t.Errorf("unexpected response after approval: %+v", resp)
	default:
		// Expected: no additional response
	}
}

func TestDomainQueue_RemoveCancelsTimeout(t *testing.T) {
	timeout := 100 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Remove immediately (this should cancel the timeout)
	q.Remove(id)

	// Wait past timeout duration
	time.Sleep(200 * time.Millisecond)

	// No timeout response should be sent
	select {
	case resp := <-respChan:
		t.Errorf("unexpected timeout response after Remove(): %+v", resp)
	default:
		// Expected: no response
	}
}

func TestDomainQueue_TimeoutWithNilChannel(t *testing.T) {
	// Verify that timeout doesn't panic with nil Response channel
	timeout := 50 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: nil, // no response channels
	}

	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Wait for timeout - should not panic
	time.Sleep(100 * time.Millisecond)

	// Request should still be removed from queue
	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after timeout", q.Len())
	}
}

func TestNewDomainQueue(t *testing.T) {
	q := NewDomainQueue()
	if q == nil {
		t.Fatal("NewDomainQueue() returned nil")
	}
	if q.Len() != 0 {
		t.Errorf("new queue should be empty, got len=%d", q.Len())
	}
}

func TestDomainQueue_Add(t *testing.T) {
	q := NewDomainQueue()
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// ID should be 16 hex characters (8 bytes)
	if len(id) != 16 {
		t.Errorf("ID length = %d, want 16", len(id))
	}

	// ID should be set on the request
	if req.ID != id {
		t.Errorf("req.ID = %q, want %q", req.ID, id)
	}

	// Queue length should be 1
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1", q.Len())
	}
}

func TestDomainQueue_AddMultiple(t *testing.T) {
	q := NewDomainQueue()

	// Add multiple requests with different domains and verify unique IDs
	// (same domain+token would be deduplicated)
	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req := &DomainRequest{
			Cloister:  "test-cloister",
			Project:   "test-project",
			Domain:    fmt.Sprintf("example%d.com", i), // unique domain to avoid deduplication
			Timestamp: time.Now(),
		}
		id, err := q.Add(req)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if ids[id] {
			t.Errorf("duplicate ID generated: %q", id)
		}
		ids[id] = true
	}

	if q.Len() != 10 {
		t.Errorf("Len() = %d, want 10", q.Len())
	}
}

func TestDomainQueue_Get(t *testing.T) {
	q := NewDomainQueue()
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, _ := q.Add(req)

	// Get the request
	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false for existing ID")
	}
	if got == nil {
		t.Fatal("Get() returned nil request")
	}
	if got.Cloister != "test-cloister" {
		t.Errorf("Cloister = %q, want %q", got.Cloister, "test-cloister")
	}
	if got.Project != "test-project" {
		t.Errorf("Project = %q, want %q", got.Project, "test-project")
	}
	if got.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", got.Domain, "example.com")
	}
	if len(got.Responses) == 0 {
		t.Error("Responses slice should not be empty")
	}
}

func TestDomainQueue_GetNotFound(t *testing.T) {
	q := NewDomainQueue()

	got, ok := q.Get("nonexistent")
	if ok {
		t.Error("Get() should return ok=false for nonexistent ID")
	}
	if got != nil {
		t.Errorf("Get() should return nil for nonexistent ID, got %+v", got)
	}
}

func TestDomainQueue_Remove(t *testing.T) {
	q := NewDomainQueue()

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}

	id, _ := q.Add(req)
	if q.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", q.Len())
	}

	q.Remove(id)

	if q.Len() != 0 {
		t.Errorf("Len() after Remove = %d, want 0", q.Len())
	}

	// Get should return not found
	_, ok := q.Get(id)
	if ok {
		t.Error("Get() should return ok=false after Remove")
	}
}

func TestDomainQueue_RemoveNonexistent(t *testing.T) {
	q := NewDomainQueue()

	// Should not panic
	q.Remove("nonexistent")

	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0", q.Len())
	}
}

func TestDomainQueue_List(t *testing.T) {
	q := NewDomainQueue()
	now := time.Now()
	respChan := make(chan DomainResponse, 1)

	requests := []*DomainRequest{
		{Cloister: "cloister-1", Project: "project-1", Domain: "example.com", Timestamp: now, Responses: []chan<- DomainResponse{respChan}},
		{Cloister: "cloister-2", Project: "project-2", Domain: "test.com", Timestamp: now.Add(time.Second), Responses: []chan<- DomainResponse{respChan}},
		{Cloister: "cloister-3", Project: "project-3", Domain: "demo.org", Timestamp: now.Add(2 * time.Second), Responses: []chan<- DomainResponse{respChan}},
	}

	for _, req := range requests {
		_, _ = q.Add(req)
	}

	list := q.List()
	if len(list) != 3 {
		t.Fatalf("List() returned %d items, want 3", len(list))
	}

	// Verify all items are present (order not guaranteed due to map)
	found := make(map[string]bool)
	for _, item := range list {
		found[item.Cloister] = true
		// Verify Responses slice is NOT included in List output
		if len(item.Responses) != 0 {
			t.Errorf("Responses should be empty in List() output, got %v", item.Responses)
		}
	}

	for _, req := range requests {
		if !found[req.Cloister] {
			t.Errorf("List() missing request with Cloister=%q", req.Cloister)
		}
	}
}

func TestDomainQueue_ListEmpty(t *testing.T) {
	q := NewDomainQueue()

	list := q.List()
	if list == nil {
		t.Error("List() should not return nil for empty queue")
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d items for empty queue, want 0", len(list))
	}
}

func TestDomainQueue_ListIsCopy(t *testing.T) {
	q := NewDomainQueue()

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}
	_, _ = q.Add(req)

	list := q.List()
	if len(list) != 1 {
		t.Fatalf("List() returned %d items, want 1", len(list))
	}

	// Modify the returned list
	list[0].Cloister = "modified"

	// Original should be unchanged
	got, _ := q.Get(req.ID)
	if got.Cloister != "test-cloister" {
		t.Errorf("List() returned data that modifies original: Cloister = %q", got.Cloister)
	}
}

func TestDomainQueue_ConcurrentAccess(t *testing.T) {
	q := NewDomainQueue()
	var wg sync.WaitGroup
	numGoroutines := 100
	idsPerGoroutine := 10

	// Collect all generated IDs for cleanup check
	idsChan := make(chan string, numGoroutines*idsPerGoroutine)

	// Concurrent adds - use unique token+domain combinations to avoid deduplication
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				req := &DomainRequest{
					Cloister:  "test-cloister",
					Project:   "test-project",
					Domain:    fmt.Sprintf("example%d-%d.com", n, j), // unique domain
					Timestamp: time.Now(),
				}
				id, err := q.Add(req)
				if err != nil {
					t.Errorf("Add() error = %v", err)
					return
				}
				idsChan <- id
			}
		}(i)
	}
	wg.Wait()
	close(idsChan)

	// Collect all IDs
	var allIDs []string
	for id := range idsChan {
		allIDs = append(allIDs, id)
	}

	expectedLen := numGoroutines * idsPerGoroutine
	if q.Len() != expectedLen {
		t.Errorf("Len() = %d, want %d", q.Len(), expectedLen)
	}

	// Concurrent reads and removes
	wg.Add(numGoroutines * 2)
	for i := 0; i < numGoroutines; i++ {
		// Reader goroutine
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				_ = q.List()
				_ = q.Len()
			}
		}()
		// Remover goroutine
		go func(n int) {
			defer wg.Done()
			start := n * idsPerGoroutine
			end := start + idsPerGoroutine
			if end > len(allIDs) {
				end = len(allIDs)
			}
			for j := start; j < end; j++ {
				q.Remove(allIDs[j])
			}
		}(i)
	}
	wg.Wait()

	// All should be removed
	if q.Len() != 0 {
		t.Errorf("Len() after concurrent removes = %d, want 0", q.Len())
	}
}

func TestDomainQueue_ConcurrentGetAndRemove(t *testing.T) {
	q := NewDomainQueue()

	// Add a request
	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}
	id, _ := q.Add(req)

	var wg sync.WaitGroup
	wg.Add(2)

	// Concurrent Get
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = q.Get(id)
		}
	}()

	// Concurrent Remove
	go func() {
		defer wg.Done()
		q.Remove(id)
	}()

	wg.Wait()

	// After concurrent operations, request should be removed
	_, ok := q.Get(id)
	if ok {
		t.Error("request should have been removed")
	}
}

func TestDomainQueue_ResponseChannelWorks(t *testing.T) {
	q := NewDomainQueue()
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, _ := q.Add(req)

	// Get the request and send a response
	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}

	// Send response through the channels
	expectedResp := DomainResponse{
		Status: "approved",
		Scope:  "project",
		Reason: "User approved",
	}
	for _, ch := range got.Responses {
		ch <- expectedResp
	}

	// Verify response received
	select {
	case received := <-respChan:
		if received.Status != "approved" {
			t.Errorf("Status = %q, want %q", received.Status, "approved")
		}
		if received.Scope != "project" {
			t.Errorf("Scope = %q, want %q", received.Scope, "project")
		}
		if received.Reason != "User approved" {
			t.Errorf("Reason = %q, want %q", received.Reason, "User approved")
		}
	default:
		t.Error("expected response on channel")
	}
}

func TestDomainQueue_SetEventHub(t *testing.T) {
	q := NewDomainQueue()
	hub := NewEventHub()

	// Set the event hub
	q.SetEventHub(hub)

	// Verify it's set by checking the internal state (via a goroutine race-safe approach)
	// We'll verify by adding a request and checking if the hub receives the event
	eventCh := hub.Subscribe()
	defer hub.Unsubscribe(eventCh)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}
	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should receive domain-request-added event
	select {
	case event := <-eventCh:
		if event.Type != EventDomainRequestAdded {
			t.Errorf("event.Type = %q, want %q", event.Type, EventDomainRequestAdded)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected domain-request-added event, but none received")
	}
}

func TestDomainQueue_BroadcastsOnAdd(t *testing.T) {
	q := NewDomainQueue()
	hub := NewEventHub()
	q.SetEventHub(hub)

	eventCh := hub.Subscribe()
	defer hub.Unsubscribe(eventCh)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}

	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Verify domain-request-added event is broadcast
	select {
	case event := <-eventCh:
		if event.Type != EventDomainRequestAdded {
			t.Errorf("event.Type = %q, want %q", event.Type, EventDomainRequestAdded)
		}
		// Event data should contain domain information (HTML rendered)
		if event.Data == "" {
			t.Error("event.Data should not be empty")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected domain-request-added event, but none received")
	}
}

func TestDomainQueue_BroadcastsOnTimeout(t *testing.T) {
	timeout := 50 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	hub := NewEventHub()
	q.SetEventHub(hub)

	eventCh := hub.Subscribe()
	defer hub.Unsubscribe(eventCh)

	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Drain the domain-request-added event
	select {
	case <-eventCh:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected domain-request-added event")
	}

	// Wait for timeout and verify domain-request-removed event
	select {
	case event := <-eventCh:
		if event.Type != EventDomainRequestRemoved {
			t.Errorf("event.Type = %q, want %q", event.Type, EventDomainRequestRemoved)
		}
		// Data should contain the request ID
		if event.Data == "" {
			t.Error("event.Data should not be empty")
		}
		// Check that the ID is in the JSON data
		if !strings.Contains(event.Data, id) {
			t.Errorf("event.Data %q should contain request ID %q", event.Data, id)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("expected domain-request-removed event after timeout, but none received")
	}
}

func TestDomainQueue_NoBroadcastWithoutEventHub(t *testing.T) {
	// Verify that queue operations work without an event hub (no panic)
	q := NewDomainQueue()

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}

	// Should not panic without event hub
	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1", q.Len())
	}
}

func TestDomainQueue_DenyBeforeTimeout(t *testing.T) {
	// Use a longer timeout to ensure we deny before it fires
	timeout := 500 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "malicious.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Deny immediately (simulate denial before timeout)
	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}

	// Send denial response and remove from queue
	denialResp := DomainResponse{
		Status: "denied",
		Reason: "Domain not allowed",
	}
	for _, ch := range got.Responses {
		ch <- denialResp
	}
	q.Remove(id)

	// Receive the denial response
	select {
	case resp := <-respChan:
		if resp.Status != "denied" {
			t.Errorf("Status = %q, want %q", resp.Status, "denied")
		}
		if resp.Reason != "Domain not allowed" {
			t.Errorf("Reason = %q, want %q", resp.Reason, "Domain not allowed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected denial response")
	}

	// Wait past the original timeout to ensure no timeout response is sent
	time.Sleep(600 * time.Millisecond)

	// Channel should be empty (no timeout response)
	select {
	case resp := <-respChan:
		t.Errorf("unexpected response after denial: %+v", resp)
	default:
		// Expected: no additional response
	}
}

func TestDomainQueue_ExpiresAtSet(t *testing.T) {
	timeout := 5 * time.Minute
	q := NewDomainQueueWithTimeout(timeout)

	before := time.Now()
	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	after := time.Now()

	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}

	// ExpiresAt should be approximately timeout duration from now
	expectedMin := before.Add(timeout)
	expectedMax := after.Add(timeout)

	if got.ExpiresAt.Before(expectedMin) || got.ExpiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt = %v, want between %v and %v", got.ExpiresAt, expectedMin, expectedMax)
	}
}

// Deduplication tests

func TestDomainQueue_Deduplication_SameTokenDomain(t *testing.T) {
	q := NewDomainQueue()
	respChan1 := make(chan DomainResponse, 1)
	respChan2 := make(chan DomainResponse, 1)

	// First request
	req1 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan1},
	}

	id1, err := q.Add(req1)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Second request with same token+domain should coalesce
	req2 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan2},
	}

	id2, err := q.Add(req2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should return same ID
	if id1 != id2 {
		t.Errorf("Expected same ID for deduplicated requests, got %q and %q", id1, id2)
	}

	// Queue should still have only 1 entry
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1 after deduplication", q.Len())
	}

	// The request should have both response channels
	got, ok := q.Get(id1)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}
	if len(got.Responses) != 2 {
		t.Errorf("Expected 2 response channels, got %d", len(got.Responses))
	}
}

func TestDomainQueue_Deduplication_DifferentTokens(t *testing.T) {
	q := NewDomainQueue()
	respChan1 := make(chan DomainResponse, 1)
	respChan2 := make(chan DomainResponse, 1)

	// First request with token1
	req1 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan1},
	}

	id1, err := q.Add(req1)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Second request with different token (same domain) should NOT coalesce
	req2 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-xyz",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan2},
	}

	id2, err := q.Add(req2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should have different IDs
	if id1 == id2 {
		t.Errorf("Expected different IDs for different tokens, both got %q", id1)
	}

	// Queue should have 2 entries
	if q.Len() != 2 {
		t.Errorf("Len() = %d, want 2 for different tokens", q.Len())
	}
}

func TestDomainQueue_Deduplication_BothReceiveApproval(t *testing.T) {
	q := NewDomainQueue()
	respChan1 := make(chan DomainResponse, 1)
	respChan2 := make(chan DomainResponse, 1)

	// First request
	req1 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan1},
	}

	id, err := q.Add(req1)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Second request with same token+domain
	req2 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan2},
	}

	_, err = q.Add(req2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Send approval to both channels through the queue's stored request
	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}

	approvalResp := DomainResponse{
		Status: "approved",
		Scope:  "session",
	}
	for _, ch := range got.Responses {
		ch <- approvalResp
	}
	q.Remove(id)

	// Both should receive the response
	select {
	case resp := <-respChan1:
		if resp.Status != "approved" {
			t.Errorf("respChan1: Status = %q, want %q", resp.Status, "approved")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("respChan1: expected response, timed out")
	}

	select {
	case resp := <-respChan2:
		if resp.Status != "approved" {
			t.Errorf("respChan2: Status = %q, want %q", resp.Status, "approved")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("respChan2: expected response, timed out")
	}
}

func TestDomainQueue_Deduplication_TimeoutBroadcast(t *testing.T) {
	timeout := 50 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	respChan1 := make(chan DomainResponse, 1)
	respChan2 := make(chan DomainResponse, 1)

	// First request
	req1 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan1},
	}

	_, err := q.Add(req1)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Second request with same token+domain (coalesces)
	req2 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan2},
	}

	_, err = q.Add(req2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Wait for timeout - both channels should receive timeout response
	time.Sleep(100 * time.Millisecond)

	select {
	case resp := <-respChan1:
		if resp.Status != "timeout" {
			t.Errorf("respChan1: Status = %q, want %q", resp.Status, "timeout")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("respChan1: expected timeout response, timed out waiting")
	}

	select {
	case resp := <-respChan2:
		if resp.Status != "timeout" {
			t.Errorf("respChan2: Status = %q, want %q", resp.Status, "timeout")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("respChan2: expected timeout response, timed out waiting")
	}

	// Queue should be empty
	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after timeout", q.Len())
	}
}

func TestDomainQueue_Deduplication_CleanupOnRemove(t *testing.T) {
	q := NewDomainQueue()
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Remove the request
	q.Remove(id)

	// Now add a new request with same token+domain - should create new entry, not coalesce
	respChan2 := make(chan DomainResponse, 1)
	req2 := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Token:     "token-abc",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan2},
	}

	id2, err := q.Add(req2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should have a new ID (not the old one)
	if id == id2 {
		t.Errorf("Expected new ID after removal, got same ID %q", id)
	}

	// Queue should have 1 entry
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1", q.Len())
	}
}

func TestDomainResponse_JSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		resp DomainResponse
	}{
		{
			name: "approved with scope",
			resp: DomainResponse{
				Status: "approved",
				Scope:  "project",
			},
		},
		{
			name: "denied with scope and pattern",
			resp: DomainResponse{
				Status:  "denied",
				Scope:   "global",
				Pattern: "*.example.com",
			},
		},
		{
			name: "timeout",
			resp: DomainResponse{
				Status: "timeout",
				Reason: "request timed out",
			},
		},
		{
			name: "approved with all fields",
			resp: DomainResponse{
				Status:           "approved",
				Scope:            "session",
				Pattern:          "*.cdn.example.com",
				PersistenceError: "write failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			var got DomainResponse
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if got != tt.resp {
				t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, tt.resp)
			}
		})
	}
}
