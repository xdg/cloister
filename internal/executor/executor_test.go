package executor

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecuteRequestJSONRoundTrip verifies ExecuteRequest serializes and deserializes correctly.
func TestExecuteRequestJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  ExecuteRequest
	}{
		{
			name: "minimal request",
			req: ExecuteRequest{
				Token:   "tok_abc123",
				Command: "echo",
			},
		},
		{
			name: "full request",
			req: ExecuteRequest{
				Token:     "tok_xyz789",
				Command:   "docker",
				Args:      []string{"compose", "up", "-d"},
				Workdir:   "/work",
				Env:       map[string]string{"FOO": "bar", "BAZ": "qux"},
				TimeoutMs: 30000,
			},
		},
		{
			name: "empty args",
			req: ExecuteRequest{
				Token:   "tok_empty",
				Command: "pwd",
				Args:    []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got ExecuteRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Compare fields
			if got.Token != tt.req.Token {
				t.Errorf("Token: got %q, want %q", got.Token, tt.req.Token)
			}
			if got.Command != tt.req.Command {
				t.Errorf("Command: got %q, want %q", got.Command, tt.req.Command)
			}
			if got.Workdir != tt.req.Workdir {
				t.Errorf("Workdir: got %q, want %q", got.Workdir, tt.req.Workdir)
			}
			if got.TimeoutMs != tt.req.TimeoutMs {
				t.Errorf("TimeoutMs: got %d, want %d", got.TimeoutMs, tt.req.TimeoutMs)
			}
			if len(got.Args) != len(tt.req.Args) {
				t.Errorf("Args length: got %d, want %d", len(got.Args), len(tt.req.Args))
			} else {
				for i, arg := range tt.req.Args {
					if got.Args[i] != arg {
						t.Errorf("Args[%d]: got %q, want %q", i, got.Args[i], arg)
					}
				}
			}
			if len(got.Env) != len(tt.req.Env) {
				t.Errorf("Env length: got %d, want %d", len(got.Env), len(tt.req.Env))
			} else {
				for k, v := range tt.req.Env {
					if got.Env[k] != v {
						t.Errorf("Env[%s]: got %q, want %q", k, got.Env[k], v)
					}
				}
			}
		})
	}
}

// TestExecuteResponseJSONRoundTrip verifies ExecuteResponse serializes and deserializes correctly.
func TestExecuteResponseJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		resp ExecuteResponse
	}{
		{
			name: "completed success",
			resp: ExecuteResponse{
				Status:   StatusCompleted,
				ExitCode: 0,
				Stdout:   "hello world\n",
			},
		},
		{
			name: "completed with exit code",
			resp: ExecuteResponse{
				Status:   StatusCompleted,
				ExitCode: 1,
				Stdout:   "partial output",
				Stderr:   "error: something failed",
			},
		},
		{
			name: "timeout",
			resp: ExecuteResponse{
				Status:   StatusTimeout,
				ExitCode: -1,
				Stdout:   "partial output before timeout",
				Error:    "command timed out after 30s",
			},
		},
		{
			name: "error",
			resp: ExecuteResponse{
				Status: StatusError,
				Error:  "executable not found: foo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got ExecuteResponse
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if got.Status != tt.resp.Status {
				t.Errorf("Status: got %q, want %q", got.Status, tt.resp.Status)
			}
			if got.ExitCode != tt.resp.ExitCode {
				t.Errorf("ExitCode: got %d, want %d", got.ExitCode, tt.resp.ExitCode)
			}
			if got.Stdout != tt.resp.Stdout {
				t.Errorf("Stdout: got %q, want %q", got.Stdout, tt.resp.Stdout)
			}
			if got.Stderr != tt.resp.Stderr {
				t.Errorf("Stderr: got %q, want %q", got.Stderr, tt.resp.Stderr)
			}
			if got.Error != tt.resp.Error {
				t.Errorf("Error: got %q, want %q", got.Error, tt.resp.Error)
			}
		})
	}
}

// TestStatusConstants verifies the status constants have expected values.
func TestStatusConstants(t *testing.T) {
	if StatusCompleted != "completed" {
		t.Errorf("StatusCompleted: got %q, want %q", StatusCompleted, "completed")
	}
	if StatusTimeout != "timeout" {
		t.Errorf("StatusTimeout: got %q, want %q", StatusTimeout, "timeout")
	}
	if StatusError != "error" {
		t.Errorf("StatusError: got %q, want %q", StatusError, "error")
	}
}

// TestExecutorInterface verifies that mock implementations can satisfy the interface.
func TestExecutorInterface(t *testing.T) {
	// This test verifies the interface compiles and can be implemented.
	var _ Executor = &mockExecutor{}
}

// mockExecutor is a test implementation of Executor.
type mockExecutor struct {
	response ExecuteResponse
}

func (m *mockExecutor) Execute(ctx context.Context, req ExecuteRequest) ExecuteResponse {
	return m.response
}

// TestMockExecutor verifies the mock executor works as expected.
func TestMockExecutor(t *testing.T) {
	expected := ExecuteResponse{
		Status:   StatusCompleted,
		ExitCode: 0,
		Stdout:   "test output",
	}

	mock := &mockExecutor{response: expected}
	req := ExecuteRequest{
		Token:   "test-token",
		Command: "echo",
		Args:    []string{"hello"},
	}

	got := mock.Execute(context.Background(), req)

	if got.Status != expected.Status {
		t.Errorf("Status: got %q, want %q", got.Status, expected.Status)
	}
	if got.Stdout != expected.Stdout {
		t.Errorf("Stdout: got %q, want %q", got.Stdout, expected.Stdout)
	}
}
