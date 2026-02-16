package guardian

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/guardian/approval"
	"github.com/xdg/cloister/internal/token"
)

// Ensure helpers used by later test phases don't trigger "unused" lint warnings.
var _ = (*mockTunnelHandler).getCalls

// mockTunnelHandler records CONNECT targets and completes the tunnel handshake.
type mockTunnelHandler struct {
	mu    sync.Mutex
	calls []string // records targetHostPort of each call
}

func (m *mockTunnelHandler) ServeTunnel(w http.ResponseWriter, _ *http.Request, target string) {
	m.mu.Lock()
	m.calls = append(m.calls, target)
	m.mu.Unlock()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "cannot hijack", 500)
		return
	}
	conn, _, _ := hijacker.Hijack()
	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	_ = conn.Close()
}

func (m *mockTunnelHandler) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	copy(out, m.calls)
	return out
}

// proxyTestHarness wires together all the components needed for integration-style
// proxy approval tests without requiring Docker or external services.
type proxyTestHarness struct {
	Proxy          *ProxyServer
	ApprovalServer *approval.Server
	Reloader       *CacheReloader
	ConfigDir      string // t.TempDir() used as XDG_CONFIG_HOME
	Token          string
	ProjectName    string
	CloisterName   string
	TunnelHandler  *mockTunnelHandler
	StaticAllow    []config.AllowEntry
	t              *testing.T
}

func newProxyTestHarness(t *testing.T) *proxyTestHarness {
	return newProxyTestHarnessWithConfigDir(t, "")
}

// newProxyTestHarnessWithConfigDir creates a test harness. If configDir is
// empty, a new t.TempDir() is used; otherwise the given directory is reused
// (simulating a guardian restart with persisted decisions on disk).
func newProxyTestHarnessWithConfigDir(t *testing.T, configDir string) *proxyTestHarness {
	t.Helper()

	if configDir == "" {
		configDir = t.TempDir()
	}
	// Set XDG_CONFIG_HOME so config.DecisionDir() uses our temp dir
	t.Setenv("XDG_CONFIG_HOME", configDir)
	// Also set XDG_STATE_HOME to prevent writes to real state dir
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	tok := "test-token-12345"
	projectName := "test-project"
	cloisterName := "test-project-main"

	// Use real token.Registry (same as production in internal/cmd/guardian.go).
	registry := token.NewRegistry()
	registry.RegisterFull(tok, cloisterName, projectName, "")

	// Load production default config for static allow/deny entries.
	cfg := config.DefaultGlobalConfig()
	staticAllow := cfg.Proxy.Allow
	staticDeny := cfg.Proxy.Deny

	// Build global allowlist from static config (matches setupAllowlistCache in production).
	globalAllowlist := NewAllowlistFromConfig(staticAllow)
	allowlistCache := NewAllowlistCache(globalAllowlist)

	// Wire global denylist if static deny entries exist.
	globalDeny := config.MergeDenylists(staticDeny, nil)
	if len(globalDeny) > 0 {
		allowlistCache.SetGlobalDeny(NewAllowlistFromConfig(globalDeny))
	}

	// CacheReloader with production-matching static config.
	reloader := NewCacheReloader(allowlistCache, registry, staticAllow, staticDeny, &config.Decisions{})
	allowlistCache.SetProjectLoader(reloader.LoadProjectAllowlist)
	allowlistCache.SetDenylistLoader(reloader.LoadProjectDenylist)

	// Create approval server components
	queue := approval.NewQueue()
	domainQueue := approval.NewDomainQueueWithTimeout(10 * time.Second)

	persister := &ConfigPersisterImpl{}

	approvalSrv := approval.NewServer(queue, nil) // nil audit logger
	approvalSrv.SetDomainQueue(domainQueue)
	approvalSrv.ConfigPersister = persister
	approvalSrv.Addr = "127.0.0.1:0"

	// Create session lists
	sessionAllowlist := NewSessionAllowlist()
	sessionDenylist := NewSessionDenylist()

	// Create domain approver
	domainApprover := NewDomainApprover(domainQueue, sessionAllowlist, sessionDenylist, allowlistCache, nil)

	// Create tunnel handler mock (can't do real upstream connections in unit tests).
	tunnelHandler := &mockTunnelHandler{}

	// Create proxy server with production-faithful global allowlist.
	proxy := NewProxyServerWithConfig(":0", globalAllowlist)
	proxy.TokenValidator = registry
	proxy.AllowlistCache = allowlistCache
	proxy.TokenLookup = TokenLookupFromRegistry(registry)
	proxy.DomainApprover = domainApprover
	proxy.SessionAllowlist = sessionAllowlist
	proxy.SessionDenylist = sessionDenylist
	proxy.TunnelHandler = tunnelHandler

	// Set up ConfigPersister reload notifier using CacheReloader.
	persister.ReloadNotifier = reloader.Reload

	// Start servers
	if err := approvalSrv.Start(); err != nil {
		t.Fatalf("failed to start approval server: %v", err)
	}

	if err := proxy.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}

	// Cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Stop(ctx)
		_ = approvalSrv.Stop(ctx)
	})

	return &proxyTestHarness{
		Proxy:          proxy,
		ApprovalServer: approvalSrv,
		Reloader:       reloader,
		ConfigDir:      configDir,
		Token:          tok,
		ProjectName:    projectName,
		CloisterName:   cloisterName,
		TunnelHandler:  tunnelHandler,
		StaticAllow:    staticAllow,
		t:              t,
	}
}

// sendCONNECT sends an authenticated CONNECT request through the proxy
// and returns the HTTP status code from the proxy's response.
func (h *proxyTestHarness) sendCONNECT(domain string) (int, error) {
	h.t.Helper()

	proxyAddr := h.Proxy.ListenAddr()
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		return 0, fmt.Errorf("dial proxy: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Build CONNECT request with auth
	target := domain + ":443"
	auth := base64.StdEncoding.EncodeToString([]byte("cloister:" + h.Token))
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n",
		target, target, auth)

	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return 0, fmt.Errorf("set deadline: %w", err)
	}
	if _, err := conn.Write([]byte(req)); err != nil {
		return 0, fmt.Errorf("write CONNECT: %w", err)
	}

	// Read response
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

// pendingDomainID polls the approval server's /pending-domains endpoint
// until at least one request appears and returns its ID.
func (h *proxyTestHarness) pendingDomainID() (string, error) {
	h.t.Helper()

	approvalAddr := h.ApprovalServer.ListenAddr()
	url := fmt.Sprintf("http://%s/pending-domains", approvalAddr)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
		if reqErr != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var result struct {
			Requests []struct {
				ID string `json:"id"`
			} `json:"requests"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if len(result.Requests) > 0 {
			return result.Requests[0].ID, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", fmt.Errorf("no pending domain requests found within timeout")
}

// approveDomain posts an approval decision to the approval server for a
// pending domain request.
func (h *proxyTestHarness) approveDomain(id, scope, pattern string) error {
	h.t.Helper()

	approvalAddr := h.ApprovalServer.ListenAddr()
	url := fmt.Sprintf("http://%s/approve-domain/%s", approvalAddr, id)

	body := map[string]string{"scope": scope}
	if pattern != "" {
		body["pattern"] = pattern
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	approveReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create approve request: %w", err)
	}
	approveReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(approveReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("approve returned %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// denyDomain posts a denial decision to the approval server for a
// pending domain request.
func (h *proxyTestHarness) denyDomain(id, scope string, wildcard bool) error {
	h.t.Helper()

	approvalAddr := h.ApprovalServer.ListenAddr()
	url := fmt.Sprintf("http://%s/deny-domain/%s", approvalAddr, id)

	body := map[string]interface{}{"scope": scope, "wildcard": wildcard}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	denyReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create deny request: %w", err)
	}
	denyReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(denyReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deny returned %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// TestProxyTestHarness_BasicDenyFlow verifies the full proxy approval pipeline:
// CONNECT to unlisted domain -> blocks -> deny via approval server -> 403 returned.
func TestProxyTestHarness_BasicDenyFlow(t *testing.T) {
	h := newProxyTestHarness(t)

	// Send CONNECT in goroutine (blocks until decision)
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT("unlisted.example.com")
	}()

	// Wait for pending domain to appear
	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}
	if id == "" {
		t.Fatal("pendingDomainID() returned empty id")
	}

	// Deny it
	if err := h.denyDomain(id, "once", false); err != nil {
		t.Fatalf("denyDomain() error: %v", err)
	}

	// Wait for CONNECT to complete
	<-done

	if connectErr != nil {
		t.Fatalf("sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, statusCode)
	}
}

// TestProxyApproval_AllowOnce verifies that scope=once allows a single CONNECT
// but does not remember the decision for subsequent requests to the same domain.
func TestProxyApproval_AllowOnce(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "once-allow.example.com"

	// First CONNECT: should block until approved.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.approveDomain(id, "once", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Second CONNECT to same domain: should block again (not remembered).
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()

	id2, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("second pendingDomainID() error: %v", err)
	}

	// Deny the second request to unblock it.
	if err := h.denyDomain(id2, "once", false); err != nil {
		t.Fatalf("denyDomain() error: %v", err)
	}

	<-done2
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusForbidden {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode2)
	}
}

// TestProxyApproval_AllowSession verifies that scope=session remembers the
// allow decision for subsequent requests without writing a decisions file.
func TestProxyApproval_AllowSession(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "session-allow.example.com"

	// First CONNECT: should block until approved.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.approveDomain(id, "session", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Second CONNECT to same domain: should return 200 immediately (no blocking).
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (domain should be remembered in session)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}

	// Verify no decisions file was written for the project.
	decisionPath := config.ProjectDecisionPath(h.ProjectName)
	if _, err := os.Stat(decisionPath); !os.IsNotExist(err) {
		t.Errorf("expected no project decisions file at %s, but it exists (or stat error: %v)", decisionPath, err)
	}
}

// TestProxyApproval_AllowProject verifies that scope=project persists the
// decision to a project decisions file and remembers it for subsequent requests.
func TestProxyApproval_AllowProject(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "project-allow.example.com"

	// First CONNECT: should block until approved.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.approveDomain(id, "project", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Verify the project decisions file contains the domain.
	decisions, err := config.LoadProjectDecisions(h.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	allowedDomains := decisions.AllowedDomains()
	found := false
	for _, d := range allowedDomains {
		if d == domain {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected domain %q in project decisions allow list, got: %v", domain, allowedDomains)
	}

	// Second CONNECT to same domain: should return 200 immediately.
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (domain should be remembered in project decisions)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}

	// Verify the tunnel handler was called twice (both connections tunneled).
	calls := h.TunnelHandler.getCalls()
	if len(calls) != 2 {
		t.Errorf("expected 2 tunnel handler calls, got %d: %v", len(calls), calls)
	}
}

// TestProxyApproval_AllowGlobal verifies that scope=global persists the
// decision to the global decisions file.
func TestProxyApproval_AllowGlobal(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "global-allow.example.com"

	// CONNECT should block until approved.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.approveDomain(id, "global", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Verify the global decisions file contains the domain.
	decisions, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error: %v", err)
	}
	allowedDomains := decisions.AllowedDomains()
	found := false
	for _, d := range allowedDomains {
		if d == domain {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected domain %q in global decisions allow list, got: %v", domain, allowedDomains)
	}

	// Second CONNECT to the same domain should be allowed immediately
	// (the ReloadNotifier updates the cache after persistence).
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()

	select {
	case <-done2:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block for a globally-allowed domain")
	}

	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}

	// Verify tunnel handler was called twice.
	calls := h.TunnelHandler.getCalls()
	if len(calls) != 2 {
		t.Errorf("expected 2 tunnel handler calls, got %d: %v", len(calls), calls)
	}
}

// TestProxyApproval_AllowProjectWildcard verifies that approving with a wildcard
// pattern persists the pattern and matches subsequent requests to different subdomains.
func TestProxyApproval_AllowProjectWildcard(t *testing.T) {
	h := newProxyTestHarness(t)

	// First CONNECT to a specific subdomain.
	domain1 := "api.wildcard-test.com"
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain1)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	// Approve with wildcard pattern.
	if err := h.approveDomain(id, "project", "*.wildcard-test.com"); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Verify the project decisions file contains the wildcard pattern.
	decisions, err := config.LoadProjectDecisions(h.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	allowedPatterns := decisions.AllowedPatterns()
	found := false
	for _, p := range allowedPatterns {
		if p == "*.wildcard-test.com" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pattern %q in project decisions allow list, got: %v", "*.wildcard-test.com", allowedPatterns)
	}

	// Second CONNECT to a different subdomain: should match the wildcard and return immediately.
	domain2 := "cdn.wildcard-test.com"
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain2)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (wildcard pattern should match)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}

	// Third CONNECT to a nested subdomain should also match the wildcard.
	domain3 := "deep.cdn.wildcard-test.com"
	done3 := make(chan struct{})
	var statusCode3 int
	var connectErr3 error
	go func() {
		defer close(done3)
		statusCode3, connectErr3 = h.sendCONNECT(domain3)
	}()
	select {
	case <-done3:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("third CONNECT should not block (wildcard pattern should match nested subdomain)")
	}
	if connectErr3 != nil {
		t.Fatalf("third sendCONNECT() error: %v", connectErr3)
	}
	if statusCode3 != http.StatusOK {
		t.Errorf("third CONNECT: expected status %d, got %d", http.StatusOK, statusCode3)
	}
}

// TestProxyApproval_CaseInsensitivity verifies that approvals are normalized to
// avoid repeated prompts when casing differs between requests.
func TestProxyApproval_CaseInsensitivity(t *testing.T) {
	h := newProxyTestHarness(t)
	firstDomain := "MiXeD.Example.com"
	secondDomain := "mixed.example.com"

	// First CONNECT blocks until approved.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(firstDomain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.approveDomain(id, "project", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Second CONNECT with different casing should not block.
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(secondDomain)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (case-insensitive match)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}

	// Verify the project decisions file stored normalized domain.
	decisions, err := config.LoadProjectDecisions(h.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	allowedDomains := decisions.AllowedDomains()
	found := false
	for _, d := range allowedDomains {
		if strings.EqualFold(d, firstDomain) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected normalized domain %q in project decisions allow list, got: %v", strings.ToLower(firstDomain), allowedDomains)
	}
}

// TestProxyApproval_DenyOnce verifies that scope=once denies a single CONNECT
// but does not remember the decision for subsequent requests to the same domain.
func TestProxyApproval_DenyOnce(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "once-deny.example.com"

	// First CONNECT: should block until denied.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.denyDomain(id, "once", false); err != nil {
		t.Fatalf("denyDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode)
	}

	// Second CONNECT to same domain: should block again (not remembered).
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()

	id2, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("second pendingDomainID() error: %v", err)
	}

	// Approve the second request to unblock it.
	if err := h.approveDomain(id2, "once", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done2
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}
}

// TestProxyApproval_DenySession verifies that scope=session remembers the
// deny decision for subsequent requests without writing a decisions file.
func TestProxyApproval_DenySession(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "session-deny.example.com"

	// First CONNECT: should block until denied.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.denyDomain(id, "session", false); err != nil {
		t.Fatalf("denyDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode)
	}

	// Second CONNECT to same domain: should return 403 immediately (session denylist remembered).
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (domain should be remembered in session denylist)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusForbidden {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode2)
	}

	// Verify no decisions file was written for the project.
	decisionPath := config.ProjectDecisionPath(h.ProjectName)
	if _, err := os.Stat(decisionPath); !os.IsNotExist(err) {
		t.Errorf("expected no project decisions file at %s, but it exists (or stat error: %v)", decisionPath, err)
	}
}

// TestProxyApproval_DenyProject verifies that scope=project persists the
// deny decision to a project decisions file and remembers it for subsequent requests.
func TestProxyApproval_DenyProject(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "project-deny.example.com"

	// First CONNECT: should block until denied.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.denyDomain(id, "project", false); err != nil {
		t.Fatalf("denyDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode)
	}

	// Verify the project decisions file contains the domain in the deny list.
	decisions, err := config.LoadProjectDecisions(h.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	deniedDomains := decisions.DeniedDomains()
	found := false
	for _, d := range deniedDomains {
		if d == domain {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected domain %q in project decisions deny list, got: %v", domain, deniedDomains)
	}

	// Second CONNECT to same domain: should return 403 immediately (denylist cache).
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (domain should be remembered in project denylist)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusForbidden {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode2)
	}
}

// TestProxyApproval_DenyGlobal verifies that scope=global persists the
// deny decision to the global decisions file.
func TestProxyApproval_DenyGlobal(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "global-deny.example.com"

	// CONNECT should block until denied.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.denyDomain(id, "global", false); err != nil {
		t.Fatalf("denyDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode)
	}

	// Verify the global decisions file contains the domain in the deny list.
	decisions, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error: %v", err)
	}
	deniedDomains := decisions.DeniedDomains()
	found := false
	for _, d := range deniedDomains {
		if d == domain {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected domain %q in global decisions deny list, got: %v", domain, deniedDomains)
	}

	// Second CONNECT to same domain: should return 403 immediately.
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (domain should be remembered in global denylist)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusForbidden {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode2)
	}
}

// TestProxyApproval_DenyProjectWildcard verifies that denying with wildcard=true
// persists a wildcard pattern in the deny list and blocks subsequent requests
// to different subdomains of the same base domain.
func TestProxyApproval_DenyProjectWildcard(t *testing.T) {
	h := newProxyTestHarness(t)

	// First CONNECT to a specific subdomain.
	domain1 := "api.deny-wild.com"
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain1)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	// Deny with wildcard.
	if err := h.denyDomain(id, "project", true); err != nil {
		t.Fatalf("denyDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode)
	}

	// Verify the project decisions file contains the wildcard pattern in the deny list.
	decisions, err := config.LoadProjectDecisions(h.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	deniedPatterns := decisions.DeniedPatterns()
	found := false
	for _, p := range deniedPatterns {
		if p == "*.deny-wild.com" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pattern %q in project decisions deny list, got: %v", "*.deny-wild.com", deniedPatterns)
	}

	// Second CONNECT to a different subdomain: should match the wildcard and return 403 immediately.
	domain2 := "cdn.deny-wild.com"
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain2)
	}()
	select {
	case <-done2:
		// good - returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (wildcard deny pattern should match)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusForbidden {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusForbidden, statusCode2)
	}
}

// TestProxyApproval_PreExistingProjectAllow verifies that a project decisions file
// written before the proxy starts is loaded lazily and allows the domain immediately
// without prompting.
func TestProxyApproval_PreExistingProjectAllow(t *testing.T) {
	h := newProxyTestHarness(t)

	// Write project decisions file with a pre-allowed domain.
	// The harness already set XDG_CONFIG_HOME to a temp dir, and the
	// AllowlistCache project loader is lazy, so writing the file before
	// the first CONNECT is sufficient.
	err := config.WriteProjectDecisions(h.ProjectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "pre-allowed.example.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions() error: %v", err)
	}

	// Send CONNECT — should return 200 immediately (no approval prompt).
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT("pre-allowed.example.com")
	}()

	select {
	case <-done:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("CONNECT should not block for a pre-existing project allow entry")
	}

	if connectErr != nil {
		t.Fatalf("sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, statusCode)
	}
}

// TestProxyApproval_PreExistingGlobalDeny verifies that a global decisions file
// with a deny entry written before the proxy starts causes immediate 403 responses
// without invoking the DomainApprover.
func TestProxyApproval_PreExistingGlobalDeny(t *testing.T) {
	h := newProxyTestHarness(t)

	// Write global decisions file with a denied domain.
	err := config.WriteGlobalDecisions(&config.Decisions{
		Proxy: config.DecisionsProxy{
			Deny: []config.AllowEntry{{Domain: "pre-denied.example.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteGlobalDecisions() error: %v", err)
	}

	// Reload the cache to pick up the global decisions file (exercises the production path).
	h.Reloader.Reload()

	// Send CONNECT — should return 403 immediately (no approval prompt).
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT("pre-denied.example.com")
	}()

	select {
	case <-done:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("CONNECT should not block for a pre-existing global deny entry")
	}

	if connectErr != nil {
		t.Fatalf("sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, statusCode)
	}

	// Verify the tunnel handler was NOT called (denylist short-circuits before tunneling).
	calls := h.TunnelHandler.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 tunnel handler calls, got %d: %v", len(calls), calls)
	}
}

// TestProxyApproval_PreExistingDenyPattern verifies that a deny pattern in a
// project decisions file written before the proxy starts blocks matching domains
// immediately without prompting.
func TestProxyApproval_PreExistingDenyPattern(t *testing.T) {
	h := newProxyTestHarness(t)

	// Write project decisions file with a deny pattern.
	err := config.WriteProjectDecisions(h.ProjectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Deny: []config.AllowEntry{{Pattern: "*.evil-startup.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions() error: %v", err)
	}

	// Send CONNECT to a subdomain that matches the pattern — should return 403 immediately.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT("api.evil-startup.com")
	}()

	select {
	case <-done:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("CONNECT should not block for a pre-existing deny pattern match")
	}

	if connectErr != nil {
		t.Fatalf("sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, statusCode)
	}
}

// TestProxyApproval_DenyOverridesAllow verifies that when both allow and deny
// entries exist for the same domain in a project decisions file, the deny
// entry takes precedence and the domain is blocked.
func TestProxyApproval_DenyOverridesAllow(t *testing.T) {
	h := newProxyTestHarness(t)

	// Write project decisions file with both allow and deny for the same domain.
	err := config.WriteProjectDecisions(h.ProjectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "conflict.example.com"}},
			Deny:  []config.AllowEntry{{Domain: "conflict.example.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions() error: %v", err)
	}

	// Send CONNECT — deny should win, returning 403 immediately.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT("conflict.example.com")
	}()

	select {
	case <-done:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("CONNECT should not block when deny overrides allow")
	}

	if connectErr != nil {
		t.Fatalf("sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, statusCode)
	}
}

// sendCONNECTPort sends an authenticated CONNECT request with a custom port
// through the proxy and returns the HTTP status code from the proxy's response.
func (h *proxyTestHarness) sendCONNECTPort(domain string, port int) (int, error) {
	h.t.Helper()

	proxyAddr := h.Proxy.ListenAddr()
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		return 0, fmt.Errorf("dial proxy: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Build CONNECT request with auth using the custom port
	target := fmt.Sprintf("%s:%d", domain, port)
	auth := base64.StdEncoding.EncodeToString([]byte("cloister:" + h.Token))
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n",
		target, target, auth)

	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return 0, fmt.Errorf("set deadline: %w", err)
	}
	if _, err := conn.Write([]byte(req)); err != nil {
		return 0, fmt.Errorf("write CONNECT: %w", err)
	}

	// Read response
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

// pendingDomainCount polls the approval server's /pending-domains endpoint
// and returns the number of pending requests.
func (h *proxyTestHarness) pendingDomainCount() (int, error) {
	h.t.Helper()

	approvalAddr := h.ApprovalServer.ListenAddr()
	url := fmt.Sprintf("http://%s/pending-domains", approvalAddr)

	countReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(countReq)
	if err != nil {
		return 0, fmt.Errorf("GET /pending-domains: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	var result struct {
		Requests []struct {
			ID string `json:"id"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unmarshal: %w", err)
	}
	return len(result.Requests), nil
}

// TestProxyApproval_InvalidDomain verifies that a CONNECT request to an
// invalid domain returns 403 immediately without being queued for approval.
// ValidateDomain rejects empty hostnames (among other things).
func TestProxyApproval_InvalidDomain(t *testing.T) {
	h := newProxyTestHarness(t)

	// CONNECT to ":443" — after stripPort the domain is "", which
	// ValidateDomain rejects with "domain is empty". This triggers
	// the "Forbidden - invalid domain" code path.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECTPort("", 443)
	}()

	// Should return quickly without blocking for approval.
	select {
	case <-done:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("CONNECT to invalid domain should not block for approval")
	}

	if connectErr != nil {
		t.Fatalf("sendCONNECTPort() error: %v", connectErr)
	}
	if statusCode != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, statusCode)
	}

	// Verify the tunnel handler was NOT called.
	calls := h.TunnelHandler.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 tunnel handler calls, got %d: %v", len(calls), calls)
	}
}

// TestProxyApproval_PortStrippingConsistency verifies that port is stripped
// before persisting decisions: approve via :443 stores domain only, and a
// subsequent CONNECT on a different port (:8443) is allowed without re-prompting.
func TestProxyApproval_PortStrippingConsistency(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "port-test.example.com"

	// First CONNECT (default :443 via sendCONNECT): should block until approved.
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	// Approve with scope=project so the decision is persisted.
	if err := h.approveDomain(id, "project", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Verify the project decisions file contains the domain WITHOUT port.
	decisions, err := config.LoadProjectDecisions(h.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	allowedDomains := decisions.AllowedDomains()
	found := false
	for _, d := range allowedDomains {
		if d == domain {
			found = true
		}
		// Ensure no port-suffixed entry leaked through.
		if d == domain+":443" || d == domain+":8443" {
			t.Errorf("decisions file should not contain port-suffixed domain, got: %q", d)
		}
	}
	if !found {
		t.Fatalf("expected domain %q in project decisions allow list, got: %v", domain, allowedDomains)
	}

	// Second CONNECT on a different port (:8443): should return 200 immediately
	// because port is stripped and the domain is already allowed.
	var statusCode2 int
	var connectErr2 error
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECTPort(domain, 8443)
	}()

	select {
	case <-done2:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT on different port should not block (same domain, port stripped)")
	}

	if connectErr2 != nil {
		t.Fatalf("second sendCONNECTPort() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}
}

// TestProxyApproval_DuplicateCONNECTDuringPending verifies that two concurrent
// CONNECT requests for the same domain produce only one approval prompt, and
// both unblock when a single approval decision is made.
func TestProxyApproval_DuplicateCONNECTDuringPending(t *testing.T) {
	h := newProxyTestHarness(t)
	domain := "dedup-test.example.com"

	// Launch two concurrent CONNECT requests for the same domain.
	var statusCode1 int
	var connectErr1 error
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		statusCode1, connectErr1 = h.sendCONNECT(domain)
	}()

	var statusCode2 int
	var connectErr2 error
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()

	// Wait for at least one request to appear in the pending queue.
	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	// Brief pause to let both requests reach the queue.
	time.Sleep(200 * time.Millisecond)

	// Verify exactly 1 pending entry (deduplication).
	count, err := h.pendingDomainCount()
	if err != nil {
		t.Fatalf("pendingDomainCount() error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 pending domain request (deduplication), got %d", count)
	}

	// Approve the single entry — both goroutines should unblock.
	if err := h.approveDomain(id, "once", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	// Wait for both to complete.
	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("first CONNECT did not unblock after approval")
	}
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("second CONNECT did not unblock after approval")
	}

	if connectErr1 != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr1)
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode1 != http.StatusOK {
		t.Errorf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode1)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}
}

// sendHTTPViaProxy sends a regular HTTP HEAD request through the proxy (not CONNECT).
// This simulates what curl does with http:// URLs when HTTP_PROXY is set.
func (h *proxyTestHarness) sendHTTPViaProxy(rawURL string) (int, error) {
	h.t.Helper()

	proxyAddr := h.Proxy.ListenAddr()
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		return 0, fmt.Errorf("dial proxy: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// For forward proxy mode, send the full URL in the request line
	auth := base64.StdEncoding.EncodeToString([]byte("cloister:" + h.Token))
	req := fmt.Sprintf("HEAD %s HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: Basic %s\r\nConnection: close\r\n\r\n",
		rawURL, auth)

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, fmt.Errorf("set deadline: %w", err)
	}
	if _, err := conn.Write([]byte(req)); err != nil {
		return 0, fmt.Errorf("write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

// TestProxyApproval_NonCONNECTReturns405 verifies that HTTP requests (not
// CONNECT) sent through the proxy return 405 Method Not Allowed. This is what
// happens when curl uses http:// URLs with HTTP_PROXY set — curl sends a
// regular HEAD through the proxy instead of CONNECT.
func TestProxyApproval_NonCONNECTReturns405(t *testing.T) {
	h := newProxyTestHarness(t)

	statusCode, err := h.sendHTTPViaProxy("http://some-domain.example.com/")
	if err != nil {
		t.Fatalf("sendHTTPViaProxy() error: %v", err)
	}
	if statusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d (Method Not Allowed), got %d", http.StatusMethodNotAllowed, statusCode)
	}

	// Verify no tunnel handler calls.
	calls := h.TunnelHandler.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 tunnel handler calls, got %d: %v", len(calls), calls)
	}

	// Verify no pending domain requests (non-CONNECT should never reach approval).
	count, err := h.pendingDomainCount()
	if err != nil {
		t.Fatalf("pendingDomainCount() error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 pending domain requests, got %d", count)
	}
}

// TestProxyApproval_AllowProjectWithStaticConfig verifies project approval
// persistence with static allow entries populated. The harness now uses
// production default config (api.anthropic.com, proxy.golang.org, etc.)
// so no manual injection is needed.
func TestProxyApproval_AllowProjectWithStaticConfig(t *testing.T) {
	h := newProxyTestHarness(t)

	// Verify the harness has production static config loaded.
	if len(h.StaticAllow) == 0 {
		t.Fatal("expected non-empty StaticAllow from production defaults")
	}

	domain := "project-static-test.example.com"

	// First CONNECT: should block until approved (domain not in static config).
	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h.sendCONNECT(domain)
	}()

	id, err := h.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h.approveDomain(id, "project", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first CONNECT: expected status %d, got %d", http.StatusOK, statusCode)
	}

	// Verify the project decisions file contains the domain.
	decisions, err := config.LoadProjectDecisions(h.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	found := false
	for _, d := range decisions.AllowedDomains() {
		if d == domain {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected domain %q in project decisions allow list, got: %v", domain, decisions.AllowedDomains())
	}

	// Second CONNECT to same domain: should return 200 immediately.
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h.sendCONNECT(domain)
	}()
	select {
	case <-done2:
		// good — returned quickly
	case <-time.After(3 * time.Second):
		t.Fatal("second CONNECT should not block (domain should be remembered from project decisions)")
	}
	if connectErr2 != nil {
		t.Fatalf("second sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("second CONNECT: expected status %d, got %d", http.StatusOK, statusCode2)
	}

	// Also verify a static-config domain still works after the reload.
	done3 := make(chan struct{})
	var statusCode3 int
	var connectErr3 error
	go func() {
		defer close(done3)
		statusCode3, connectErr3 = h.sendCONNECT("api.anthropic.com")
	}()
	select {
	case <-done3:
	case <-time.After(3 * time.Second):
		t.Fatal("CONNECT to static-config domain should not block")
	}
	if connectErr3 != nil {
		t.Fatalf("static domain sendCONNECT() error: %v", connectErr3)
	}
	if statusCode3 != http.StatusOK {
		t.Errorf("static domain CONNECT: expected status %d, got %d", http.StatusOK, statusCode3)
	}
}

// TestProxyApproval_AllowProjectSurvivesRestart verifies that project-persisted
// domain approvals survive a full guardian restart. This simulates:
//  1. Approve domain with scope=project → decisions file written
//  2. Shut down guardian (stop proxy + approval server)
//  3. Start new guardian (new proxy + approval server, same config dir)
//  4. CONNECT to same domain → should be allowed without re-prompting
func TestProxyApproval_AllowProjectSurvivesRestart(t *testing.T) {
	// Phase 1: Create harness, approve domain at project scope.
	h1 := newProxyTestHarness(t)
	domain := "restart-test.example.com"
	configDir := h1.ConfigDir

	var statusCode int
	var connectErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		statusCode, connectErr = h1.sendCONNECT(domain)
	}()

	id, err := h1.pendingDomainID()
	if err != nil {
		t.Fatalf("pendingDomainID() error: %v", err)
	}

	if err := h1.approveDomain(id, "project", ""); err != nil {
		t.Fatalf("approveDomain() error: %v", err)
	}

	<-done
	if connectErr != nil {
		t.Fatalf("first session sendCONNECT() error: %v", connectErr)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("first session CONNECT: expected %d, got %d", http.StatusOK, statusCode)
	}

	// Verify decisions file was written.
	decisions, err := config.LoadProjectDecisions(h1.ProjectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error: %v", err)
	}
	found := false
	for _, d := range decisions.AllowedDomains() {
		if d == domain {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected domain %q in project decisions, got: %v", domain, decisions.AllowedDomains())
	}

	// Phase 2: Shut down first harness (simulates guardian stop).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h1.Proxy.Stop(ctx)
	_ = h1.ApprovalServer.Stop(ctx)

	// Phase 3: Create new harness reusing same config dir (simulates restart).
	h2 := newProxyTestHarnessWithConfigDir(t, configDir)

	// Phase 4: CONNECT to same domain — should be allowed immediately.
	done2 := make(chan struct{})
	var statusCode2 int
	var connectErr2 error
	go func() {
		defer close(done2)
		statusCode2, connectErr2 = h2.sendCONNECT(domain)
	}()

	select {
	case <-done2:
		// good — returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("CONNECT after restart should not block (project decisions should be loaded from disk)")
	}

	if connectErr2 != nil {
		t.Fatalf("post-restart sendCONNECT() error: %v", connectErr2)
	}
	if statusCode2 != http.StatusOK {
		t.Errorf("post-restart CONNECT: expected %d, got %d", http.StatusOK, statusCode2)
	}
}
