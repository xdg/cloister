// Package container provides configuration and management for cloister containers.
package container

import (
	"fmt"
	"regexp"
	"strings"
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
// cloister-<project>-<branch>
//
// Both project and branch are sanitized for Docker compatibility.
func (c *Config) ContainerName() string {
	project := SanitizeName(c.Project)
	branch := SanitizeName(c.Branch)
	return "cloister-" + project + "-" + branch
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

// sanitizePattern matches characters that are not alphanumeric or hyphen.
var sanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

// leadingHyphensPattern matches leading hyphens.
var leadingHyphensPattern = regexp.MustCompile(`^-+`)

// trailingHyphensPattern matches trailing hyphens.
var trailingHyphensPattern = regexp.MustCompile(`-+$`)

// multipleHyphensPattern matches multiple consecutive hyphens.
var multipleHyphensPattern = regexp.MustCompile(`-{2,}`)

// SanitizeName converts a string into a Docker-compatible name component.
//
// Docker container names must match [a-zA-Z0-9][a-zA-Z0-9_.-]* but we use
// a stricter format: lowercase alphanumeric with hyphens only.
//
// Transformations applied:
//   - Convert to lowercase
//   - Replace slashes with hyphens (for branch names like "feature/foo")
//   - Replace any non-alphanumeric, non-hyphen characters with hyphens
//   - Collapse multiple consecutive hyphens into one
//   - Remove leading/trailing hyphens
//   - Truncate to 63 characters (DNS label limit)
//   - If result is empty, return "default"
func SanitizeName(name string) string {
	if name == "" {
		return "default"
	}

	// Convert to lowercase
	result := strings.ToLower(name)

	// Replace slashes with hyphens (common in branch names)
	result = strings.ReplaceAll(result, "/", "-")

	// Replace any remaining non-alphanumeric, non-hyphen characters
	result = sanitizePattern.ReplaceAllString(result, "-")

	// Collapse multiple consecutive hyphens
	result = multipleHyphensPattern.ReplaceAllString(result, "-")

	// Remove leading hyphens
	result = leadingHyphensPattern.ReplaceAllString(result, "")

	// Remove trailing hyphens
	result = trailingHyphensPattern.ReplaceAllString(result, "")

	// Truncate to 63 characters (DNS label limit)
	if len(result) > 63 {
		result = result[:63]
		// Remove trailing hyphen that might result from truncation
		result = trailingHyphensPattern.ReplaceAllString(result, "")
	}

	// If empty after sanitization, use default
	if result == "" {
		return "default"
	}

	return result
}

// GenerateCloisterName creates the user-facing cloister name from project and branch.
// The cloister name is <project>-<branch> (e.g., "myproject-main").
// This is the identifier shown in CLI output like `cloister list`.
func GenerateCloisterName(project, branch string) string {
	p := SanitizeName(project)
	b := SanitizeName(branch)
	return p + "-" + b
}

// CloisterNameToContainerName converts a user-facing cloister name to the internal
// Docker container name by adding the "cloister-" prefix.
func CloisterNameToContainerName(cloisterName string) string {
	return "cloister-" + cloisterName
}

// ContainerNameToCloisterName converts an internal Docker container name to the
// user-facing cloister name by removing the "cloister-" prefix.
// Returns the input unchanged if it doesn't have the prefix.
func ContainerNameToCloisterName(containerName string) string {
	return strings.TrimPrefix(containerName, "cloister-")
}

// GenerateContainerName is a convenience function that creates a container
// name from project and branch strings without needing a full Config.
func GenerateContainerName(project, branch string) string {
	cfg := &Config{Project: project, Branch: branch}
	return cfg.ContainerName()
}
