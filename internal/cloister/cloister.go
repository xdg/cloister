// Package cloister provides high-level orchestration for starting and stopping
// cloister containers with proper guardian integration.
package cloister

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/token"
)

// containerHomeDir is the home directory for the cloister user inside containers.
const containerHomeDir = "/home/cloister"

// StartOptions contains the configuration for starting a cloister container.
type StartOptions struct {
	// ProjectPath is the absolute path to the project directory on the host.
	ProjectPath string

	// ProjectName is the name for the container (used in container naming).
	ProjectName string

	// BranchName is the current git branch (used in container naming).
	BranchName string

	// Image is the Docker image to use. If empty, defaults to container.DefaultImage.
	Image string
}

// Start orchestrates starting a cloister container with all necessary setup:
// 1. Ensures guardian is running
// 2. Generates a new token
// 3. Registers the token with guardian
// 4. Creates the container (without starting)
// 5. Injects user settings (~/.claude/) into the container
// 6. Starts the container
//
// Returns the container ID and token. The token is returned so it can be used
// for cleanup later (revocation when stopping the container).
func Start(opts StartOptions) (containerID string, tok string, err error) {
	// Step 1: Ensure guardian is running
	if err := guardian.EnsureRunning(); err != nil {
		return "", "", fmt.Errorf("guardian failed to start: %w", err)
	}

	// Step 2: Generate a new token
	tok = token.Generate()

	// Step 3: Build container name for registration
	containerName := container.GenerateContainerName(opts.ProjectName, opts.BranchName)

	// Step 4: Persist token to disk (for recovery after guardian restart)
	tokenDir, err := token.DefaultTokenDir()
	if err != nil {
		return "", "", err
	}
	store, err := token.NewStore(tokenDir)
	if err != nil {
		return "", "", err
	}
	if err := store.Save(containerName, tok); err != nil {
		return "", "", err
	}

	// Step 5: Register the token with guardian
	if err := guardian.RegisterToken(tok, containerName); err != nil {
		// Clean up persisted token on failure
		_ = store.Remove(containerName)
		return "", "", err
	}

	// If container creation fails after token registration, we should revoke the token
	defer func() {
		if err != nil {
			// Best effort cleanup - ignore revocation errors
			_ = guardian.RevokeToken(tok)
			_ = store.Remove(containerName)
		}
	}()

	// Step 5: Create container config with token, proxy env vars, and credentials
	// Combine proxy env vars with any credential env vars from the host.
	// TEMPORARY: Credential passthrough is a Phase 1 workaround. In Phase 3,
	// credentials will be managed via `cloister setup claude` instead.
	envVars := token.ProxyEnvVars(tok, "")
	envVars = append(envVars, token.CredentialEnvVars()...)

	cfg := &container.Config{
		Project:     opts.ProjectName,
		Branch:      opts.BranchName,
		ProjectPath: opts.ProjectPath,
		Image:       opts.Image,
		Network:     docker.CloisterNetworkName,
		EnvVars:     envVars,
	}

	// Step 6: Create the container (without starting)
	mgr := container.NewManager()
	containerID, err = mgr.Create(cfg)
	if err != nil {
		return "", "", err
	}

	// If starting fails after container creation, remove the container
	defer func() {
		if err != nil {
			// Best effort cleanup - ignore removal errors
			_ = mgr.Stop(containerName)
		}
	}()

	// Step 7: Inject user settings (~/.claude/) into the container
	// This is a one-way snapshot - writes inside container are isolated
	if copyErr := injectUserSettings(containerName); copyErr != nil {
		// Log but don't fail - missing settings is not fatal
		// (user might not have ~/.claude/ on a fresh install)
		_ = copyErr
	}

	// Step 8: Start the container
	err = mgr.StartContainer(containerName)
	if err != nil {
		return "", "", err
	}

	return containerID, tok, nil
}

// injectUserSettings copies the host's ~/.claude/ directory into the container.
// This provides a one-way snapshot so the agent inherits user's settings, skills,
// memory, and CLAUDE.md. Writes inside the container are isolated and do not
// modify the host config.
//
// Returns nil if ~/.claude/ doesn't exist on the host (fresh install).
// Returns an error only if the directory exists but cannot be copied.
func injectUserSettings(containerName string) error {
	// Get the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	claudeDir := filepath.Join(homeDir, ".claude")

	// Check if ~/.claude/ exists
	info, err := os.Stat(claudeDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist - this is fine, just skip
			return nil
		}
		return err
	}

	// Ensure it's a directory
	if !info.IsDir() {
		// ~/.claude is a file, not a directory - skip
		return nil
	}

	// Copy ~/.claude/ to /home/cloister/.claude/ in the container
	// docker cp copies the directory contents when the source has a trailing slash,
	// or the entire directory when it doesn't. We want to copy the entire directory.
	destPath := filepath.Join(containerHomeDir, ".claude")
	return docker.CopyToContainer(claudeDir, containerName, destPath)
}

// Stop orchestrates stopping a cloister container with proper cleanup:
// 1. Revokes the token from guardian
// 2. Removes the token from disk
// 3. Stops and removes the container
//
// The token parameter should be the token returned from Start().
// If the token is empty, only the container is stopped (token revocation is skipped).
func Stop(containerName string, tok string) error {
	// Step 1: Revoke the token from guardian (if provided)
	// We ignore revocation errors and continue with container stop.
	// The token will become orphaned but won't cause security issues
	// since the container will no longer exist.
	if tok != "" {
		_ = guardian.RevokeToken(tok)
	}

	// Step 2: Remove the token from disk (best effort)
	tokenDir, err := token.DefaultTokenDir()
	if err == nil {
		store, err := token.NewStore(tokenDir)
		if err == nil {
			_ = store.Remove(containerName)
		}
	}

	// Step 3: Stop and remove the container
	mgr := container.NewManager()
	return mgr.Stop(containerName)
}

// Attach attaches an interactive shell to a running cloister container.
// It connects stdin/stdout/stderr to the container's shell and allocates a TTY.
//
// The containerID parameter can be either the container ID or name.
//
// Returns the exit code from the shell session:
//   - 0: Shell exited successfully
//   - Non-zero: Shell exited with an error or was terminated
//
// Ctrl+C inside the container is handled by the shell; it does not terminate
// the attachment or kill the container.
func Attach(containerID string) (exitCode int, err error) {
	mgr := container.NewManager()
	return mgr.Attach(containerID)
}
