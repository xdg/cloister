package guardian

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
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

	// CONNECT request should succeed (200 OK) for allowed domain
	t.Run("CONNECT returns 200 for allowed domain", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodConnect, fmt.Sprintf("http://%s", addr), nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Host = "api.anthropic.com:443"

		client := &http.Client{
			// Don't follow redirects
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

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
			defer resp.Body.Close()

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

// startMockUpstream creates a mock TCP server that echoes data back to the client.
// It returns the server address and a cleanup function.
func startMockUpstream(t *testing.T, handler func(net.Conn)) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock upstream: %v", err)
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				handler(c)
			}(conn)
		}
	}()

	cleanup := func() {
		close(done)
		listener.Close()
		wg.Wait()
	}

	return listener.Addr().String(), cleanup
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
		defer conn.Close()

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
		defer conn.Close()

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
	defer conn.Close()

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
	defer conn.Close()

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
		conn.Close()
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
	defer conn.Close()

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
			// Create proxy with token validator
			p := NewProxyServer(":0")
			p.Allowlist = NewAllowlist([]string{upstreamHost})
			p.TokenValidator = newMockTokenValidator(tc.validTokens...)

			// Use a buffer to capture log output
			var logBuf bytes.Buffer
			p.Logger = log.New(&logBuf, "", 0)

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
			defer conn.Close()

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
	defer conn.Close()

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
	defer conn.Close()

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
