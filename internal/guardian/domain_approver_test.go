package guardian

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/audit"
	"github.com/xdg/cloister/internal/guardian/approval"
)

// mockDecisionRecorder implements DecisionRecorder for testing.
type mockDecisionRecorder struct {
	mu    sync.Mutex
	calls []RecordDecisionParams
}

func newMockDecisionRecorder() *mockDecisionRecorder {
	return &mockDecisionRecorder{}
}

func (m *mockDecisionRecorder) RecordDecision(p RecordDecisionParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, p)
	return nil
}

func (m *mockDecisionRecorder) getCalls() []RecordDecisionParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]RecordDecisionParams, len(m.calls))
	copy(result, m.calls)
	return result
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
		errMsg  string // substring that should appear in error message
	}{
		// Valid domains
		{
			name:    "simple domain",
			domain:  "example.com",
			wantErr: false,
		},
		{
			name:    "subdomain",
			domain:  "api.example.com",
			wantErr: false,
		},
		{
			name:    "domain with port 443",
			domain:  "example.com:443",
			wantErr: false,
		},
		{
			name:    "domain with port 80",
			domain:  "example.com:80",
			wantErr: false,
		},
		{
			name:    "domain with port 8080",
			domain:  "localhost:8080",
			wantErr: false,
		},
		{
			name:    "domain with port 8443",
			domain:  "api.example.com:8443",
			wantErr: false,
		},
		{
			name:    "localhost",
			domain:  "localhost",
			wantErr: false,
		},
		{
			name:    "IPv4 address",
			domain:  "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "IPv4 with port",
			domain:  "192.168.1.1:443",
			wantErr: false,
		},
		{
			name:    "single-label hostname",
			domain:  "myserver",
			wantErr: false,
		},
		{
			name:    "high port allowed",
			domain:  "example.com:9999",
			wantErr: false,
		},
		{
			name:    "random high port for dev server",
			domain:  "localhost:50123",
			wantErr: false,
		},

		// Invalid: scheme prefixes
		{
			name:    "http scheme",
			domain:  "http://example.com",
			wantErr: true,
			errMsg:  "scheme prefix",
		},
		{
			name:    "https scheme",
			domain:  "https://api.example.com",
			wantErr: true,
			errMsg:  "scheme prefix",
		},
		{
			name:    "ftp scheme",
			domain:  "ftp://files.example.com",
			wantErr: true,
			errMsg:  "scheme prefix",
		},

		// Invalid: non-HTTP ports (well-known services)
		{
			name:    "SSH port",
			domain:  "example.com:22",
			wantErr: true,
			errMsg:  "port 22 not allowed",
		},
		{
			name:    "MySQL port",
			domain:  "db.example.com:3306",
			wantErr: true,
			errMsg:  "port 3306 not allowed",
		},
		{
			name:    "SMTP port",
			domain:  "mail.example.com:25",
			wantErr: true,
			errMsg:  "port 25 not allowed",
		},
		{
			name:    "PostgreSQL port",
			domain:  "db.example.com:5432",
			wantErr: true,
			errMsg:  "port 5432 not allowed",
		},
		{
			name:    "Redis port",
			domain:  "cache.example.com:6379",
			wantErr: true,
			errMsg:  "port 6379 not allowed",
		},

		// Invalid: empty or malformed
		{
			name:    "empty string",
			domain:  "",
			wantErr: true,
			errMsg:  "empty",
		},
		{
			name:    "only port",
			domain:  ":443",
			wantErr: true,
			errMsg:  "hostname is empty",
		},
		{
			name:    "invalid port non-numeric",
			domain:  "example.com:abc",
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name:    "port 0",
			domain:  "example.com:0",
			wantErr: true,
			errMsg:  "out of valid range",
		},
		{
			name:    "port too high",
			domain:  "example.com:65536",
			wantErr: true,
			errMsg:  "out of valid range",
		},
		{
			name:    "negative port",
			domain:  "example.com:-1",
			wantErr: true,
			errMsg:  "out of valid range",
		},

		// Invalid: URL-like characters in hostname
		{
			name:    "slash in hostname",
			domain:  "example.com/path",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "query string",
			domain:  "example.com?query=1",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "space in hostname",
			domain:  "example .com",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "at sign",
			domain:  "user@example.com",
			wantErr: true,
			errMsg:  "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomain(tt.domain)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateDomain(%q) expected error, got nil", tt.domain)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateDomain(%q) error = %q, want error containing %q", tt.domain, err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("ValidateDomain(%q) unexpected error: %v", tt.domain, err)
			}
		})
	}
}

// contains checks if s contains substr (case-sensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(s != "" && substr != "" && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDomainApproverImpl_RequestApproval_Timeout(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(50 * time.Millisecond)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request and wait for timeout
	result, err := approver.RequestApproval("test-project", "test-cloister", "example.com:443", "test-token")

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for timeout, got true")
	}
}

func TestDomainApproverImpl_RequestApproval_Denied(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Verify port was stripped from queued domain
	if req.Domain != "example.com" {
		t.Errorf("Queue domain = %q, want %q (port should be stripped)", req.Domain, "example.com")
	}

	// Send denial
	req.Responses[0] <- approval.DomainResponse{
		Status: "denied",
		Reason: "test denial",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}
}

func TestDomainApproverImpl_RequestApproval_SessionScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and approve with session scope
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Verify port was stripped from queued domain
	if req.Domain != "example.com" {
		t.Errorf("Queue domain = %q, want %q (port should be stripped)", req.Domain, "example.com")
	}

	// Send approval with session scope
	req.Responses[0] <- approval.DomainResponse{
		Status: "approved",
		Scope:  "session",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if !result.Approved {
		t.Errorf("Expected Approved=true for approval, got false")
	}
	if result.Scope != "session" {
		t.Errorf("Expected Scope=session, got %s", result.Scope)
	}

	// Verify recorder was called with correct params
	calls := recorder.getCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 recorder call, got %d", len(calls))
	}
	call := calls[0]
	if call.Token != "test-token" {
		t.Errorf("Expected Token=test-token, got %s", call.Token)
	}
	if call.Project != "test-project" {
		t.Errorf("Expected Project=test-project, got %s", call.Project)
	}
	if call.Domain != "example.com" {
		t.Errorf("Expected Domain=example.com, got %s", call.Domain)
	}
	if call.Scope != ScopeSession {
		t.Errorf("Expected Scope=session, got %s", call.Scope)
	}
	if !call.Allowed {
		t.Errorf("Expected Allowed=true, got false")
	}
}

func TestDomainApproverImpl_RequestApproval_ProjectScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and approve with project scope
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send approval with project scope
	req.Responses[0] <- approval.DomainResponse{
		Status: "approved",
		Scope:  "project",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if !result.Approved {
		t.Errorf("Expected Approved=true for approval, got false")
	}
	if result.Scope != "project" {
		t.Errorf("Expected Scope=project, got %s", result.Scope)
	}

	// For project scope, recorder should NOT be called (ConfigPersister handles it)
	calls := recorder.getCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 recorder calls for project scope approval, got %d", len(calls))
	}
}

func TestDomainApproverImpl_RequestApproval_GlobalScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and approve with global scope
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send approval with global scope
	req.Responses[0] <- approval.DomainResponse{
		Status: "approved",
		Scope:  "global",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if !result.Approved {
		t.Errorf("Expected Approved=true for approval, got false")
	}
	if result.Scope != "global" {
		t.Errorf("Expected Scope=global, got %s", result.Scope)
	}

	// For global scope, recorder should NOT be called (ConfigPersister handles it)
	calls := recorder.getCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 recorder calls for global scope approval, got %d", len(calls))
	}
}

func TestRequestApproval_DenialSessionScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "evil.example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it with session scope
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send denial with session scope
	req.Responses[0] <- approval.DomainResponse{
		Status: "denied",
		Scope:  "session",
		Reason: "test denial",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}

	// Verify recorder was called with session denial
	calls := recorder.getCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 recorder call, got %d", len(calls))
	}
	call := calls[0]
	if call.Token != "test-token" {
		t.Errorf("Expected Token=test-token, got %s", call.Token)
	}
	if call.Domain != "evil.example.com" {
		t.Errorf("Expected Domain=evil.example.com, got %s", call.Domain)
	}
	if call.Scope != ScopeSession {
		t.Errorf("Expected Scope=session, got %s", call.Scope)
	}
	if call.Allowed {
		t.Errorf("Expected Allowed=false, got true")
	}
}

func TestRequestApproval_DenialProjectScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "evil.example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it with project scope
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send denial with project scope (no wildcard)
	req.Responses[0] <- approval.DomainResponse{
		Status: "denied",
		Scope:  "project",
		Reason: "test denial",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}

	// Verify recorder was called with project denial
	calls := recorder.getCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 recorder call, got %d", len(calls))
	}
	call := calls[0]
	if call.Project != "test-project" {
		t.Errorf("Expected Project=test-project, got %s", call.Project)
	}
	if call.Domain != "evil.example.com" {
		t.Errorf("Expected Domain=evil.example.com, got %s", call.Domain)
	}
	if call.Scope != ScopeProject {
		t.Errorf("Expected Scope=project, got %s", call.Scope)
	}
	if call.Allowed {
		t.Errorf("Expected Allowed=false, got true")
	}
	if call.IsPattern {
		t.Errorf("Expected IsPattern=false, got true")
	}
}

func TestRequestApproval_DenialGlobalScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "evil.example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it with global scope
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send denial with global scope (no wildcard)
	req.Responses[0] <- approval.DomainResponse{
		Status: "denied",
		Scope:  "global",
		Reason: "test denial",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}

	// Verify recorder was called with global denial
	calls := recorder.getCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 recorder call, got %d", len(calls))
	}
	call := calls[0]
	if call.Domain != "evil.example.com" {
		t.Errorf("Expected Domain=evil.example.com, got %s", call.Domain)
	}
	if call.Scope != ScopeGlobal {
		t.Errorf("Expected Scope=global, got %s", call.Scope)
	}
	if call.Allowed {
		t.Errorf("Expected Allowed=false, got true")
	}
}

func TestRequestApproval_DenialWithWildcard(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "api.evil.example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it with project scope and wildcard pattern
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send denial with project scope and wildcard pattern
	req.Responses[0] <- approval.DomainResponse{
		Status:  "denied",
		Scope:   "project",
		Reason:  "test denial",
		Pattern: "*.evil.example.com",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}

	// Verify recorder was called with pattern
	calls := recorder.getCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 recorder call, got %d", len(calls))
	}
	call := calls[0]
	if call.Domain != "*.evil.example.com" {
		t.Errorf("Expected Domain=*.evil.example.com, got %s", call.Domain)
	}
	if call.Scope != ScopeProject {
		t.Errorf("Expected Scope=project, got %s", call.Scope)
	}
	if !call.IsPattern {
		t.Errorf("Expected IsPattern=true, got false")
	}
	if call.Allowed {
		t.Errorf("Expected Allowed=false, got true")
	}
}

func TestRequestApproval_DenialAuditLog(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	var buf bytes.Buffer
	auditLogger := audit.NewLogger(&buf)

	approver := NewDomainApprover(queue, recorder, auditLogger)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "evil.example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it with session scope
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send denial with session scope
	req.Responses[0] <- approval.DomainResponse{
		Status: "denied",
		Scope:  "session",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}

	// Verify audit log was written
	got := buf.String()
	if !strings.Contains(got, "DOMAIN DOMAIN_DENY") {
		t.Errorf("Audit log should contain 'DOMAIN DOMAIN_DENY': %s", got)
	}
	if !strings.Contains(got, `domain="evil.example.com"`) {
		t.Errorf("Audit log should contain domain: %s", got)
	}
	if !strings.Contains(got, `scope="session"`) {
		t.Errorf("Audit log should contain scope: %s", got)
	}
}

func TestRequestApproval_DenialAuditLog_WithPattern(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	var buf bytes.Buffer
	auditLogger := audit.NewLogger(&buf)

	approver := NewDomainApprover(queue, recorder, auditLogger)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "api.evil.example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it with pattern
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send denial with project scope and wildcard pattern
	req.Responses[0] <- approval.DomainResponse{
		Status:  "denied",
		Scope:   "project",
		Pattern: "*.evil.example.com",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}

	// Verify audit log was written with scope and pattern
	got := buf.String()
	if !strings.Contains(got, "DOMAIN DOMAIN_DENY") {
		t.Errorf("Audit log should contain 'DOMAIN DOMAIN_DENY': %s", got)
	}
	if !strings.Contains(got, `scope="project"`) {
		t.Errorf("Audit log should contain scope: %s", got)
	}
	if !strings.Contains(got, `pattern="*.evil.example.com"`) {
		t.Errorf("Audit log should contain pattern: %s", got)
	}
}

func TestRequestApproval_DenialNoAuditLog_NilLogger(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	recorder := newMockDecisionRecorder()

	// No audit logger (nil)
	approver := NewDomainApprover(queue, recorder, nil)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "evil.example.com:443", "test-token")
		close(done)
	}()

	// Give it a moment to be added to queue
	time.Sleep(10 * time.Millisecond)

	// Find the request and deny it
	requests := queue.List()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 request in queue, got %d", len(requests))
	}
	req, ok := queue.Get(requests[0].ID)
	if !ok {
		t.Fatalf("Failed to get request from queue")
	}

	// Send denial -- should not panic with nil audit logger
	req.Responses[0] <- approval.DomainResponse{
		Status: "denied",
		Scope:  "once",
	}
	queue.Remove(requests[0].ID)

	// Wait for result
	<-done

	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if result.Approved {
		t.Errorf("Expected Approved=false for denial, got true")
	}

	// "once" scope should not record anything
	calls := recorder.getCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 recorder calls for 'once' scope denial, got %d", len(calls))
	}
}
