// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xdg/cloister/internal/docker"
)

// Container constants for the guardian service.
const (
	// ContainerName is the name of the guardian container.
	ContainerName = "cloister-guardian"

	// DefaultImage is the Docker image used for the guardian container.
	DefaultImage = "cloister:latest"

	// BridgeNetwork is the default Docker bridge network for external access.
	BridgeNetwork = "bridge"
)

// ErrGuardianNotRunning indicates the guardian container is not running.
var ErrGuardianNotRunning = errors.New("guardian container is not running")

// ErrGuardianAlreadyRunning indicates the guardian container is already running.
var ErrGuardianAlreadyRunning = errors.New("guardian container is already running")

// containerState represents the state of the guardian container.
type containerState struct {
	ID      string `json:"ID"`
	Name    string `json:"Names"`
	State   string `json:"State"`
	Status  string `json:"Status"`
	Running bool
}

// IsRunning checks if the guardian container is running.
// Returns true if the container exists and is in the running state.
func IsRunning() (bool, error) {
	state, err := getContainerState()
	if err != nil {
		return false, err
	}
	return state != nil && state.State == "running", nil
}

// Start starts the guardian container if it is not already running.
// The container is configured with:
//   - Connection to cloister-net (internal network) for proxy traffic
//   - Connection to bridge network for upstream server access
//   - Port 3128 exposed on cloister-net for the proxy
//   - The cloister binary running in guardian mode (cloister guardian run)
//
// Returns ErrGuardianAlreadyRunning if the container is already running.
func Start() error {
	// Check if container already exists
	state, err := getContainerState()
	if err != nil {
		return err
	}

	if state != nil {
		if state.State == "running" {
			return ErrGuardianAlreadyRunning
		}
		// Container exists but not running - remove it and start fresh
		if err := removeContainer(); err != nil {
			return err
		}
	}

	// Ensure cloister-net exists
	if err := docker.EnsureCloisterNetwork(); err != nil {
		return err
	}

	// Build docker run arguments
	// Port 9997 is exposed to the host for the token management API
	// (used by CLI to register/revoke tokens)
	args := []string{
		"run", "-d",
		"--name", ContainerName,
		"--network", docker.CloisterNetworkName,
		"-p", "127.0.0.1:9997:9997",
		DefaultImage,
		"cloister", "guardian", "run",
	}

	// Create and start the container
	_, err = docker.Run(args...)
	if err != nil {
		return err
	}

	// Connect to bridge network for external access to upstream servers
	_, err = docker.Run("network", "connect", BridgeNetwork, ContainerName)
	if err != nil {
		// If connecting to bridge fails, clean up the container
		_ = removeContainer()
		return err
	}

	return nil
}

// Stop stops and removes the guardian container.
// Returns nil if the container doesn't exist (idempotent).
func Stop() error {
	state, err := getContainerState()
	if err != nil {
		return err
	}

	if state == nil {
		// Container doesn't exist, nothing to do
		return nil
	}

	return removeContainer()
}

// EnsureRunning ensures the guardian container is running.
// If the container is already running, this is a no-op.
// If the container is not running, it starts it.
func EnsureRunning() error {
	running, err := IsRunning()
	if err != nil {
		return err
	}

	if running {
		return nil
	}

	return Start()
}

// getContainerState retrieves the current state of the guardian container.
// Returns nil if the container doesn't exist.
func getContainerState() (*containerState, error) {
	var containers []containerState

	err := docker.RunJSONLines(&containers, "ps", "-a", "--filter", "name=^"+ContainerName+"$")
	if err != nil {
		return nil, err
	}

	// Check for exact name match (docker filter is substring match)
	for _, c := range containers {
		name := strings.TrimPrefix(c.Name, "/")
		if name == ContainerName {
			return &c, nil
		}
	}

	return nil, nil
}

// removeContainer stops and removes the guardian container.
func removeContainer() error {
	// Stop the container (ignore errors if already stopped)
	_, _ = docker.Run("stop", ContainerName)

	// Remove the container
	_, err := docker.Run("rm", ContainerName)
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			// Ignore "no such container" error
			if strings.Contains(cmdErr.Stderr, "No such container") {
				return nil
			}
		}
		return err
	}

	return nil
}

// DefaultAPIAddr is the address where the guardian API is exposed to the host.
const DefaultAPIAddr = "localhost:9997"

// RegisterToken registers a token with the guardian for a cloister.
// The guardian must be running before calling this function.
func RegisterToken(token, cloisterName string) error {
	running, err := IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check guardian status: %w", err)
	}
	if !running {
		return ErrGuardianNotRunning
	}

	client := NewClient(DefaultAPIAddr)
	return client.RegisterToken(token, cloisterName)
}

// RevokeToken revokes a token from the guardian.
// Returns nil if the guardian is not running or if the token doesn't exist.
func RevokeToken(token string) error {
	running, err := IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check guardian status: %w", err)
	}
	if !running {
		// Guardian not running, nothing to revoke
		return nil
	}

	client := NewClient(DefaultAPIAddr)
	return client.RevokeToken(token)
}
