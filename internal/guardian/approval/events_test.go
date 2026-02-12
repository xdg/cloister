package approval

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEventHub_SubscribeUnsubscribe(t *testing.T) {
	hub := NewEventHub()

	// Subscribe should return a channel
	ch := hub.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	// Client count should be 1
	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	// Unsubscribe should remove the client
	hub.Unsubscribe(ch)
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after unsubscribe, got %d", hub.ClientCount())
	}
}

func TestEventHub_Broadcast(t *testing.T) {
	hub := NewEventHub()

	ch1 := hub.Subscribe()
	ch2 := hub.Subscribe()
	defer hub.Unsubscribe(ch1)
	defer hub.Unsubscribe(ch2)

	event := Event{Type: EventRequestAdded, Data: "test data"}
	hub.Broadcast(event)

	// Both clients should receive the event
	select {
	case received := <-ch1:
		if received.Type != EventRequestAdded {
			t.Errorf("ch1: expected type %s, got %s", EventRequestAdded, received.Type)
		}
		if received.Data != "test data" {
			t.Errorf("ch1: expected data 'test data', got %q", received.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1: timeout waiting for event")
	}

	select {
	case received := <-ch2:
		if received.Type != EventRequestAdded {
			t.Errorf("ch2: expected type %s, got %s", EventRequestAdded, received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2: timeout waiting for event")
	}
}

func TestEventHub_BroadcastDropsWhenFull(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	// Fill the buffer (default size is 16)
	for i := 0; i < 20; i++ {
		hub.Broadcast(Event{Type: EventRequestAdded, Data: "test"})
	}

	// Should not panic or block - events beyond buffer size are dropped
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count > 16 {
		t.Errorf("expected at most 16 events (buffer size), got %d", count)
	}
}

func TestEventHub_Close(t *testing.T) {
	hub := NewEventHub()

	ch1 := hub.Subscribe()
	ch2 := hub.Subscribe()

	if hub.ClientCount() != 2 {
		t.Errorf("expected 2 clients, got %d", hub.ClientCount())
	}

	hub.Close()

	// Channels should be closed
	_, open := <-ch1
	if open {
		t.Error("ch1 should be closed")
	}
	_, open = <-ch2
	if open {
		t.Error("ch2 should be closed")
	}

	// Client count should be 0
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after close, got %d", hub.ClientCount())
	}

	// Subscribe after close should return nil
	ch3 := hub.Subscribe()
	if ch3 != nil {
		t.Error("Subscribe after close should return nil")
	}
}

func TestEventHub_ConcurrentAccess(t *testing.T) {
	hub := NewEventHub()
	var wg sync.WaitGroup

	// Spawn multiple goroutines doing concurrent operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := hub.Subscribe()
			if ch == nil {
				return
			}
			hub.Broadcast(Event{Type: EventRequestAdded, Data: "test"})
			hub.Unsubscribe(ch)
		}()
	}

	wg.Wait()
	// Should not panic or deadlock
}

func TestEventHub_BroadcastRequestAdded(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	req := templateRequest{
		ID:        "test123",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Agent:     "claude",
		Cmd:       "docker compose up",
		Timestamp: "2024-01-15T14:32:05Z",
	}

	hub.BroadcastRequestAdded(req)

	select {
	case event := <-ch:
		if event.Type != EventRequestAdded {
			t.Errorf("expected type %s, got %s", EventRequestAdded, event.Type)
		}
		// Data should contain rendered HTML
		if !strings.Contains(event.Data, "test-cloister") {
			t.Errorf("expected data to contain cloister name, got %q", event.Data)
		}
		if !strings.Contains(event.Data, "docker compose up") {
			t.Errorf("expected data to contain command, got %q", event.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestEventHub_BroadcastRequestRemoved(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	hub.BroadcastRequestRemoved("abc123")

	select {
	case event := <-ch:
		if event.Type != EventRequestRemoved {
			t.Errorf("expected type %s, got %s", EventRequestRemoved, event.Type)
		}
		if !strings.Contains(event.Data, "abc123") {
			t.Errorf("expected data to contain ID, got %q", event.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestFormatSSE(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected string
	}{
		{
			name:     "request-added event",
			event:    Event{Type: EventRequestAdded, Data: `{"id":"123"}`},
			expected: "event: request-added\ndata: {\"id\":\"123\"}\n\n",
		},
		{
			name:     "request-removed event",
			event:    Event{Type: EventRequestRemoved, Data: `{"id":"abc"}`},
			expected: "event: request-removed\ndata: {\"id\":\"abc\"}\n\n",
		},
		{
			name:     "heartbeat event",
			event:    Event{Type: EventHeartbeat, Data: ""},
			expected: "event: heartbeat\ndata: \n\n",
		},
		{
			name:     "multiline data",
			event:    Event{Type: EventRequestAdded, Data: "line1\nline2"},
			expected: "event: request-added\ndata: line1\ndata: line2\n\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatSSE(tc.event)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestServer_HandleEvents_Headers(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	// Create a request that will cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	server.handleEvents(rr, req)

	// Check SSE headers were set
	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", rr.Header().Get("Content-Type"))
	}
	if rr.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control 'no-cache', got %q", rr.Header().Get("Cache-Control"))
	}
	if rr.Header().Get("Connection") != "keep-alive" {
		t.Errorf("expected Connection 'keep-alive', got %q", rr.Header().Get("Connection"))
	}
	if rr.Header().Get("X-Accel-Buffering") != "no" {
		t.Errorf("expected X-Accel-Buffering 'no', got %q", rr.Header().Get("X-Accel-Buffering"))
	}
}

func TestServer_HandleEvents_ReceivesEvents(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)
	server.Addr = "127.0.0.1:0"

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	baseURL := "http://" + server.ListenAddr()

	// Create HTTP client with context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request to /events failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Verify Content-Type header
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", resp.Header.Get("Content-Type"))
	}

	// Broadcast an event
	server.Events.BroadcastRequestRemoved("test123")

	// Read the event from the stream
	reader := bufio.NewReader(resp.Body)
	eventLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read event line: %v", err)
	}
	if !strings.Contains(eventLine, "event: request-removed") {
		t.Errorf("expected event line to contain 'event: request-removed', got %q", eventLine)
	}

	dataLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read data line: %v", err)
	}
	if !strings.Contains(dataLine, "test123") {
		t.Errorf("expected data line to contain 'test123', got %q", dataLine)
	}
}

func TestServer_HandleEvents_ClientDisconnect(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)
	server.Addr = "127.0.0.1:0"

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	baseURL := "http://" + server.ListenAddr()

	// Create HTTP client with context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/events", nil)

	// Start the request in a goroutine
	done := make(chan struct{})
	connected := make(chan struct{})
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			// Signal that we're connected and read at least the headers
			close(connected)
			// Try to read from body to keep connection open
			buf := make([]byte, 1)
			_, _ = resp.Body.Read(buf)
			_ = resp.Body.Close()
		}
		close(done)
	}()

	// Wait for connection to establish
	select {
	case <-connected:
		// Connected
	case <-done:
		// Request completed already (might be an error)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for connection")
	}

	// Give a little more time for the subscribe to complete
	time.Sleep(100 * time.Millisecond)

	// Verify client is connected
	clientCount := server.Events.ClientCount()
	if clientCount != 1 {
		t.Errorf("expected 1 connected client, got %d", clientCount)
	}

	// Cancel the context to disconnect
	cancel()

	// Wait for handler to clean up
	select {
	case <-done:
		// Request finished
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for request to finish")
	}

	// Give the server time to unsubscribe
	time.Sleep(100 * time.Millisecond)

	// Client should be disconnected
	if server.Events.ClientCount() != 0 {
		t.Errorf("expected 0 connected clients after disconnect, got %d", server.Events.ClientCount())
	}
}

func TestServer_HandleEvents_ServerShutdown(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	// Test that Events.Close returns nil for Subscribe
	server.Events.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	server.handleEvents(rr, req)

	// Should return 503 Service Unavailable
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestEventHub_BroadcastDomainRequestAdded(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	req := &DomainRequest{
		ID:        "domain123",
		Domain:    "example.com",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Timestamp: time.Date(2024, 1, 15, 14, 32, 5, 0, time.UTC),
	}

	hub.BroadcastDomainRequestAdded(req)

	select {
	case event := <-ch:
		if event.Type != EventDomainRequestAdded {
			t.Errorf("expected type %s, got %s", EventDomainRequestAdded, event.Type)
		}
		// Data should contain rendered HTML
		if !strings.Contains(event.Data, "test-cloister") {
			t.Errorf("expected data to contain cloister name, got %q", event.Data)
		}
		if !strings.Contains(event.Data, "example.com") {
			t.Errorf("expected data to contain domain, got %q", event.Data)
		}
		if !strings.Contains(event.Data, "test-project") {
			t.Errorf("expected data to contain project, got %q", event.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestEventHub_BroadcastDomainRequestRemoved(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	hub.BroadcastDomainRequestRemoved("domain456")

	select {
	case event := <-ch:
		if event.Type != EventDomainRequestRemoved {
			t.Errorf("expected type %s, got %s", EventDomainRequestRemoved, event.Type)
		}
		// Data should be valid JSON with ID field
		var data RemovedEventData
		if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
			t.Fatalf("failed to parse event data as JSON: %v", err)
		}
		if data.ID != "domain456" {
			t.Errorf("expected id 'domain456', got %q", data.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

// TestDomainToWildcard_StripsPort verifies that domainToWildcard strips the
// port before constructing the wildcard. CONNECT requests include port
// (e.g. "api.example.com:443"), and the wildcard should be "*.example.com"
// not "*.example.com:443".
func TestDomainToWildcard_StripsPort(t *testing.T) {
	tests := []struct {
		domain   string
		expected string
	}{
		{"api.example.com:443", "*.example.com"},
		{"cdn.example.com:8443", "*.example.com"},
		{"api.example.com", "*.example.com"}, // no port, unchanged behavior
		{"example.com:443", ""},              // too few components even without port
		{"a.b.example.com:443", "*.b.example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			got := domainToWildcard(tc.domain)
			if got != tc.expected {
				t.Errorf("domainToWildcard(%q) = %q, want %q", tc.domain, got, tc.expected)
			}
		})
	}
}

func TestFormatSSE_DomainEvents(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected string
	}{
		{
			name:     "domain-request-added event",
			event:    Event{Type: EventDomainRequestAdded, Data: `{"id":"123","domain":"example.com"}`},
			expected: "event: domain-request-added\ndata: {\"id\":\"123\",\"domain\":\"example.com\"}\n\n",
		},
		{
			name:     "domain-request-removed event",
			event:    Event{Type: EventDomainRequestRemoved, Data: `{"id":"abc"}`},
			expected: "event: domain-request-removed\ndata: {\"id\":\"abc\"}\n\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatSSE(tc.event)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
