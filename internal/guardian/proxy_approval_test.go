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
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/guardian/approval"
)

// Ensure helpers used by later test phases don't trigger "unused" lint warnings.
var (
	_ = (*mockTunnelHandler).getCalls
	_ = (*proxyTestHarness).approveDomain
)

// mockTunnelHandler records CONNECT targets and completes the tunnel handshake.
type mockTunnelHandler struct {
	mu    sync.Mutex
	calls []string // records targetHostPort of each call
}

func (m *mockTunnelHandler) ServeTunnel(w http.ResponseWriter, r *http.Request, target string) {
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
	ConfigDir      string // t.TempDir() used as XDG_CONFIG_HOME
	Token          string
	ProjectName    string
	CloisterName   string
	TunnelHandler  *mockTunnelHandler
	t              *testing.T
}

func newProxyTestHarness(t *testing.T) *proxyTestHarness {
	t.Helper()

	configDir := t.TempDir()
	// Set XDG_CONFIG_HOME so config.DecisionDir() uses our temp dir
	t.Setenv("XDG_CONFIG_HOME", configDir)
	// Also set XDG_STATE_HOME to prevent writes to real state dir
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	token := "test-token-12345"
	projectName := "test-project"
	cloisterName := "test-project-main"

	// Create approval server components
	queue := approval.NewQueue()
	domainQueue := approval.NewDomainQueueWithTimeout(10 * time.Second)

	// Create config persister and allowlist cache
	globalAllowlist := NewAllowlist(nil) // empty -- no pre-allowed domains
	allowlistCache := NewAllowlistCache(globalAllowlist)

	persister := &ConfigPersisterImpl{}

	// Create approval server
	approvalSrv := approval.NewServer(queue, nil) // nil audit logger
	approvalSrv.SetDomainQueue(domainQueue)
	approvalSrv.ConfigPersister = persister
	approvalSrv.Addr = "127.0.0.1:0"

	// Create session lists
	sessionAllowlist := NewSessionAllowlist()
	sessionDenylist := NewSessionDenylist()

	// Create domain approver
	domainApprover := NewDomainApprover(domainQueue, sessionAllowlist, sessionDenylist, allowlistCache, nil)

	// Set up project allowlist/denylist loaders
	allowlistCache.SetProjectLoader(func(pName string) *Allowlist {
		decisions, err := config.LoadProjectDecisions(pName)
		if err != nil {
			return NewAllowlist(nil)
		}
		return NewAllowlistFromConfig(decisions.Proxy.Allow)
	})
	allowlistCache.SetDenylistLoader(func(pName string) *Allowlist {
		decisions, err := config.LoadProjectDecisions(pName)
		if err != nil {
			return NewAllowlist(nil)
		}
		return NewAllowlistFromConfig(decisions.Proxy.Deny)
	})

	// Create tunnel handler mock
	tunnelHandler := &mockTunnelHandler{}

	// Create token validator using the existing newMockTokenValidator helper
	tokenValidator := newMockTokenValidator(token)

	// Token lookup function
	tokenLookup := func(tok string) (TokenLookupResult, bool) {
		if tok == token {
			return TokenLookupResult{ProjectName: projectName, CloisterName: cloisterName}, true
		}
		return TokenLookupResult{}, false
	}

	// Create proxy server with empty global allowlist
	proxy := NewProxyServerWithConfig(":0", globalAllowlist)
	proxy.TokenValidator = tokenValidator
	proxy.AllowlistCache = allowlistCache
	proxy.TokenLookup = tokenLookup
	proxy.DomainApprover = domainApprover
	proxy.SessionAllowlist = sessionAllowlist
	proxy.SessionDenylist = sessionDenylist
	proxy.TunnelHandler = tunnelHandler

	// Set up ConfigPersister reload notifier
	persister.ReloadNotifier = func() {
		// Reload project allowlist cache after persistence
		decisions, err := config.LoadProjectDecisions(projectName)
		if err == nil {
			allowlistCache.SetProject(projectName, NewAllowlistFromConfig(decisions.Proxy.Allow))
			allowlistCache.SetProjectDeny(projectName, NewAllowlistFromConfig(decisions.Proxy.Deny))
		}
		// Reload global decisions
		globalDecisions, err := config.LoadGlobalDecisions()
		if err == nil {
			allowlistCache.SetGlobal(NewAllowlistFromConfig(globalDecisions.Proxy.Allow))
			allowlistCache.SetGlobalDeny(NewAllowlistFromConfig(globalDecisions.Proxy.Deny))
		}
	}

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
		ConfigDir:      configDir,
		Token:          token,
		ProjectName:    projectName,
		CloisterName:   cloisterName,
		TunnelHandler:  tunnelHandler,
		t:              t,
	}
}

// sendCONNECT sends an authenticated CONNECT request through the proxy
// and returns the HTTP status code from the proxy's response.
func (h *proxyTestHarness) sendCONNECT(domain string) (int, error) {
	h.t.Helper()

	proxyAddr := h.Proxy.ListenAddr()
	conn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
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
		resp, err := http.Get(url)
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

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
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

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
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
