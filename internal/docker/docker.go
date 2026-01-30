// Package docker provides CLI wrapper functions for Docker operations.
//
// This package is runtime-agnostic and works with any Docker-compatible CLI
// implementation including Docker Desktop, OrbStack, Colima, Podman (with
// docker CLI compatibility), and others. It relies solely on the `docker`
// binary in PATH and does not reference specific socket paths or runtime
// internals. The docker CLI handles runtime-specific configuration through
// standard mechanisms (DOCKER_HOST environment variable, context settings,
// ~/.docker/config.json, etc.).
package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Sentinel errors for docker operations.
var (
	// ErrDockerNotRunning indicates the Docker daemon is not running or accessible.
	ErrDockerNotRunning = errors.New("docker daemon is not running")

	// ErrNoResults indicates the docker command returned no results.
	// This is not necessarily an error - it may just mean no matching objects were found.
	ErrNoResults = errors.New("no results from docker command")
)

// CommandError represents a failed Docker command with stderr output.
type CommandError struct {
	Command string
	Args    []string
	Stderr  string
	Err     error
}

func (e *CommandError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("docker %s failed: %v\nstderr: %s", e.Command, e.Err, e.Stderr)
	}
	return fmt.Sprintf("docker %s failed: %v", e.Command, e.Err)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

// cmdNameFromArgs extracts the command name from a slice of arguments.
// Returns empty string if args is empty.
func cmdNameFromArgs(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// Run executes a docker CLI command and returns stdout.
// On error, returns a CommandError containing stderr for debugging.
func Run(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", &CommandError{
			Command: cmdNameFromArgs(args),
			Args:    args,
			Stderr:  stderr.String(),
			Err:     err,
		}
	}

	return stdout.String(), nil
}

// RunJSONLines executes a docker CLI command with JSON output format and
// unmarshals newline-separated JSON objects (JSONL) into the provided slice.
// The --format '{{json .}}' flag is automatically appended.
//
// This is designed for commands that output one JSON object per line:
// docker network ls, docker container ls, docker image ls, etc.
//
// If strict is false and the command returns empty output (no matching objects),
// the result slice is left unchanged and nil is returned.
// If strict is true, empty output returns ErrNoResults.
//
// Example:
//
//	var networks []NetworkInfo
//	err := RunJSONLines(&networks, false, "network", "ls")
func RunJSONLines[T any](result *[]T, strict bool, args ...string) error {
	args = append(args, "--format", "{{json .}}")
	out, err := Run(args...)
	if err != nil {
		return err
	}

	out = strings.TrimSpace(out)
	if out == "" {
		if strict {
			return ErrNoResults
		}
		return nil
	}

	return parseJSONLines(result, out, args)
}

// RunJSONLinesStrict is like RunJSONLines but returns ErrNoResults when the
// command produces no output.
//
// Deprecated: Use RunJSONLines with strict=true instead.
func RunJSONLinesStrict[T any](result *[]T, args ...string) error {
	return RunJSONLines(result, true, args...)
}

// parseJSONLines parses newline-separated JSON objects into a slice.
func parseJSONLines[T any](result *[]T, out string, args []string) error {
	cmdName := cmdNameFromArgs(args)
	lines := strings.Split(out, "\n")
	items := make([]T, 0, len(lines))

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return fmt.Errorf("docker %s: failed to parse JSON on line %d: %w", cmdName, i+1, err)
		}
		items = append(items, item)
	}

	*result = items
	return nil
}

// RunJSON executes a docker CLI command with JSON output format and unmarshals
// the result into the provided value. The --format '{{json .}}' flag is
// automatically appended to the command arguments.
//
// This works with commands that output a single JSON value (object or array):
// docker inspect, docker info, etc.
//
// For commands that output newline-separated JSON (docker network ls, container ls),
// use RunJSONLines instead.
//
// If strict is false and the command returns empty output (no matching objects),
// result is left unchanged and nil is returned.
// If strict is true, empty output returns ErrNoResults.
//
// Example:
//
//	var info DockerInfo
//	err := RunJSON(&info, false, "info")
//
//	// For inspect, docker returns an array even for single items
//	var containers []ContainerInfo
//	err := RunJSON(&containers, false, "inspect", containerID)
func RunJSON(result any, strict bool, args ...string) error {
	args = append(args, "--format", "{{json .}}")
	out, err := Run(args...)
	if err != nil {
		return err
	}

	// Handle empty output (no results)
	out = strings.TrimSpace(out)
	if out == "" {
		if strict {
			return ErrNoResults
		}
		return nil
	}

	if err := json.Unmarshal([]byte(out), result); err != nil {
		return fmt.Errorf("docker %s: failed to parse JSON output: %w", cmdNameFromArgs(args), err)
	}

	return nil
}

// RunJSONStrict is like RunJSON but returns ErrNoResults when the command
// produces no output. This is useful when you need to distinguish between
// "no matching objects" and actual errors.
//
// Deprecated: Use RunJSON with strict=true instead.
func RunJSONStrict(result any, args ...string) error {
	return RunJSON(result, true, args...)
}

// CheckDaemon verifies the Docker daemon is running and accessible.
// Returns ErrDockerNotRunning if the daemon cannot be reached.
func CheckDaemon() error {
	_, err := Run("info", "--format", "{{.ServerVersion}}")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			// Check if docker binary not found
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				return fmt.Errorf("docker CLI not found: %w", err)
			}
		}
		return fmt.Errorf("%w: %v", ErrDockerNotRunning, err)
	}
	return nil
}

// CopyToContainer copies a file or directory from the host to a container.
// The container can be created but not running.
//
// srcPath is the path on the host (file or directory).
// containerName is the name or ID of the container.
// destPath is the path inside the container.
//
// This wraps `docker cp srcPath containerName:destPath`.
func CopyToContainer(srcPath, containerName, destPath string) error {
	_, err := Run("cp", srcPath, containerName+":"+destPath)
	return err
}

// StartContainer starts a created container.
// This wraps `docker start containerName`.
func StartContainer(containerName string) error {
	_, err := Run("start", containerName)
	return err
}

// WriteFileToContainer writes content to a file inside a container.
// The container must be created (can be running or stopped).
//
// containerName is the name or ID of the container.
// destPath is the absolute path inside the container.
// content is the file content to write.
//
// This creates a temporary file on the host with the content, then uses
// `docker cp` to copy it into the container. This approach works on both
// running and stopped containers and avoids permission issues.
func WriteFileToContainer(containerName, destPath, content string) error {
	// Create a temp file with the content
	tmpFile, err := os.CreateTemp("", "cloister-inject-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Use docker cp to copy into container
	// docker cp copies the file directly to the destination path
	return CopyToContainer(tmpFile.Name(), containerName, destPath)
}

// ContainerInfo holds information about a Docker container.
type ContainerInfo struct {
	ID    string `json:"ID"`
	Names string `json:"Names"`
	State string `json:"State"`
}

// Name returns the container name with the leading slash removed.
// Docker often returns names with a "/" prefix (e.g., "/cloister-foo").
func (c *ContainerInfo) Name() string {
	return strings.TrimPrefix(c.Names, "/")
}

// FindContainerByExactName finds a container with the exact name specified.
// Returns nil, nil if no container with that exact name exists.
// Returns the container info if found.
//
// Note: Docker's --filter name= performs substring matching, so this function
// applies additional filtering to ensure exact matches only.
func FindContainerByExactName(name string) (*ContainerInfo, error) {
	var containers []ContainerInfo

	// Docker filter uses regex, so anchor with ^ and $
	err := RunJSONLines(&containers, false, "ps", "-a", "--filter", "name=^"+name+"$")
	if err != nil {
		return nil, err
	}

	// Docker filter is still substring match even with anchors in some cases,
	// so verify exact match
	for i := range containers {
		if containers[i].Name() == name {
			return &containers[i], nil
		}
	}

	return nil, nil
}
