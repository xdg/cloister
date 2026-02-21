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
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/token"
)

// mockTokenValidator is a simple TokenValidator for testing.
type mockTokenValidator struct {
	validTokens map[string]bool
}

func newMockTokenValidator(tokens ...string) *mockTokenValidator {
	v := &mockTokenValidator{validTokens: make(map[string]bool)}
	for _, t := range tokens {
		v.validTokens[t] = true
	}
	return v
}

func (v *mockTokenValidator) Validate(token string) bool {
	return v.validTokens[token]
}

// mockPolicyChecker is a lightweight PolicyChecker for proxy tests that
// delegates to a caller-provided function.
type mockPolicyChecker struct {
	checkFunc func(token, project, domain string) Decision
}

func (m *mockPolicyChecker) Check(token, project, domain string) Decision {
	return m.checkFunc(token, project, domain)
}

// newTestProxyPolicyEngine creates a PolicyEngine for proxy tests with
// the given global allow domains. No project lister or file loaders are
// configured (session/project/global reload not needed for basic proxy tests).
func newTestProxyPolicyEngine(allowDomains, denyDomains []string) *PolicyEngine { //nolint:unparam // denyDomains used by callers constructing deny-specific scenarios
	return &PolicyEngine{
		global: ProxyPolicy{
			Allow: NewDomainSet(allowDomains, nil),
			Deny:  NewDomainSet(denyDomains, nil),
		},
		projects: make(map[string]*ProxyPolicy),
		tokens:   make(map[string]*ProxyPolicy),
	}
}

func TestNewProxyServer(t *testing.T) {
	t.Run("default address", func(t *testing.T) {
		p := NewProxyServer("")
		if p.Addr != ":3128" {
			t.Errorf("expected default address :3128, got %s", p.Addr)
		}
	})

	t.Run("custom address", func(t *testing.T) {
		p := NewProxyServer(":8080")
		if p.Addr != ":8080" {
			t.Errorf("expected address :8080, got %s", p.Addr)
		}
	})
}

func TestProxyServer_StartStop(t *testing.T) {
	p := NewProxyServer(":0") // Use random available port

	// Start the server
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}

	// Verify server is listening
	addr := p.ListenAddr()
	if addr == "" {
		t.Fatal("expected non-empty listen address")
	}

	// Starting again should fail
	if err := p.Start(); err == nil {
		t.Error("expected error when starting already running server")
	}

	// Stop the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Stop(ctx); err != nil {
		t.Fatalf("failed to stop proxy server: %v", err)
	}

	// Stopping again should be idempotent
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("expected no error when stopping already stopped server: %v", err)
	}
}

func TestProxyServer_ConnectMethod(t *testing.T) {
	// Start a mock upstream
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, echoHandler)
	defer cleanupUpstream()

	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	addr := p.ListenAddr()

	// CONNECT request should succeed (200 OK) for allowed domain
	t.Run("CONNECT returns 200 for allowed domain", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodConnect, fmt.Sprintf("http://%s", addr), http.NoBody)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Host = upstreamAddr

		client := noProxyClient()
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})
}

func TestProxyServer_NonConnectMethods(t *testing.T) {
	p := NewProxyServer(":0")
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	addr := p.ListenAddr()
	baseURL := fmt.Sprintf("http://%s", addr)

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method+" returns 405", func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), method, baseURL, http.NoBody)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405, got %d", resp.StatusCode)
			}

			// Verify response body contains informative message (except HEAD, which has no body)
			if method != http.MethodHead {
				body, _ := io.ReadAll(resp.Body)
				if len(body) == 0 {
					t.Error("expected non-empty error message in response body")
				}
			}
		})
	}
}

func TestProxyServer_ListenAddr(t *testing.T) {
	p := NewProxyServer(":0")

	// Before starting, should return empty string
	if addr := p.ListenAddr(); addr != "" {
		t.Errorf("expected empty address before start, got %s", addr)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	// After starting, should return actual address with port
	addr := p.ListenAddr()
	if addr == "" {
		t.Error("expected non-empty address after start")
	}
}

func TestProxyServer_TunnelEstablishment(t *testing.T) {
	// Start a mock upstream that echoes data
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, echoHandler)
	defer cleanupUpstream()

	// Extract host from upstream address for allowlist
	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	// Start proxy with a PolicyEngine that includes our mock upstream
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	t.Run("tunnel establishment and bidirectional copy", func(t *testing.T) {
		// Connect to proxy
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
		if err != nil {
			t.Fatalf("failed to connect to proxy: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Send CONNECT request
		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamAddr, upstreamAddr)
		_, err = conn.Write([]byte(connectReq))
		if err != nil {
			t.Fatalf("failed to send CONNECT request: %v", err)
		}

		// Read response
		reader := bufio.NewReader(conn)
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read status line: %v", err)
		}

		if !strings.Contains(statusLine, "200 Connection Established") {
			t.Fatalf("expected 200 Connection Established, got: %s", statusLine)
		}

		// Read remaining headers (empty line)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("failed to read headers: %v", err)
			}
			if line == "\r\n" {
				break
			}
		}

		// Now tunnel is established - send data through it
		testData := "Hello through the tunnel!"
		_, err = conn.Write([]byte(testData))
		if err != nil {
			t.Fatalf("failed to send data through tunnel: %v", err)
		}

		// Read echoed response
		response := make([]byte, len(testData))
		_, err = io.ReadFull(reader, response)
		if err != nil {
			t.Fatalf("failed to read echoed data: %v", err)
		}

		if string(response) != testData {
			t.Errorf("expected echoed data %q, got %q", testData, string(response))
		}
	})

	t.Run("tunnel handles multiple round trips", func(t *testing.T) {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
		if err != nil {
			t.Fatalf("failed to connect to proxy: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Establish tunnel
		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamAddr, upstreamAddr)
		_, _ = conn.Write([]byte(connectReq))

		reader := bufio.NewReader(conn)
		// Skip to empty line (end of headers)
		for {
			line, _ := reader.ReadString('\n')
			if line == "\r\n" {
				break
			}
		}

		// Send multiple messages and verify echo
		messages := []string{"First message", "Second message", "Third message"}
		for _, msg := range messages {
			_, err = conn.Write([]byte(msg))
			if err != nil {
				t.Fatalf("failed to send message: %v", err)
			}

			response := make([]byte, len(msg))
			_, err = io.ReadFull(reader, response)
			if err != nil {
				t.Fatalf("failed to read response: %v", err)
			}

			if string(response) != msg {
				t.Errorf("expected %q, got %q", msg, string(response))
			}
		}
	})
}

func TestProxyServer_TunnelBlockedDomain(t *testing.T) {
	p := NewProxyServer(":0")
	// PolicyEngine with no domains allowed — all requests should be denied or AskHuman.
	p.PolicyEngine = newTestProxyPolicyEngine(nil, nil)
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Try to CONNECT to a non-allowed domain
	connectReq := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "403") {
		t.Errorf("expected 403 Forbidden, got: %s", statusLine)
	}
}

func TestProxyServer_TunnelUpstreamConnectionFailure(t *testing.T) {
	p := NewProxyServer(":0")
	// Allow localhost but try to connect to a port that's not listening
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"127.0.0.1"}, nil)
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Try to CONNECT to a port that's definitely not listening
	connectReq := "CONNECT 127.0.0.1:59999 HTTP/1.1\r\nHost: 127.0.0.1:59999\r\n\r\n"
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "502") {
		t.Errorf("expected 502 Bad Gateway, got: %s", statusLine)
	}
}

func TestProxyServer_TunnelConnectionTimeout(t *testing.T) {
	// Start a listener that accepts connections but never responds (simulates timeout)
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start slow upstream: %v", err)
	}
	defer func() { _ = listener.Close() }()

	slowAddr := listener.Addr().String()
	slowHost, _, _ := net.SplitHostPort(slowAddr)

	// Accept connections but don't do anything (let them hang)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// Hold the connection open but never respond
			// It will be closed when the test ends
			go func(c net.Conn) {
				// Read to prevent RST
				buf := make([]byte, 1024)
				for {
					_, err := c.Read(buf)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{slowHost}, nil)

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	// Note: This test uses the real dialTimeout (30s) which is too slow for unit tests.
	// We verify the timeout handling logic works, but use connection refused for quick tests.
	// To actually test timeout behavior, you would need to temporarily reduce dialTimeout
	// or use a test-specific dialer. For now, we verify the isTimeoutError function directly.

	// Test isTimeoutError helper function
	t.Run("isTimeoutError helper", func(t *testing.T) {
		// nil error should return false
		if isTimeoutError(nil) {
			t.Error("expected isTimeoutError(nil) to return false")
		}

		// Regular error should return false
		regularErr := fmt.Errorf("some error")
		if isTimeoutError(regularErr) {
			t.Error("expected isTimeoutError for regular error to return false")
		}

		// Timeout error should return true
		timeoutErr := &net.OpError{
			Err: &timeoutError{},
		}
		if !isTimeoutError(timeoutErr) {
			t.Error("expected isTimeoutError for timeout error to return true")
		}
	})
}

// timeoutError implements net.Error with Timeout() returning true
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestProxyServer_TunnelIdleTimeout(t *testing.T) {
	// Test that idle connections are properly closed after the timeout.
	// This is a unit test for the copyWithIdleTimeout function behavior.

	t.Run("copyWithIdleTimeout closes on idle", func(t *testing.T) {
		// Create two pipes to simulate the bidirectional tunnel
		// srcRead/srcWrite is the "source" connection (client side)
		// dstRead/dstWrite is the "destination" connection (server side)
		srcRead, srcWrite := net.Pipe()
		dstRead, dstWrite := net.Pipe()
		defer func() { _ = srcRead.Close() }()
		defer func() { _ = srcWrite.Close() }()
		defer func() { _ = dstRead.Close() }()
		defer func() { _ = dstWrite.Close() }()

		// Use a very short timeout for testing
		shortTimeout := 100 * time.Millisecond

		done := make(chan struct{})
		go func() {
			// Copy from srcRead to dstWrite
			copyWithIdleTimeout(dstWrite, srcRead, shortTimeout)
			close(done)
		}()

		// Don't send any data - the copy should exit due to idle timeout
		select {
		case <-done:
			// Success - copy exited due to timeout
		case <-time.After(2 * time.Second):
			t.Error("copyWithIdleTimeout did not exit after idle timeout")
		}
	})

	t.Run("copyWithIdleTimeout resets on activity", func(t *testing.T) {
		srcRead, srcWrite := net.Pipe()
		dstRead, dstWrite := net.Pipe()
		defer func() { _ = srcRead.Close() }()
		defer func() { _ = srcWrite.Close() }()
		defer func() { _ = dstRead.Close() }()
		defer func() { _ = dstWrite.Close() }()

		shortTimeout := 100 * time.Millisecond

		done := make(chan struct{})
		go func() {
			// Copy from srcRead to dstWrite
			copyWithIdleTimeout(dstWrite, srcRead, shortTimeout)
			close(done)
		}()

		// Send data at intervals less than the timeout to keep connection alive
		for i := 0; i < 3; i++ {
			time.Sleep(50 * time.Millisecond)
			// Write to the source write end
			_, err := srcWrite.Write([]byte("ping"))
			if err != nil {
				t.Fatalf("failed to write: %v", err)
			}
			// Read from the destination read end
			buf := make([]byte, 4)
			_, err = dstRead.Read(buf)
			if err != nil {
				t.Fatalf("failed to read: %v", err)
			}
		}

		// Now stop sending - should timeout
		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Error("copyWithIdleTimeout did not exit after activity stopped")
		}
	})
}

func TestProxyServer_TunnelCleanShutdown(t *testing.T) {
	// Test that tunnel connections clean up properly when upstream closes
	var upstreamClosed bool
	var mu sync.Mutex

	handler := func(conn net.Conn) {
		// Read one message, echo it back, then close
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		_, _ = conn.Write(buf[:n])
		_ = conn.Close()
		mu.Lock()
		upstreamClosed = true
		mu.Unlock()
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, handler)
	defer cleanupUpstream()

	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Establish tunnel
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamAddr, upstreamAddr)
	_, _ = conn.Write([]byte(connectReq))

	reader := bufio.NewReader(conn)
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" {
			break
		}
	}

	// Send data
	_, _ = conn.Write([]byte("test"))

	// Read echoed response
	response := make([]byte, 4)
	_, _ = io.ReadFull(reader, response)

	// Wait a bit and verify upstream closed
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if !upstreamClosed {
		t.Error("expected upstream to have closed")
	}
	mu.Unlock()

	// Further reads should return EOF or error
	_, err = reader.ReadByte()
	if err == nil {
		t.Error("expected read to fail after upstream close")
	}
}

func TestProxyServer_Authentication(t *testing.T) {
	// Start a mock upstream for successful auth tests
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, echoHandler)
	defer cleanupUpstream()

	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	tests := []struct {
		name           string
		authHeader     string
		validTokens    []string
		expectedStatus int
	}{
		{
			name:           "valid token succeeds",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("user:valid-token-123")),
			validTokens:    []string{"valid-token-123"},
			expectedStatus: 200,
		},
		{
			name:           "missing auth header returns 407",
			authHeader:     "",
			validTokens:    []string{"some-token"},
			expectedStatus: 407,
		},
		{
			name:           "invalid token returns 407",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("user:wrong-token")),
			validTokens:    []string{"valid-token"},
			expectedStatus: 407,
		},
		{
			name:           "malformed auth header returns 407",
			authHeader:     "Bearer some-token",
			validTokens:    []string{"some-token"},
			expectedStatus: 407,
		},
		{
			name:           "invalid base64 returns 407",
			authHeader:     "Basic not-valid-base64!!!",
			validTokens:    []string{"some-token"},
			expectedStatus: 407,
		},
		{
			name:           "missing colon in credentials returns 407",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("no-colon-here")),
			validTokens:    []string{"no-colon-here"},
			expectedStatus: 407,
		},
		{
			name:           "empty password returns 407 when token required",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("user:")),
			validTokens:    []string{"valid-token"},
			expectedStatus: 407,
		},
		{
			name:           "empty username with valid token succeeds",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte(":valid-token")),
			validTokens:    []string{"valid-token"},
			expectedStatus: 200,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Capture clog output for verifying auth failure logging
			var logBuf bytes.Buffer
			testLogger := clog.TestLogger(&logBuf)
			oldLogger := clog.ReplaceGlobal(testLogger)
			defer clog.ReplaceGlobal(oldLogger)

			// Create proxy with token validator
			p := NewProxyServer(":0")
			p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
			p.TokenValidator = newMockTokenValidator(tc.validTokens...)

			if err := p.Start(); err != nil {
				t.Fatalf("failed to start proxy server: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = p.Stop(ctx)
			}()

			proxyAddr := p.ListenAddr()

			// Connect to proxy
			conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
			if err != nil {
				t.Fatalf("failed to connect to proxy: %v", err)
			}
			defer func() { _ = conn.Close() }()

			// Build CONNECT request with optional auth header
			var reqBuilder strings.Builder
			reqBuilder.WriteString(fmt.Sprintf("CONNECT %s HTTP/1.1\r\n", upstreamAddr))
			reqBuilder.WriteString(fmt.Sprintf("Host: %s\r\n", upstreamAddr))
			if tc.authHeader != "" {
				reqBuilder.WriteString(fmt.Sprintf("Proxy-Authorization: %s\r\n", tc.authHeader))
			}
			reqBuilder.WriteString("\r\n")

			_, err = conn.Write([]byte(reqBuilder.String()))
			if err != nil {
				t.Fatalf("failed to send CONNECT request: %v", err)
			}

			// Read response
			reader := bufio.NewReader(conn)
			statusLine, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("failed to read status line: %v", err)
			}

			// Extract status code from response
			var statusCode int
			_, err = fmt.Sscanf(statusLine, "HTTP/1.1 %d", &statusCode)
			if err != nil {
				t.Fatalf("failed to parse status line: %v (line: %q)", err, statusLine)
			}

			if statusCode != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, statusCode)
			}

			// For 407 responses, verify Proxy-Authenticate header
			if tc.expectedStatus == 407 {
				// Read headers to find Proxy-Authenticate
				foundProxyAuth := false
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					if line == "\r\n" {
						break
					}
					if strings.HasPrefix(line, "Proxy-Authenticate:") {
						foundProxyAuth = true
						if !strings.Contains(line, `Basic realm="cloister"`) {
							t.Errorf("expected Proxy-Authenticate header with realm, got: %s", line)
						}
					}
				}
				if !foundProxyAuth {
					t.Error("expected Proxy-Authenticate header in 407 response")
				}

				// Verify logging occurred
				logOutput := logBuf.String()
				if !strings.Contains(logOutput, "proxy auth failure") {
					t.Errorf("expected auth failure to be logged, got: %s", logOutput)
				}
			}
		})
	}
}

func TestProxyServer_NoTokenValidatorAllowsAll(t *testing.T) {
	// When TokenValidator is nil, all requests should be allowed (no auth required)
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, echoHandler)
	defer cleanupUpstream()

	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	// Explicitly no TokenValidator set

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	// Connect without any auth header
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamAddr, upstreamAddr)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "200") {
		t.Errorf("expected 200 OK without TokenValidator, got: %s", statusLine)
	}
}

func TestProxyServer_AuthWithTunnelData(t *testing.T) {
	// Verify that after successful auth, the tunnel works correctly
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, echoHandler)
	defer cleanupUpstream()

	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	p.TokenValidator = newMockTokenValidator("my-secret-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send authenticated CONNECT request
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:my-secret-token"))
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
		upstreamAddr, upstreamAddr, authHeader)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)

	// Read and verify 200 response
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}
	if !strings.Contains(statusLine, "200") {
		t.Fatalf("expected 200 OK, got: %s", statusLine)
	}

	// Skip headers
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" {
			break
		}
	}

	// Send data through tunnel
	testData := "Hello authenticated tunnel!"
	_, err = conn.Write([]byte(testData))
	if err != nil {
		t.Fatalf("failed to send data: %v", err)
	}

	// Read echo
	response := make([]byte, len(testData))
	_, err = io.ReadFull(reader, response)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if string(response) != testData {
		t.Errorf("expected %q, got %q", testData, string(response))
	}
}

func TestProxyServer_PolicyEngineDerivedAllowlist(t *testing.T) {
	// Start a mock upstream
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, echoHandler)
	defer cleanupUpstream()

	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	// Create proxy with PolicyEngine
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	// Test that PolicyEngine-derived domain check works
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// CONNECT to allowed host
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamAddr, upstreamAddr)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "200") {
		t.Errorf("expected 200 OK for allowed host, got: %s", statusLine)
	}
}

func TestProxyServer_HandleSighupNoopWithoutPolicyEngine(_ *testing.T) {
	// handleSighup with nil PolicyEngine does nothing.
	p := NewProxyServer(":0")
	// No PolicyEngine set — handleSighup should be a no-op (no panic).
	p.handleSighup()
}

func TestProxyServer_PolicyEngineSighup(t *testing.T) {
	// Test that handleSighup calls PolicyEngine.ReloadAll when set.

	t.Run("handleSighup calls PolicyEngine.ReloadAll", func(t *testing.T) {
		configDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configDir)
		t.Setenv("XDG_STATE_HOME", t.TempDir())

		pe := newTestProxyPolicyEngine([]string{"initial.example.com"}, nil)
		// Override loaders to return updated config on reload.
		pe.configLoader = func() (*config.GlobalConfig, error) {
			return &config.GlobalConfig{}, nil
		}
		pe.decisionLoader = func() (*config.Decisions, error) {
			return &config.Decisions{
				Proxy: config.DecisionsProxy{
					Allow: []config.AllowEntry{{Domain: "reloaded.example.com"}},
				},
			}, nil
		}

		p := NewProxyServer(":0")
		p.PolicyEngine = pe

		// Manually call handleSighup
		p.handleSighup()

		// Verify PolicyEngine was reloaded: reloaded domain should now be allowed
		// (global policy rebuilt from mocked loaders).
		if pe.Check("", "", "reloaded.example.com") != Allow {
			t.Error("reloaded domain should be allowed after SIGHUP reload")
		}
	})

	t.Run("handleSighup calls OnReload after successful reload", func(t *testing.T) {
		pe := newTestProxyPolicyEngine([]string{"initial.example.com"}, nil)
		pe.configLoader = func() (*config.GlobalConfig, error) {
			return &config.GlobalConfig{}, nil
		}
		pe.decisionLoader = func() (*config.Decisions, error) {
			return &config.Decisions{}, nil
		}

		p := NewProxyServer(":0")
		p.PolicyEngine = pe

		onReloadCalled := false
		p.OnReload = func() {
			onReloadCalled = true
		}

		p.handleSighup()

		if !onReloadCalled {
			t.Error("OnReload callback should have been called after successful reload")
		}
	})

	t.Run("handleSighup does not call OnReload on error", func(t *testing.T) {
		var logBuf bytes.Buffer
		testLogger := clog.TestLogger(&logBuf)
		oldLogger := clog.ReplaceGlobal(testLogger)
		defer clog.ReplaceGlobal(oldLogger)

		pe := newTestProxyPolicyEngine([]string{"original.example.com"}, nil)
		pe.configLoader = func() (*config.GlobalConfig, error) {
			return nil, fmt.Errorf("disk error")
		}

		p := NewProxyServer(":0")
		p.PolicyEngine = pe

		onReloadCalled := false
		p.OnReload = func() {
			onReloadCalled = true
		}

		p.handleSighup()

		if onReloadCalled {
			t.Error("OnReload callback should not be called when reload fails")
		}
	})

	t.Run("handleSighup logs PolicyEngine reload error", func(t *testing.T) {
		var logBuf bytes.Buffer
		testLogger := clog.TestLogger(&logBuf)
		oldLogger := clog.ReplaceGlobal(testLogger)
		defer clog.ReplaceGlobal(oldLogger)

		pe := newTestProxyPolicyEngine([]string{"original.example.com"}, nil)
		pe.configLoader = func() (*config.GlobalConfig, error) {
			return nil, fmt.Errorf("disk error")
		}

		p := NewProxyServer(":0")
		p.PolicyEngine = pe

		p.handleSighup()

		// Original policy should remain unchanged (ReloadAll failed).
		if pe.Check("", "", "original.example.com") != Allow {
			t.Error("original domain should still be allowed after failed reload")
		}

		// Error should be logged
		if !strings.Contains(logBuf.String(), "PolicyEngine reload failed") {
			t.Errorf("expected PolicyEngine reload error to be logged, got: %s", logBuf.String())
		}
	})
}

func TestProxyServer_SighupReloadsTokensFromDisk(t *testing.T) {
	// When a token file is manually deleted from disk and reload (SIGHUP) is
	// triggered, the in-memory registry should revoke tokens that no longer
	// have backing files on disk.

	tokenDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a token store and persist a token.
	store, err := token.NewStore(tokenDir)
	if err != nil {
		t.Fatalf("failed to create token store: %v", err)
	}
	if err := store.SaveFull("test-cloister", "tok-secret-123", "myproject", "/work"); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Load tokens into a registry (simulates guardian startup).
	registry := token.NewRegistry()
	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load tokens: %v", err)
	}
	for tok, info := range tokens {
		registry.RegisterFull(tok, info.CloisterName, info.ProjectName, info.WorktreePath)
	}

	// Sanity: token is valid before deletion.
	if !registry.Validate("tok-secret-123") {
		t.Fatal("token should be valid before deletion")
	}

	// Delete the token file from disk (simulates manual cleanup).
	if err := store.Remove("test-cloister"); err != nil {
		t.Fatalf("failed to remove token file: %v", err)
	}

	// Build a PolicyEngine that uses this registry as its project lister.
	pe := newTestProxyPolicyEngine([]string{"example.com"}, nil)
	pe.projectLister = registry
	pe.configLoader = func() (*config.GlobalConfig, error) {
		return &config.GlobalConfig{}, nil
	}
	pe.decisionLoader = func() (*config.Decisions, error) {
		return &config.Decisions{}, nil
	}

	// Wire up the proxy server and trigger reload.
	p := NewProxyServer(":0")
	p.PolicyEngine = pe
	p.TokenValidator = registry
	p.OnTokenReload = func() {
		if err := token.ReconcileWithStore(registry, store); err != nil {
			t.Errorf("token reconciliation failed: %v", err)
		}
	}

	p.handleSighup()

	// After reload, the deleted token should no longer be valid.
	if registry.Validate("tok-secret-123") {
		t.Error("token deleted from disk should be revoked from registry after SIGHUP reload")
	}
}

func TestProxyServer_PerProjectAllowlist(t *testing.T) {
	// Start a mock upstream for testing allowed connections
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}

	upstreamAddr, cleanup := startMockUpstream(t, echoHandler)
	defer cleanup()
	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	// Create PolicyEngine with per-project policies.
	pe := newTestProxyPolicyEngine([]string{"global.example.com"}, nil)
	// Project A can access the upstream host AND project-a-domain.com
	pe.projects["project-a"] = &ProxyPolicy{
		Allow: NewDomainSet([]string{upstreamHost, "project-a-domain.com"}, nil),
	}
	// Project B can only access project-b-domain.com (NOT the upstream host)
	pe.projects["project-b"] = &ProxyPolicy{
		Allow: NewDomainSet([]string{"project-b-domain.com"}, nil),
	}

	// Token lookup function
	tokenLookup := func(token string) (TokenLookupResult, bool) {
		switch token {
		case "token-a":
			return TokenLookupResult{ProjectName: "project-a"}, true
		case "token-b":
			return TokenLookupResult{ProjectName: "project-b"}, true
		default:
			return TokenLookupResult{}, false
		}
	}

	// Create proxy with per-project support via PolicyEngine
	p := NewProxyServer(":0")
	p.PolicyEngine = pe
	p.TokenLookup = tokenLookup
	p.TokenValidator = newMockTokenValidator("token-a", "token-b")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	// Helper to make proxy request
	makeRequest := func(token, targetAddr string) (int, error) {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
		if err != nil {
			return 0, err
		}
		defer func() { _ = conn.Close() }()

		authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:"+token))
		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
			targetAddr, targetAddr, authHeader)
		_, err = conn.Write([]byte(connectReq))
		if err != nil {
			return 0, err
		}

		reader := bufio.NewReader(conn)
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}

		var statusCode int
		_, err = fmt.Sscanf(statusLine, "HTTP/1.1 %d", &statusCode)
		return statusCode, err
	}

	// Test: Project A can access the upstream (200 = tunnel established)
	status, err := makeRequest("token-a", upstreamAddr)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if status != 200 {
		t.Errorf("project-a to upstream: expected 200, got %d", status)
	}

	// Test: Project B cannot access the upstream (403 = forbidden)
	status, err = makeRequest("token-b", upstreamAddr)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if status != 403 {
		t.Errorf("project-b to upstream: expected 403, got %d", status)
	}

	// Test: Project A cannot access a domain not in its allowlist
	status, err = makeRequest("token-a", "blocked-domain.com:443")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if status != 403 {
		t.Errorf("project-a to blocked domain: expected 403, got %d", status)
	}

	// Test: With PolicyEngine, global allow is checked across all tiers.
	// Global domain IS accessible because global allow matches.
	status, err = makeRequest("token-a", "global.example.com:443")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	// PolicyEngine allows global domains for all projects (global allow pass).
	// This returns 502 because there's no real upstream, but NOT 403.
	if status == 403 {
		t.Errorf("project-a to global domain: should not get 403 (global allow matches)")
	}
}

// mockDomainApprover is a test implementation of DomainApprover.
type mockDomainApprover struct {
	approveFunc func(project, cloister, domain, token string) (DomainApprovalResult, error)
	callCount   int
	mu          sync.Mutex
}

func (m *mockDomainApprover) RequestApproval(project, cloister, domain, token string) (DomainApprovalResult, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	if m.approveFunc != nil {
		return m.approveFunc(project, cloister, domain, token)
	}
	return DomainApprovalResult{Approved: false}, nil
}

func (m *mockDomainApprover) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestProxyServer_DomainApproval_NilApproverRejects(t *testing.T) {
	// Test that nil DomainApprover returns 403 for unlisted domains (AskHuman path)
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"allowed.com"}, nil)
	// DomainApprover is nil by default

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Try to connect to unlisted domain
	connectReq := "CONNECT unlisted.com:443 HTTP/1.1\r\nHost: unlisted.com:443\r\n\r\n"
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "403") {
		t.Errorf("expected 403 Forbidden for unlisted domain with nil approver, got: %s", statusLine)
	}
}

func TestProxyServer_DomainApproval_ApprovalAllowsConnection(t *testing.T) {
	// Test that when DomainApprover approves, connection proceeds
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanup := startMockUpstream(t, echoHandler)
	defer cleanup()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine(nil, nil) // Empty — all domains go to AskHuman
	p.DomainApprover = &mockDomainApprover{
		approveFunc: func(_, _, _, _ string) (DomainApprovalResult, error) {
			return DomainApprovalResult{Approved: true, Scope: "session"}, nil
		},
	}
	p.TokenLookup = func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send CONNECT request with auth
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:test-token"))
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
		upstreamAddr, upstreamAddr, authHeader)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "200") {
		t.Errorf("expected 200 Connection Established after approval, got: %s", statusLine)
	}
}

func TestProxyServer_DomainApproval_DenialReturns403(t *testing.T) {
	// Test that when DomainApprover denies, returns 403
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine(nil, nil) // Empty — all domains go to AskHuman
	p.DomainApprover = &mockDomainApprover{
		approveFunc: func(_, _, _, _ string) (DomainApprovalResult, error) {
			return DomainApprovalResult{Approved: false}, nil
		},
	}
	p.TokenLookup = func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send CONNECT request with auth
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:test-token"))
	connectReq := "CONNECT denied.com:443 HTTP/1.1\r\nHost: denied.com:443\r\nProxy-Authorization: " + authHeader + "\r\n\r\n"
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "403") {
		t.Errorf("expected 403 Forbidden after denial, got: %s", statusLine)
	}
}

func TestProxyServer_DomainApproval_SessionAllowlistBypass(t *testing.T) {
	// Test that token-level allow in PolicyEngine bypasses DomainApprover entirely
	approver := &mockDomainApprover{
		approveFunc: func(_, _, _, _ string) (DomainApprovalResult, error) {
			t.Error("DomainApprover should not be called when token allow matches")
			return DomainApprovalResult{Approved: false}, nil
		},
	}

	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanup := startMockUpstream(t, echoHandler)
	defer cleanup()
	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	// Create PolicyEngine with empty global allow, but add token-level allow for the upstream.
	pe := newTestProxyPolicyEngine(nil, nil)
	pe.tokens["test-token"] = &ProxyPolicy{
		Allow: NewDomainSet([]string{upstreamHost}, nil),
	}

	p := NewProxyServer(":0")
	p.PolicyEngine = pe
	p.DomainApprover = approver
	p.TokenLookup = func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send CONNECT request with auth
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:test-token"))
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
		upstreamAddr, upstreamAddr, authHeader)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "200") {
		t.Errorf("expected 200 Connection Established via session allowlist, got: %s", statusLine)
	}

	// Verify approver was not called
	if approver.CallCount() != 0 {
		t.Errorf("expected DomainApprover to not be called, but it was called %d times", approver.CallCount())
	}
}

func TestProxyServer_DomainApproval_SingleTokenLookup(t *testing.T) {
	// Test that TokenLookup is only called once per request (via resolveRequest)
	lookupCount := 0
	var mu sync.Mutex

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine(nil, nil) // Empty — all domains go to AskHuman
	p.DomainApprover = &mockDomainApprover{
		approveFunc: func(project, _, _, _ string) (DomainApprovalResult, error) {
			// Verify project name was extracted correctly
			if project != "test-project" {
				t.Errorf("expected project 'test-project', got '%s'", project)
			}
			return DomainApprovalResult{Approved: false}, nil
		},
	}

	// Configure per-project support with counting TokenLookup
	p.TokenLookup = func(token string) (TokenLookupResult, bool) {
		mu.Lock()
		lookupCount++
		mu.Unlock()
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send CONNECT request with auth
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:test-token"))
	connectReq := "CONNECT unlisted.com:443 HTTP/1.1\r\nHost: unlisted.com:443\r\nProxy-Authorization: " + authHeader + "\r\n\r\n"
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	// Read response (should be 403)
	reader := bufio.NewReader(conn)
	_, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	// Verify TokenLookup was called exactly once (in resolveRequest)
	mu.Lock()
	count := lookupCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected TokenLookup to be called exactly 1 time, but was called %d times", count)
	}
}

func TestProxyServer_DomainApproval_EmptyHostRejected(t *testing.T) {
	// Test that empty host is rejected early
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine(nil, nil)

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send CONNECT request with empty host
	connectReq := "CONNECT HTTP/1.1\r\n\r\n"
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "400") {
		t.Errorf("expected 400 Bad Request for empty host, got: %s", statusLine)
	}
}

func TestProxyServer_DenylistPrecedence_StaticDenyOverridesAllowlist(t *testing.T) {
	// Denied domain in global deny blocks request even if in project allow.

	// Start a mock upstream for the non-denied domain test case.
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanup := startMockUpstream(t, echoHandler)
	defer cleanup()
	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	// Create PolicyEngine with global allow and deny.
	pe := &PolicyEngine{
		global: ProxyPolicy{
			Allow: NewDomainSet([]string{"denied-but-allowed.example.com", upstreamHost}, nil),
			Deny:  NewDomainSet([]string{"denied-but-allowed.example.com"}, nil),
		},
		projects: map[string]*ProxyPolicy{
			"test-project": {
				Allow: NewDomainSet([]string{"denied-but-allowed.example.com", upstreamHost}, nil),
			},
		},
		tokens: make(map[string]*ProxyPolicy),
	}

	tokenLookup := func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}

	p := NewProxyServer(":0")
	p.PolicyEngine = pe
	p.TokenLookup = tokenLookup
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	makeRequest := func(token, targetAddr string) (int, error) {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
		if err != nil {
			return 0, err
		}
		defer func() { _ = conn.Close() }()

		authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:"+token))
		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
			targetAddr, targetAddr, authHeader)
		_, err = conn.Write([]byte(connectReq))
		if err != nil {
			return 0, err
		}

		reader := bufio.NewReader(conn)
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}

		var statusCode int
		_, err = fmt.Sscanf(statusLine, "HTTP/1.1 %d", &statusCode)
		return statusCode, err
	}

	// Denied domain should be blocked (403) even though it's in the project allowlist.
	t.Run("denied domain blocked despite allowlist", func(t *testing.T) {
		status, err := makeRequest("test-token", "denied-but-allowed.example.com:443")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if status != 403 {
			t.Errorf("expected 403 for denied domain, got %d", status)
		}
	})

	// Non-denied domain in the allowlist should succeed (200 via mock upstream).
	t.Run("non-denied domain allowed", func(t *testing.T) {
		status, err := makeRequest("test-token", upstreamAddr)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if status != 200 {
			t.Errorf("expected 200 for non-denied domain, got %d", status)
		}
	})
}

func TestProxyServer_DenylistPrecedence_SessionDenyBlocks(t *testing.T) {
	// Token-level deny blocks request even when token-level allow has
	// the same domain for the same token.

	approver := &mockDomainApprover{
		approveFunc: func(_, _, _, _ string) (DomainApprovalResult, error) {
			t.Error("DomainApprover should not be called when token deny blocks")
			return DomainApprovalResult{Approved: true, Scope: "session"}, nil
		},
	}

	// Create PolicyEngine with token-level deny AND allow for the same domain.
	pe := newTestProxyPolicyEngine(nil, nil)
	pe.tokens["test-token"] = &ProxyPolicy{
		Allow: NewDomainSet([]string{"denied-session.example.com"}, nil),
		Deny:  NewDomainSet([]string{"denied-session.example.com"}, nil),
	}

	tokenLookup := func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}

	p := NewProxyServer(":0")
	p.PolicyEngine = pe
	p.TokenLookup = tokenLookup
	p.TokenValidator = newMockTokenValidator("test-token")
	p.DomainApprover = approver

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:test-token"))
	connectReq := fmt.Sprintf("CONNECT denied-session.example.com:443 HTTP/1.1\r\nHost: denied-session.example.com:443\r\nProxy-Authorization: %s\r\n\r\n", authHeader)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		t.Fatalf("failed to send CONNECT request: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "403") {
		t.Errorf("expected 403 Forbidden for session-denied domain, got: %s", statusLine)
	}

	// Verify DomainApprover was never called (session deny short-circuits).
	if approver.CallCount() != 0 {
		t.Errorf("expected DomainApprover to not be called, but it was called %d times", approver.CallCount())
	}
}

func TestProxyServer_DenylistPrecedence_DenyPatternBlocksSubdomain(t *testing.T) {
	// Denied pattern blocks matching subdomain even if the exact domain
	// is in the allowlist. Non-denied domains on the allowlist still work.

	// Start a mock upstream for the positive control test
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanup := startMockUpstream(t, echoHandler)
	defer cleanup()
	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	// Create PolicyEngine with global allow (including denied subdomain) and
	// global deny pattern for *.evil.com.
	pe := &PolicyEngine{
		global: ProxyPolicy{
			Allow: NewDomainSet([]string{"api.evil.com", upstreamHost}, nil),
			Deny:  NewDomainSet(nil, []string{"*.evil.com"}),
		},
		projects: map[string]*ProxyPolicy{
			"test-project": {
				Allow: NewDomainSet([]string{"api.evil.com", upstreamHost}, nil),
			},
		},
		tokens: make(map[string]*ProxyPolicy),
	}

	tokenLookup := func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}

	p := NewProxyServer(":0")
	p.PolicyEngine = pe
	p.TokenLookup = tokenLookup
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:test-token"))

	makeRequest := func(targetAddr string) (int, error) {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
		if err != nil {
			return 0, err
		}
		defer func() { _ = conn.Close() }()

		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
			targetAddr, targetAddr, authHeader)
		_, err = conn.Write([]byte(connectReq))
		if err != nil {
			return 0, err
		}

		reader := bufio.NewReader(conn)
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}

		var statusCode int
		_, err = fmt.Sscanf(statusLine, "HTTP/1.1 %d", &statusCode)
		return statusCode, err
	}

	// Denied subdomain blocked by pattern
	status, err := makeRequest("api.evil.com:443")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if status != 403 {
		t.Errorf("expected 403 Forbidden for pattern-denied subdomain, got %d", status)
	}

	// Positive control: non-denied domain on the allowlist succeeds
	status, err = makeRequest(upstreamAddr)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if status != 200 {
		t.Errorf("expected 200 for non-denied allowed domain, got %d", status)
	}
}

func TestProxyServer_MockPolicyChecker(t *testing.T) {
	// Verify that ProxyServer works with a mockPolicyChecker (non-*PolicyEngine)
	// implementation of PolicyChecker, demonstrating interface decoupling.

	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanup := startMockUpstream(t, echoHandler)
	defer cleanup()
	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	checker := &mockPolicyChecker{
		checkFunc: func(_, _, domain string) Decision {
			if domain == upstreamHost {
				return Allow
			}
			return Deny
		},
	}

	p := NewProxyServer(":0")
	p.PolicyEngine = checker

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	t.Run("allowed domain via mock", func(t *testing.T) {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
		if err != nil {
			t.Fatalf("failed to connect to proxy: %v", err)
		}
		defer func() { _ = conn.Close() }()

		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamAddr, upstreamAddr)
		_, err = conn.Write([]byte(connectReq))
		if err != nil {
			t.Fatalf("failed to send CONNECT request: %v", err)
		}

		reader := bufio.NewReader(conn)
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read status line: %v", err)
		}

		if !strings.Contains(statusLine, "200") {
			t.Errorf("expected 200 for allowed domain via mock, got: %s", statusLine)
		}
	})

	t.Run("denied domain via mock", func(t *testing.T) {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", proxyAddr)
		if err != nil {
			t.Fatalf("failed to connect to proxy: %v", err)
		}
		defer func() { _ = conn.Close() }()

		connectReq := "CONNECT denied.example.com:443 HTTP/1.1\r\nHost: denied.example.com:443\r\n\r\n"
		_, err = conn.Write([]byte(connectReq))
		if err != nil {
			t.Fatalf("failed to send CONNECT request: %v", err)
		}

		reader := bufio.NewReader(conn)
		statusLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read status line: %v", err)
		}

		if !strings.Contains(statusLine, "403") {
			t.Errorf("expected 403 for denied domain via mock, got: %s", statusLine)
		}
	})

	t.Run("SIGHUP skips reload for non-PolicyEngine", func(_ *testing.T) {
		// mockPolicyChecker doesn't implement *PolicyEngine, so SIGHUP
		// should log "skipped" and not panic.
		p.handleSighup()
	})
}

// sendRawHTTPViaProxy opens a raw TCP connection to proxyAddr and sends an
// HTTP request with an absolute URI (forward-proxy style). It returns the
// response status code and body. If token is non-empty, a Proxy-Authorization
// header with Basic auth (username "cloister") is included.
func sendRawHTTPViaProxy(t *testing.T, proxyAddr, method, rawURL, token string) (int, string, error) {
	t.Helper()

	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		return 0, "", fmt.Errorf("dial proxy: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, "", fmt.Errorf("set deadline: %w", err)
	}

	// Extract host from the raw URL for the Host header.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, "", fmt.Errorf("parse URL: %w", err)
	}
	host := parsed.Host

	var reqBuilder strings.Builder
	reqBuilder.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", method, rawURL))
	reqBuilder.WriteString(fmt.Sprintf("Host: %s\r\n", host))
	if token != "" {
		auth := base64.StdEncoding.EncodeToString([]byte("cloister:" + token))
		reqBuilder.WriteString(fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth))
	}
	reqBuilder.WriteString("Connection: close\r\n")
	reqBuilder.WriteString("\r\n")

	if _, err := conn.Write([]byte(reqBuilder.String())); err != nil {
		return 0, "", fmt.Errorf("write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, "", fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func TestProxyServer_PlainHTTP_AllowedDomain(t *testing.T) {
	// Send a plain HTTP GET through the proxy to an allowed domain.
	// Current code returns 405 for all non-CONNECT; this test asserts
	// the desired behavior: 200 (upstream reachable) or 502 (upstream
	// unreachable), but NOT 405.

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"allowed.example.com"}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", "http://allowed.example.com/", "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	switch status {
	case http.StatusOK, http.StatusBadGateway:
		// expected — 200 if upstream reachable, 502 if not
	case http.StatusMethodNotAllowed:
		t.Errorf("plain HTTP GET returned 405; proxy is not routing non-CONNECT requests")
	default:
		t.Errorf("expected 200 or 502 for allowed domain, got %d", status)
	}
}

func TestProxyServer_PlainHTTP_DeniedDomain(t *testing.T) {
	// Send a plain HTTP GET to a denied domain with valid token.
	// Current code returns 405 for all non-CONNECT; this test asserts
	// the desired behavior: 403 Forbidden, NOT 405.

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine(nil, []string{"denied.example.com"})
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", "http://denied.example.com/", "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	switch status {
	case http.StatusForbidden:
		// expected — denied domain returns 403
	case http.StatusMethodNotAllowed:
		t.Errorf("plain HTTP GET returned 405; proxy is not routing non-CONNECT requests")
	default:
		t.Errorf("expected 403 for denied domain, got %d", status)
	}
}

func TestProxyServer_PlainHTTP_NoAuth(t *testing.T) {
	// Send a plain HTTP GET without Proxy-Authorization header.
	// Current code returns 405 for all non-CONNECT; this test asserts
	// the desired behavior: 407 Proxy Authentication Required, NOT 405.

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"example.com"}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", "http://example.com/", "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	switch status {
	case http.StatusProxyAuthRequired:
		// expected — missing auth returns 407
	case http.StatusMethodNotAllowed:
		t.Errorf("plain HTTP GET returned 405; proxy is not routing non-CONNECT requests")
	default:
		t.Errorf("expected 407 for missing auth, got %d", status)
	}
}

func TestProxyServer_PlainHTTP_InvalidToken(t *testing.T) {
	// Send a plain HTTP GET with an invalid token.
	// Current code returns 405 for all non-CONNECT; this test asserts
	// the desired behavior: 407 Proxy Authentication Required, NOT 405.

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"example.com"}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", "http://example.com/", "bad-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	switch status {
	case http.StatusProxyAuthRequired:
		// expected — invalid token returns 407
	case http.StatusMethodNotAllowed:
		t.Errorf("plain HTTP GET returned 405; proxy is not routing non-CONNECT requests")
	default:
		t.Errorf("expected 407 for invalid token, got %d", status)
	}
}

func TestProxyServer_PlainHTTP_Methods(t *testing.T) {
	// Table-driven test: send various HTTP methods through the proxy to an
	// allowed domain. Current code returns 405 for all non-CONNECT; this
	// test asserts that none of the standard methods should return 405.

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"methods.example.com"}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			status, _, err := sendRawHTTPViaProxy(t, proxyAddr, method, "http://methods.example.com/", "test-token")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if status == http.StatusMethodNotAllowed {
				t.Errorf("plain HTTP %s returned 405; proxy is not routing non-CONNECT requests", method)
			}
		})
	}
}

func TestProxyServer_PlainHTTP_DomainApproval(t *testing.T) {
	// When a plain HTTP request targets an unlisted domain with a valid token,
	// the proxy should invoke the DomainApprover rather than returning 405.
	// Current code returns 405 for all non-CONNECT; this test documents the
	// expected behavior.

	var (
		mu             sync.Mutex
		capturedDomain string
	)

	approver := &mockDomainApprover{
		approveFunc: func(project, cloister, domain, token string) (DomainApprovalResult, error) {
			mu.Lock()
			capturedDomain = domain
			mu.Unlock()
			return DomainApprovalResult{Approved: true}, nil
		},
	}

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine(nil, nil) // Empty — domain is unlisted
	p.TokenValidator = newMockTokenValidator("test-token")
	p.DomainApprover = approver

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", "http://unlisted.example.com/", "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status == http.StatusMethodNotAllowed {
		t.Errorf("plain HTTP GET returned 405; proxy is not routing non-CONNECT requests through domain approval")
	}

	if count := approver.CallCount(); count != 1 {
		t.Errorf("expected DomainApprover to be called once, got %d calls", count)
	}

	mu.Lock()
	got := capturedDomain
	mu.Unlock()
	if got != "unlisted.example.com" {
		t.Errorf("DomainApprover called with domain %q, want %q", got, "unlisted.example.com")
	}
}

func TestProxyServer_PlainHTTP_PolicyCheck(t *testing.T) {
	// When a plain HTTP request arrives with a valid token and TokenLookup,
	// the proxy should invoke the PolicyChecker with the correct token,
	// project, and domain (port stripped). Current code returns 405 for all
	// non-CONNECT; this test documents the expected behavior.

	var (
		mu              sync.Mutex
		capturedToken   string
		capturedProject string
		capturedDomain  string
	)

	checker := &mockPolicyChecker{
		checkFunc: func(token, project, domain string) Decision {
			mu.Lock()
			capturedToken = token
			capturedProject = project
			capturedDomain = domain
			mu.Unlock()
			return Allow
		},
	}

	p := NewProxyServer(":0")
	p.PolicyEngine = checker
	p.TokenValidator = newMockTokenValidator("test-token")
	p.TokenLookup = func(tok string) (TokenLookupResult, bool) {
		if tok == "test-token" {
			return TokenLookupResult{ProjectName: "myproject", CloisterName: "myproject-main"}, true
		}
		return TokenLookupResult{}, false
	}

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", "http://check.example.com:8080/path?q=1", "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status == http.StatusMethodNotAllowed {
		t.Errorf("plain HTTP GET returned 405; proxy is not routing non-CONNECT requests through policy check")
	}

	mu.Lock()
	gotToken := capturedToken
	gotProject := capturedProject
	gotDomain := capturedDomain
	mu.Unlock()

	if gotToken != "test-token" {
		t.Errorf("PolicyChecker called with token %q, want %q", gotToken, "test-token")
	}
	if gotProject != "myproject" {
		t.Errorf("PolicyChecker called with project %q, want %q", gotProject, "myproject")
	}
	if gotDomain != "check.example.com" {
		t.Errorf("PolicyChecker called with domain %q, want %q (port should be stripped)", gotDomain, "check.example.com")
	}
}

// sendRawHTTPViaProxyFull sends a plain HTTP request through the proxy with full control
// over method, URL, headers, and body. Returns status code, response headers, and body.
func sendRawHTTPViaProxyFull(t *testing.T, proxyAddr, method, rawURL, token string, headers map[string]string, body string) (int, http.Header, string, error) {
	t.Helper()

	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		return 0, nil, "", fmt.Errorf("dial proxy: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, nil, "", fmt.Errorf("set deadline: %w", err)
	}

	// Extract host from the raw URL for the Host header.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, nil, "", fmt.Errorf("parse URL: %w", err)
	}
	host := parsed.Host

	var reqBuilder strings.Builder
	reqBuilder.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", method, rawURL))
	reqBuilder.WriteString(fmt.Sprintf("Host: %s\r\n", host))
	if token != "" {
		auth := base64.StdEncoding.EncodeToString([]byte("cloister:" + token))
		reqBuilder.WriteString(fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth))
	}
	for k, v := range headers {
		reqBuilder.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	if body != "" {
		reqBuilder.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	}
	reqBuilder.WriteString("Connection: close\r\n")
	reqBuilder.WriteString("\r\n")
	if body != "" {
		reqBuilder.WriteString(body)
	}

	if _, err := conn.Write([]byte(reqBuilder.String())); err != nil {
		return 0, nil, "", fmt.Errorf("write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, nil, "", fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(respBody), nil
}

func TestProxyServer_PlainHTTP_ForwardGET(t *testing.T) {
	// Start a mock upstream that echoes request details as JSON.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := map[string]interface{}{
			"method":  r.Method,
			"url":     r.URL.String(),
			"host":    r.Host,
			"headers": r.Header,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	}))
	defer upstream.Close()

	// Extract host from upstream URL for the policy engine allowlist.
	upstreamURL, _ := url.Parse(upstream.URL)
	upstreamHost := upstreamURL.Hostname()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	targetURL := fmt.Sprintf("http://%s/path?q=1", upstreamURL.Host)

	status, _, respBody, err := sendRawHTTPViaProxyFull(t, proxyAddr, "GET", targetURL, "test-token", nil, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// This test should FAIL with 502 until forwarding is implemented.
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", status, respBody)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(respBody), &result); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if got := result["method"]; got != "GET" {
		t.Errorf("upstream saw method %q, want GET", got)
	}
	if got, ok := result["url"].(string); !ok || got != "/path?q=1" {
		t.Errorf("upstream saw url %q, want /path?q=1", got)
	}
	if got, ok := result["host"].(string); !ok || got != upstreamURL.Host {
		t.Errorf("upstream saw host %q, want %q", got, upstreamURL.Host)
	}
}

func TestProxyServer_PlainHTTP_ForwardPOST(t *testing.T) {
	// Start a mock upstream that echoes the request body back.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	upstreamHost := upstreamURL.Hostname()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	targetURL := fmt.Sprintf("http://%s/echo", upstreamURL.Host)

	status, _, respBody, err := sendRawHTTPViaProxyFull(t, proxyAddr, "POST", targetURL, "test-token", map[string]string{
		"Content-Type": "text/plain",
	}, "hello world")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// This test should FAIL with 502 until forwarding is implemented.
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", status, respBody)
	}

	if !strings.Contains(respBody, "hello world") {
		t.Errorf("expected response body to contain %q, got %q", "hello world", respBody)
	}
}

func TestProxyServer_PlainHTTP_PreservesHeaders(t *testing.T) {
	// Start a mock upstream that echoes request headers as JSON.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(r.Header)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	upstreamHost := upstreamURL.Hostname()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	targetURL := fmt.Sprintf("http://%s/headers", upstreamURL.Host)

	status, _, respBody, err := sendRawHTTPViaProxyFull(t, proxyAddr, "GET", targetURL, "test-token", map[string]string{
		"X-Custom": "foo",
	}, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// This test should FAIL with 502 until forwarding is implemented.
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", status, respBody)
	}

	var headers http.Header
	if err := json.Unmarshal([]byte(respBody), &headers); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if got := headers.Get("X-Custom"); got != "foo" {
		t.Errorf("upstream received X-Custom=%q, want %q", got, "foo")
	}
}

func TestProxyServer_PlainHTTP_StripsHopByHop(t *testing.T) {
	// Start a mock upstream that echoes request headers as JSON.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(r.Header)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	upstreamHost := upstreamURL.Hostname()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	targetURL := fmt.Sprintf("http://%s/headers", upstreamURL.Host)

	// Send request with hop-by-hop headers that should be stripped,
	// plus a normal header that should be preserved.
	status, _, respBody, err := sendRawHTTPViaProxyFull(t, proxyAddr, "GET", targetURL, "test-token", map[string]string{
		"Proxy-Connection": "keep-alive",
		"TE":               "trailers",
		"X-Custom":         "preserved",
	}, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// This test should FAIL with 502 until forwarding is implemented.
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", status, respBody)
	}

	var headers http.Header
	if err := json.Unmarshal([]byte(respBody), &headers); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	// Proxy-Authorization must be stripped (carries the cloister token).
	if got := headers.Get("Proxy-Authorization"); got != "" {
		t.Errorf("upstream received Proxy-Authorization=%q, want it stripped", got)
	}

	// Proxy-Connection is a hop-by-hop header and must be stripped.
	if got := headers.Get("Proxy-Connection"); got != "" {
		t.Errorf("upstream received Proxy-Connection=%q, want it stripped", got)
	}

	// Normal headers must be preserved.
	if got := headers.Get("X-Custom"); got != "preserved" {
		t.Errorf("upstream received X-Custom=%q, want %q", got, "preserved")
	}

	// Connection header should be handled: either stripped entirely
	// or set to "close" (not forwarded as "keep-alive").
	if got := headers.Get("Connection"); got == "keep-alive" {
		t.Errorf("upstream received Connection=%q, want it stripped or set to close", got)
	}
}

func TestProxyServer_PlainHTTP_NoProxyAuthToUpstream(t *testing.T) {
	// Start a mock upstream that echoes request headers as JSON.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(r.Header)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	upstreamHost := upstreamURL.Hostname()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{upstreamHost}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	targetURL := fmt.Sprintf("http://%s/headers", upstreamURL.Host)

	// Send request with valid token (Proxy-Authorization is added by the helper).
	status, _, respBody, err := sendRawHTTPViaProxyFull(t, proxyAddr, "GET", targetURL, "test-token", nil, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// This test should FAIL with 502 until forwarding is implemented.
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", status, respBody)
	}

	var headers http.Header
	if err := json.Unmarshal([]byte(respBody), &headers); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	// Security critical: the cloister token in Proxy-Authorization must
	// never be forwarded to the upstream server.
	if got := headers.Get("Proxy-Authorization"); got != "" {
		t.Errorf("SECURITY: upstream received Proxy-Authorization=%q; cloister token leaked to upstream", got)
	}
}

// --- Phase 5.1: Upstream failure tests ---

func TestProxyServer_PlainHTTP_UpstreamRefused(t *testing.T) {
	// Create a listener to grab a port, then close it so connections are refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	closedAddr := ln.Addr().String()
	ln.Close() // port is now unreachable

	closedHost, _, _ := net.SplitHostPort(closedAddr)

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{closedHost}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	targetURL := fmt.Sprintf("http://%s/", closedAddr)

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", targetURL, "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway for connection refused, got %d", status)
	}
}

func TestProxyServer_PlainHTTP_UpstreamTimeout(t *testing.T) {
	// Create a TCP listener that accepts connections but never responds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer ln.Close()

	slowAddr := ln.Addr().String()
	slowHost, _, _ := net.SplitHostPort(slowAddr)

	// Accept connections in a goroutine but never send a response.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			// Read forever, never respond.
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{slowHost}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")
	// Set a very short response header timeout so the test doesn't wait long.
	p.Transport = &http.Transport{
		ResponseHeaderTimeout: 100 * time.Millisecond,
	}

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()
	targetURL := fmt.Sprintf("http://%s/", slowAddr)

	status, _, err := sendRawHTTPViaProxy(t, proxyAddr, "GET", targetURL, "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Phase 6 should map timeout errors to 504; current code returns 502 for all errors.
	if status != http.StatusGatewayTimeout {
		t.Errorf("expected 504 Gateway Timeout for unresponsive upstream, got %d", status)
	}
}

// --- Phase 5.2: Edge case tests ---

// sendRawRequestLine sends an arbitrary raw HTTP request line through the proxy
// without any URL parsing. This allows sending malformed or unusual request lines
// that sendRawHTTPViaProxy cannot produce (relative URIs, HTTPS scheme, empty host).
func sendRawRequestLine(t *testing.T, proxyAddr, requestLine, hostHeader, token string) (int, string, error) {
	t.Helper()

	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		return 0, "", fmt.Errorf("dial proxy: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, "", fmt.Errorf("set deadline: %w", err)
	}

	var reqBuilder strings.Builder
	reqBuilder.WriteString(requestLine + "\r\n")
	if hostHeader != "" {
		reqBuilder.WriteString(fmt.Sprintf("Host: %s\r\n", hostHeader))
	}
	if token != "" {
		auth := base64.StdEncoding.EncodeToString([]byte("cloister:" + token))
		reqBuilder.WriteString(fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth))
	}
	reqBuilder.WriteString("Connection: close\r\n")
	reqBuilder.WriteString("\r\n")

	if _, err := conn.Write([]byte(reqBuilder.String())); err != nil {
		return 0, "", fmt.Errorf("write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, "", fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func TestProxyServer_PlainHTTP_RelativeURI(t *testing.T) {
	// Send a relative URI (not an absolute URI) — not valid for a forward proxy.
	// Phase 6 should return 400; current code returns 405 (falls through to default branch).
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"example.com"}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawRequestLine(t, proxyAddr, "GET /path HTTP/1.1", "example.com", "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for relative URI, got %d", status)
	}
}

func TestProxyServer_PlainHTTP_HTTPSScheme(t *testing.T) {
	// Send GET with https:// scheme — HTTPS should use CONNECT, not plain HTTP.
	// Phase 6 should return 400; current code returns 405.
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{"example.com"}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawRequestLine(t, proxyAddr, "GET https://example.com/ HTTP/1.1", "example.com", "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for HTTPS scheme in plain HTTP request, got %d", status)
	}
}

func TestProxyServer_PlainHTTP_EmptyHost(t *testing.T) {
	// Send an absolute URI with an empty host — not valid for forwarding.
	// Phase 6 should return 400; current code returns 405.
	p := NewProxyServer(":0")
	p.PolicyEngine = newTestProxyPolicyEngine([]string{""}, nil)
	p.TokenValidator = newMockTokenValidator("test-token")

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	status, _, err := sendRawRequestLine(t, proxyAddr, "GET http:///path HTTP/1.1", "", "test-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty host in absolute URI, got %d", status)
	}
}
