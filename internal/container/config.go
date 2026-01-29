// Package container provides configuration and management for cloister containers.
package container

import (
	"fmt"
)

// DefaultImage is the default Docker image for cloister containers.
const DefaultImage = "cloister:latest"

// DefaultWorkDir is the working directory inside the container where
// the project is mounted.
const DefaultWorkDir = "/work"

// DefaultUID is the default user ID for running processes in the container.
const DefaultUID = 1000

// Config holds all configuration for creating a cloister container.
type Config struct {
	// Project is the logical project name (sanitized for Docker compatibility).
	Project string

	// Branch is the git branch name (sanitized for Docker compatibility).
	Branch string

	// ProjectPath is the absolute path to the project directory on the host.
	ProjectPath string

	// Image is the Docker image to use. Defaults to DefaultImage if empty.
	Image string

	// EnvVars is a list of environment variables in "KEY=VALUE" format.
	EnvVars []string

	// Network is the Docker network to connect to.
	Network string

	// UID is the user ID to run as. Defaults to DefaultUID if zero.
	UID int
}

// ContainerName returns the Docker container name in the format:
// cloister-<project>
//
// The project name is sanitized for Docker compatibility.
// Note: In Phase 1 (no worktree support), we only use the project name.
// Worktree support will be added in a future phase.
func (c *Config) ContainerName() string {
	return "cloister-" + SanitizeName(c.Project)
}

// ImageName returns the Docker image to use, defaulting to DefaultImage.
func (c *Config) ImageName() string {
	if c.Image == "" {
		return DefaultImage
	}
	return c.Image
}

// UserID returns the UID to run as, defaulting to DefaultUID.
func (c *Config) UserID() int {
	if c.UID == 0 {
		return DefaultUID
	}
	return c.UID
}

// BuildRunArgs returns the docker run arguments for creating a container
// with the configured settings.
//
// The returned args include:
//   - Container name
//   - Volume mount for project directory
//   - Working directory set to /work
//   - Environment variables
//   - Network connection
//   - Security hardening (cap-drop=ALL, no-new-privileges, non-root user)
//   - Image name
func (c *Config) BuildRunArgs() []string {
	args := []string{
		"--name", c.ContainerName(),
		"-v", c.ProjectPath + ":" + DefaultWorkDir,
		"-w", DefaultWorkDir,
	}

	// Add environment variables
	for _, env := range c.EnvVars {
		args = append(args, "-e", env)
	}

	// Add network if specified
	if c.Network != "" {
		args = append(args, "--network", c.Network)
	}

	// Security hardening: drop all capabilities
	args = append(args, "--cap-drop=ALL")

	// Security hardening: prevent privilege escalation
	args = append(args, "--security-opt=no-new-privileges")

	// Security hardening: run as non-root user
	args = append(args, "--user", fmt.Sprintf("%d", c.UserID()))

	// Add image name last
	args = append(args, c.ImageName())

	return args
}
