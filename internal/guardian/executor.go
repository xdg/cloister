// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/token"
)

// SharedSecretEnvVar is the environment variable name for the shared secret.
const SharedSecretEnvVar = "CLOISTER_SHARED_SECRET" //nolint:gosec // G101: not a credential

// ExecutableEnvVar is the environment variable to override the cloister binary path.
// Used in tests to point to a built binary instead of os.Executable().
const ExecutableEnvVar = "CLOISTER_EXECUTABLE"

// getExecutablePath returns the path to the cloister binary.
// Checks CLOISTER_EXECUTABLE env var first, falls back to os.Executable().
func getExecutablePath() (string, error) {
	if path := os.Getenv(ExecutableEnvVar); path != "" {
		return path, nil
	}
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	return path, nil
}

// ExecutorInfo contains information about a started executor process.
type ExecutorInfo struct {
	SocketPath string // Deprecated: use TCPPort
	TCPPort    int    // Port for TCP mode
	Secret     string
	Process    *os.Process
}

// ExecutorPortEnvVar is the environment variable for the executor TCP port.
const ExecutorPortEnvVar = "CLOISTER_EXECUTOR_PORT"

// StartExecutor starts the executor daemon process.
// It cleans up any stale state, generates a shared secret, and spawns the executor.
// Returns ExecutorInfo needed to start the guardian container.
func StartExecutor() (*ExecutorInfo, error) {
	// Clean up any stale executor state
	if err := executor.CleanupStaleState(); err != nil {
		// Log warning but continue - stale state shouldn't block startup
		_ = err
	}

	// Generate shared secret for executor authentication
	secret := token.Generate()

	// Get path to cloister binary
	executablePath, err := getExecutablePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	// Start the executor process in the background
	cmd := exec.CommandContext(context.Background(), executablePath, "executor", "run") //nolint:gosec // G204: args are not user-controlled
	cmd.Env = append(os.Environ(), SharedSecretEnvVar+"="+secret)
	// Detach from parent process group so it survives parent exit
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	// Capture stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start executor: %w", err)
	}

	// Poll for the daemon state file (contains the port) to be created (up to 2 seconds)
	var state *executor.DaemonState
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, err = executor.LoadDaemonState()
		if err == nil && state != nil && state.TCPPort > 0 {
			// Daemon state written with port
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Final check - verify daemon state was written with port
	if state == nil || state.TCPPort == 0 {
		// Executor may have failed to start, try to clean up
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("executor failed to start (no port in daemon state)")
	}

	return &ExecutorInfo{
		TCPPort: state.TCPPort,
		Secret:  secret,
		Process: cmd.Process,
	}, nil
}

// StopExecutor stops the executor daemon process.
// It loads the daemon state, sends SIGTERM, and cleans up state files.
// Returns nil if no executor is running (idempotent).
func StopExecutor() error {
	state, err := executor.LoadDaemonState()
	if err != nil {
		return fmt.Errorf("failed to load executor state: %w", err)
	}

	if state == nil {
		// No executor running
		return nil
	}

	// Send SIGTERM for graceful shutdown
	if err := executor.StopDaemon(state); err != nil {
		// Log but continue - process may already be dead
		_ = err
	}

	// Give it a moment to shut down gracefully
	time.Sleep(100 * time.Millisecond)

	// Clean up state file (executor should have done this, but be safe)
	if err := executor.RemoveDaemonState(); err != nil {
		// Ignore errors - file may already be removed
		_ = err
	}

	// Clean up socket file if it still exists
	if state.SocketPath != "" {
		_ = os.Remove(state.SocketPath)
	}

	return nil
}
