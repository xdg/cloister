package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/guardian/approval"
)

// mockConfigPersister is a test double for ConfigPersister.
type mockConfigPersister struct {
	addDomainToProjectCalls []struct {
		project string
		domain  string
	}
	addDomainToGlobalCalls []struct {
		domain string
	}
	addPatternToProjectCalls []struct {
		project string
		pattern string
	}
	addPatternToGlobalCalls []struct {
		pattern string
	}
	addDomainToProjectErr  error
	addDomainToGlobalErr   error
	addPatternToProjectErr error
	addPatternToGlobalErr  error
}

func (m *mockConfigPersister) AddDomainToProject(project, domain string) error {
	m.addDomainToProjectCalls = append(m.addDomainToProjectCalls, struct {
		project string
		domain  string
	}{project, domain})
	return m.addDomainToProjectErr
}

func (m *mockConfigPersister) AddDomainToGlobal(domain string) error {
	m.addDomainToGlobalCalls = append(m.addDomainToGlobalCalls, struct {
		domain string
	}{domain})
	return m.addDomainToGlobalErr
}

func (m *mockConfigPersister) AddPatternToProject(project, pattern string) error {
	m.addPatternToProjectCalls = append(m.addPatternToProjectCalls, struct {
		project string
		pattern string
	}{project, pattern})
	return m.addPatternToProjectErr
}

func (m *mockConfigPersister) AddPatternToGlobal(pattern string) error {
	m.addPatternToGlobalCalls = append(m.addPatternToGlobalCalls, struct {
		pattern string
	}{pattern})
	return m.addPatternToGlobalErr
}

// TestDomainApprovalIntegration_FullFlow tests the complete domain approval workflow
// without Docker, using httptest for HTTP interactions.
//
// This test verifies Phase 6.7.1 acceptance criteria:
// 1. Create all components (DomainQueue, SessionAllowlist, ConfigPersister, DomainApprover, ProxyServer, ApprovalServer)
// 2. Submit domain request through proxy mock
// 3. Approve via server endpoint
// 4. Verify domain is allowed on next request
func TestDomainApprovalIntegration_FullFlow(t *testing.T) {
	// 1. Create all components
	domainQueue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	globalAllowlist := NewAllowlist([]string{}) // Empty - all domains require approval
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))

	// Create mock config persister to track persistence calls
	mockPersister := &mockConfigPersister{}

	// Create domain approver
	domainApprover := NewDomainApprover(domainQueue, sessionAllowlist, nil, cache, nil)

	// Create approval server
	cmdQueue := approval.NewQueue()
	approvalServer := approval.NewServer(cmdQueue, nil)
	approvalServer.Addr = "127.0.0.1:0" // Use random port
	approvalServer.SetDomainQueue(domainQueue)
	approvalServer.ConfigPersister = mockPersister

	// Start the approval server
	if err := approvalServer.Start(); err != nil {
		t.Fatalf("failed to start approval server: %v", err)
	}
	defer func() { _ = approvalServer.Stop(context.TODO()) }()

	baseURL := "http://" + approvalServer.ListenAddr()

	// 2. Submit domain request (simulating proxy behavior)
	// testDomain includes port to verify RequestApproval strips it
	const testDomain = "example.com:443"
	const wantDomain = "example.com"
	const testProject = "test-project"
	const testCloister = "test-cloister"

	// Start approval request in background (this will block until approved)
	done := make(chan struct{})
	var approvalResult DomainApprovalResult
	var approvalErr error

	go func() {
		approvalResult, approvalErr = domainApprover.RequestApproval(testProject, testCloister, testDomain, "test-token")
		close(done)
	}()

	// Wait for request to be added to queue
	time.Sleep(50 * time.Millisecond)

	// Verify request is in queue with port stripped
	requests := domainQueue.List()
	if len(requests) != 1 {
		t.Fatalf("expected 1 domain request in queue, got %d", len(requests))
	}

	requestID := requests[0].ID
	if requests[0].Domain != wantDomain {
		t.Errorf("expected domain %s, got %s", wantDomain, requests[0].Domain)
	}
	if requests[0].Project != testProject {
		t.Errorf("expected project %s, got %s", testProject, requests[0].Project)
	}
	if requests[0].Cloister != testCloister {
		t.Errorf("expected cloister %s, got %s", testCloister, requests[0].Cloister)
	}

	// 3. Approve via server endpoint with "session" scope
	approveBody := map[string]string{"scope": "session"}
	bodyBytes, err := json.Marshal(approveBody)
	if err != nil {
		t.Fatalf("failed to marshal approve body: %v", err)
	}

	approveReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/approve-domain/"+requestID, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	approveReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(approveReq)
	if err != nil {
		t.Fatalf("failed to POST approve: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, resp.StatusCode, string(body))
	}

	// Verify approval response
	var approveResp struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Scope  string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&approveResp); err != nil {
		t.Fatalf("failed to decode approve response: %v", err)
	}

	if approveResp.Status != "approved" {
		t.Errorf("expected status 'approved', got %s", approveResp.Status)
	}
	if approveResp.Scope != "session" {
		t.Errorf("expected scope 'session', got %s", approveResp.Scope)
	}

	// Wait for approver to receive response
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for approval result")
	}

	// Verify approval result
	if approvalErr != nil {
		t.Fatalf("approval request returned error: %v", approvalErr)
	}
	if !approvalResult.Approved {
		t.Error("expected approval result to be approved")
	}
	if approvalResult.Scope != "session" {
		t.Errorf("expected approval result scope 'session', got %s", approvalResult.Scope)
	}

	// 4. Verify domain is allowed on next request

	// Check session allowlist
	if !sessionAllowlist.IsAllowed("test-token", testDomain) {
		t.Error("domain should be in session allowlist after approval")
	}

	// Check cached allowlist (domain should be added without port)
	projectAllowlist, getErr := cache.GetProject(testProject)
	if getErr != nil {
		t.Fatalf("GetProject error: %v", getErr)
	}
	if projectAllowlist == nil {
		t.Fatal("project allowlist should exist")
	}
	if !projectAllowlist.IsAllowed(testDomain) {
		t.Error("domain should be in cached allowlist after session approval")
	}

	// Verify no persistence calls were made for session scope
	if len(mockPersister.addDomainToProjectCalls) > 0 {
		t.Error("session scope should not trigger AddDomainToProject")
	}
	if len(mockPersister.addDomainToGlobalCalls) > 0 {
		t.Error("session scope should not trigger AddDomainToGlobal")
	}

	// Verify second request for same domain doesn't require approval
	// (would be caught by session allowlist or cached allowlist)
	if !sessionAllowlist.IsAllowed("test-token", testDomain) {
		t.Error("second check should still find domain in session allowlist")
	}
}

// TestDomainApprovalIntegration_ProjectScope tests domain approval with project scope persistence.
func TestDomainApprovalIntegration_ProjectScope(t *testing.T) {
	// Create components
	domainQueue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))

	mockPersister := &mockConfigPersister{}
	domainApprover := NewDomainApprover(domainQueue, sessionAllowlist, nil, cache, nil)

	cmdQueue := approval.NewQueue()
	approvalServer := approval.NewServer(cmdQueue, nil)
	approvalServer.Addr = "127.0.0.1:0"
	approvalServer.SetDomainQueue(domainQueue)
	approvalServer.ConfigPersister = mockPersister

	if err := approvalServer.Start(); err != nil {
		t.Fatalf("failed to start approval server: %v", err)
	}
	defer func() { _ = approvalServer.Stop(context.TODO()) }()

	baseURL := "http://" + approvalServer.ListenAddr()

	const testDomain = "trusted.com:443"
	const wantDomain = "trusted.com"
	const testProject = "test-project"
	const testCloister = "test-cloister"

	// Submit approval request
	done := make(chan struct{})
	var approvalResult DomainApprovalResult
	var approvalErr error

	go func() {
		approvalResult, approvalErr = domainApprover.RequestApproval(testProject, testCloister, testDomain, "test-token")
		close(done)
	}()

	// Wait for request to be added
	time.Sleep(50 * time.Millisecond)

	requests := domainQueue.List()
	if len(requests) != 1 {
		t.Fatalf("expected 1 domain request in queue, got %d", len(requests))
	}

	requestID := requests[0].ID

	// Approve with "project" scope
	approveBody := map[string]string{"scope": "project"}
	bodyBytes, err := json.Marshal(approveBody)
	if err != nil {
		t.Fatalf("failed to marshal approve body: %v", err)
	}

	approveReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/approve-domain/"+requestID, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	approveReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(approveReq)
	if err != nil {
		t.Fatalf("failed to POST approve: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, resp.StatusCode, string(body))
	}

	// Wait for approval result
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for approval result")
	}

	// Verify approval result
	if approvalErr != nil {
		t.Fatalf("approval request returned error: %v", approvalErr)
	}
	if !approvalResult.Approved {
		t.Error("expected approval result to be approved")
	}
	if approvalResult.Scope != "project" {
		t.Errorf("expected approval result scope 'project', got %s", approvalResult.Scope)
	}

	// Verify persistence was called
	if len(mockPersister.addDomainToProjectCalls) != 1 {
		t.Fatalf("expected AddDomainToProject to be called once, got %d calls", len(mockPersister.addDomainToProjectCalls))
	}

	call := mockPersister.addDomainToProjectCalls[0]
	if call.project != testProject {
		t.Errorf("expected project %s, got %s", testProject, call.project)
	}
	if call.domain != wantDomain {
		t.Errorf("expected domain %s, got %s", wantDomain, call.domain)
	}

	// Verify domain is NOT in session allowlist for project scope
	if sessionAllowlist.IsAllowed("test-token", wantDomain) {
		t.Error("project scope should not add domain to session allowlist")
	}
}

// TestDomainApprovalIntegration_Denial tests domain approval denial flow.
func TestDomainApprovalIntegration_Denial(t *testing.T) {
	// Create components
	domainQueue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))

	domainApprover := NewDomainApprover(domainQueue, sessionAllowlist, nil, cache, nil)

	cmdQueue := approval.NewQueue()
	approvalServer := approval.NewServer(cmdQueue, nil)
	approvalServer.Addr = "127.0.0.1:0"
	approvalServer.SetDomainQueue(domainQueue)

	if err := approvalServer.Start(); err != nil {
		t.Fatalf("failed to start approval server: %v", err)
	}
	defer func() { _ = approvalServer.Stop(context.TODO()) }()

	baseURL := "http://" + approvalServer.ListenAddr()

	const testDomain = "malicious.com:443"
	const testProject = "test-project"
	const testCloister = "test-cloister"

	// Submit approval request
	done := make(chan struct{})
	var approvalResult DomainApprovalResult
	var approvalErr error

	go func() {
		approvalResult, approvalErr = domainApprover.RequestApproval(testProject, testCloister, testDomain, "test-token")
		close(done)
	}()

	// Wait for request to be added
	time.Sleep(50 * time.Millisecond)

	requests := domainQueue.List()
	if len(requests) != 1 {
		t.Fatalf("expected 1 domain request in queue, got %d", len(requests))
	}

	requestID := requests[0].ID

	// Deny the request
	denyBody := map[string]string{"reason": "Domain is known malware"}
	bodyBytes, err := json.Marshal(denyBody)
	if err != nil {
		t.Fatalf("failed to marshal deny body: %v", err)
	}

	denyReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/deny-domain/"+requestID, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	denyReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(denyReq)
	if err != nil {
		t.Fatalf("failed to POST deny: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, resp.StatusCode, string(body))
	}

	// Wait for approval result
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for approval result")
	}

	// Verify denial result
	if approvalErr != nil {
		t.Fatalf("approval request returned error: %v", approvalErr)
	}
	if approvalResult.Approved {
		t.Error("expected approval result to be denied")
	}

	// Verify domain is NOT in allowlists
	if sessionAllowlist.IsAllowed("test-token", testDomain) {
		t.Error("denied domain should not be in session allowlist")
	}

	projectAllowlist, getErr := cache.GetProject(testProject)
	if getErr != nil {
		t.Fatalf("GetProject error: %v", getErr)
	}
	if projectAllowlist != nil && projectAllowlist.IsAllowed(testDomain) {
		t.Error("denied domain should not be in cached allowlist")
	}
}

// TestDomainApprovalIntegration_Timeout tests domain approval timeout flow.
func TestDomainApprovalIntegration_Timeout(t *testing.T) {
	// Create components with very short timeout
	domainQueue := approval.NewDomainQueueWithTimeout(50 * time.Millisecond)
	sessionAllowlist := NewSessionAllowlist()
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)

	domainApprover := NewDomainApprover(domainQueue, sessionAllowlist, nil, cache, nil)

	const testDomain = "slow-response.com:443"
	const testProject = "test-project"
	const testCloister = "test-cloister"

	// Submit approval request (will timeout before approval)
	approvalResult, err := domainApprover.RequestApproval(testProject, testCloister, testDomain, "test-token")

	// Verify timeout result
	if err != nil {
		t.Fatalf("approval request returned error: %v", err)
	}
	if approvalResult.Approved {
		t.Error("expected timeout to result in denial")
	}

	// Verify domain is NOT in allowlists
	if sessionAllowlist.IsAllowed("test-token", testDomain) {
		t.Error("timed-out domain should not be in session allowlist")
	}
}

// TestDomainApprovalIntegration_GetPendingDomains tests the /pending-domains endpoint.
func TestDomainApprovalIntegration_GetPendingDomains(t *testing.T) {
	// Create components
	domainQueue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)

	domainApprover := NewDomainApprover(domainQueue, sessionAllowlist, nil, cache, nil)

	cmdQueue := approval.NewQueue()
	approvalServer := approval.NewServer(cmdQueue, nil)
	approvalServer.Addr = "127.0.0.1:0"
	approvalServer.SetDomainQueue(domainQueue)

	if err := approvalServer.Start(); err != nil {
		t.Fatalf("failed to start approval server: %v", err)
	}
	defer func() { _ = approvalServer.Stop(context.TODO()) }()

	baseURL := "http://" + approvalServer.ListenAddr()

	const testDomain = "pending.com:443"
	const wantDomain = "pending.com"
	const testProject = "test-project"
	const testCloister = "test-cloister"

	// Submit approval request
	go func() {
		_, _ = domainApprover.RequestApproval(testProject, testCloister, testDomain, "test-token")
	}()

	// Wait for request to be added
	time.Sleep(50 * time.Millisecond)

	// Query pending domains endpoint
	pendingHTTPReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("%s/pending-domains", baseURL), http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(pendingHTTPReq)
	if err != nil {
		t.Fatalf("failed to GET pending-domains: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, resp.StatusCode, string(body))
	}

	// Verify response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var pendingResp struct {
		Requests []struct {
			ID        string `json:"id"`
			Domain    string `json:"domain"`
			Project   string `json:"project"`
			Cloister  string `json:"cloister"`
			Timestamp string `json:"timestamp"`
		} `json:"requests"`
	}

	if err := json.Unmarshal(body, &pendingResp); err != nil {
		t.Fatalf("failed to decode pending response: %v", err)
	}

	if len(pendingResp.Requests) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pendingResp.Requests))
	}

	pendingReq := pendingResp.Requests[0]
	if pendingReq.Domain != wantDomain {
		t.Errorf("expected domain %s, got %s", wantDomain, pendingReq.Domain)
	}
	if pendingReq.Project != testProject {
		t.Errorf("expected project %s, got %s", testProject, pendingReq.Project)
	}
	if pendingReq.Cloister != testCloister {
		t.Errorf("expected cloister %s, got %s", testCloister, pendingReq.Cloister)
	}
}
