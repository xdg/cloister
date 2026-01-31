// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/token"
)

// HostSocketDir returns the directory for the hostexec socket on the host.
// This is ~/.local/share/cloister.
func HostSocketDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "cloister"), nil
}

// HostSocketPath returns the path to the hostexec socket on the host.
// This is ~/.local/share/cloister/hostexec.sock.
func HostSocketPath() (string, error) {
	dir, err := HostSocketDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hostexec.sock"), nil
}

// ContainerSocketPath is the path to the hostexec socket inside the guardian container.
const ContainerSocketPath = "/var/run/hostexec.sock"

// SharedSecretEnvVar is the environment variable name for the shared secret.
const SharedSecretEnvVar = "CLOISTER_SHARED_SECRET"

// ExecutableEnvVar is the environment variable to override the cloister binary path.
// Used in tests to point to a built binary instead of os.Executable().
const ExecutableEnvVar = "CLOISTER_EXECUTABLE"

// getExecutablePath returns the path to the cloister binary.
// Checks CLOISTER_EXECUTABLE env var first, falls back to os.Executable().
func getExecutablePath() (string, error) {
	if path := os.Getenv(ExecutableEnvVar); path != "" {
		return path, nil
	}
	return os.Executable()
}

// ExecutorInfo contains information about a started executor process.
type ExecutorInfo struct {
	SocketPath string
	Secret     string
	Process    *os.Process
}

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

	// Get socket path
	socketPath, err := HostSocketPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get socket path: %w", err)
	}

	// Get path to cloister binary
	executablePath, err := getExecutablePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	// Start the executor process in the background
	cmd := exec.Command(executablePath, "executor", "run")
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

	// Poll for the socket to be created (up to 2 seconds)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			// Socket exists
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Final check - verify socket was created
	if _, err := os.Stat(socketPath); err != nil {
		// Executor may have failed to start, try to clean up
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("executor failed to create socket: %w", err)
	}

	return &ExecutorInfo{
		SocketPath: socketPath,
		Secret:     secret,
		Process:    cmd.Process,
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
