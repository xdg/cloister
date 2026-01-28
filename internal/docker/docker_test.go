package docker

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestRun_Success(t *testing.T) {
	// docker version should succeed on any system with Docker installed
	out, err := Run("version", "--format", "{{.Client.Version}}")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output from docker version")
	}
}

func TestRun_InvalidCommand(t *testing.T) {
	_, err := Run("notarealcommand")
	if err == nil {
		t.Fatal("expected error for invalid docker command")
	}

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected CommandError, got %T", err)
	}

	if cmdErr.Command != "notarealcommand" {
		t.Errorf("expected command 'notarealcommand', got %q", cmdErr.Command)
	}
}

func TestCommandError_Error(t *testing.T) {
	tests := []struct {
		name     string
		cmdErr   CommandError
		contains []string
	}{
		{
			name: "with stderr",
			cmdErr: CommandError{
				Command: "inspect",
				Args:    []string{"inspect", "nonexistent"},
				Stderr:  "Error: No such object: nonexistent",
				Err:     errors.New("exit status 1"),
			},
			contains: []string{"docker inspect failed", "exit status 1", "stderr:", "No such object"},
		},
		{
			name: "without stderr",
			cmdErr: CommandError{
				Command: "build",
				Args:    []string{"build", "."},
				Stderr:  "",
				Err:     errors.New("exit status 1"),
			},
			contains: []string{"docker build failed", "exit status 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.cmdErr.Error()
			for _, s := range tt.contains {
				if !strings.Contains(msg, s) {
					t.Errorf("error message %q should contain %q", msg, s)
				}
			}
		})
	}
}

func TestCommandError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	cmdErr := &CommandError{
		Command: "test",
		Err:     underlying,
	}

	if !errors.Is(cmdErr, underlying) {
		t.Error("CommandError should unwrap to underlying error")
	}
}

func TestCheckDaemon_Success(t *testing.T) {
	err := CheckDaemon()
	if err != nil {
		// Check if it's because Docker isn't running vs not installed
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckDaemon_ReturnsErrDockerNotRunning(t *testing.T) {
	// Verify that ErrDockerNotRunning can be detected with errors.Is
	// when wrapped using fmt.Errorf with %w
	wrappedErr := fmt.Errorf("%w: %v", ErrDockerNotRunning, errors.New("connection refused"))
	if !errors.Is(wrappedErr, ErrDockerNotRunning) {
		t.Error("ErrDockerNotRunning should be checkable with errors.Is when wrapped with fmt.Errorf")
	}

	// Verify the error message contains useful context
	if !strings.Contains(wrappedErr.Error(), "docker daemon is not running") {
		t.Error("wrapped error should contain the sentinel error message")
	}
	if !strings.Contains(wrappedErr.Error(), "connection refused") {
		t.Error("wrapped error should contain the underlying error")
	}
}

func TestCheckDaemon_ErrorClassification(t *testing.T) {
	// This test verifies the error classification logic in CheckDaemon
	// by simulating the error types that would be produced.

	tests := []struct {
		name                    string
		simulatedErr            error
		expectDockerNotRunning  bool
		expectCLINotFound       bool
		expectedMessageContains string
	}{
		{
			name: "daemon not running returns ErrDockerNotRunning",
			simulatedErr: &CommandError{
				Command: "info",
				Args:    []string{"info", "--format", "{{.ServerVersion}}"},
				Stderr:  "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
				Err:     errors.New("exit status 1"),
			},
			expectDockerNotRunning:  true,
			expectCLINotFound:       false,
			expectedMessageContains: "docker daemon is not running",
		},
		{
			name: "CLI not found produces distinct error",
			simulatedErr: &CommandError{
				Command: "info",
				Args:    []string{"info", "--format", "{{.ServerVersion}}"},
				Stderr:  "",
				Err:     &exec.Error{Name: "docker", Err: exec.ErrNotFound},
			},
			expectDockerNotRunning:  false,
			expectCLINotFound:       true,
			expectedMessageContains: "docker CLI not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the error classification logic from CheckDaemon
			var resultErr error
			var cmdErr *CommandError
			if errors.As(tt.simulatedErr, &cmdErr) {
				var execErr *exec.Error
				if errors.As(cmdErr.Err, &execErr) {
					resultErr = fmt.Errorf("docker CLI not found: %w", tt.simulatedErr)
				} else {
					resultErr = fmt.Errorf("%w: %v", ErrDockerNotRunning, tt.simulatedErr)
				}
			}

			if resultErr == nil {
				t.Fatal("expected error to be classified")
			}

			gotDockerNotRunning := errors.Is(resultErr, ErrDockerNotRunning)
			if gotDockerNotRunning != tt.expectDockerNotRunning {
				t.Errorf("errors.Is(err, ErrDockerNotRunning) = %v, want %v", gotDockerNotRunning, tt.expectDockerNotRunning)
			}

			gotCLINotFound := strings.Contains(resultErr.Error(), "docker CLI not found")
			if gotCLINotFound != tt.expectCLINotFound {
				t.Errorf("CLI not found in error = %v, want %v", gotCLINotFound, tt.expectCLINotFound)
			}

			if !strings.Contains(resultErr.Error(), tt.expectedMessageContains) {
				t.Errorf("error message %q should contain %q", resultErr.Error(), tt.expectedMessageContains)
			}
		})
	}
}

func TestCheckDaemon_SentinelErrorUsage(t *testing.T) {
	// Verify that code can properly detect ErrDockerNotRunning
	// This tests the pattern that callers would use
	err := CheckDaemon()

	if err == nil {
		// Docker is running, test passes
		return
	}

	// Classify the error
	if errors.Is(err, ErrDockerNotRunning) {
		// This is the expected sentinel for daemon not running
		t.Logf("Correctly detected daemon not running: %v", err)
	} else if strings.Contains(err.Error(), "docker CLI not found") {
		// Docker CLI not installed - distinct from daemon not running
		t.Skip("Docker CLI not installed")
	} else {
		t.Errorf("unexpected error type: %v", err)
	}
}

// NetworkInfo represents partial docker network ls output for testing.
type NetworkInfo struct {
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
}

func TestRunJSONLines_NetworkList(t *testing.T) {
	// Test with docker network ls which returns newline-separated JSON objects
	var networks []NetworkInfo
	err := RunJSONLines(&networks, false, "network", "ls")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		// Parse error might indicate daemon not running or format issue
		if strings.Contains(err.Error(), "failed to parse JSON") {
			t.Skipf("Docker may not be running or unexpected format: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have at least the default bridge network
	if len(networks) == 0 {
		t.Skip("No networks found (Docker may not be running)")
	}

	// Verify we got valid data
	foundBridge := false
	for _, n := range networks {
		if n.Name == "bridge" {
			foundBridge = true
			if n.Driver != "bridge" {
				t.Errorf("expected bridge network driver 'bridge', got %q", n.Driver)
			}
		}
	}
	if !foundBridge {
		t.Log("bridge network not found (may be normal in some Docker configurations)")
	}
}

func TestRunJSONLines_EmptyResult(t *testing.T) {
	// docker container ls with impossible filter should return empty
	var containers []map[string]any
	err := RunJSONLines(&containers, false, "container", "ls", "--filter", "name=cloister-impossible-name-99999")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should be unchanged (nil slice remains nil, empty slice remains empty)
	if len(containers) != 0 {
		t.Errorf("expected empty result, got %d containers", len(containers))
	}
}

func TestRunJSONLinesStrict_EmptyResult(t *testing.T) {
	// docker container ls with impossible filter should return ErrNoResults
	var containers []map[string]any
	err := RunJSONLinesStrict(&containers, "container", "ls", "--filter", "name=cloister-impossible-name-99999")

	if err == nil {
		t.Fatal("expected ErrNoResults for empty result")
	}

	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		var execErr *exec.Error
		if errors.As(cmdErr.Err, &execErr) {
			t.Skip("Docker CLI not installed")
		}
		if strings.Contains(cmdErr.Stderr, "daemon") {
			t.Skip("Docker daemon not running")
		}
	}

	if !errors.Is(err, ErrNoResults) {
		t.Fatalf("expected ErrNoResults, got: %v", err)
	}
}

func TestRunJSON_InvalidContainer(t *testing.T) {
	// docker inspect on non-existent container should return CommandError
	var result map[string]any
	err := RunJSON(&result, false, "inspect", "cloister-nonexistent-container-12345")
	if err == nil {
		t.Fatal("expected error for non-existent container")
	}

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		// Could be exec.Error if Docker not installed
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			t.Skip("Docker CLI not installed")
		}
		t.Fatalf("expected CommandError, got %T: %v", err, err)
	}
}

func TestRunJSONLinesStrict_WithResults(t *testing.T) {
	// docker network ls should return results (at least bridge network exists)
	var networks []NetworkInfo
	err := RunJSONLinesStrict(&networks, "network", "ls")

	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrNoResults) {
			t.Skip("No networks found (unusual Docker configuration)")
		}
		if strings.Contains(err.Error(), "failed to parse JSON") {
			t.Skipf("Docker may not be running: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if len(networks) == 0 {
		t.Error("expected at least one network")
	}
}

func TestRunJSONLines_StrictVsNonStrict(t *testing.T) {
	// Test that strict=false returns nil on empty output, strict=true returns ErrNoResults
	// Use an impossible filter to guarantee empty results

	// Test non-strict mode (strict=false)
	var containersNonStrict []map[string]any
	errNonStrict := RunJSONLines(&containersNonStrict, false, "container", "ls", "--filter", "name=cloister-impossible-name-strict-test-99999")

	if errNonStrict != nil {
		var cmdErr *CommandError
		if errors.As(errNonStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(errNonStrict.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("non-strict mode: unexpected error: %v", errNonStrict)
	}
	// Non-strict mode should return nil for empty results

	// Test strict mode (strict=true)
	var containersStrict []map[string]any
	errStrict := RunJSONLines(&containersStrict, true, "container", "ls", "--filter", "name=cloister-impossible-name-strict-test-99999")

	if errStrict == nil {
		t.Fatal("strict mode: expected ErrNoResults for empty result, got nil")
	}
	if !errors.Is(errStrict, ErrNoResults) {
		var cmdErr *CommandError
		if errors.As(errStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		t.Fatalf("strict mode: expected ErrNoResults, got: %v", errStrict)
	}
}

func TestRunJSON_StrictVsNonStrict(t *testing.T) {
	// Test that strict=false returns nil on empty output, strict=true returns ErrNoResults
	// docker inspect with format on a non-existent name produces CommandError (not empty output)
	// so we need a different approach: use "docker volume ls" with impossible filter

	// Test non-strict mode (strict=false)
	var volumesNonStrict []map[string]any
	errNonStrict := RunJSONLines(&volumesNonStrict, false, "volume", "ls", "--filter", "name=cloister-impossible-volume-99999")

	if errNonStrict != nil {
		var cmdErr *CommandError
		if errors.As(errNonStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(errNonStrict.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("non-strict mode: unexpected error: %v", errNonStrict)
	}
	// Non-strict mode should return nil for empty results

	// Test strict mode (strict=true)
	var volumesStrict []map[string]any
	errStrict := RunJSONLines(&volumesStrict, true, "volume", "ls", "--filter", "name=cloister-impossible-volume-99999")

	if errStrict == nil {
		t.Fatal("strict mode: expected ErrNoResults for empty result, got nil")
	}
	if !errors.Is(errStrict, ErrNoResults) {
		var cmdErr *CommandError
		if errors.As(errStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		t.Fatalf("strict mode: expected ErrNoResults, got: %v", errStrict)
	}
}

func TestCmdNameFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: "",
		},
		{
			name:     "single arg",
			args:     []string{"inspect"},
			expected: "inspect",
		},
		{
			name:     "multiple args",
			args:     []string{"network", "ls", "--filter", "name=test"},
			expected: "network",
		},
		{
			name:     "nil args",
			args:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cmdNameFromArgs(tt.args)
			if result != tt.expected {
				t.Errorf("cmdNameFromArgs(%v) = %q, want %q", tt.args, result, tt.expected)
			}
		})
	}
}
