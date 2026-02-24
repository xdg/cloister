package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitCommand creates a git command with context and isolated config.
func gitCommand(args ...string) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Env = append(cmd.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	return cmd
}

// Create creates a git worktree at worktreePath for the given branch,
// rooted in repoRoot. If the branch does not exist, it is created from HEAD
// before the worktree is added.
func Create(repoRoot, worktreePath, branch string) error {
	// Check if the branch already exists.
	check := gitCommand("-C", repoRoot, "rev-parse", "--verify", "refs/heads/"+branch)
	if err := check.Run(); err != nil {
		// Branch doesn't exist; create it from HEAD.
		create := gitCommand("-C", repoRoot, "branch", branch)
		if out, cerr := create.CombinedOutput(); cerr != nil {
			return fmt.Errorf("create branch %q: %s: %w", branch, bytes.TrimSpace(out), cerr)
		}
	}

	add := gitCommand("-C", repoRoot, "worktree", "add", worktreePath, branch)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", bytes.TrimSpace(out), err)
	}
	return nil
}

// Remove removes a git worktree at the given path. If force is false, it
// checks for uncommitted changes first and returns a descriptive error.
// If force is true, the --force flag is passed to git worktree remove.
func Remove(worktreePath string, force bool) error {
	if !force {
		dirty, err := IsDirty(worktreePath)
		if err != nil {
			return fmt.Errorf("check worktree status: %w", err)
		}
		if dirty {
			return fmt.Errorf("worktree %q has uncommitted changes; commit, stash, or use force to remove", worktreePath)
		}
	}

	// Resolve the main repo root so git worktree remove can find the worktree.
	rootCmd := gitCommand("-C", worktreePath, "rev-parse", "--git-common-dir")
	rootOut, err := rootCmd.Output()
	if err != nil {
		return fmt.Errorf("find repo root: %w", err)
	}
	commonDir := strings.TrimSpace(string(rootOut))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(worktreePath, commonDir)
	}
	// The common dir is the .git directory; the repo root is its parent.
	repoRoot := filepath.Dir(commonDir)

	args := []string{"-C", repoRoot, "worktree", "remove", worktreePath}
	if force {
		args = []string{"-C", repoRoot, "worktree", "remove", "--force", worktreePath}
	}

	cmd := gitCommand(args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", bytes.TrimSpace(out), err)
	}
	return nil
}

// IsDirty reports whether the worktree at the given path has uncommitted
// changes (including untracked files). It runs git status --porcelain and
// checks for non-empty output.
func IsDirty(worktreePath string) (bool, error) {
	cmd := gitCommand("-C", worktreePath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// ResolveBranch checks if a branch exists locally in the given repo. If it
// exists, it returns (true, nil). If not, it checks for a remote tracking
// branch at refs/remotes/origin/<branch> and creates a local branch tracking
// it. If neither exists, it creates the branch from HEAD. The returned bool
// indicates whether the branch already existed locally.
func ResolveBranch(repoRoot, branch string) (existed bool, err error) {
	// Check if the branch exists locally.
	local := gitCommand("-C", repoRoot, "rev-parse", "--verify", "refs/heads/"+branch)
	if local.Run() == nil {
		return true, nil
	}

	// Check if a remote tracking branch exists.
	remote := gitCommand("-C", repoRoot, "rev-parse", "--verify", "refs/remotes/origin/"+branch)
	if remote.Run() == nil {
		// Create local branch tracking the remote.
		create := gitCommand("-C", repoRoot, "branch", branch, "refs/remotes/origin/"+branch)
		if out, cerr := create.CombinedOutput(); cerr != nil {
			return false, fmt.Errorf("create branch %q from remote: %s: %w", branch, bytes.TrimSpace(out), cerr)
		}
		return false, nil
	}

	// No local or remote branch; create from HEAD.
	create := gitCommand("-C", repoRoot, "branch", branch)
	if out, cerr := create.CombinedOutput(); cerr != nil {
		return false, fmt.Errorf("create branch %q from HEAD: %s: %w", branch, bytes.TrimSpace(out), cerr)
	}
	return false, nil
}

// IsWorktree reports whether the given path is a git worktree (as opposed to
// a main checkout). It compares the output of git rev-parse --git-common-dir
// and --git-dir. If they differ, the path is a worktree. Returns false on any
// error.
func IsWorktree(path string) bool {
	commonCmd := gitCommand("-C", path, "rev-parse", "--git-common-dir")
	commonOut, err := commonCmd.Output()
	if err != nil {
		return false
	}

	dirCmd := gitCommand("-C", path, "rev-parse", "--git-dir")
	dirOut, err := dirCmd.Output()
	if err != nil {
		return false
	}

	commonDir := strings.TrimSpace(string(commonOut))
	gitDir := strings.TrimSpace(string(dirOut))

	// Resolve to absolute paths for reliable comparison.
	commonAbs, err := filepath.Abs(filepath.Join(path, commonDir))
	if err != nil {
		return false
	}
	gitAbs, err := filepath.Abs(filepath.Join(path, gitDir))
	if err != nil {
		return false
	}

	return commonAbs != gitAbs
}
