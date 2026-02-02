package approval

import (
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

func TestNewDomainQueue(t *testing.T) {
	q := NewDomainQueue()
	if q == nil {
		t.Fatal("NewDomainQueue() returned nil")
	}
	if q.timeout != DefaultDomainTimeout {
		t.Errorf("timeout = %v, want %v", q.timeout, DefaultDomainTimeout)
	}
	if q.Len() != 0 {
		t.Errorf("new queue should be empty, got len=%d", q.Len())
	}
}

func TestDomainQueue_Add(t *testing.T) {
	q := NewDomainQueue()
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  respChan,
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

	// Expires should be set
	if req.Expires.IsZero() {
		t.Error("Expires should not be zero")
	}
}

func TestDomainQueue_AddCoalescesDuplicates(t *testing.T) {
	q := NewDomainQueue()
	respChan1 := make(chan DomainResponse, 1)
	respChan2 := make(chan DomainResponse, 1)

	req1 := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  respChan1,
	}

	req2 := &DomainRequest{
		Token:     "test-token", // Same token
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com", // Same domain
		Timestamp: time.Now(),
		Response:  respChan2,
	}

	id1, err := q.Add(req1)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	id2, err := q.Add(req2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should return the same ID (coalesced)
	if id1 != id2 {
		t.Errorf("expected same ID for duplicate requests, got %q and %q", id1, id2)
	}

	// Queue should only have 1 request
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1", q.Len())
	}
}

func TestDomainQueue_AddDifferentTokensNotCoalesced(t *testing.T) {
	q := NewDomainQueue()

	req1 := &DomainRequest{
		Token:     "token-1",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}

	req2 := &DomainRequest{
		Token:     "token-2", // Different token
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com", // Same domain
		Timestamp: time.Now(),
	}

	id1, _ := q.Add(req1)
	id2, _ := q.Add(req2)

	// Should have different IDs
	if id1 == id2 {
		t.Errorf("expected different IDs for different tokens, got same %q", id1)
	}

	// Queue should have 2 requests
	if q.Len() != 2 {
		t.Errorf("Len() = %d, want 2", q.Len())
	}
}

func TestDomainQueue_Get(t *testing.T) {
	q := NewDomainQueue()
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  respChan,
	}

	id, _ := q.Add(req)

	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false for existing ID")
	}
	if got == nil {
		t.Fatal("Get() returned nil request")
	}
	if got.Token != "test-token" {
		t.Errorf("Token = %q, want %q", got.Token, "test-token")
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
	if got.Response == nil {
		t.Error("Response channel should not be nil")
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
		Token:     "test-token",
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

func TestDomainQueue_RemoveAllowsReAdd(t *testing.T) {
	q := NewDomainQueue()

	req := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}

	id1, _ := q.Add(req)
	q.Remove(id1)

	// Should be able to add same token+domain again
	req2 := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}

	id2, err := q.Add(req2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should get a new ID
	if id1 == id2 {
		t.Errorf("expected different ID after remove and re-add, got same %q", id1)
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
		{Token: "token-1", Cloister: "cloister-1", Project: "project-1", Domain: "example.com", Timestamp: now, Response: respChan},
		{Token: "token-2", Cloister: "cloister-2", Project: "project-2", Domain: "test.com", Timestamp: now.Add(time.Second), Response: respChan},
		{Token: "token-3", Cloister: "cloister-3", Project: "project-3", Domain: "api.example.com", Timestamp: now.Add(2 * time.Second), Response: respChan},
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
		found[item.Domain] = true
		// Verify Response channel is NOT included in List output
		if item.Response != nil {
			t.Errorf("Response channel should be nil in List() output, got %v", item.Response)
		}
	}

	for _, req := range requests {
		if !found[req.Domain] {
			t.Errorf("List() missing request with Domain=%q", req.Domain)
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
		Token:     "test-token",
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
	list[0].Domain = "modified.com"

	// Original should be unchanged
	got, _ := q.Get(req.ID)
	if got.Domain != "example.com" {
		t.Errorf("List() returned data that modifies original: Domain = %q", got.Domain)
	}
}

func TestDomainQueue_TimeoutSendsResponse(t *testing.T) {
	timeout := 50 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  respChan,
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
	timeout := 500 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)
	respChan := make(chan DomainResponse, 1)

	req := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  respChan,
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Approve immediately
	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}

	approvalResp := DomainResponse{
		Status: "approved",
		Scope:  ScopeSession,
	}
	got.Response <- approvalResp
	q.Remove(id)

	// Receive the approval response
	select {
	case resp := <-respChan:
		if resp.Status != "approved" {
			t.Errorf("Status = %q, want %q", resp.Status, "approved")
		}
		if resp.Scope != ScopeSession {
			t.Errorf("Scope = %q, want %q", resp.Scope, ScopeSession)
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
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  respChan,
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
	timeout := 50 * time.Millisecond
	q := NewDomainQueueWithTimeout(timeout)

	req := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  nil, // nil channel
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

func TestDomainQueue_ConcurrentAccess(t *testing.T) {
	q := NewDomainQueue()
	var wg sync.WaitGroup
	numGoroutines := 100
	idsPerGoroutine := 10

	// Collect all generated IDs for cleanup check
	idsChan := make(chan string, numGoroutines*idsPerGoroutine)

	// Concurrent adds
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				req := &DomainRequest{
					Token:     "token-" + string(rune('a'+n)),
					Cloister:  "test-cloister",
					Project:   "test-project",
					Domain:    "example" + string(rune('0'+j)) + ".com",
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

func TestDomainQueue_SetEventHub(t *testing.T) {
	q := NewDomainQueue()
	hub := NewEventHub()

	// Set the event hub
	q.SetEventHub(hub)

	eventCh := hub.Subscribe()
	defer hub.Unsubscribe(eventCh)

	req := &DomainRequest{
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
	}
	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should receive domain-added event
	select {
	case event := <-eventCh:
		if event.Type != EventDomainAdded {
			t.Errorf("event.Type = %q, want %q", event.Type, EventDomainAdded)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected domain-added event, but none received")
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
		Token:     "test-token",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Response:  respChan,
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Drain the domain-added event
	select {
	case <-eventCh:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected domain-added event")
	}

	// Wait for timeout and verify domain-removed event
	select {
	case event := <-eventCh:
		if event.Type != EventDomainRemoved {
			t.Errorf("event.Type = %q, want %q", event.Type, EventDomainRemoved)
		}
		// Data should contain the request ID
		if event.Data == "" {
			t.Error("event.Data should not be empty")
		}
		if !contains(event.Data, id) {
			t.Errorf("event.Data %q should contain request ID %q", event.Data, id)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("expected domain-removed event after timeout, but none received")
	}
}

func TestDomainQueue_NoBroadcastWithoutEventHub(t *testing.T) {
	q := NewDomainQueue()

	req := &DomainRequest{
		Token:     "test-token",
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

func TestDomainScope_Constants(t *testing.T) {
	// Verify scope constants have expected values
	if ScopeSession != "session" {
		t.Errorf("ScopeSession = %q, want %q", ScopeSession, "session")
	}
	if ScopeProject != "project" {
		t.Errorf("ScopeProject = %q, want %q", ScopeProject, "project")
	}
	if ScopeGlobal != "global" {
		t.Errorf("ScopeGlobal = %q, want %q", ScopeGlobal, "global")
	}
}

func TestDomainResponse_Fields(t *testing.T) {
	resp := DomainResponse{
		Status: "approved",
		Scope:  ScopeProject,
		Reason: "",
	}

	if resp.Status != "approved" {
		t.Errorf("Status = %q, want %q", resp.Status, "approved")
	}
	if resp.Scope != ScopeProject {
		t.Errorf("Scope = %q, want %q", resp.Scope, ScopeProject)
	}

	// Denied response
	resp = DomainResponse{
		Status: "denied",
		Reason: "Domain not allowed",
	}
	if resp.Status != "denied" {
		t.Errorf("Status = %q, want %q", resp.Status, "denied")
	}
	if resp.Reason != "Domain not allowed" {
		t.Errorf("Reason = %q, want %q", resp.Reason, "Domain not allowed")
	}
}
