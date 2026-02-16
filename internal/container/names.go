// Package container provides configuration and management for cloister containers.
package container

import (
	"regexp"
	"strings"
)

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

// GenerateCloisterName creates the cloister name for a main checkout.
// Returns just the sanitized project name (e.g., "foo").
// This is the identifier shown in CLI output like `cloister list`.
func GenerateCloisterName(project string) string {
	return SanitizeName(project)
}

// GenerateWorktreeCloisterName creates the cloister name for a worktree.
// Returns <project>-<branch> (e.g., "foo-new-feature").
func GenerateWorktreeCloisterName(project, branch string) string {
	return SanitizeName(project) + "-" + SanitizeName(branch)
}

// CloisterNameToContainerName converts a user-facing cloister name to the internal
// Docker container name by adding the "cloister-" prefix.
func CloisterNameToContainerName(cloisterName string) string {
	return "cloister-" + cloisterName
}

// NameToCloisterName converts an internal Docker container name to the
// user-facing cloister name by removing the "cloister-" prefix.
// Returns the input unchanged if it doesn't have the prefix.
func NameToCloisterName(containerName string) string {
	return strings.TrimPrefix(containerName, "cloister-")
}

// GenerateContainerName is a convenience function that creates a container
// name from a project string without needing a full Config.
// Note: In Phase 1 (no worktree support), we only use the project name.
func GenerateContainerName(project string) string {
	cfg := &Config{Project: project}
	return cfg.ContainerName()
}
