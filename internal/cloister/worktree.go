package cloister

import (
	"fmt"
	"os"
)

// StartWorktree orchestrates creating a git worktree and starting a cloister
// container for it. The flow is:
//  1. Resolve the branch (create if needed)
//  2. Determine the worktree target path
//  3. Create the git worktree (skip if already exists)
//  4. Delegate to Start with modified options (worktree path, IsWorktree=true)
//
// opts.ProjectPath should be the main checkout path (repo root).
// opts.BranchName must be set to the desired branch.
//
// If the container already exists (Start returns ErrContainerExists), the error
// is returned unchanged so the caller can handle re-entry (attach to existing).
func StartWorktree(opts StartOptions, options ...Option) (containerID, tok string, err error) {
	deps := applyOptions(options...)

	repoRoot := opts.ProjectPath

	// Step 1: Resolve the branch (ensure it exists locally).
	if _, err := deps.worktreeOps.ResolveBranch(repoRoot, opts.BranchName); err != nil {
		return "", "", fmt.Errorf("resolve branch %q: %w", opts.BranchName, err)
	}

	// Step 2: Get the worktree target directory.
	worktreePath, err := deps.worktreeOps.Dir(opts.ProjectName, opts.BranchName)
	if err != nil {
		return "", "", fmt.Errorf("worktree dir: %w", err)
	}

	// Step 3: Create the git worktree if the directory does not already exist.
	// git worktree add fails if the dir exists, so we skip creation in that case
	// (the worktree already exists from a prior run).
	if _, statErr := os.Stat(worktreePath); os.IsNotExist(statErr) {
		if createErr := deps.worktreeOps.Create(repoRoot, worktreePath, opts.BranchName); createErr != nil {
			return "", "", fmt.Errorf("create worktree: %w", createErr)
		}
	}

	// Step 4: Delegate to Start with the worktree path and IsWorktree flag.
	opts.ProjectPath = worktreePath
	opts.IsWorktree = true

	return Start(opts, options...)
}
