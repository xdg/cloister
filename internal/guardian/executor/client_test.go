package executor

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/executor"
)

// shortTempDir creates a short temp directory for socket files.
// Unix socket paths have a length limit (~104 chars on macOS, ~108 on Linux).
// Go's t.TempDir() can create very long paths that exceed this limit.
// Uses os.TempDir() which respects TMPDIR for sandbox compatibility.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "sock")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// mockServer is a helper for creating mock Unix socket servers in tests.
type mockServer struct {
	listener net.Listener
	sockPath string
}

// newMockServer creates a mock Unix socket server.
func newMockServer(t *testing.T) *mockServer {
	t.Helper()
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	t.Cleanup(func() {
		listener.Close()
	})

	return &mockServer{
		listener: listener,
		sockPath: sockPath,
	}
}

// TestClientExecuteSuccess verifies successful command execution.
func TestClientExecuteSuccess(t *testing.T) {
	mock := newMockServer(t)

	// Handle connection in background
	go func() {
		conn, err := mock.listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		reader := bufio.NewReader(conn)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Errorf("Failed to read request: %v", err)
			return
		}

		// Verify wire format
		var req executor.SocketRequest
		if err := json.Unmarshal(line, &req); err != nil {
			t.Errorf("Failed to unmarshal request: %v", err)
			return
		}

		// Validate expected fields
		if req.Secret != "test-secret" {
			t.Errorf("Secret: got %q, want %q", req.Secret, "test-secret")
		}
		if req.Request.Command != "echo" {
			t.Errorf("Command: got %q, want %q", req.Request.Command, "echo")
		}
		if len(req.Request.Args) != 1 || req.Request.Args[0] != "hello" {
			t.Errorf("Args: got %v, want [hello]", req.Request.Args)
		}
		if req.Request.Workdir != "/work" {
			t.Errorf("Workdir: got %q, want %q", req.Request.Workdir, "/work")
		}

		// Send success response
		resp := executor.SocketResponse{
			Success: true,
			Response: executor.ExecuteResponse{
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "hello world\n",
			},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		_, _ = conn.Write(data)
	}()

	// Create client and execute
	client := NewClient(mock.sockPath, "test-secret")

	req := executor.ExecuteRequest{
		Command: "echo",
		Args:    []string{"hello"},
		Workdir: "/work",
	}

	resp, err := client.Execute(req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if resp.Status != executor.StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Status, executor.StatusCompleted)
	}
	if resp.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", resp.ExitCode)
	}
	if resp.Stdout != "hello world\n" {
		t.Errorf("Stdout: got %q, want %q", resp.Stdout, "hello world\n")
	}
}

// TestClientExecuteConnectionError verifies error handling when socket doesn't exist.
func TestClientExecuteConnectionError(t *testing.T) {
	tmpDir := shortTempDir(t)
	nonExistentSocket := filepath.Join(tmpDir, "nonexistent.sock")

	client := NewClient(nonExistentSocket, "test-secret")

	req := executor.ExecuteRequest{
		Command: "echo",
	}

	resp, err := client.Execute(req)
	if err == nil {
		t.Fatal("Expected error for non-existent socket")
	}
	if resp != nil {
		t.Errorf("Expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("Error should contain 'failed to connect', got: %q", err.Error())
	}
}

// TestClientExecuteInvalidResponse verifies error handling for malformed JSON response.
func TestClientExecuteInvalidResponse(t *testing.T) {
	mock := newMockServer(t)

	// Handle connection in background - send malformed JSON
	go func() {
		conn, err := mock.listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadBytes('\n')

		// Send malformed JSON response
		_, _ = conn.Write([]byte("not valid json\n"))
	}()

	client := NewClient(mock.sockPath, "test-secret")

	req := executor.ExecuteRequest{
		Command: "echo",
	}

	resp, err := client.Execute(req)
	if err == nil {
		t.Fatal("Expected error for invalid JSON response")
	}
	if resp != nil {
		t.Errorf("Expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("Error should contain 'failed to parse response', got: %q", err.Error())
	}
}

// TestClientExecuteSocketError verifies error handling for socket-level error response.
func TestClientExecuteSocketError(t *testing.T) {
	mock := newMockServer(t)

	// Handle connection in background - send error response
	go func() {
		conn, err := mock.listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadBytes('\n')

		// Send error response
		resp := executor.SocketResponse{
			Success: false,
			Error:   "invalid secret",
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		_, _ = conn.Write(data)
	}()

	client := NewClient(mock.sockPath, "test-secret")

	req := executor.ExecuteRequest{
		Command: "echo",
	}

	resp, err := client.Execute(req)
	if err == nil {
		t.Fatal("Expected error for socket-level error response")
	}
	if resp != nil {
		t.Errorf("Expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "executor error") {
		t.Errorf("Error should contain 'executor error', got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid secret") {
		t.Errorf("Error should contain 'invalid secret', got: %q", err.Error())
	}
}

// TestClientExecuteWithEnvAndTimeout verifies wire format includes env and timeout.
func TestClientExecuteWithEnvAndTimeout(t *testing.T) {
	mock := newMockServer(t)

	receivedReq := make(chan executor.SocketRequest, 1)

	// Handle connection in background
	go func() {
		conn, err := mock.listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		reader := bufio.NewReader(conn)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var req executor.SocketRequest
		if err := json.Unmarshal(line, &req); err == nil {
			receivedReq <- req
		}

		// Send success response
		resp := executor.SocketResponse{
			Success: true,
			Response: executor.ExecuteResponse{
				Status:   executor.StatusCompleted,
				ExitCode: 0,
			},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		_, _ = conn.Write(data)
	}()

	client := NewClient(mock.sockPath, "test-secret")

	req := executor.ExecuteRequest{
		Command:   "env",
		Args:      []string{},
		Workdir:   "/work",
		Env:       map[string]string{"FOO": "bar", "BAZ": "qux"},
		TimeoutMs: 5000,
	}

	_, err := client.Execute(req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify the wire format included env and timeout
	select {
	case received := <-receivedReq:
		if received.Request.TimeoutMs != 5000 {
			t.Errorf("TimeoutMs: got %d, want 5000", received.Request.TimeoutMs)
		}
		if received.Request.Env["FOO"] != "bar" {
			t.Errorf("Env[FOO]: got %q, want %q", received.Request.Env["FOO"], "bar")
		}
		if received.Request.Env["BAZ"] != "qux" {
			t.Errorf("Env[BAZ]: got %q, want %q", received.Request.Env["BAZ"], "qux")
		}
	default:
		t.Fatal("Did not receive request")
	}
}

// TestClientExecuteReadError verifies error handling when connection closes before response.
func TestClientExecuteReadError(t *testing.T) {
	mock := newMockServer(t)

	// Handle connection in background - close without sending response
	go func() {
		conn, err := mock.listener.Accept()
		if err != nil {
			return
		}

		// Read request
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadBytes('\n')

		// Close connection without sending response
		conn.Close()
	}()

	client := NewClient(mock.sockPath, "test-secret")

	req := executor.ExecuteRequest{
		Command: "echo",
	}

	resp, err := client.Execute(req)
	if err == nil {
		t.Fatal("Expected error when connection closed before response")
	}
	if resp != nil {
		t.Errorf("Expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "failed to read response") {
		t.Errorf("Error should contain 'failed to read response', got: %q", err.Error())
	}
}

// TestClientWireFormat verifies the exact JSON wire format matches protocol spec.
func TestClientWireFormat(t *testing.T) {
	mock := newMockServer(t)

	var rawLine []byte

	// Handle connection in background - capture raw request
	go func() {
		conn, err := mock.listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read raw request
		reader := bufio.NewReader(conn)
		rawLine, _ = reader.ReadBytes('\n')

		// Send success response
		resp := executor.SocketResponse{
			Success: true,
			Response: executor.ExecuteResponse{
				Status:   executor.StatusCompleted,
				ExitCode: 0,
			},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		_, _ = conn.Write(data)
	}()

	client := NewClient(mock.sockPath, "my-secret")

	req := executor.ExecuteRequest{
		Command: "ls",
		Args:    []string{"-la"},
		Workdir: "/home",
	}

	_, err := client.Execute(req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify raw JSON structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(rawLine, &parsed); err != nil {
		t.Fatalf("Failed to parse raw request: %v", err)
	}

	// Verify top-level "secret" field exists
	if _, ok := parsed["secret"]; !ok {
		t.Error("Wire format missing 'secret' field at top level")
	}

	// Verify top-level "request" field exists
	if _, ok := parsed["request"]; !ok {
		t.Error("Wire format missing 'request' field at top level")
	}

	// Verify nested structure
	reqMap, ok := parsed["request"].(map[string]interface{})
	if !ok {
		t.Fatal("Wire format 'request' is not an object")
	}

	expectedFields := []string{"command", "args", "workdir"}
	for _, field := range expectedFields {
		if _, ok := reqMap[field]; !ok {
			t.Errorf("Wire format request missing '%s' field", field)
		}
	}
}
