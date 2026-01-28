// Package container provides configuration and management for cloister containers.
package container

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/xdg/cloister/internal/docker"
)

// ErrContainerExists indicates that a container with the requested name already exists.
var ErrContainerExists = errors.New("container already exists")

// ErrContainerNotFound indicates that the specified container does not exist.
var ErrContainerNotFound = errors.New("container not found")

// ContainerInfo holds information about a running cloister container.
type ContainerInfo struct {
	ID         string `json:"ID"`
	Name       string `json:"Names"`
	Image      string `json:"Image"`
	Status     string `json:"Status"`
	State      string `json:"State"`
	CreatedAt  string `json:"CreatedAt"`
	RunningFor string `json:"RunningFor"`
}

// Manager handles cloister container lifecycle operations.
type Manager struct{}

// NewManager creates a new container Manager.
func NewManager() *Manager {
	return &Manager{}
}

// Start creates and starts a new cloister container using the provided configuration.
// Returns the container ID on success.
// Returns ErrContainerExists if a container with the same name already exists.
//
// Note: This method creates and immediately starts the container. If you need to
// perform operations between creation and start (e.g., copying files), use
// Create() followed by StartContainer() instead.
func (m *Manager) Start(cfg *Config) (string, error) {
	return m.createContainer(cfg, "run", "-d")
}

// Create creates a new cloister container without starting it.
// Returns the container ID on success.
// Returns ErrContainerExists if a container with the same name already exists.
//
// Use this with StartContainer() when you need to perform operations
// (like copying files) between creation and start.
func (m *Manager) Create(cfg *Config) (string, error) {
	return m.createContainer(cfg, "create")
}

// createContainer is a shared helper that creates a container using the specified
// docker command ("run" or "create") with optional extra arguments.
func (m *Manager) createContainer(cfg *Config, dockerCmd string, extraArgs ...string) (string, error) {
	containerName := cfg.ContainerName()

	// Check if container already exists
	exists, err := m.containerExists(containerName)
	if err != nil {
		return "", err
	}
	if exists {
		return "", ErrContainerExists
	}

	// Build docker arguments
	args := []string{dockerCmd}
	args = append(args, extraArgs...)
	args = append(args, cfg.BuildRunArgs()...)

	// Add a command that keeps the container running (sleep infinity)
	args = append(args, "sleep", "infinity")

	// Execute the docker command
	output, err := docker.Run(args...)
	if err != nil {
		// Check if the error is due to container already existing (race condition)
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) && strings.Contains(cmdErr.Stderr, "already in use") {
			return "", ErrContainerExists
		}
		return "", err
	}

	// Return container ID
	containerID := strings.TrimSpace(output)
	return containerID, nil
}

// StartContainer starts a previously created container by name.
// Returns ErrContainerNotFound if the container does not exist.
func (m *Manager) StartContainer(containerName string) error {
	// Check if container exists
	exists, err := m.containerExists(containerName)
	if err != nil {
		return err
	}
	if !exists {
		return ErrContainerNotFound
	}

	return docker.StartContainer(containerName)
}

// Stop stops and removes a cloister container by name.
// Returns ErrContainerNotFound if the container does not exist.
func (m *Manager) Stop(containerName string) error {
	// Check if container exists
	exists, err := m.containerExists(containerName)
	if err != nil {
		return err
	}
	if !exists {
		return ErrContainerNotFound
	}

	// Stop the container with short timeout (1s grace period)
	// Containers with tini will exit immediately on SIGTERM; others hit the timeout
	_, err = docker.Run("stop", "-t", "1", containerName)
	if err != nil {
		// If container is not running, that's okay - continue to removal
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			if !strings.Contains(cmdErr.Stderr, "is not running") {
				return err
			}
		} else {
			return err
		}
	}

	// Remove the container
	_, err = docker.Run("rm", containerName)
	if err != nil {
		return err
	}

	return nil
}

// List returns information about all running cloister containers.
// Cloister containers are identified by the "cloister-" name prefix.
func (m *Manager) List() ([]ContainerInfo, error) {
	var containers []ContainerInfo

	err := docker.RunJSONLines(&containers, false, "ps", "-a", "--filter", "name=^cloister-")
	if err != nil {
		return nil, err
	}

	// Filter to only include containers that start with "cloister-"
	// (docker filter is a substring match, so we need exact prefix matching)
	result := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		// Names field may have leading slash from docker (e.g., "/cloister-foo")
		name := strings.TrimPrefix(c.Name, "/")
		if strings.HasPrefix(name, "cloister-") {
			c.Name = name
			result = append(result, c)
		}
	}

	return result, nil
}

// Attach attaches an interactive shell to a running container.
// It connects stdin/stdout/stderr to the container and allocates a TTY.
//
// The function returns the exit code from the shell session:
//   - 0: Shell exited successfully
//   - Non-zero: Shell exited with an error or was terminated
//
// Ctrl+C inside the container is handled by the shell; it does not terminate
// the attach process or the container itself.
//
// Returns ErrContainerNotFound if the container does not exist.
func (m *Manager) Attach(containerName string) (int, error) {
	// Check if container exists
	exists, err := m.containerExists(containerName)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrContainerNotFound
	}

	// Build docker exec command with interactive TTY
	// -i: Keep STDIN open even if not attached
	// -t: Allocate a pseudo-TTY
	cmd := exec.Command("docker", "exec", "-it", containerName, "/bin/bash")

	// Connect to current process's stdin/stdout/stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and wait for it to complete
	err = cmd.Run()
	if err != nil {
		// Extract exit code from ExitError
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		// Other errors (e.g., docker not found, command failed to start)
		return 0, err
	}

	return 0, nil
}

// ContainerStatus checks if a container with the given name exists and whether it's running.
// Returns (exists, running, error). If exists is false, running is always false.
// This performs a single Docker call to retrieve both pieces of information.
func (m *Manager) ContainerStatus(name string) (exists bool, running bool, err error) {
	info, err := docker.FindContainerByExactName(name)
	if err != nil {
		return false, false, err
	}
	if info == nil {
		return false, false, nil
	}
	return true, info.State == "running", nil
}

// containerExists checks if a container with the given name exists (running or stopped).
func (m *Manager) containerExists(name string) (bool, error) {
	exists, _, err := m.ContainerStatus(name)
	return exists, err
}

// IsRunning checks if a container with the given name exists and is running.
// Returns (true, nil) if running, (false, nil) if exists but not running or doesn't exist,
// and (false, err) if there was an error checking.
func (m *Manager) IsRunning(name string) (bool, error) {
	_, running, err := m.ContainerStatus(name)
	return running, err
}
