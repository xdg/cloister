// Package executor provides the interface and types for host command execution.
package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// DaemonState tracks the state of the executor daemon.
type DaemonState struct {
	PID        int    `json:"pid"`
	Secret     string `json:"secret"`
	SocketPath string `json:"socket_path"`
}

// DaemonStateDir returns the directory for daemon state files.
// This is ~/.local/share/cloister.
func DaemonStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "cloister"), nil
}

// DaemonStatePath returns the path to the daemon state file.
func DaemonStatePath() (string, error) {
	dir, err := DaemonStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "executor.json"), nil
}

// SaveDaemonState saves the daemon state to disk.
func SaveDaemonState(state *DaemonState) error {
	path, err := DaemonStatePath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}

// LoadDaemonState loads the daemon state from disk.
// Returns nil if the state file doesn't exist.
func LoadDaemonState() (*DaemonState, error) {
	path, err := DaemonStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read state: %w", err)
	}

	var state DaemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}

// RemoveDaemonState removes the daemon state file.
func RemoveDaemonState() error {
	path, err := DaemonStatePath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state: %w", err)
	}
	return nil
}

// IsDaemonRunning checks if the daemon process is still running.
func IsDaemonRunning(state *DaemonState) bool {
	if state == nil || state.PID == 0 {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(state.PID)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds.
	// Send signal 0 to check if process exists.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// StopDaemon stops the daemon process.
func StopDaemon(state *DaemonState) error {
	if state == nil || state.PID == 0 {
		return nil
	}

	process, err := os.FindProcess(state.PID)
	if err != nil {
		// On Unix, FindProcess should never fail, but if it does,
		// there's nothing to stop, so we treat this as success.
		return nil //nolint:nilerr // process doesn't exist, nothing to stop
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead, which is fine
		return nil //nolint:nilerr // process already dead, nothing to stop
	}

	return nil
}

// CleanupStaleState removes the state file if the process is not running.
// This handles cases where the daemon crashed without cleanup.
func CleanupStaleState() error {
	state, err := LoadDaemonState()
	if err != nil {
		return err
	}

	if state != nil && !IsDaemonRunning(state) {
		// Remove stale socket file
		if state.SocketPath != "" {
			os.Remove(state.SocketPath)
		}
		// Remove stale state file
		return RemoveDaemonState()
	}

	return nil
}

// GetPIDString returns the PID as a string for environment variable passing.
func GetPIDString() string {
	return strconv.Itoa(os.Getpid())
}
