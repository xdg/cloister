//go:build integration

package guardian

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/executor"
)

// requireDocker skips the test if Docker is not available.
// Note: We can't use testutil.RequireDocker here because testutil imports guardian,
// which would create an import cycle.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := docker.CheckDaemon(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

// requireCloisterBinary ensures the cloister binary is built and sets CLOISTER_EXECUTABLE.
// Skips the test if the binary doesn't exist.
func requireCloisterBinary(t *testing.T) {
	t.Helper()

	// Find repo root (go up from this file's directory)
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("Could not determine test file location")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	binaryPath := filepath.Join(repoRoot, "cloister")

	if _, err := os.Stat(binaryPath); err != nil {
		t.Skipf("cloister binary not found at %s (run 'make build' first)", binaryPath)
	}

	// Set env var for StartExecutor to use
	t.Setenv(ExecutableEnvVar, binaryPath)
}

// requireCleanGuardianState ensures no guardian is running and registers cleanup.
// Generates a unique instance ID for test isolation, preventing conflicts with
// production guardians or other concurrent tests.
func requireCleanGuardianState(t *testing.T) {
	t.Helper()
	requireDocker(t)
	// Isolate XDG dirs to avoid writing to real ~/.config/cloister.
	// Can't use testutil.IsolateXDGDirs due to import cycle.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	// Generate unique instance ID for test isolation
	t.Setenv(InstanceIDEnvVar, GenerateInstanceID())
	// Capture container name now while instance ID is set
	containerName := ContainerName()
	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error: %v", err)
	}
	if running {
		// With instance isolation this should not happen, but check anyway
		t.Skip("Skipping: guardian instance already running")
	}
	t.Cleanup(func() {
		// Best effort cleanup - ignore errors
		_ = StopExecutor()
		_, _ = docker.Run("stop", containerName)
		_, _ = docker.Run("rm", containerName)
	})
}

// TestGuardian_WhenNotRunning tests guardian behavior when no container exists.
func TestGuardian_WhenNotRunning(t *testing.T) {
	requireCleanGuardianState(t)

	t.Run("IsRunning_ReturnsFalse", func(t *testing.T) {
		running, err := IsRunning()
		if err != nil {
			t.Fatalf("IsRunning() error: %v", err)
		}
		if running {
			t.Error("IsRunning() = true, want false when container doesn't exist")
		}
	})

	t.Run("GetContainerState_ReturnsNil", func(t *testing.T) {
		state, err := getContainerState()
		if err != nil {
			t.Fatalf("getContainerState() error: %v", err)
		}
		if state != nil {
			t.Error("getContainerState() should return nil when container doesn't exist")
		}
	})

	t.Run("Stop_IsIdempotent", func(t *testing.T) {
		if err := Stop(); err != nil {
			t.Fatalf("Stop() error when not running: %v", err)
		}
	})
}

// TestGuardian_Lifecycle tests guardian start/stop behavior.
func TestGuardian_Lifecycle(t *testing.T) {
	requireCleanGuardianState(t)
	requireCloisterBinary(t)

	// Start the guardian (required for all subtests)
	err := EnsureRunning()
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Test requires cloister:latest image: %v", err)
		}
		t.Fatalf("EnsureRunning() error: %v", err)
	}

	t.Run("IsRunning_ReturnsTrue", func(t *testing.T) {
		running, err := IsRunning()
		if err != nil {
			t.Fatalf("IsRunning() error: %v", err)
		}
		if !running {
			t.Error("Guardian should be running after EnsureRunning()")
		}
	})

	t.Run("EnsureRunning_IsIdempotent", func(t *testing.T) {
		if err := EnsureRunning(); err != nil {
			t.Fatalf("Second EnsureRunning() error: %v", err)
		}
	})

	t.Run("Start_ReturnsErrorWhenRunning", func(t *testing.T) {
		err := Start()
		if !errors.Is(err, ErrGuardianAlreadyRunning) {
			t.Errorf("Start() = %v, want ErrGuardianAlreadyRunning", err)
		}
	})

	t.Run("Executor_TCPPortAvailable", func(t *testing.T) {
		state, err := executor.LoadDaemonState()
		if err != nil {
			t.Fatalf("LoadDaemonState() error: %v", err)
		}
		if state == nil {
			t.Fatal("Expected daemon state to exist after guardian start")
		}
		if state.TCPPort == 0 {
			t.Error("Expected TCPPort to be set in daemon state")
		}
	})

	t.Run("Executor_DaemonRunning", func(t *testing.T) {
		state, err := executor.LoadDaemonState()
		if err != nil {
			t.Fatalf("LoadDaemonState() error: %v", err)
		}
		if state == nil {
			t.Fatal("Expected daemon state to exist after guardian start")
		}
		if !executor.IsDaemonRunning(state) {
			t.Errorf("Executor daemon should be running (PID %d)", state.PID)
		}
	})

	t.Run("Executor_ClientCanExecuteCommand", func(t *testing.T) {
		// Load daemon state to get the TCP port and secret
		state, err := executor.LoadDaemonState()
		if err != nil {
			t.Fatalf("LoadDaemonState() error: %v", err)
		}
		if state == nil {
			t.Fatal("Expected daemon state to exist after guardian start")
		}

		// Connect to executor via TCP (using localhost since we're on the host)
		addr := fmt.Sprintf("127.0.0.1:%d", state.TCPPort)
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", addr)
		if err != nil {
			t.Fatalf("Failed to connect to executor at %s: %v", addr, err)
		}
		defer func() { _ = conn.Close() }()

		// Build request
		// Token validation is handled by the guardian before forwarding to executor
		req := executor.SocketRequest{
			Secret: state.Secret,
			Request: executor.ExecuteRequest{
				Command: "echo",
				Args:    []string{"hello"},
			},
		}

		// Send request
		reqData, _ := json.Marshal(req)
		reqData = append(reqData, '\n')
		if _, err := conn.Write(reqData); err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}

		// Read response
		reader := bufio.NewReader(conn)
		respLine, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		var socketResp executor.SocketResponse
		if err := json.Unmarshal(respLine, &socketResp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if !socketResp.Success {
			t.Fatalf("Request failed: %s", socketResp.Error)
		}

		resp := socketResp.Response

		// Verify the response
		if resp.Status != executor.StatusCompleted {
			t.Errorf("Status = %q, want %q", resp.Status, executor.StatusCompleted)
		}
		if resp.ExitCode != 0 {
			t.Errorf("ExitCode = %d, want 0", resp.ExitCode)
		}
		// Note: echo adds a newline, so we expect "hello\n"
		expectedStdout := "hello\n"
		if resp.Stdout != expectedStdout {
			t.Errorf("Stdout = %q, want %q", resp.Stdout, expectedStdout)
		}
		if resp.Stderr != "" {
			t.Errorf("Stderr = %q, want empty", resp.Stderr)
		}
	})

	// Stop guardian and verify cleanup
	t.Run("Stop_CleansUpExecutor", func(t *testing.T) {
		// Stop the guardian
		if err := Stop(); err != nil {
			t.Fatalf("Stop() error: %v", err)
		}

		// Verify guardian is stopped
		running, err := IsRunning()
		if err != nil {
			t.Fatalf("IsRunning() error: %v", err)
		}
		if running {
			t.Error("Guardian should not be running after Stop()")
		}

		// Verify executor state is cleaned up
		state, err := executor.LoadDaemonState()
		if err != nil {
			t.Fatalf("LoadDaemonState() error: %v", err)
		}
		if state != nil && executor.IsDaemonRunning(state) {
			t.Errorf("Executor daemon should not be running after Stop() (PID %d)", state.PID)
		}
	})
}
