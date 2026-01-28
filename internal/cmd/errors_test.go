package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/project"
)

func TestDockerNotRunningError(t *testing.T) {
	err := dockerNotRunningError()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "Docker is not running") {
		t.Errorf("expected error to mention Docker not running, got: %s", msg)
	}
	if !strings.Contains(msg, "please start Docker") {
		t.Errorf("expected error to include hint to start Docker, got: %s", msg)
	}
}

func TestGitDetectionError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expectNil   bool
		expectMatch string
	}{
		{
			name:        "ErrNotGitRepo",
			err:         project.ErrNotGitRepo,
			expectNil:   false,
			expectMatch: "not in a git repository",
		},
		{
			name:        "wrapped ErrNotGitRepo",
			err:         errors.Join(errors.New("some context"), project.ErrNotGitRepo),
			expectNil:   false,
			expectMatch: "not in a git repository",
		},
		{
			name:        "ErrGitNotInstalled",
			err:         project.ErrGitNotInstalled,
			expectNil:   false,
			expectMatch: "git is not installed",
		},
		{
			name:        "wrapped ErrGitNotInstalled",
			err:         errors.Join(errors.New("some context"), project.ErrGitNotInstalled),
			expectNil:   false,
			expectMatch: "git is not installed",
		},
		{
			name:      "unrelated error",
			err:       errors.New("some other error"),
			expectNil: true,
		},
		{
			name:      "nil error",
			err:       nil,
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gitDetectionError(tc.err)
			if tc.expectNil {
				if result != nil {
					t.Errorf("expected nil, got: %v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(result.Error(), tc.expectMatch) {
				t.Errorf("expected error to contain %q, got: %s", tc.expectMatch, result.Error())
			}
		})
	}
}

func TestGitDetectionErrorWithHint(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		hint        string
		expectNil   bool
		expectMatch string
	}{
		{
			name:        "ErrNotGitRepo with custom hint",
			err:         project.ErrNotGitRepo,
			hint:        "specify cloister name or run from within a git project",
			expectNil:   false,
			expectMatch: "specify cloister name",
		},
		{
			name:        "ErrGitNotInstalled ignores hint",
			err:         project.ErrGitNotInstalled,
			hint:        "specify cloister name",
			expectNil:   false,
			expectMatch: "git is not installed",
		},
		{
			name:      "unrelated error",
			err:       errors.New("some other error"),
			hint:      "some hint",
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gitDetectionErrorWithHint(tc.err, tc.hint)
			if tc.expectNil {
				if result != nil {
					t.Errorf("expected nil, got: %v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(result.Error(), tc.expectMatch) {
				t.Errorf("expected error to contain %q, got: %s", tc.expectMatch, result.Error())
			}
		})
	}
}
