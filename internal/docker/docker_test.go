package docker

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

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

func TestCheckDaemon_ReturnsErrDockerNotRunning(t *testing.T) {
	wrappedErr := fmt.Errorf("%w: %w", ErrDockerNotRunning, errors.New("connection refused"))
	if !errors.Is(wrappedErr, ErrDockerNotRunning) {
		t.Error("ErrDockerNotRunning should be checkable with errors.Is when wrapped with fmt.Errorf")
	}

	if !strings.Contains(wrappedErr.Error(), "docker daemon is not running") {
		t.Error("wrapped error should contain the sentinel error message")
	}
	if !strings.Contains(wrappedErr.Error(), "connection refused") {
		t.Error("wrapped error should contain the underlying error")
	}
}

func TestCheckDaemon_ErrorClassification(t *testing.T) {
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
			var resultErr error
			var cmdErr *CommandError
			if errors.As(tt.simulatedErr, &cmdErr) {
				var execErr *exec.Error
				if errors.As(cmdErr.Err, &execErr) {
					resultErr = fmt.Errorf("docker CLI not found: %w", tt.simulatedErr)
				} else {
					resultErr = fmt.Errorf("%w: %w", ErrDockerNotRunning, tt.simulatedErr)
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

func TestContainerInfo_Name(t *testing.T) {
	tests := []struct {
		name     string
		names    string
		expected string
	}{
		{
			name:     "with leading slash",
			names:    "/my-container",
			expected: "my-container",
		},
		{
			name:     "without leading slash",
			names:    "my-container",
			expected: "my-container",
		},
		{
			name:     "empty",
			names:    "",
			expected: "",
		},
		{
			name:     "slash only",
			names:    "/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &ContainerInfo{Names: tt.names}
			if got := info.Name(); got != tt.expected {
				t.Errorf("ContainerInfo{Names: %q}.Name() = %q, want %q", tt.names, got, tt.expected)
			}
		})
	}
}
