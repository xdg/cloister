package guardian

import (
	"testing"
	"time"

	"github.com/xdg/cloister/internal/guardian/approval"
)

func TestDomainApproverImpl_RequestApproval_Timeout(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(50 * time.Millisecond)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, cache)

	// Submit request and wait for timeout
	result, err := approver.RequestApproval("test-project", "test-cloister", "example.com:443")

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

	approver := NewDomainApprover(queue, sessionAllowlist, cache)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443")
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

	// Send denial
	req.Response <- approval.DomainResponse{
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

	approver := NewDomainApprover(queue, sessionAllowlist, cache)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443")
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

	// Send approval with session scope
	req.Response <- approval.DomainResponse{
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

	// Verify domain was added to session allowlist
	if !sessionAllowlist.IsAllowed("test-project", "example.com:443") {
		t.Errorf("Domain not added to session allowlist")
	}

	// Verify domain was added to cached allowlist
	projectAllowlist := cache.GetProject("test-project")
	if !projectAllowlist.IsAllowed("example.com:443") {
		t.Errorf("Domain not added to cached allowlist")
	}
}

func TestDomainApproverImpl_RequestApproval_ProjectScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, cache)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443")
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
	req.Response <- approval.DomainResponse{
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
	if sessionAllowlist.IsAllowed("test-project", "example.com:443") {
		t.Errorf("Domain should not be in session allowlist for project scope")
	}
}

func TestDomainApproverImpl_RequestApproval_GlobalScope(t *testing.T) {
	queue := approval.NewDomainQueueWithTimeout(5 * time.Second)
	sessionAllowlist := NewSessionAllowlist()
	cache := NewAllowlistCache(NewDefaultAllowlist())

	approver := NewDomainApprover(queue, sessionAllowlist, cache)

	// Submit request in background
	done := make(chan struct{})
	var result DomainApprovalResult
	var err error
	go func() {
		result, err = approver.RequestApproval("test-project", "test-cloister", "example.com:443")
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
	req.Response <- approval.DomainResponse{
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
	if sessionAllowlist.IsAllowed("test-project", "example.com:443") {
		t.Errorf("Domain should not be in session allowlist for global scope")
	}
}
