package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// shortTempDir creates a short temp directory for socket files.
// Unix socket paths have a length limit (~104 chars on macOS, ~108 on Linux).
// Go's t.TempDir() can create very long paths that exceed this limit.
func shortTempDir(t *testing.T) string {
	t.Helper()
	// Use /tmp directly for short paths
	dir, err := os.MkdirTemp("/tmp", "sock")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TestSocketRequestJSONRoundTrip verifies SocketRequest serializes correctly.
func TestSocketRequestJSONRoundTrip(t *testing.T) {
	req := SocketRequest{
		Secret: "test-secret-123",
		Request: ExecuteRequest{
			Token:     "tok_abc",
			Command:   "echo",
			Args:      []string{"hello"},
			Workdir:   "/work",
			Env:       map[string]string{"FOO": "bar"},
			TimeoutMs: 5000,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got SocketRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.Secret != req.Secret {
		t.Errorf("Secret: got %q, want %q", got.Secret, req.Secret)
	}
	if got.Request.Token != req.Request.Token {
		t.Errorf("Token: got %q, want %q", got.Request.Token, req.Request.Token)
	}
	if got.Request.Command != req.Request.Command {
		t.Errorf("Command: got %q, want %q", got.Request.Command, req.Request.Command)
	}
}

// TestSocketResponseJSONRoundTrip verifies SocketResponse serializes correctly.
func TestSocketResponseJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		resp SocketResponse
	}{
		{
			name: "success",
			resp: SocketResponse{
				Success: true,
				Response: ExecuteResponse{
					Status:   StatusCompleted,
					ExitCode: 0,
					Stdout:   "hello world\n",
				},
			},
		},
		{
			name: "error",
			resp: SocketResponse{
				Success: false,
				Error:   "invalid secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got SocketResponse
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if got.Success != tt.resp.Success {
				t.Errorf("Success: got %v, want %v", got.Success, tt.resp.Success)
			}
			if got.Error != tt.resp.Error {
				t.Errorf("Error: got %q, want %q", got.Error, tt.resp.Error)
			}
			if got.Response.Status != tt.resp.Response.Status {
				t.Errorf("Response.Status: got %q, want %q", got.Response.Status, tt.resp.Response.Status)
			}
		})
	}
}

// mockExecutorForSocket is a test executor that records calls and returns configured responses.
// It is thread-safe for use with concurrent connections.
type mockExecutorForSocket struct {
	mu       sync.Mutex
	calls    []ExecuteRequest
	response ExecuteResponse
}

func (m *mockExecutorForSocket) Execute(ctx context.Context, req ExecuteRequest) ExecuteResponse {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	m.mu.Unlock()
	return m.response
}

func (m *mockExecutorForSocket) getCalls() []ExecuteRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ExecuteRequest{}, m.calls...)
}

// TestSocketServerStartStop verifies the server starts and stops cleanly.
func TestSocketServerStartStop(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	server := NewSocketServer("test-secret", mock, WithSocketPath(sockPath))

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify socket file exists with correct permissions
	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("Socket file not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Socket permissions: got %o, want 0600", info.Mode().Perm())
	}

	// Stop server
	if err := server.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify socket file is removed
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("Socket file should be removed after Stop")
	}
}

// TestSocketServerCreateDirectory verifies parent directory is created.
func TestSocketServerCreateDirectory(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "n", "d", "t.sock") // Short path for socket limit

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	server := NewSocketServer("test-secret", mock, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Verify nested directory was created
	dirInfo, err := os.Stat(filepath.Dir(sockPath))
	if err != nil {
		t.Fatalf("Parent directory not created: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("Expected parent path to be a directory")
	}
}

// TestSocketServerValidRequest verifies a valid request is processed correctly.
func TestSocketServerValidRequest(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{
			Status:   StatusCompleted,
			ExitCode: 0,
			Stdout:   "hello world\n",
		},
	}

	server := NewSocketServer("test-secret", mock, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Connect to socket
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send request
	req := SocketRequest{
		Secret: "test-secret",
		Request: ExecuteRequest{
			Token:   "tok_123",
			Command: "echo",
			Args:    []string{"hello"},
			Workdir: "/work",
		},
	}
	sendRequest(t, conn, req)

	// Read response
	resp := readResponse(t, conn)

	if !resp.Success {
		t.Errorf("Expected success, got error: %s", resp.Error)
	}
	if resp.Response.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Response.Status, StatusCompleted)
	}
	if resp.Response.Stdout != "hello world\n" {
		t.Errorf("Stdout: got %q, want %q", resp.Response.Stdout, "hello world\n")
	}

	// Verify executor was called
	calls := mock.getCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 executor call, got %d", len(calls))
	}
	if calls[0].Command != "echo" {
		t.Errorf("Executor Command: got %q, want %q", calls[0].Command, "echo")
	}
}

// TestSocketServerInvalidSecret verifies requests with wrong secret are rejected.
func TestSocketServerInvalidSecret(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	server := NewSocketServer("correct-secret", mock, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Connect to socket
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send request with wrong secret
	req := SocketRequest{
		Secret: "wrong-secret",
		Request: ExecuteRequest{
			Token:   "tok_123",
			Command: "echo",
		},
	}
	sendRequest(t, conn, req)

	// Read response
	resp := readResponse(t, conn)

	if resp.Success {
		t.Error("Expected failure for invalid secret")
	}
	if !strings.Contains(resp.Error, "invalid secret") {
		t.Errorf("Error should contain 'invalid secret', got: %q", resp.Error)
	}

	// Verify executor was NOT called
	calls := mock.getCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 executor calls, got %d", len(calls))
	}
}

// TestSocketServerInvalidJSON verifies malformed JSON is rejected.
func TestSocketServerInvalidJSON(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	server := NewSocketServer("test-secret", mock, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Connect to socket
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send invalid JSON
	if _, err := conn.Write([]byte("not valid json\n")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read response
	resp := readResponse(t, conn)

	if resp.Success {
		t.Error("Expected failure for invalid JSON")
	}
	if !strings.Contains(resp.Error, "invalid JSON") {
		t.Errorf("Error should contain 'invalid JSON', got: %q", resp.Error)
	}
}

// TestSocketServerTokenValidation verifies token validation is called.
func TestSocketServerTokenValidation(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	validatorCalled := false
	tokenValidator := func(token string) (string, error) {
		validatorCalled = true
		if token != "valid-token" {
			return "", ErrInvalidToken
		}
		return "/expected/worktree", nil
	}

	server := NewSocketServer("test-secret", mock,
		WithSocketPath(sockPath),
		WithTokenValidator(tokenValidator),
	)

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Test with invalid token
	conn1, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	req := SocketRequest{
		Secret: "test-secret",
		Request: ExecuteRequest{
			Token:   "invalid-token",
			Command: "echo",
		},
	}
	sendRequest(t, conn1, req)
	resp := readResponse(t, conn1)
	conn1.Close()

	if !validatorCalled {
		t.Error("Token validator was not called")
	}
	if resp.Success {
		t.Error("Expected failure for invalid token")
	}
	if !strings.Contains(resp.Error, "invalid token") {
		t.Errorf("Error should contain 'invalid token', got: %q", resp.Error)
	}

	// Test with valid token
	conn2, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn2.Close()

	req.Request.Token = "valid-token"
	sendRequest(t, conn2, req)
	resp = readResponse(t, conn2)

	if !resp.Success {
		t.Errorf("Expected success for valid token, got error: %s", resp.Error)
	}
}

// TestSocketServerWorkdirValidation verifies workdir validation is called.
func TestSocketServerWorkdirValidation(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	tokenValidator := func(token string) (string, error) {
		return "/registered/worktree", nil
	}

	workdirValidatorCalled := false
	workdirValidator := func(requestedWorkdir, registeredWorktree string) error {
		workdirValidatorCalled = true
		if requestedWorkdir != registeredWorktree {
			return ErrWorkdirMismatch
		}
		return nil
	}

	server := NewSocketServer("test-secret", mock,
		WithSocketPath(sockPath),
		WithTokenValidator(tokenValidator),
		WithWorkdirValidator(workdirValidator),
	)

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Test with mismatched workdir
	conn1, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	req := SocketRequest{
		Secret: "test-secret",
		Request: ExecuteRequest{
			Token:   "tok_123",
			Command: "echo",
			Workdir: "/different/path",
		},
	}
	sendRequest(t, conn1, req)
	resp := readResponse(t, conn1)
	conn1.Close()

	if !workdirValidatorCalled {
		t.Error("Workdir validator was not called")
	}
	if resp.Success {
		t.Error("Expected failure for workdir mismatch")
	}
	if !strings.Contains(resp.Error, "workdir mismatch") {
		t.Errorf("Error should contain 'workdir mismatch', got: %q", resp.Error)
	}

	// Test with matching workdir
	conn2, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn2.Close()

	req.Request.Workdir = "/registered/worktree"
	sendRequest(t, conn2, req)
	resp = readResponse(t, conn2)

	if !resp.Success {
		t.Errorf("Expected success for matching workdir, got error: %s", resp.Error)
	}
}

// TestSocketServerMultipleConnections verifies concurrent connections work.
func TestSocketServerMultipleConnections(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	mock := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	server := NewSocketServer("test-secret", mock, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Make multiple concurrent connections
	const numConns = 5
	done := make(chan bool, numConns)

	for i := 0; i < numConns; i++ {
		go func(idx int) {
			conn, err := net.Dial("unix", sockPath)
			if err != nil {
				t.Errorf("Connection %d: Dial failed: %v", idx, err)
				done <- false
				return
			}
			defer conn.Close()

			req := SocketRequest{
				Secret: "test-secret",
				Request: ExecuteRequest{
					Token:   "tok_123",
					Command: "echo",
					Args:    []string{"hello"},
				},
			}

			data, err := json.Marshal(req)
			if err != nil {
				t.Errorf("Connection %d: Marshal failed: %v", idx, err)
				done <- false
				return
			}
			data = append(data, '\n')
			if _, err := conn.Write(data); err != nil {
				t.Errorf("Connection %d: Write failed: %v", idx, err)
				done <- false
				return
			}

			reader := bufio.NewReader(conn)
			line, err := reader.ReadBytes('\n')
			if err != nil {
				t.Errorf("Connection %d: Read failed: %v", idx, err)
				done <- false
				return
			}

			var resp SocketResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				t.Errorf("Connection %d: Unmarshal failed: %v", idx, err)
				done <- false
				return
			}

			done <- resp.Success
		}(i)
	}

	// Wait for all connections
	successCount := 0
	for i := 0; i < numConns; i++ {
		select {
		case success := <-done:
			if success {
				successCount++
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for connections")
		}
	}

	if successCount != numConns {
		t.Errorf("Expected %d successful connections, got %d", numConns, successCount)
	}
}

// TestSocketServerGracefulShutdown verifies shutdown waits for pending connections.
func TestSocketServerGracefulShutdown(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	// Executor that takes some time
	slowExecutor := &mockExecutorForSocket{
		response: ExecuteResponse{Status: StatusCompleted, ExitCode: 0},
	}

	server := NewSocketServer("test-secret", slowExecutor, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Start a connection but don't finish it yet
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// Send request in background
	go func() {
		req := SocketRequest{
			Secret: "test-secret",
			Request: ExecuteRequest{
				Token:   "tok_123",
				Command: "echo",
			},
		}
		sendRequest(t, conn, req)
	}()

	// Give time for request to be sent
	time.Sleep(50 * time.Millisecond)

	// Stop should complete (not hang)
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- server.Stop()
	}()

	select {
	case err := <-stopDone:
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not complete within timeout")
	}

	conn.Close()
}

// TestSocketServerSocketPath verifies SocketPath returns the configured path.
func TestSocketServerSocketPath(t *testing.T) {
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "custom.sock")

	mock := &mockExecutorForSocket{}
	server := NewSocketServer("secret", mock, WithSocketPath(sockPath))

	if got := server.SocketPath(); got != sockPath {
		t.Errorf("SocketPath: got %q, want %q", got, sockPath)
	}
}

// Helper functions for tests

func sendRequest(t *testing.T, conn net.Conn, req SocketRequest) {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal request failed: %v", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Write request failed: %v", err)
	}
}

func readResponse(t *testing.T, conn net.Conn) SocketResponse {
	t.Helper()
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read response failed: %v", err)
	}

	var resp SocketResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}
	return resp
}
