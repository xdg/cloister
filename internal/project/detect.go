// Package project provides functions for detecting and extracting project
// information from git repositories.
package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrNotGitRepo indicates the path is not within a git repository.
var ErrNotGitRepo = errors.New("not a git repository")

// ErrGitNotInstalled indicates git is not installed or not in PATH.
var ErrGitNotInstalled = errors.New("git is not installed or not in PATH")

// GitError represents a failed git command with stderr output.
type GitError struct {
	Command string
	Args    []string
	Stderr  string
	Err     error
}

func (e *GitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("git %s failed: %v\nstderr: %s", e.Command, e.Err, e.Stderr)
	}
	return fmt.Sprintf("git %s failed: %v", e.Command, e.Err)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

// runGit executes a git command in the specified directory and returns stdout.
// If dir is empty, uses the current working directory.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check if git binary not found
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return "", ErrGitNotInstalled
		}

		cmdName := ""
		if len(args) > 0 {
			cmdName = args[0]
		}

		stderrStr := stderr.String()

		// Check for "not a git repository" in stderr
		if strings.Contains(stderrStr, "not a git repository") {
			return "", ErrNotGitRepo
		}

		return "", &GitError{
			Command: cmdName,
			Args:    args,
			Stderr:  stderrStr,
			Err:     err,
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// DetectGitRoot finds the git repository root from a given path.
// If path is empty, uses the current working directory.
// Returns the absolute path to the git repository root.
func DetectGitRoot(path string) (string, error) {
	out, err := runGit(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	// Ensure the path is absolute and clean
	absPath, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	return filepath.Clean(absPath), nil
}

// DetectBranch gets the current branch name from the git repository at gitRoot.
// If on a detached HEAD, returns the short commit hash instead.
func DetectBranch(gitRoot string) (string, error) {
	// Try to get the current branch name
	out, err := runGit(gitRoot, "symbolic-ref", "--short", "HEAD")
	if err == nil {
		return out, nil
	}

	// If symbolic-ref fails, we might be on a detached HEAD
	// Try to get the short commit hash
	var gitErr *GitError
	if errors.As(err, &gitErr) {
		out, err = runGit(gitRoot, "rev-parse", "--short", "HEAD")
		if err != nil {
			return "", err
		}
		return out, nil
	}

	// For other errors (ErrNotGitRepo, ErrGitNotInstalled), propagate them
	return "", err
}

// remoteURLPattern matches common git remote URL formats to extract the project name.
// Handles:
//   - git@github.com:user/repo.git
//   - https://github.com/user/repo.git
//   - https://github.com/user/repo
//   - ssh://git@github.com/user/repo.git
var remoteURLPattern = regexp.MustCompile(`[/:]([^/:]+?)(?:\.git)?$`)

// Name extracts a short project name from the git repository at gitRoot.
// It first tries to extract the name from the git remote URL (origin).
// If no remote is configured or parsing fails, falls back to the directory name.
func Name(gitRoot string) (string, error) {
	// Try to get the remote URL for origin
	out, err := runGit(gitRoot, "config", "--get", "remote.origin.url")
	if err == nil && out != "" {
		// Extract project name from remote URL
		if name := extractProjectName(out); name != "" {
			return name, nil
		}
	}

	// Fall back to directory name
	absPath, err := filepath.Abs(gitRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	dirName := filepath.Base(absPath)
	if dirName == "" || dirName == "." || dirName == "/" {
		return "", errors.New("unable to determine project name from directory")
	}

	return dirName, nil
}

// extractProjectName extracts the project name from a git remote URL.
// Returns empty string if the URL format is not recognized.
func extractProjectName(remoteURL string) string {
	matches := remoteURLPattern.FindStringSubmatch(remoteURL)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// Info contains detected information about a git project.
type Info struct {
	Name   string // Project name (from remote URL or directory)
	Root   string // Absolute path to git root
	Remote string // Remote URL (origin), may be empty
	Branch string // Current branch or commit hash
}

// GetRemoteURL returns the origin remote URL for the git repository at gitRoot.
// Returns empty string if no remote is configured (this is not an error).
func GetRemoteURL(gitRoot string) string {
	out, err := runGit(gitRoot, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return out
}

// DetectProject detects project information from a path within a git repository.
// If path is empty, uses the current working directory.
// Returns ErrNotGitRepo if the path is not within a git repository.
func DetectProject(path string) (*Info, error) {
	// Find the git root
	gitRoot, err := DetectGitRoot(path)
	if err != nil {
		return nil, err
	}

	// Get the project name
	name, err := Name(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get project name: %w", err)
	}

	// Get the remote URL (empty string is OK)
	remote := GetRemoteURL(gitRoot)

	// Get the current branch
	branch, err := DetectBranch(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to detect branch: %w", err)
	}

	return &Info{
		Name:   name,
		Root:   gitRoot,
		Remote: remote,
		Branch: branch,
	}, nil
}
