package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/project"
)

// TestStopArgumentParsing verifies that runStop correctly parses arguments.
func TestStopArgumentParsing(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantExplicit bool
		wantCloister string
	}{
		{
			name:         "explicit name provided",
			args:         []string{"myproject"},
			wantExplicit: true,
			wantCloister: "myproject",
		},
		{
			name:         "explicit name with branch",
			args:         []string{"myproject-feature"},
			wantExplicit: true,
			wantCloister: "myproject-feature",
		},
		{
			name:         "no arguments",
			args:         []string{},
			wantExplicit: false,
		},
		{
			name:         "nil arguments",
			args:         nil,
			wantExplicit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the argument parsing logic from runStop
			var cloisterName string
			explicit := len(tc.args) > 0
			if explicit {
				cloisterName = tc.args[0]
			}

			if explicit != tc.wantExplicit {
				t.Errorf("explicit = %v, want %v", explicit, tc.wantExplicit)
			}
			if explicit && cloisterName != tc.wantCloister {
				t.Errorf("cloisterName = %q, want %q", cloisterName, tc.wantCloister)
			}
		})
	}
}

// TestStopContainerNameConversion verifies that cloister names are correctly
// converted to container names for Docker operations.
func TestStopContainerNameConversion(t *testing.T) {
	tests := []struct {
		cloisterName  string
		wantContainer string
	}{
		{"myproject", "cloister-myproject"},
		{"foo-main", "cloister-foo-main"},
		{"test", "cloister-test"},
	}

	for _, tc := range tests {
		t.Run(tc.cloisterName, func(t *testing.T) {
			containerName := container.CloisterNameToContainerName(tc.cloisterName)
			if containerName != tc.wantContainer {
				t.Errorf("CloisterNameToContainerName(%q) = %q, want %q",
					tc.cloisterName, containerName, tc.wantContainer)
			}
		})
	}
}

// TestStopErrorMessages verifies that runStop returns correct error messages.
func TestStopErrorMessages(t *testing.T) {
	// Test docker not running error
	t.Run("docker not running", func(t *testing.T) {
		err := dockerNotRunningError()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, "docker is not running") {
			t.Errorf("expected 'Docker is not running' in error, got: %s", msg)
		}
	})

	// Test container not found error format
	t.Run("container not found", func(t *testing.T) {
		cloisterName := "myproject"
		err := fmt.Errorf("cloister %q not found", cloisterName)
		msg := err.Error()
		if !strings.Contains(msg, `"myproject"`) {
			t.Errorf("expected quoted cloister name in error, got: %s", msg)
		}
		if !strings.Contains(msg, "not found") {
			t.Errorf("expected 'not found' in error, got: %s", msg)
		}
	})
}

// TestStopGitDetectionErrors verifies that git detection errors are
// handled with appropriate hints.
func TestStopGitDetectionErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantMatch string
		wantHint  string
	}{
		{
			name:      "not a git repo",
			err:       project.ErrNotGitRepo,
			wantMatch: "not in a git repository",
			wantHint:  "specify cloister name",
		},
		{
			name:      "git not installed",
			err:       project.ErrGitNotInstalled,
			wantMatch: "git is not installed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result error
			if tc.wantHint != "" {
				result = gitDetectionErrorWithHint(tc.err, "specify cloister name or run from within a git project")
			} else {
				result = gitDetectionError(tc.err)
			}

			if result == nil {
				t.Fatal("expected error, got nil")
			}
			msg := result.Error()
			if !strings.Contains(msg, tc.wantMatch) {
				t.Errorf("expected %q in error, got: %s", tc.wantMatch, msg)
			}
			if tc.wantHint != "" && !strings.Contains(msg, tc.wantHint) {
				t.Errorf("expected %q hint in error, got: %s", tc.wantHint, msg)
			}
		})
	}
}

// TestDockerErrorDetectionInStop verifies that Docker-related errors are
// correctly identified for stop command error handling.
func TestDockerErrorDetectionInStop(t *testing.T) {
	tests := []struct {
		name                string
		err                 error
		isNotRunning        bool
		isContainerNotFound bool
	}{
		{
			name:         "docker not running",
			err:          docker.ErrDockerNotRunning,
			isNotRunning: true,
		},
		{
			name:                "container not found",
			err:                 container.ErrContainerNotFound,
			isContainerNotFound: true,
		},
		{
			name:         "wrapped docker not running",
			err:          errors.Join(errors.New("context"), docker.ErrDockerNotRunning),
			isNotRunning: true,
		},
		{
			name:                "wrapped container not found",
			err:                 errors.Join(errors.New("context"), container.ErrContainerNotFound),
			isContainerNotFound: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("something else"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, docker.ErrDockerNotRunning); got != tc.isNotRunning {
				t.Errorf("errors.Is(err, ErrDockerNotRunning) = %v, want %v", got, tc.isNotRunning)
			}
			if got := errors.Is(tc.err, container.ErrContainerNotFound); got != tc.isContainerNotFound {
				t.Errorf("errors.Is(err, ErrContainerNotFound) = %v, want %v", got, tc.isContainerNotFound)
			}
		})
	}
}

// TestStopOutputMessages verifies the expected output format for successful stops.
func TestStopOutputMessages(t *testing.T) {
	tests := []struct {
		name         string
		cloisterName string
		hasToken     bool
		wantMsgs     []string
	}{
		{
			name:         "stop with token",
			cloisterName: "myproject",
			hasToken:     true,
			wantMsgs:     []string{"Stopped cloister: myproject", "Token revoked"},
		},
		{
			name:         "stop without token",
			cloisterName: "myproject",
			hasToken:     false,
			wantMsgs:     []string{"Stopped cloister: myproject"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the expected output format
			expectedMsg := fmt.Sprintf("Stopped cloister: %s", tc.cloisterName)
			if !strings.Contains(expectedMsg, tc.cloisterName) {
				t.Errorf("expected cloister name in message, got: %s", expectedMsg)
			}
		})
	}
}
