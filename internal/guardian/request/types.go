// Package request defines types for hostexec command requests and responses
// between cloister containers and the guardian request server.
package request

// CommandRequest represents a command execution request from a cloister container.
type CommandRequest struct {
	// Cmd is DEPRECATED and ignored. The canonical command string is now
	// reconstructed from Args using shell quoting rules. This field is kept
	// for backwards compatibility but has no effect on request processing.
	Cmd string `json:"cmd,omitempty"`

	// Args is the tokenized argument array for execution and pattern matching.
	// Args[0] is the command/executable, Args[1:] are the arguments.
	// Using a pre-tokenized array prevents shell injection attacks.
	// The guardian reconstructs the canonical command string from Args.
	Args []string `json:"args"`
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
