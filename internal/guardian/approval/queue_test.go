package approval

import (
	"sync"
	"testing"
	"time"
)

func TestNewQueueWithTimeout(t *testing.T) {
	timeout := 30 * time.Second
	q := NewQueueWithTimeout(timeout)
	if q == nil {
		t.Fatal("NewQueueWithTimeout() returned nil")
	}
	if q.timeout != timeout {
		t.Errorf("timeout = %v, want %v", q.timeout, timeout)
	}
	if q.Len() != 0 {
		t.Errorf("new queue should be empty, got len=%d", q.Len())
	}
}

func TestQueue_TimeoutSendsResponse(t *testing.T) {
	// Use a very short timeout for testing
	timeout := 50 * time.Millisecond
	q := NewQueueWithTimeout(timeout)
	respChan := make(chan Response, 1)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Agent:     "claude",
		Cmd:       "docker compose up -d",
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

func TestQueue_ApproveBeforeTimeout(t *testing.T) {
	// Use a longer timeout to ensure we approve before it fires
	timeout := 500 * time.Millisecond
	q := NewQueueWithTimeout(timeout)
	respChan := make(chan Response, 1)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Agent:     "claude",
		Cmd:       "docker compose ps",
		Timestamp: time.Now(),
		Response:  respChan,
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
	approvalResp := Response{
		Status:   "approved",
		ExitCode: 0,
		Stdout:   "container running",
	}
	got.Response <- approvalResp
	q.Remove(id)

	// Receive the approval response
	select {
	case resp := <-respChan:
		if resp.Status != "approved" {
			t.Errorf("Status = %q, want %q", resp.Status, "approved")
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

func TestQueue_RemoveCancelsTimeout(t *testing.T) {
	timeout := 100 * time.Millisecond
	q := NewQueueWithTimeout(timeout)
	respChan := make(chan Response, 1)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
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

func TestQueue_TimeoutWithNilChannel(t *testing.T) {
	// Verify that timeout doesn't panic with nil Response channel
	timeout := 50 * time.Millisecond
	q := NewQueueWithTimeout(timeout)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
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

func TestNewQueue(t *testing.T) {
	q := NewQueue()
	if q == nil {
		t.Fatal("NewQueue() returned nil")
	}
	if q.Len() != 0 {
		t.Errorf("new queue should be empty, got len=%d", q.Len())
	}
}

func TestQueue_Add(t *testing.T) {
	q := NewQueue()
	respChan := make(chan Response, 1)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Agent:     "claude",
		Cmd:       "docker compose ps",
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
}

func TestQueue_AddMultiple(t *testing.T) {
	q := NewQueue()

	// Add multiple requests and verify unique IDs
	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req := &PendingRequest{
			Cloister:  "test-cloister",
			Project:   "test-project",
			Cmd:       "test command",
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

func TestQueue_Get(t *testing.T) {
	q := NewQueue()
	respChan := make(chan Response, 1)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Agent:     "claude",
		Cmd:       "docker compose ps",
		Timestamp: time.Now(),
		Response:  respChan,
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
	if got.Agent != "claude" {
		t.Errorf("Agent = %q, want %q", got.Agent, "claude")
	}
	if got.Cmd != "docker compose ps" {
		t.Errorf("Cmd = %q, want %q", got.Cmd, "docker compose ps")
	}
	if got.Response == nil {
		t.Error("Response channel should not be nil")
	}
}

func TestQueue_GetNotFound(t *testing.T) {
	q := NewQueue()

	got, ok := q.Get("nonexistent")
	if ok {
		t.Error("Get() should return ok=false for nonexistent ID")
	}
	if got != nil {
		t.Errorf("Get() should return nil for nonexistent ID, got %+v", got)
	}
}

func TestQueue_Remove(t *testing.T) {
	q := NewQueue()

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
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

func TestQueue_RemoveNonexistent(t *testing.T) {
	q := NewQueue()

	// Should not panic
	q.Remove("nonexistent")

	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0", q.Len())
	}
}

func TestQueue_List(t *testing.T) {
	q := NewQueue()
	now := time.Now()
	respChan := make(chan Response, 1)

	requests := []*PendingRequest{
		{Cloister: "cloister-1", Project: "project-1", Agent: "claude", Cmd: "cmd1", Timestamp: now, Response: respChan},
		{Cloister: "cloister-2", Project: "project-2", Agent: "codex", Cmd: "cmd2", Timestamp: now.Add(time.Second), Response: respChan},
		{Cloister: "cloister-3", Project: "project-3", Agent: "gemini", Cmd: "cmd3", Timestamp: now.Add(2 * time.Second), Response: respChan},
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
		// Verify Response channel is NOT included in List output
		if item.Response != nil {
			t.Errorf("Response channel should be nil in List() output, got %v", item.Response)
		}
	}

	for _, req := range requests {
		if !found[req.Cloister] {
			t.Errorf("List() missing request with Cloister=%q", req.Cloister)
		}
	}
}

func TestQueue_ListEmpty(t *testing.T) {
	q := NewQueue()

	list := q.List()
	if list == nil {
		t.Error("List() should not return nil for empty queue")
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d items for empty queue, want 0", len(list))
	}
}

func TestQueue_ListIsCopy(t *testing.T) {
	q := NewQueue()

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
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

func TestQueue_ConcurrentAccess(t *testing.T) {
	q := NewQueue()
	var wg sync.WaitGroup
	numGoroutines := 100
	idsPerGoroutine := 10

	// Collect all generated IDs for cleanup check
	idsChan := make(chan string, numGoroutines*idsPerGoroutine)

	// Concurrent adds
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				req := &PendingRequest{
					Cloister:  "test-cloister",
					Project:   "test-project",
					Cmd:       "test command",
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

func TestQueue_ConcurrentGetAndRemove(t *testing.T) {
	q := NewQueue()

	// Add a request
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
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

func TestGenerateID_Length(t *testing.T) {
	// Generate multiple IDs and verify format
	for i := 0; i < 100; i++ {
		id, err := generateID()
		if err != nil {
			t.Fatalf("generateID() error = %v", err)
		}
		if len(id) != 16 {
			t.Errorf("ID length = %d, want 16", len(id))
		}
		// Verify it's valid hex
		for _, c := range id {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Errorf("ID contains invalid character: %c", c)
			}
		}
	}
}

func TestGenerateID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id, err := generateID()
		if err != nil {
			t.Fatalf("generateID() error = %v", err)
		}
		if ids[id] {
			t.Errorf("duplicate ID generated: %q", id)
		}
		ids[id] = true
	}
}

func TestQueue_ResponseChannelWorks(t *testing.T) {
	q := NewQueue()
	respChan := make(chan Response, 1)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
		Timestamp: time.Now(),
		Response:  respChan,
	}

	id, _ := q.Add(req)

	// Get the request and send a response
	got, ok := q.Get(id)
	if !ok {
		t.Fatal("Get() returned ok=false")
	}

	// Send response through the channel
	expectedResp := Response{
		Status:   "approved",
		ExitCode: 0,
		Stdout:   "output",
	}
	got.Response <- expectedResp

	// Verify response received
	select {
	case received := <-respChan:
		if received.Status != "approved" {
			t.Errorf("Status = %q, want %q", received.Status, "approved")
		}
		if received.Stdout != "output" {
			t.Errorf("Stdout = %q, want %q", received.Stdout, "output")
		}
	default:
		t.Error("expected response on channel")
	}
}

func TestQueue_SetEventHub(t *testing.T) {
	q := NewQueue()
	hub := NewEventHub()

	// Set the event hub
	q.SetEventHub(hub)

	// Verify it's set by checking the internal state (via a goroutine race-safe approach)
	// We'll verify by adding a request and checking if the hub receives the event
	eventCh := hub.Subscribe()
	defer hub.Unsubscribe(eventCh)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
		Timestamp: time.Now(),
	}
	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Should receive request-added event
	select {
	case event := <-eventCh:
		if event.Type != EventRequestAdded {
			t.Errorf("event.Type = %q, want %q", event.Type, EventRequestAdded)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected request-added event, but none received")
	}
}

func TestQueue_BroadcastsOnAdd(t *testing.T) {
	q := NewQueue()
	hub := NewEventHub()
	q.SetEventHub(hub)

	eventCh := hub.Subscribe()
	defer hub.Unsubscribe(eventCh)

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Agent:     "claude",
		Cmd:       "docker compose up -d",
		Timestamp: time.Now(),
	}

	_, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Verify request-added event is broadcast
	select {
	case event := <-eventCh:
		if event.Type != EventRequestAdded {
			t.Errorf("event.Type = %q, want %q", event.Type, EventRequestAdded)
		}
		// Event data should contain the command (HTML rendered)
		if event.Data == "" {
			t.Error("event.Data should not be empty")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected request-added event, but none received")
	}
}

func TestQueue_BroadcastsOnTimeout(t *testing.T) {
	timeout := 50 * time.Millisecond
	q := NewQueueWithTimeout(timeout)
	hub := NewEventHub()
	q.SetEventHub(hub)

	eventCh := hub.Subscribe()
	defer hub.Unsubscribe(eventCh)

	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
		Timestamp: time.Now(),
		Response:  respChan,
	}

	id, err := q.Add(req)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Drain the request-added event
	select {
	case <-eventCh:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected request-added event")
	}

	// Wait for timeout and verify request-removed event
	select {
	case event := <-eventCh:
		if event.Type != EventRequestRemoved {
			t.Errorf("event.Type = %q, want %q", event.Type, EventRequestRemoved)
		}
		// Data should contain the request ID
		if event.Data == "" {
			t.Error("event.Data should not be empty")
		}
		// Check that the ID is in the JSON data
		if !contains(event.Data, id) {
			t.Errorf("event.Data %q should contain request ID %q", event.Data, id)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("expected request-removed event after timeout, but none received")
	}
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestQueue_NoBroadcastWithoutEventHub(t *testing.T) {
	// Verify that queue operations work without an event hub (no panic)
	q := NewQueue()

	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "test command",
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
