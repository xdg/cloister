// Package executor provides the interface and types for host command execution.
package executor

import "context"

// Executor executes commands on the host system.
type Executor interface {
	Execute(ctx context.Context, req ExecuteRequest) ExecuteResponse
}

// ExecuteRequest contains the command execution parameters.
type ExecuteRequest struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Workdir   string            `json:"workdir"`
	Env       map[string]string `json:"env,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
}

// ExecuteResponse contains the result of command execution.
type ExecuteResponse struct {
	Status   string `json:"status"` // "completed", "timeout", "error"
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Status constants for ExecuteResponse.Status.
const (
	StatusCompleted = "completed"
	StatusTimeout   = "timeout"
	StatusError     = "error"
)
