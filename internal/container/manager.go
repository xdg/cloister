// Package container provides configuration and management for cloister containers.
package container

import (
	"errors"
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
func (m *Manager) Start(cfg *Config) (string, error) {
	containerName := cfg.ContainerName()

	// Check if container already exists
	exists, err := m.containerExists(containerName)
	if err != nil {
		return "", err
	}
	if exists {
		return "", ErrContainerExists
	}

	// Build docker run arguments
	args := []string{"run", "-d"}
	args = append(args, cfg.BuildRunArgs()...)

	// Add a command that keeps the container running (sleep infinity)
	args = append(args, "sleep", "infinity")

	// Run the container
	output, err := docker.Run(args...)
	if err != nil {
		// Check if the error is due to container already existing (race condition)
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) && strings.Contains(cmdErr.Stderr, "already in use") {
			return "", ErrContainerExists
		}
		return "", err
	}

	// Return container ID (docker run -d outputs the container ID)
	containerID := strings.TrimSpace(output)
	return containerID, nil
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

	// Stop the container (graceful shutdown)
	_, err = docker.Run("stop", containerName)
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

	err := docker.RunJSONLines(&containers, "ps", "-a", "--filter", "name=^cloister-")
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

// containerExists checks if a container with the given name exists (running or stopped).
func (m *Manager) containerExists(name string) (bool, error) {
	var containers []struct {
		Names string `json:"Names"`
	}

	err := docker.RunJSONLines(&containers, "ps", "-a", "--filter", "name=^"+name+"$")
	if err != nil {
		return false, err
	}

	// Check for exact name match (docker filter is substring match)
	for _, c := range containers {
		containerName := strings.TrimPrefix(c.Names, "/")
		if containerName == name {
			return true, nil
		}
	}

	return false, nil
}
