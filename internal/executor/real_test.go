package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealExecutorInterface verifies RealExecutor implements Executor.
func TestRealExecutorInterface(_ *testing.T) {
	var _ Executor = &RealExecutor{}
	var _ Executor = NewRealExecutor()
}

// TestRealExecutorEchoHello verifies basic command execution.
func TestRealExecutorEchoHello(t *testing.T) {
	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command: "echo",
		Args:    []string{"hello"},
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusCompleted)
	}
	if resp.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", resp.ExitCode)
	}
	if !strings.Contains(resp.Stdout, "hello") {
		t.Errorf("Stdout should contain 'hello', got: %q", resp.Stdout)
	}
	if resp.Error != "" {
		t.Errorf("Error should be empty, got: %q", resp.Error)
	}
}

// TestRealExecutorNonexistentCommand verifies error handling for missing executables.
func TestRealExecutorNonexistentCommand(t *testing.T) {
	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command: "this-command-definitely-does-not-exist-anywhere",
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusError {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusError)
	}
	if !strings.Contains(resp.Error, "executable not found") {
		t.Errorf("Error should contain 'executable not found', got: %q", resp.Error)
	}
}

// TestRealExecutorTimeout verifies timeout handling.
func TestRealExecutorTimeout(t *testing.T) {
	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command:   "sleep",
		Args:      []string{"10"},
		TimeoutMs: 100, // 100ms timeout, command sleeps for 10s
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusTimeout {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusTimeout)
	}
	if resp.ExitCode != -1 {
		t.Errorf("ExitCode: got %d, want -1", resp.ExitCode)
	}
	if !strings.Contains(resp.Error, "timed out") {
		t.Errorf("Error should contain 'timed out', got: %q", resp.Error)
	}
}

// TestRealExecutorWorkdir verifies working directory is set correctly.
func TestRealExecutorWorkdir(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command: "pwd",
		Workdir: tmpDir,
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusCompleted)
	}
	if resp.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", resp.ExitCode)
	}
	// On macOS, /tmp is a symlink to /private/tmp, so resolve both
	expectedDir, _ := filepath.EvalSymlinks(tmpDir)
	actualDir := strings.TrimSpace(resp.Stdout)
	actualDir, _ = filepath.EvalSymlinks(actualDir)
	if actualDir != expectedDir {
		t.Errorf("Workdir: got %q, want %q", actualDir, expectedDir)
	}
}

// TestRealExecutorEnv verifies environment variables are merged correctly.
func TestRealExecutorEnv(t *testing.T) {
	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command: "sh",
		Args:    []string{"-c", "echo $TEST_VAR"},
		Env:     map[string]string{"TEST_VAR": "test_value_12345"},
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusCompleted)
	}
	if !strings.Contains(resp.Stdout, "test_value_12345") {
		t.Errorf("Stdout should contain 'test_value_12345', got: %q", resp.Stdout)
	}
}

// TestRealExecutorExitCode verifies non-zero exit codes are captured.
func TestRealExecutorExitCode(t *testing.T) {
	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusCompleted)
	}
	if resp.ExitCode != 42 {
		t.Errorf("ExitCode: got %d, want 42", resp.ExitCode)
	}
}

// TestRealExecutorStderr verifies stderr is captured.
func TestRealExecutorStderr(t *testing.T) {
	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command: "sh",
		Args:    []string{"-c", "echo error_message >&2"},
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusCompleted)
	}
	if !strings.Contains(resp.Stderr, "error_message") {
		t.Errorf("Stderr should contain 'error_message', got: %q", resp.Stderr)
	}
}

// TestRealExecutorContextCancelled verifies context cancellation is handled.
func TestRealExecutorContextCancelled(t *testing.T) {
	executor := NewRealExecutor()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := ExecuteRequest{
		Command: "sleep",
		Args:    []string{"10"},
	}

	resp := executor.Execute(ctx, req)

	// Should get timeout status since context was canceled
	if resp.Status != StatusTimeout {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusTimeout)
	}
}

// TestRealExecutorPreserveInheritedEnv verifies that when custom env is set,
// the inherited environment is preserved.
func TestRealExecutorPreserveInheritedEnv(t *testing.T) {
	// Set a test env var that should be inherited
	_ = os.Setenv("EXECUTOR_TEST_INHERITED", "inherited_value")
	defer func() { _ = os.Unsetenv("EXECUTOR_TEST_INHERITED") }()

	executor := NewRealExecutor()
	req := ExecuteRequest{
		Command: "sh",
		Args:    []string{"-c", "echo $EXECUTOR_TEST_INHERITED $TEST_CUSTOM"},
		Env:     map[string]string{"TEST_CUSTOM": "custom_value"},
	}

	resp := executor.Execute(context.Background(), req)

	if resp.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", resp.Status, StatusCompleted)
	}
	if !strings.Contains(resp.Stdout, "inherited_value") {
		t.Errorf("Stdout should contain inherited env 'inherited_value', got: %q", resp.Stdout)
	}
	if !strings.Contains(resp.Stdout, "custom_value") {
		t.Errorf("Stdout should contain custom env 'custom_value', got: %q", resp.Stdout)
	}
}
