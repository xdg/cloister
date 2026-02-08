package guardian

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/clog"
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
	p.Allowlist = NewAllowlist([]string{upstreamHost})

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
		req, err := http.NewRequest(http.MethodConnect, fmt.Sprintf("http://%s", addr), nil)
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
			req, err := http.NewRequest(method, baseURL, nil)
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

	// Start proxy with a custom allowlist that includes our mock upstream
	p := NewProxyServer(":0")
	p.Allowlist = NewAllowlist([]string{upstreamHost})
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
		conn, err := net.Dial("tcp", proxyAddr)
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
		conn, err := net.Dial("tcp", proxyAddr)
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
	// Default allowlist does not include localhost
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := net.Dial("tcp", proxyAddr)
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
	p.Allowlist = NewAllowlist([]string{"127.0.0.1"})
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := net.Dial("tcp", proxyAddr)
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
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
	p.Allowlist = NewAllowlist([]string{slowHost})

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
	p.Allowlist = NewAllowlist([]string{upstreamHost})
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := net.Dial("tcp", proxyAddr)
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
			p.Allowlist = NewAllowlist([]string{upstreamHost})
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
			conn, err := net.Dial("tcp", proxyAddr)
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
	p.Allowlist = NewAllowlist([]string{upstreamHost})
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
	conn, err := net.Dial("tcp", proxyAddr)
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
	p.Allowlist = NewAllowlist([]string{upstreamHost})
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

	conn, err := net.Dial("tcp", proxyAddr)
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

func TestNewProxyServerWithConfig(t *testing.T) {
	t.Run("with custom allowlist", func(t *testing.T) {
		customAllowlist := NewAllowlist([]string{"custom.example.com"})
		p := NewProxyServerWithConfig(":0", customAllowlist)

		if p.Addr != ":0" {
			t.Errorf("expected address :0, got %s", p.Addr)
		}
		if p.Allowlist != customAllowlist {
			t.Error("expected custom allowlist to be set")
		}
	})

	t.Run("with nil allowlist uses default", func(t *testing.T) {
		p := NewProxyServerWithConfig(":0", nil)

		if p.Allowlist == nil {
			t.Error("expected default allowlist when nil is provided")
		}
		// Should have default domains
		if !p.Allowlist.IsAllowed("api.anthropic.com") {
			t.Error("default allowlist should allow api.anthropic.com")
		}
	})

	t.Run("with empty address uses default port", func(t *testing.T) {
		p := NewProxyServerWithConfig("", NewAllowlist(nil))

		if p.Addr != ":3128" {
			t.Errorf("expected default address :3128, got %s", p.Addr)
		}
	})
}

func TestProxyServer_SetAllowlist(t *testing.T) {
	p := NewProxyServer(":0")

	// Verify initial default allowlist
	if !p.Allowlist.IsAllowed("api.anthropic.com") {
		t.Error("initial allowlist should allow api.anthropic.com")
	}

	// Set new allowlist
	newAllowlist := NewAllowlist([]string{"new.example.com"})
	p.SetAllowlist(newAllowlist)

	// Verify new allowlist is in effect
	if p.Allowlist.IsAllowed("api.anthropic.com") {
		t.Error("new allowlist should not allow api.anthropic.com")
	}
	if !p.Allowlist.IsAllowed("new.example.com") {
		t.Error("new allowlist should allow new.example.com")
	}
}

func TestProxyServer_ConfigDerivedAllowlist(t *testing.T) {
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

	// Create proxy with config-derived allowlist
	customAllowlist := NewAllowlist([]string{upstreamHost})
	p := NewProxyServerWithConfig(":0", customAllowlist)

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	// Test that config-derived allowlist works
	conn, err := net.Dial("tcp", proxyAddr)
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

func TestProxyServer_ConfigReloader(t *testing.T) {
	// Test that SetConfigReloader and handleSighup work correctly

	t.Run("handleSighup updates allowlist", func(t *testing.T) {
		p := NewProxyServer(":0")

		// Initial allowlist
		p.Allowlist = NewAllowlist([]string{"initial.example.com"})

		reloadCount := 0
		newDomains := []string{"reloaded.example.com", "another.example.com"}

		// Set config reloader
		p.SetConfigReloader(func() (*Allowlist, error) {
			reloadCount++
			return NewAllowlist(newDomains), nil
		})

		// Manually call handleSighup (simulating signal)
		p.handleSighup()

		// Verify allowlist was updated
		if p.Allowlist.IsAllowed("initial.example.com") {
			t.Error("initial domain should no longer be allowed after reload")
		}
		if !p.Allowlist.IsAllowed("reloaded.example.com") {
			t.Error("reloaded domain should be allowed after reload")
		}
		if reloadCount != 1 {
			t.Errorf("reload function called %d times, expected 1", reloadCount)
		}
	})

	t.Run("handleSighup ignores error", func(t *testing.T) {
		// Capture clog output
		var logBuf bytes.Buffer
		testLogger := clog.TestLogger(&logBuf)
		oldLogger := clog.ReplaceGlobal(testLogger)
		defer clog.ReplaceGlobal(oldLogger)

		p := NewProxyServer(":0")
		p.Allowlist = NewAllowlist([]string{"original.example.com"})

		// Set reloader that returns error
		p.SetConfigReloader(func() (*Allowlist, error) {
			return nil, fmt.Errorf("config load failed")
		})

		// Manually call handleSighup
		p.handleSighup()

		// Allowlist should remain unchanged
		if !p.Allowlist.IsAllowed("original.example.com") {
			t.Error("original domain should still be allowed after failed reload")
		}

		// Error should be logged
		if !strings.Contains(logBuf.String(), "config reload failed") {
			t.Errorf("expected error to be logged, got: %s", logBuf.String())
		}
	})

	t.Run("handleSighup with nil reloader does nothing", func(t *testing.T) {
		p := NewProxyServer(":0")
		p.Allowlist = NewAllowlist([]string{"example.com"})

		// No config reloader set
		p.handleSighup()

		// Should still be valid
		if !p.Allowlist.IsAllowed("example.com") {
			t.Error("allowlist should be unchanged when no reloader is set")
		}
	})
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

	// Create allowlist cache with project-specific allowlists
	// Use different domain names to test the logic (even though only one upstream exists)
	globalAllowlist := NewAllowlist([]string{"global.example.com"})
	cache := NewAllowlistCache(globalAllowlist)

	// Project A can access the upstream host AND project-a-domain.com
	cache.SetProject("project-a", NewAllowlist([]string{upstreamHost, "project-a-domain.com"}))
	// Project B can only access project-b-domain.com (NOT the upstream host)
	cache.SetProject("project-b", NewAllowlist([]string{"project-b-domain.com"}))

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

	// Create proxy with per-project support
	p := NewProxyServer(":0")
	p.Allowlist = globalAllowlist
	p.AllowlistCache = cache
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
		conn, err := net.Dial("tcp", proxyAddr)
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

	// Test: Both projects cannot access the global-only domain (because they have project-specific allowlists)
	status, err = makeRequest("token-a", "global.example.com:443")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if status != 403 {
		t.Errorf("project-a to global domain: expected 403, got %d", status)
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
	// Test backward compatibility: nil DomainApprover returns 403 for unlisted domains
	p := NewProxyServer(":0")
	p.Allowlist = NewAllowlist([]string{"allowed.com"})
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

	conn, err := net.Dial("tcp", proxyAddr)
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
	p.Allowlist = NewAllowlist([]string{}) // Empty persistent allowlist
	p.DomainApprover = &mockDomainApprover{
		approveFunc: func(project, cloister, domain, token string) (DomainApprovalResult, error) {
			return DomainApprovalResult{Approved: true, Scope: "session"}, nil
		},
	}
	// Configure per-project support so extractProjectName works
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))
	p.AllowlistCache = cache
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

	conn, err := net.Dial("tcp", proxyAddr)
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
	p.Allowlist = NewAllowlist([]string{}) // Empty persistent allowlist
	p.DomainApprover = &mockDomainApprover{
		approveFunc: func(project, cloister, domain, token string) (DomainApprovalResult, error) {
			return DomainApprovalResult{Approved: false}, nil
		},
	}
	// Configure per-project support
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))
	p.AllowlistCache = cache
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

	conn, err := net.Dial("tcp", proxyAddr)
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
	// Test that session allowlist hit bypasses DomainApprover entirely
	approver := &mockDomainApprover{
		approveFunc: func(project, cloister, domain, token string) (DomainApprovalResult, error) {
			t.Error("DomainApprover should not be called when session allowlist matches")
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

	p := NewProxyServer(":0")
	p.Allowlist = NewAllowlist([]string{}) // Empty persistent allowlist
	p.DomainApprover = approver
	p.SessionAllowlist = NewSessionAllowlist()
	// Add the upstream to session allowlist (using token, not project)
	_ = p.SessionAllowlist.Add("test-token", upstreamAddr)

	// Configure per-project support
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))
	p.AllowlistCache = cache
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

	conn, err := net.Dial("tcp", proxyAddr)
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
	p.Allowlist = NewAllowlist([]string{}) // Empty persistent allowlist
	p.DomainApprover = &mockDomainApprover{
		approveFunc: func(project, cloister, domain, token string) (DomainApprovalResult, error) {
			// Verify project name was extracted correctly
			if project != "test-project" {
				t.Errorf("expected project 'test-project', got '%s'", project)
			}
			return DomainApprovalResult{Approved: false}, nil
		},
	}

	// Configure per-project support with counting TokenLookup
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))
	p.AllowlistCache = cache
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

	conn, err := net.Dial("tcp", proxyAddr)
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
	p.Allowlist = NewAllowlist([]string{})

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := net.Dial("tcp", proxyAddr)
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
	// Denied domain in global config blocks request even if in project allowlist.

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

	// Create AllowlistCache with global allowlist containing both the denied domain
	// and the upstream host (for the positive test case).
	globalAllowlist := NewAllowlist([]string{"denied-but-allowed.example.com", upstreamHost})
	cache := NewAllowlistCache(globalAllowlist)

	// Set global denylist containing the denied domain.
	cache.SetGlobalDeny(NewAllowlist([]string{"denied-but-allowed.example.com"}))

	// Project allowlist also includes the denied domain.
	cache.SetProject("test-project", NewAllowlist([]string{"denied-but-allowed.example.com", upstreamHost}))

	tokenLookup := func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}

	p := NewProxyServer(":0")
	p.Allowlist = globalAllowlist
	p.AllowlistCache = cache
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
		conn, err := net.Dial("tcp", proxyAddr)
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
	// Session denied domain blocks request even when session allowlist has
	// the same domain allowed for the same token.

	approver := &mockDomainApprover{
		approveFunc: func(project, cloister, domain, token string) (DomainApprovalResult, error) {
			t.Error("DomainApprover should not be called when session denylist blocks")
			return DomainApprovalResult{Approved: true, Scope: "session"}, nil
		},
	}

	// Create session denylist and allowlist with the same domain for the same token.
	sessionDeny := NewSessionDenylist()
	if err := sessionDeny.Add("test-token", "denied-session.example.com"); err != nil {
		t.Fatalf("failed to add to session denylist: %v", err)
	}

	sessionAllow := NewSessionAllowlist()
	if err := sessionAllow.Add("test-token", "denied-session.example.com"); err != nil {
		t.Fatalf("failed to add to session allowlist: %v", err)
	}

	// Empty static allowlist so we rely on session lists.
	globalAllowlist := NewAllowlist([]string{})
	cache := NewAllowlistCache(globalAllowlist)
	cache.SetProject("test-project", NewAllowlist([]string{}))

	tokenLookup := func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}

	p := NewProxyServer(":0")
	p.Allowlist = globalAllowlist
	p.AllowlistCache = cache
	p.TokenLookup = tokenLookup
	p.TokenValidator = newMockTokenValidator("test-token")
	p.DomainApprover = approver
	p.SessionDenylist = sessionDeny
	p.SessionAllowlist = sessionAllow

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	proxyAddr := p.ListenAddr()

	conn, err := net.Dial("tcp", proxyAddr)
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

	// Global allowlist includes the denied subdomain and the upstream host.
	globalAllowlist := NewAllowlist([]string{"api.evil.com", upstreamHost})
	cache := NewAllowlistCache(globalAllowlist)

	// Global denylist uses a wildcard pattern to block all *.evil.com.
	cache.SetGlobalDeny(NewAllowlistWithPatterns(nil, []string{"*.evil.com"}))

	// Project allowlist also includes both.
	cache.SetProject("test-project", NewAllowlist([]string{"api.evil.com", upstreamHost}))

	tokenLookup := func(token string) (TokenLookupResult, bool) {
		if token == "test-token" {
			return TokenLookupResult{ProjectName: "test-project"}, true
		}
		return TokenLookupResult{}, false
	}

	p := NewProxyServer(":0")
	p.Allowlist = globalAllowlist
	p.AllowlistCache = cache
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
		conn, err := net.Dial("tcp", proxyAddr)
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
