// Package request defines types for hostexec command requests and responses
// between cloister containers and the guardian request server.
package request

// CommandRequest represents a command execution request from a cloister container.
type CommandRequest struct {
	Cmd string `json:"cmd"`
}

// CommandResponse represents the result of a command execution request.
type CommandResponse struct {
	// Status indicates the outcome of the request.
	// Possible values: "approved", "auto_approved", "denied", "timeout", "error"
	Status string `json:"status"`

	// Pattern is the matched pattern that triggered auto-approval.
	// Only set when Status is "auto_approved".
	Pattern string `json:"pattern,omitempty"`

	// Reason explains why a request was denied or timed out.
	// Only set when Status is "denied", "timeout", or "error".
	Reason string `json:"reason,omitempty"`

	// ExitCode is the command's exit code.
	// Only set when Status is "approved" or "auto_approved".
	ExitCode int `json:"exit_code,omitempty"`

	// Stdout is the command's standard output.
	// Only set when Status is "approved" or "auto_approved".
	Stdout string `json:"stdout,omitempty"`

	// Stderr is the command's standard error output.
	// Only set when Status is "approved" or "auto_approved".
	Stderr string `json:"stderr,omitempty"`
}
