package cmd

import (
	"errors"
	"fmt"

	"github.com/xdg/cloister/internal/project"
)

// dockerNotRunningError returns a user-friendly error when Docker is not running.
func dockerNotRunningError() error {
	return fmt.Errorf("Docker is not running; please start Docker and try again")
}

// gitDetectionError handles common git detection errors with user-friendly messages.
// Returns nil if the error is not a git detection error.
func gitDetectionError(err error) error {
	if errors.Is(err, project.ErrNotGitRepo) {
		return fmt.Errorf("not in a git repository; cloister must be run from within a git project")
	}
	if errors.Is(err, project.ErrGitNotInstalled) {
		return fmt.Errorf("git is not installed or not in PATH")
	}
	return nil
}

// gitDetectionErrorWithHint handles common git detection errors with a custom hint
// for specifying a name argument when not in a git repository.
// Returns nil if the error is not a git detection error.
func gitDetectionErrorWithHint(err error, hint string) error {
	if errors.Is(err, project.ErrNotGitRepo) {
		return fmt.Errorf("not in a git repository; %s", hint)
	}
	if errors.Is(err, project.ErrGitNotInstalled) {
		return fmt.Errorf("git is not installed or not in PATH")
	}
	return nil
}
