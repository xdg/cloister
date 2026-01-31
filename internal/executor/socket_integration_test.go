//go:build integration

package executor

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shortTempDirIntegration creates a short temp directory for socket files.
// Unix socket paths have a length limit (~104 chars on macOS, ~108 on Linux).
func shortTempDirIntegration(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sock")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TestSocketServerIntegration_RealExecution tests the socket server with the real executor.
func TestSocketServerIntegration_RealExecution(t *testing.T) {
	tmpDir := shortTempDirIntegration(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	// Use real executor
	executor := NewRealExecutor()
	server := NewSocketServer("integration-secret", executor, WithSocketPath(sockPath))

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

	// Set read deadline to prevent hanging tests
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Send request to execute echo
	req := SocketRequest{
		Secret: "integration-secret",
		Request: ExecuteRequest{
			Token:   "tok_integration",
			Command: "echo",
			Args:    []string{"integration", "test"},
			Workdir: tmpDir,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	data = append(data, '\n')

	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	var resp SocketResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify response
	if !resp.Success {
		t.Fatalf("Expected success, got error: %s", resp.Error)
	}
	if resp.Response.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Response.Status, StatusCompleted)
	}
	if resp.Response.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", resp.Response.ExitCode)
	}
	if !strings.Contains(resp.Response.Stdout, "integration test") {
		t.Errorf("Stdout should contain 'integration test', got: %q", resp.Response.Stdout)
	}
}

// TestSocketServerIntegration_WorkdirEnforced tests that workdir is actually used.
func TestSocketServerIntegration_WorkdirEnforced(t *testing.T) {
	tmpDir := shortTempDirIntegration(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Use real executor
	executor := NewRealExecutor()
	server := NewSocketServer("integration-secret", executor, WithSocketPath(sockPath))

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

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Send request to execute pwd in the subdir
	req := SocketRequest{
		Secret: "integration-secret",
		Request: ExecuteRequest{
			Token:   "tok_integration",
			Command: "pwd",
			Workdir: subDir,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	var resp SocketResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("Expected success, got error: %s", resp.Error)
	}

	// On macOS, paths may be symlinked, so resolve both
	expectedDir, _ := filepath.EvalSymlinks(subDir)
	actualDir := strings.TrimSpace(resp.Response.Stdout)
	actualDir, _ = filepath.EvalSymlinks(actualDir)

	if actualDir != expectedDir {
		t.Errorf("Workdir: got %q, want %q", actualDir, expectedDir)
	}
}

// TestSocketServerIntegration_CommandFailure tests handling of failed commands.
func TestSocketServerIntegration_CommandFailure(t *testing.T) {
	tmpDir := shortTempDirIntegration(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	executor := NewRealExecutor()
	server := NewSocketServer("integration-secret", executor, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Send request for a command that will fail
	req := SocketRequest{
		Secret: "integration-secret",
		Request: ExecuteRequest{
			Token:   "tok_integration",
			Command: "sh",
			Args:    []string{"-c", "echo error >&2; exit 42"},
			Workdir: tmpDir,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	var resp SocketResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("Expected success (command executed), got error: %s", resp.Error)
	}
	if resp.Response.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Response.Status, StatusCompleted)
	}
	if resp.Response.ExitCode != 42 {
		t.Errorf("ExitCode: got %d, want 42", resp.Response.ExitCode)
	}
	if !strings.Contains(resp.Response.Stderr, "error") {
		t.Errorf("Stderr should contain 'error', got: %q", resp.Response.Stderr)
	}
}

// TestSocketServerIntegration_NonexistentCommand tests handling of missing executables.
func TestSocketServerIntegration_NonexistentCommand(t *testing.T) {
	tmpDir := shortTempDirIntegration(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	executor := NewRealExecutor()
	server := NewSocketServer("integration-secret", executor, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Send request for a nonexistent command
	req := SocketRequest{
		Secret: "integration-secret",
		Request: ExecuteRequest{
			Token:   "tok_integration",
			Command: "this-command-definitely-does-not-exist-12345",
			Workdir: tmpDir,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	var resp SocketResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// The request succeeded (socket communication worked)
	// but the command execution resulted in an error
	if !resp.Success {
		t.Fatalf("Expected success (socket worked), got error: %s", resp.Error)
	}
	if resp.Response.Status != StatusError {
		t.Errorf("Status: got %q, want %q", resp.Response.Status, StatusError)
	}
	if !strings.Contains(resp.Response.Error, "executable not found") {
		t.Errorf("Error should contain 'executable not found', got: %q", resp.Response.Error)
	}
}

// TestSocketServerIntegration_Timeout tests command timeout handling.
func TestSocketServerIntegration_Timeout(t *testing.T) {
	tmpDir := shortTempDirIntegration(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	executor := NewRealExecutor()
	server := NewSocketServer("integration-secret", executor, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Send request with short timeout
	req := SocketRequest{
		Secret: "integration-secret",
		Request: ExecuteRequest{
			Token:     "tok_integration",
			Command:   "sleep",
			Args:      []string{"10"},
			Workdir:   tmpDir,
			TimeoutMs: 100, // 100ms timeout
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	var resp SocketResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("Expected success (socket worked), got error: %s", resp.Error)
	}
	if resp.Response.Status != StatusTimeout {
		t.Errorf("Status: got %q, want %q", resp.Response.Status, StatusTimeout)
	}
}

// TestSocketServerIntegration_SocketPermissions verifies socket file permissions.
func TestSocketServerIntegration_SocketPermissions(t *testing.T) {
	tmpDir := shortTempDirIntegration(t)
	sockPath := filepath.Join(tmpDir, "secure.sock")

	executor := NewRealExecutor()
	server := NewSocketServer("integration-secret", executor, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Check socket permissions
	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Socket permissions: got %o, want 0600", perm)
	}

	// Verify socket is a socket (not a regular file)
	if info.Mode().Type()&os.ModeSocket == 0 {
		t.Error("File is not a socket")
	}
}

// TestSocketServerIntegration_EnvironmentVariables tests env var passing.
func TestSocketServerIntegration_EnvironmentVariables(t *testing.T) {
	tmpDir := shortTempDirIntegration(t)
	sockPath := filepath.Join(tmpDir, "test.sock")

	executor := NewRealExecutor()
	server := NewSocketServer("integration-secret", executor, WithSocketPath(sockPath))

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Send request with custom environment variables
	req := SocketRequest{
		Secret: "integration-secret",
		Request: ExecuteRequest{
			Token:   "tok_integration",
			Command: "sh",
			Args:    []string{"-c", "echo $CUSTOM_VAR"},
			Workdir: tmpDir,
			Env:     map[string]string{"CUSTOM_VAR": "custom_value_12345"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	var resp SocketResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("Expected success, got error: %s", resp.Error)
	}
	if !strings.Contains(resp.Response.Stdout, "custom_value_12345") {
		t.Errorf("Stdout should contain 'custom_value_12345', got: %q", resp.Response.Stdout)
	}
}
