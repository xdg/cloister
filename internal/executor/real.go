// Package executor provides the interface and types for host command execution.
package executor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// RealExecutor executes commands using os/exec.
type RealExecutor struct{}

// NewRealExecutor creates a new RealExecutor.
func NewRealExecutor() *RealExecutor {
	return &RealExecutor{}
}

// Execute runs a command and returns the result.
func (e *RealExecutor) Execute(ctx context.Context, req ExecuteRequest) ExecuteResponse {
	// Apply timeout if specified
	if req.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	// Create command with context for timeout support
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)

	// Set working directory if specified
	if req.Workdir != "" {
		cmd.Dir = req.Workdir
	}

	// Merge environment variables
	if len(req.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()

	// Handle different error cases
	if err != nil {
		// Check if context was canceled or timed out
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			return ExecuteResponse{
				Status:   StatusTimeout,
				ExitCode: -1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Error:    "command timed out",
			}
		}

		// Check if executable was not found
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return ExecuteResponse{
				Status: StatusError,
				Error:  "executable not found: " + req.Command,
			}
		}

		// Check for exit error (command ran but returned non-zero)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ExecuteResponse{
				Status:   StatusCompleted,
				ExitCode: exitErr.ExitCode(),
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
			}
		}

		// Other errors (e.g., permission denied, etc.)
		return ExecuteResponse{
			Status: StatusError,
			Stdout: stdout.String(),
			Stderr: stderr.String(),
			Error:  err.Error(),
		}
	}

	// Success
	return ExecuteResponse{
		Status:   StatusCompleted,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}
