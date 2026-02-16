package guardian

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/audit"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/guardian/approval"
)

// mockSessionDenylist implements SessionDenylist for testing.
type mockSessionDenylist struct {
	mu      sync.Mutex
	blocked map[string]map[string]bool // token -> domain -> bool
}

func newMockSessionDenylist() *mockSessionDenylist {
	return &mockSessionDenylist{
		blocked: make(map[string]map[string]bool),
	}
}

func (m *mockSessionDenylist) IsBlocked(token, domain string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if domains, ok := m.blocked[token]; ok {
		return domains[domain]
	}
	return false
}

func (m *mockSessionDenylist) Add(token, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blocked[token] == nil {
		m.blocked[token] = make(map[string]bool)
	}
	m.blocked[token][domain] = true
	return nil
}

func (m *mockSessionDenylist) Clear(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blocked, token)
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
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())
	cache.SetProject("test-project", NewAllowlist([]string{}))

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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

	// Verify domain was added to session allowlist (using token, not project)
	if !sessionAllowlist.IsAllowed("test-token", "example.com") {
		t.Errorf("Domain not added to session allowlist")
	}

	// Verify domain was added to cached allowlist
	projectAllowlist := cache.GetProject("test-project")
	if !projectAllowlist.IsAllowed("example.com") {
		t.Errorf("Domain not added to cached allowlist")
	}
}

func TestDomainApproverImpl_RequestApproval_ProjectScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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

	// For project scope, the domain should NOT be added to session allowlist
	// (ConfigPersister handles persistence, cache reload happens via SIGHUP)
	if sessionAllowlist.IsAllowed("test-token", "example.com") {
		t.Errorf("Domain should not be in session allowlist for project scope")
	}
}

func TestDomainApproverImpl_RequestApproval_GlobalScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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

	// For global scope, the domain should NOT be added to session allowlist
	// (ConfigPersister handles persistence, cache reload happens via SIGHUP)
	if sessionAllowlist.IsAllowed("test-token", "example.com") {
		t.Errorf("Domain should not be in session allowlist for global scope")
	}
}

func TestRequestApproval_DenialSessionScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	sessionDenylist := newMockSessionDenylist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, sessionDenylist, cache, nil)

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

	// Verify domain was added to session denylist
	if !sessionDenylist.IsBlocked("test-token", "evil.example.com") {
		t.Errorf("Domain not added to session denylist")
	}
}

func TestRequestApproval_DenialProjectScope(t *testing.T) {
	// Set up temp config dir for decisions persistence
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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

	// Verify the domain was persisted to the project decisions file
	decisions, loadErr := config.LoadProjectDecisions("test-project")
	if loadErr != nil {
		t.Fatalf("Failed to load project decisions: %v", loadErr)
	}
	deniedDomains := decisions.DeniedDomains()
	if len(deniedDomains) != 1 || deniedDomains[0] != "evil.example.com" {
		t.Errorf("Expected DeniedDomains=[evil.example.com], got %v", deniedDomains)
	}
	deniedPatterns := decisions.DeniedPatterns()
	if len(deniedPatterns) != 0 {
		t.Errorf("Expected empty DeniedPatterns, got %v", deniedPatterns)
	}
}

func TestRequestApproval_DenialGlobalScope(t *testing.T) {
	// Set up temp config dir for decisions persistence
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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

	// Verify the domain was persisted to the global decisions file
	decisions, loadErr := config.LoadGlobalDecisions()
	if loadErr != nil {
		t.Fatalf("Failed to load global decisions: %v", loadErr)
	}
	deniedDomains := decisions.DeniedDomains()
	if len(deniedDomains) != 1 || deniedDomains[0] != "evil.example.com" {
		t.Errorf("Expected DeniedDomains=[evil.example.com], got %v", deniedDomains)
	}
	deniedPatterns := decisions.DeniedPatterns()
	if len(deniedPatterns) != 0 {
		t.Errorf("Expected empty DeniedPatterns, got %v", deniedPatterns)
	}
}

func TestRequestApproval_DenialWithWildcard(t *testing.T) {
	// Set up temp config dir for decisions persistence
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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

	// Verify the pattern was persisted to Proxy.Deny (not as a domain)
	decisions, loadErr := config.LoadProjectDecisions("test-project")
	if loadErr != nil {
		t.Fatalf("Failed to load project decisions: %v", loadErr)
	}
	deniedPatterns := decisions.DeniedPatterns()
	if len(deniedPatterns) != 1 || deniedPatterns[0] != "*.evil.example.com" {
		t.Errorf("Expected DeniedPatterns=[*.evil.example.com], got %v", deniedPatterns)
	}
	deniedDomains := decisions.DeniedDomains()
	if len(deniedDomains) != 0 {
		t.Errorf("Expected empty DeniedDomains, got %v", deniedDomains)
	}
}

func TestRequestApproval_DenialAuditLog(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	var buf bytes.Buffer
	auditLogger := audit.NewLogger(&buf)

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, auditLogger)

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
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	var buf bytes.Buffer
	auditLogger := audit.NewLogger(&buf)

	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, auditLogger)

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
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	// No audit logger (nil)
	approver := NewDomainApprover(queue, sessionAllowlist, nil, cache, nil)

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
}
