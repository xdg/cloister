// Package cloister provides high-level orchestration for starting and stopping
// cloister containers with proper guardian integration.
package cloister

import (
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/token"
)

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
// 4. Creates and starts the container with proxy env vars injected
//
// Returns the container ID and token. The token is returned so it can be used
// for cleanup later (revocation when stopping the container).
func Start(opts StartOptions) (containerID string, tok string, err error) {
	// Step 1: Ensure guardian is running
	if err := guardian.EnsureRunning(); err != nil {
		return "", "", err
	}

	// Step 2: Generate a new token
	tok = token.Generate()

	// Step 3: Build container name for registration
	containerName := container.GenerateContainerName(opts.ProjectName, opts.BranchName)

	// Step 4: Register the token with guardian
	if err := guardian.RegisterToken(tok, containerName); err != nil {
		return "", "", err
	}

	// If container creation fails after token registration, we should revoke the token
	defer func() {
		if err != nil {
			// Best effort cleanup - ignore revocation errors
			_ = guardian.RevokeToken(tok)
		}
	}()

	// Step 5: Create container config with token and proxy env vars
	cfg := &container.Config{
		Project:     opts.ProjectName,
		Branch:      opts.BranchName,
		ProjectPath: opts.ProjectPath,
		Image:       opts.Image,
		Network:     docker.CloisterNetworkName,
		EnvVars:     token.ProxyEnvVars(tok, ""),
	}

	// Step 6: Start the container
	mgr := container.NewManager()
	containerID, err = mgr.Start(cfg)
	if err != nil {
		return "", "", err
	}

	return containerID, tok, nil
}

// Stop orchestrates stopping a cloister container with proper cleanup:
// 1. Revokes the token from guardian
// 2. Stops and removes the container
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

	// Step 2: Stop and remove the container
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
