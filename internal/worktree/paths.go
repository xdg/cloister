// Package worktree provides helpers for managing worktree directories.
package worktree

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/pathutil"
)

// BaseDir returns the base directory for worktree storage.
// By default, this is ~/.local/share/cloister/worktrees/. If the
// XDG_DATA_HOME environment variable is set, it uses
// $XDG_DATA_HOME/cloister/worktrees/ instead.
// The directory is created if it does not exist (with 0700 permissions).
func BaseDir() (string, error) {
	dir := filepath.Join(pathutil.XDGDataHome(), "cloister", "worktrees")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create worktree base dir: %w", err)
	}
	return dir, nil
}

// Dir returns the directory path for a specific project worktree.
// The path is BaseDir()/<project>/<sanitized-branch>/.
// The branch name is sanitized using container.SanitizeName to handle
// characters like slashes (e.g. "feature/auth" becomes "feature-auth").
// The directory is NOT created by this function.
func Dir(projectName, branch string) (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, projectName, container.SanitizeName(branch)), nil
}
