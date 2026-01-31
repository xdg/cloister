// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xdg/cloister/internal/docker"
)

// DockerOps abstracts Docker operations for testing guardian container management.
// This interface covers all Docker CLI operations needed by the guardian package.
type DockerOps interface {
	// CheckDaemon verifies the Docker daemon is running and accessible.
	CheckDaemon() error

	// EnsureCloisterNetwork creates the standard cloister network if it doesn't exist.
	EnsureCloisterNetwork() error

	// Run executes a docker CLI command and returns stdout.
	Run(args ...string) (string, error)

	// FindContainerByExactName finds a container with the exact name specified.
	FindContainerByExactName(name string) (*docker.ContainerInfo, error)
}

// DefaultDockerOps implements DockerOps using the real Docker CLI.
type DefaultDockerOps struct{}

// Compile-time interface check
var _ DockerOps = DefaultDockerOps{}

// CheckDaemon implements DockerOps.
func (DefaultDockerOps) CheckDaemon() error {
	return docker.CheckDaemon()
}

// EnsureCloisterNetwork implements DockerOps.
func (DefaultDockerOps) EnsureCloisterNetwork() error {
	return docker.EnsureCloisterNetwork()
}

// Run implements DockerOps.
func (DefaultDockerOps) Run(args ...string) (string, error) {
	return docker.Run(args...)
}

// FindContainerByExactName implements DockerOps.
func (DefaultDockerOps) FindContainerByExactName(name string) (*docker.ContainerInfo, error) {
	return docker.FindContainerByExactName(name)
}

// defaultDockerOps is the package-level default Docker operations implementation.
// It can be replaced via SetDockerOps for testing.
var defaultDockerOps DockerOps = DefaultDockerOps{}

// SetDockerOps sets the Docker operations implementation used by this package.
// This is intended for testing purposes. Pass nil to restore the default.
func SetDockerOps(ops DockerOps) {
	if ops == nil {
		defaultDockerOps = DefaultDockerOps{}
	} else {
		defaultDockerOps = ops
	}
}

// Container constants for the guardian service.
const (
	// ContainerName is the name of the guardian container.
	ContainerName = "cloister-guardian"

	// DefaultImage is the Docker image used for the guardian container.
	DefaultImage = "cloister:latest"

	// BridgeNetwork is the default Docker bridge network for external access.
	BridgeNetwork = "bridge"

	// ContainerTokenDir is the path inside the guardian container where tokens are mounted.
	ContainerTokenDir = "/var/lib/cloister/tokens"

	// ContainerConfigDir is the path inside the guardian container where config is mounted.
	// We set XDG_CONFIG_HOME=/etc so ConfigDir() returns /etc/cloister/.
	ContainerConfigDir = "/etc/cloister"
)

// ErrGuardianNotRunning indicates the guardian container is not running.
var ErrGuardianNotRunning = errors.New("guardian container is not running")

// hostCloisterPath returns a path under ~/.config/cloister/<subdir>.
func hostCloisterPath(subdir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	if subdir == "" {
		return filepath.Join(home, ".config", "cloister"), nil
	}
	return filepath.Join(home, ".config", "cloister", subdir), nil
}

// HostTokenDir returns the token directory path on the host.
// This is ~/.config/cloister/tokens.
func HostTokenDir() (string, error) {
	return hostCloisterPath("tokens")
}

// HostConfigDir returns the config directory path on the host.
// This is ~/.config/cloister.
func HostConfigDir() (string, error) {
	return hostCloisterPath("")
}

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
// Returns docker.ErrDockerNotRunning if the Docker daemon is not accessible.
func IsRunning() (bool, error) {
	// Check Docker daemon availability first
	if err := defaultDockerOps.CheckDaemon(); err != nil {
		return false, err
	}

	state, err := getContainerState()
	if err != nil {
		return false, err
	}
	return state != nil && state.State == "running", nil
}

// StartOptions configures guardian container startup.
type StartOptions struct {
	// SocketPath is the path to the hostexec socket on the host.
	// If empty, the socket is not mounted.
	SocketPath string

	// SharedSecret is the secret for authenticating executor requests.
	// If empty, the executor is not enabled.
	SharedSecret string
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
	return StartWithOptions(StartOptions{})
}

// StartWithOptions starts the guardian container with additional options.
// See Start for container configuration details.
func StartWithOptions(opts StartOptions) error {
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
	if err := defaultDockerOps.EnsureCloisterNetwork(); err != nil {
		return fmt.Errorf("failed to create cloister network: %w", err)
	}

	// Get the host token directory for mounting
	hostTokenDir, err := HostTokenDir()
	if err != nil {
		return fmt.Errorf("failed to get token directory: %w", err)
	}

	// Ensure token directory exists (creates with 0700 permissions)
	if err := os.MkdirAll(hostTokenDir, 0700); err != nil {
		return fmt.Errorf("failed to create token directory: %w", err)
	}

	// Get the host config directory for mounting
	hostConfigDir, err := HostConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Ensure config directory exists (creates with 0700 permissions)
	if err := os.MkdirAll(hostConfigDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Build docker run arguments
	// Port 9997 is exposed to the host for the token management API
	// (used by CLI to register/revoke tokens)
	// Port 9999 is exposed to the host for the approval web UI
	// (used by humans to approve/deny hostexec requests)
	// Token directory is mounted read-only for recovery on restart
	// Config directory is mounted read-only for allowlist configuration
	// XDG_CONFIG_HOME=/etc so ConfigDir() returns /etc/cloister/
	args := []string{
		"run", "-d",
		"--name", ContainerName,
		"--network", docker.CloisterNetworkName,
		"-p", "127.0.0.1:9997:9997",
		"-p", "127.0.0.1:9999:9999",
		"-e", "XDG_CONFIG_HOME=/etc",
		"-v", hostTokenDir + ":" + ContainerTokenDir + ":ro",
		"-v", hostConfigDir + ":" + ContainerConfigDir + ":ro",
	}

	// Add executor socket mount if provided
	if opts.SocketPath != "" {
		args = append(args, "-v", opts.SocketPath+":"+ContainerSocketPath)
	}

	// Add shared secret environment variable if provided
	if opts.SharedSecret != "" {
		args = append(args, "-e", SharedSecretEnvVar+"="+opts.SharedSecret)
	}

	// Add image and command
	args = append(args, DefaultImage, "cloister", "guardian", "run")

	// Create and start the container
	_, err = defaultDockerOps.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to start guardian container: %w", err)
	}

	// Connect to bridge network for external access to upstream servers
	_, err = defaultDockerOps.Run("network", "connect", BridgeNetwork, ContainerName)
	if err != nil {
		// If connecting to bridge fails, clean up the container
		_ = removeContainer()
		return fmt.Errorf("failed to connect guardian to bridge network: %w", err)
	}

	return nil
}

// Stop stops the executor daemon and removes the guardian container.
// Returns nil if nothing is running (idempotent).
func Stop() error {
	// Stop the executor daemon first
	if err := StopExecutor(); err != nil {
		// Log but continue - we still want to stop the container
		_ = err
	}

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

// EnsureRunning ensures the guardian container and executor daemon are running.
// If the container is already running, this is a no-op.
// If the container is not running, it starts the executor daemon and container,
// then waits for API readiness.
func EnsureRunning() error {
	running, err := IsRunning()
	if err != nil {
		return err
	}

	if running {
		return nil
	}

	// Start the executor daemon first
	execInfo, err := StartExecutor()
	if err != nil {
		return fmt.Errorf("failed to start executor: %w", err)
	}

	// Start the guardian container with executor socket mounted
	opts := StartOptions{
		SocketPath:   execInfo.SocketPath,
		SharedSecret: execInfo.Secret,
	}
	if err := StartWithOptions(opts); err != nil {
		// Clean up executor if guardian fails to start
		if execInfo.Process != nil {
			_ = execInfo.Process.Kill()
		}
		_ = StopExecutor()
		return err
	}

	// Wait for API to be ready after fresh start
	return WaitReady(5 * time.Second)
}

// WaitReady polls the guardian API until it responds or timeout is reached.
// This ensures the API server inside the container has started accepting connections.
func WaitReady(timeout time.Duration) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := fmt.Sprintf("http://%s/tokens", DefaultAPIAddr)

	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	if lastErr != nil {
		return fmt.Errorf("guardian API not ready after %v: %w", timeout, lastErr)
	}
	return fmt.Errorf("guardian API not ready after %v", timeout)
}

// getContainerState retrieves the current state of the guardian container.
// Returns nil if the container doesn't exist.
func getContainerState() (*containerState, error) {
	info, err := defaultDockerOps.FindContainerByExactName(ContainerName)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}

	return &containerState{
		ID:    info.ID,
		Name:  info.Name(),
		State: info.State,
	}, nil
}

// removeContainer stops and removes the guardian container.
func removeContainer() error {
	// Stop the container (ignore errors if already stopped)
	_, _ = defaultDockerOps.Run("stop", ContainerName)

	// Remove the container
	_, err := defaultDockerOps.Run("rm", ContainerName)
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

// withGuardianClient checks if the guardian is running and returns a client.
// Returns ErrGuardianNotRunning if the guardian is not running.
func withGuardianClient() (*Client, error) {
	running, err := IsRunning()
	if err != nil {
		return nil, fmt.Errorf("failed to check guardian status: %w", err)
	}
	if !running {
		return nil, ErrGuardianNotRunning
	}
	return NewClient(DefaultAPIAddr), nil
}

// RegisterToken registers a token with the guardian for a cloister.
// The guardian must be running before calling this function.
// The projectName is used for per-project allowlist lookups.
// Deprecated: Use RegisterTokenFull to include the worktree path.
func RegisterToken(token, cloisterName, projectName string) error {
	return RegisterTokenFull(token, cloisterName, projectName, "")
}

// RegisterTokenFull registers a token with the guardian for a cloister.
// The guardian must be running before calling this function.
// The projectName is used for per-project allowlist lookups.
// The worktreePath is the absolute path to the project on the host, used for hostexec validation.
func RegisterTokenFull(token, cloisterName, projectName, worktreePath string) error {
	client, err := withGuardianClient()
	if err != nil {
		return err
	}
	return client.RegisterTokenFull(token, cloisterName, projectName, worktreePath)
}

// RevokeToken revokes a token from the guardian.
// Returns nil if the guardian is not running or if the token doesn't exist.
func RevokeToken(token string) error {
	client, err := withGuardianClient()
	if errors.Is(err, ErrGuardianNotRunning) {
		// Guardian not running, nothing to revoke
		return nil
	}
	if err != nil {
		return err
	}
	return client.RevokeToken(token)
}

// ListTokens returns a map of all registered tokens to their cloister names.
// Returns an empty map if the guardian is not running.
func ListTokens() (map[string]string, error) {
	client, err := withGuardianClient()
	if errors.Is(err, ErrGuardianNotRunning) {
		// Guardian not running, return empty map
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	return client.ListTokens()
}
