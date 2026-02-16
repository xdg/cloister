// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/version"
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
	// BridgeNetwork is the default Docker bridge network for external access.
	BridgeNetwork = "bridge"

	// ContainerTokenDir is the path inside the guardian container where tokens are mounted.
	ContainerTokenDir = "/var/lib/cloister/tokens" //nolint:gosec // G101: not a credential

	// ContainerConfigDir is the path inside the guardian container where config is mounted.
	// We set XDG_CONFIG_HOME=/etc so ConfigDir() returns /etc/cloister/.
	ContainerConfigDir = "/etc/cloister"

	// ContainerDecisionDir is the path inside the guardian container where decisions are mounted.
	// This overlays the ro config mount at ContainerConfigDir, allowing rw access for decisions.
	ContainerDecisionDir = ContainerConfigDir + "/decisions"
)

// ErrGuardianNotRunning indicates the guardian container is not running.
var ErrGuardianNotRunning = errors.New("guardian container is not running")

// hostCloisterPath returns a path under config.Dir()/<subdir>.
// Uses config.Dir() which respects XDG_CONFIG_HOME.
func hostCloisterPath(subdir string) (string, error) {
	base := config.Dir()
	if subdir == "" {
		// Dir() has trailing slash, remove it for consistency
		return strings.TrimSuffix(base, "/"), nil
	}
	return base + subdir, nil
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
	//
	// Deprecated: Use TCPPort instead.
	SocketPath string

	// TCPPort is the TCP port the executor is listening on (on the host).
	// The guardian container will connect to host.docker.internal:TCPPort.
	TCPPort int

	// SharedSecret is the secret for authenticating executor requests.
	// If empty, the executor is not enabled.
	SharedSecret string

	// TokenAPIPort is the host port to expose for the guardian token API.
	// If zero, defaults to 9997 for production or a dynamic port for test instances.
	TokenAPIPort int

	// ApprovalPort is the host port to expose for the guardian approval web UI.
	// If zero, defaults to 9999 for production or a dynamic port for test instances.
	ApprovalPort int
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
	if err := ensureCleanState(); err != nil {
		return err
	}

	if err := defaultDockerOps.EnsureCloisterNetwork(); err != nil {
		return fmt.Errorf("failed to create cloister network: %w", err)
	}

	dirs, err := ensureHostDirs()
	if err != nil {
		return err
	}

	tokenAPIPort, approvalPort, err := resolveGuardianPorts(opts)
	if err != nil {
		return err
	}

	args := buildGuardianRunArgs(opts, tokenAPIPort, approvalPort, dirs)

	if _, err = defaultDockerOps.Run(args...); err != nil {
		return fmt.Errorf("failed to start guardian container: %w", err)
	}

	if _, err = defaultDockerOps.Run("network", "connect", docker.CloisterNetworkName, ContainerName()); err != nil {
		if removeErr := removeContainer(); removeErr != nil {
			clog.Warn("failed to clean up guardian container after network connect failure: %v", removeErr)
		}
		return fmt.Errorf("failed to connect guardian to cloister network: %w", err)
	}

	return nil
}

// ensureCleanState checks for an existing container and removes it if not running.
func ensureCleanState() error {
	state, err := getContainerState()
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}
	if state.State == "running" {
		return ErrGuardianAlreadyRunning
	}
	return removeContainer()
}

// hostDirs holds the host directory paths needed for guardian container mounts.
type hostDirs struct {
	TokenDir    string
	ConfigDir   string
	DecisionDir string
}

// ensureHostDirs creates and returns the host directories needed for guardian mounts.
func ensureHostDirs() (hostDirs, error) {
	var dirs hostDirs
	var err error
	dirs.TokenDir, err = HostTokenDir()
	if err != nil {
		return dirs, fmt.Errorf("failed to get token directory: %w", err)
	}
	if err := os.MkdirAll(dirs.TokenDir, 0o700); err != nil {
		return dirs, fmt.Errorf("failed to create token directory: %w", err)
	}

	dirs.ConfigDir, err = HostConfigDir()
	if err != nil {
		return dirs, fmt.Errorf("failed to get config directory: %w", err)
	}
	if err := os.MkdirAll(dirs.ConfigDir, 0o700); err != nil {
		return dirs, fmt.Errorf("failed to create config directory: %w", err)
	}

	if _, err := config.MigrateDecisionDir(); err != nil {
		return dirs, fmt.Errorf("failed to migrate approvals directory: %w", err)
	}

	dirs.DecisionDir = dirs.ConfigDir + "/decisions"
	if err := os.MkdirAll(dirs.DecisionDir, 0o700); err != nil {
		return dirs, fmt.Errorf("failed to create decision directory: %w", err)
	}

	return dirs, nil
}

// resolveGuardianPorts returns the token API and approval ports, allocating them if needed.
func resolveGuardianPorts(opts StartOptions) (tokenAPIPort, approvalPort int, err error) {
	if opts.TokenAPIPort != 0 && opts.ApprovalPort != 0 {
		return opts.TokenAPIPort, opts.ApprovalPort, nil
	}
	tokenAPIPort, approvalPort, err = Ports()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to allocate guardian ports: %w", err)
	}
	return tokenAPIPort, approvalPort, nil
}

// buildGuardianRunArgs builds the docker run arguments for the guardian container.
func buildGuardianRunArgs(opts StartOptions, tokenAPIPort, approvalPort int, dirs hostDirs) []string {
	args := []string{
		"run", "-d",
		"--name", ContainerName(),
		"--network", BridgeNetwork,
		"-p", fmt.Sprintf("127.0.0.1:%d:9997", tokenAPIPort),
		"-p", fmt.Sprintf("127.0.0.1:%d:9999", approvalPort),
		"-e", "XDG_CONFIG_HOME=/etc",
		"-v", dirs.TokenDir + ":" + ContainerTokenDir + ":ro",
		"-v", dirs.ConfigDir + ":" + ContainerConfigDir + ":ro",
		"-v", dirs.DecisionDir + ":" + ContainerDecisionDir,
	}

	if opts.TCPPort > 0 {
		args = append(args, "-e", fmt.Sprintf("%s=%d", ExecutorPortEnvVar, opts.TCPPort))
	}
	if opts.SharedSecret != "" {
		args = append(args, "-e", SharedSecretEnvVar+"="+opts.SharedSecret)
	}

	args = append(args, version.DefaultImage(), "cloister", "guardian", "run")
	return args
}

// Stop stops the executor daemon and removes the guardian container.
// Returns nil if nothing is running (idempotent).
func Stop() error {
	// Stop the executor daemon first
	if err := StopExecutor(); err != nil {
		// Log but continue - we still want to stop the container
		clog.Warn("failed to stop executor daemon: %v", err)
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

	tokenAPIPort, approvalPort, err := Ports()
	if err != nil {
		return fmt.Errorf("failed to allocate guardian ports: %w", err)
	}

	execInfo, err := StartExecutor()
	if err != nil {
		return fmt.Errorf("failed to start executor: %w", err)
	}

	if err := saveGuardianPorts(execInfo, tokenAPIPort, approvalPort); err != nil {
		cleanupExecutor(execInfo)
		return err
	}

	opts := StartOptions{
		TCPPort:      execInfo.TCPPort,
		SharedSecret: execInfo.Secret,
		TokenAPIPort: tokenAPIPort,
		ApprovalPort: approvalPort,
	}
	if err := StartWithOptions(opts); err != nil {
		cleanupExecutor(execInfo)
		return err
	}

	return WaitReadyWithPort(tokenAPIPort, 5*time.Second)
}

// saveGuardianPorts saves the guardian ports to executor state so clients can discover them.
func saveGuardianPorts(_ *ExecutorInfo, tokenAPIPort, approvalPort int) error {
	execState, err := executor.LoadDaemonState()
	if err != nil {
		return fmt.Errorf("failed to load executor state for port storage: %w", err)
	}
	if execState == nil {
		return nil
	}
	execState.TokenAPIPort = tokenAPIPort
	execState.ApprovalPort = approvalPort
	if err := executor.SaveDaemonState(execState); err != nil {
		return fmt.Errorf("failed to save guardian ports to executor state: %w", err)
	}
	return nil
}

// cleanupExecutor kills the executor process and stops the daemon.
func cleanupExecutor(execInfo *ExecutorInfo) {
	if execInfo.Process != nil {
		if killErr := execInfo.Process.Kill(); killErr != nil {
			clog.Warn("failed to kill executor process: %v", killErr)
		}
	}
	if stopErr := StopExecutor(); stopErr != nil {
		clog.Warn("failed to stop executor: %v", stopErr)
	}
}

// WaitReady polls the guardian API until it responds or timeout is reached.
// This ensures the API server inside the container has started accepting connections.
// Uses the default API address (dynamic for test instances).
func WaitReady(timeout time.Duration) error {
	return WaitReadyWithPort(APIPort(), timeout)
}

// WaitReadyWithPort polls the guardian API on a specific port until it responds.
func WaitReadyWithPort(port int, timeout time.Duration) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	// Use 127.0.0.1 explicitly since Docker port is bound to IPv4 only
	url := fmt.Sprintf("http://127.0.0.1:%d/tokens", port)

	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
		if reqErr != nil {
			lastErr = reqErr
			time.Sleep(100 * time.Millisecond)
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
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
	info, err := defaultDockerOps.FindContainerByExactName(ContainerName())
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
	if _, err := defaultDockerOps.Run("stop", ContainerName()); err != nil {
		clog.Warn("failed to stop guardian container (may already be stopped): %v", err)
	}

	// Remove the container
	_, err := defaultDockerOps.Run("rm", ContainerName())
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
// For production, this is "127.0.0.1:9997". For test instances, use APIAddr().
// Uses explicit IPv4 since Docker ports are bound to 127.0.0.1.
const DefaultAPIAddr = "127.0.0.1:9997"

// APIPort returns the port where the guardian token API is exposed.
// For production (no instance ID), returns 9997.
// For test instances, reads from executor state.
func APIPort() int {
	if InstanceID() == "" {
		return DefaultTokenAPIPort
	}
	// For test instances, read from executor state
	state, err := executor.LoadDaemonState()
	if err != nil || state == nil || state.TokenAPIPort == 0 {
		// Fallback to default if state not available
		return DefaultTokenAPIPort
	}
	return state.TokenAPIPort
}

// APIAddr returns the address where the guardian token API is exposed.
// For production, returns "127.0.0.1:9997".
// For test instances, returns the dynamic port from executor state.
// Uses explicit IPv4 since Docker ports are bound to 127.0.0.1.
func APIAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", APIPort())
}

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
	return NewClient(APIAddr()), nil
}

// RegisterToken registers a token with the guardian for a cloister.
// The guardian must be running before calling this function.
// The projectName is used for per-project allowlist lookups.
//
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
