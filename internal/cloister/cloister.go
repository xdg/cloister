// Package cloister provides high-level orchestration for starting and stopping
// cloister containers with proper guardian integration.
package cloister

import (
	"fmt"
	"io"
	"os"

	"github.com/xdg/cloister/internal/agent"
	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/term"
	"github.com/xdg/cloister/internal/token"
)

// ContainerManager is the interface for container operations.
// This allows injecting mock implementations for testing.
type ContainerManager interface {
	ContainerExists(name string) (bool, error)
	Create(cfg *container.Config) (string, error)
	Start(cfg *container.Config) (string, error)
	StartContainer(containerName string) error
	Stop(containerName string) error
	Attach(containerName string) (int, error)
}

// ConfigLoader is the interface for loading configuration.
// This allows injecting mock implementations for testing.
type ConfigLoader interface {
	LoadGlobalConfig() (*config.GlobalConfig, error)
}

// defaultConfigLoader implements ConfigLoader using the real config package.
type defaultConfigLoader struct{}

// LoadGlobalConfig delegates to the real config package.
func (defaultConfigLoader) LoadGlobalConfig() (*config.GlobalConfig, error) {
	return config.LoadGlobalConfig()
}

// GuardianManager is the interface for guardian operations.
// This allows injecting mock implementations for testing.
type GuardianManager interface {
	// EnsureRunning ensures the guardian container is running.
	EnsureRunning() error

	// RegisterToken registers a token with the guardian for a cloister.
	//
	// Deprecated: Use RegisterTokenFull for worktree path validation.
	RegisterToken(token, cloisterName, projectName string) error

	// RegisterTokenFull registers a token with the guardian for a cloister.
	// Includes the worktree path for hostexec workdir validation.
	RegisterTokenFull(token, cloisterName, projectName, worktreePath string) error

	// RevokeToken revokes a token from the guardian.
	RevokeToken(token string) error
}

// defaultGuardianManager implements GuardianManager using the real guardian package.
type defaultGuardianManager struct{}

// EnsureRunning delegates to the real guardian package.
func (defaultGuardianManager) EnsureRunning() error {
	return guardian.EnsureRunning()
}

// RegisterToken delegates to the real guardian package.
func (defaultGuardianManager) RegisterToken(tok, cloisterName, projectName string) error {
	return guardian.RegisterTokenFull(tok, cloisterName, projectName, "")
}

// RegisterTokenFull delegates to the real guardian package.
func (defaultGuardianManager) RegisterTokenFull(tok, cloisterName, projectName, worktreePath string) error {
	return guardian.RegisterTokenFull(tok, cloisterName, projectName, worktreePath)
}

// RevokeToken delegates to the real guardian package.
func (defaultGuardianManager) RevokeToken(tok string) error {
	return guardian.RevokeToken(tok)
}

// Option configures cloister operations.
type Option func(*options)

type options struct {
	manager      ContainerManager
	guardian     GuardianManager
	configLoader ConfigLoader
	agent        agent.Agent
	stderr       io.Writer
	globalConfig *config.GlobalConfig // Pre-loaded config (avoids double-load)
}

// WithManager sets a custom container manager for dependency injection.
// If not set, a default container.NewManager() is used.
func WithManager(m ContainerManager) Option {
	return func(o *options) {
		o.manager = m
	}
}

// WithGuardian sets a custom guardian manager for dependency injection.
// If not set, the real guardian package functions are used.
func WithGuardian(g GuardianManager) Option {
	return func(o *options) {
		o.guardian = g
	}
}

// WithConfigLoader sets a custom config loader for dependency injection.
// If not set, the real config.LoadGlobalConfig() is used.
func WithConfigLoader(c ConfigLoader) Option {
	return func(o *options) {
		o.configLoader = c
	}
}

// WithAgent sets a custom agent for dependency injection.
// If not set, the agent is looked up from the registry based on config.
func WithAgent(a agent.Agent) Option {
	return func(o *options) {
		o.agent = a
	}
}

// WithStderr sets a custom writer for stderr output (warnings, deprecation notices).
// If not set, os.Stderr is used.
func WithStderr(w io.Writer) Option {
	return func(o *options) {
		o.stderr = w
	}
}

// WithGlobalConfig sets a pre-loaded global config.
// If set, Start() won't reload the config, avoiding duplicate log messages.
func WithGlobalConfig(cfg *config.GlobalConfig) Option {
	return func(o *options) {
		o.globalConfig = cfg
	}
}

// applyOptions applies options and returns resolved dependencies.
func applyOptions(opts ...Option) *options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	if o.manager == nil {
		o.manager = container.NewManager()
	}
	if o.guardian == nil {
		o.guardian = defaultGuardianManager{}
	}
	if o.configLoader == nil {
		o.configLoader = defaultConfigLoader{}
	}
	// Note: o.agent is resolved later in Start() based on config, unless explicitly set
	if o.stderr == nil {
		o.stderr = os.Stderr
	}
	return o
}

// getTokenStore creates a token store using the default token directory.
// This helper consolidates the repeated pattern of getting the token directory
// and creating a store from it.
func getTokenStore() (*token.Store, error) {
	tokenDir, err := token.DefaultTokenDir()
	if err != nil {
		return nil, err
	}
	return token.NewStore(tokenDir)
}

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

	// Agent is the name of the agent to use (e.g., "claude", "codex").
	// If empty, uses the config's defaults.agent or falls back to "claude".
	Agent string
}

// Start orchestrates starting a cloister container with all necessary setup:
// 1. Ensures guardian is running
// 2. Generates a new token
// 3. Registers the token with guardian
// 4. Loads global config and gets credential injection config
// 5. Creates the container (without starting)
// 6. Injects user settings (~/.claude/) into the container
// 7. Injects credential files into the container
// 8. Starts the container
//
// Returns the container ID and token. The token is returned so it can be used
// for cleanup later (revocation when stopping the container).
//
// Options can be used to inject dependencies for testing:
//
//	Start(opts, WithManager(mockManager), WithGuardian(mockGuardian))
func Start(opts StartOptions, options ...Option) (containerID, tok string, err error) {
	deps := applyOptions(options...)

	// Step 1: Check if container already exists before mutating any state.
	cloisterName := container.GenerateCloisterName(opts.ProjectName)
	containerName := container.CloisterNameToContainerName(cloisterName)
	exists, err := deps.manager.ContainerExists(containerName)
	if err != nil {
		return "", "", fmt.Errorf("failed to check container status: %w", err)
	}
	if exists {
		return "", "", container.ErrContainerExists
	}

	// Step 2: Ensure guardian is running
	if err := deps.guardian.EnsureRunning(); err != nil {
		return "", "", fmt.Errorf("guardian failed to start: %w", err)
	}

	// Steps 3-5: Generate, persist, and register token
	tok, store, err := registerCloisterToken(deps, cloisterName, opts)
	if err != nil {
		return "", "", err
	}

	// If container creation fails after token registration, revoke the token
	defer func() {
		if err != nil {
			if revokeErr := deps.guardian.RevokeToken(tok); revokeErr != nil {
				clog.Warn("failed to revoke token on cleanup: %v", revokeErr)
			}
			if removeErr := store.Remove(cloisterName); removeErr != nil {
				clog.Warn("failed to remove token on cleanup: %v", removeErr)
			}
		}
	}()

	// Steps 6-9: Resolve agent, create container, start, and run agent setup
	agentImpl, agentCfg, envVars := resolveAgentAndEnv(deps, opts, tok)

	containerID, err = createAndStartContainer(deps, opts, containerSetup{
		ContainerName: containerName,
		EnvVars:       envVars,
		AgentImpl:     agentImpl,
		AgentCfg:      agentCfg,
	})
	if err != nil {
		return "", "", err
	}

	return containerID, tok, nil
}

// containerSetup groups the parameters needed to create and start a container.
type containerSetup struct {
	ContainerName string
	EnvVars       []string
	AgentImpl     agent.Agent
	AgentCfg      *config.AgentConfig
}

// createAndStartContainer creates, starts, and sets up the agent in a container.
// On failure, it cleans up the container.
func createAndStartContainer(deps *options, opts StartOptions, cs containerSetup) (string, error) {
	cfg := &container.Config{
		Project:     opts.ProjectName,
		Branch:      opts.BranchName,
		ProjectPath: opts.ProjectPath,
		Image:       opts.Image,
		Network:     docker.CloisterNetworkName,
		EnvVars:     cs.EnvVars,
	}

	containerID, err := deps.manager.Create(cfg)
	if err != nil {
		return "", err
	}

	if err := deps.manager.StartContainer(cs.ContainerName); err != nil {
		if stopErr := deps.manager.Stop(cs.ContainerName); stopErr != nil {
			clog.Warn("failed to stop container on cleanup: %v", stopErr)
		}
		return "", err
	}

	if cs.AgentImpl != nil {
		if _, setupErr := cs.AgentImpl.Setup(cs.ContainerName, cs.AgentCfg); setupErr != nil {
			if stopErr := deps.manager.Stop(cs.ContainerName); stopErr != nil {
				clog.Warn("failed to stop container on cleanup: %v", stopErr)
			}
			return "", setupErr
		}
	}

	return containerID, nil
}

// registerCloisterToken generates a token, persists it to disk, and registers with guardian.
func registerCloisterToken(deps *options, cloisterName string, opts StartOptions) (string, *token.Store, error) {
	tok := token.Generate()

	store, err := getTokenStore()
	if err != nil {
		return "", nil, err
	}
	if err := store.SaveFull(cloisterName, tok, opts.ProjectName, opts.ProjectPath); err != nil {
		return "", nil, err
	}

	if err := deps.guardian.RegisterTokenFull(tok, cloisterName, opts.ProjectName, opts.ProjectPath); err != nil {
		if removeErr := store.Remove(cloisterName); removeErr != nil {
			clog.Warn("failed to remove token on cleanup: %v", removeErr)
		}
		return "", nil, err
	}

	return tok, store, nil
}

// resolveAgentAndEnv resolves the agent implementation and builds container env vars.
func resolveAgentAndEnv(deps *options, opts StartOptions, tok string) (agent.Agent, *config.AgentConfig, []string) {
	envVars := guardian.ProxyEnvVars(tok, "")

	globalCfg := deps.globalConfig
	if globalCfg == nil {
		var cfgErr error
		globalCfg, cfgErr = deps.configLoader.LoadGlobalConfig()
		if cfgErr != nil {
			clog.Warn("failed to load global config: %v", cfgErr)
		}
	}

	agentImpl, agentName, agentCfg := resolveAgent(deps, globalCfg, opts.Agent)

	if agentImpl != nil {
		containerEnvVars, envErr := agent.GetContainerEnvVars(agentImpl, agentCfg)
		if envErr != nil {
			clog.Warn("failed to get container env vars: %v", envErr)
		} else {
			for key, value := range containerEnvVars {
				envVars = append(envVars, key+"="+value)
			}
		}
	}

	if agentCfg == nil || agentCfg.AuthMethod == "" {
		usedEnvVars := token.CredentialEnvVarsUsed() //nolint:staticcheck // intentional fallback
		if len(usedEnvVars) > 0 {
			term.Warn("Using %s from environment. Run 'cloister setup %s' to store credentials in config.", usedEnvVars[0], agentName)
		}
		envVars = append(envVars, token.CredentialEnvVars()...) //nolint:staticcheck // intentional fallback
	}

	return agentImpl, agentCfg, envVars
}

// resolveAgent determines the agent implementation, name, and config.
func resolveAgent(deps *options, globalCfg *config.GlobalConfig, cliAgent string) (agent.Agent, string, *config.AgentConfig) {
	agentImpl := deps.agent
	agentName := "claude"
	if globalCfg != nil && globalCfg.Defaults.Agent != "" {
		agentName = globalCfg.Defaults.Agent
	}
	if cliAgent != "" {
		agentName = cliAgent
	}
	var agentCfg *config.AgentConfig
	if globalCfg != nil {
		if cfg, ok := globalCfg.Agents[agentName]; ok {
			agentCfg = &cfg
		}
	}
	if agentImpl == nil {
		agentImpl = agent.Get(agentName)
	}
	return agentImpl, agentName, agentCfg
}

// Stop orchestrates stopping a cloister container with proper cleanup:
// 1. Revokes the token from guardian
// 2. Removes the token from disk
// 3. Stops and removes the container
//
// The token parameter should be the token returned from Start().
// If the token is empty, only the container is stopped (token revocation is skipped).
//
// Options can be used to inject dependencies for testing:
//
//	Stop(containerName, tok, WithManager(mockManager), WithGuardian(mockGuardian))
func Stop(containerName, tok string, options ...Option) error {
	deps := applyOptions(options...)
	// Step 1: Revoke the token from guardian (if provided)
	// We ignore revocation errors and continue with container stop.
	// The token will become orphaned but won't cause security issues
	// since the container will no longer exist.
	if tok != "" {
		if revokeErr := deps.guardian.RevokeToken(tok); revokeErr != nil {
			clog.Warn("failed to revoke token: %v", revokeErr)
		}
	}

	// Step 2: Remove the token from disk (best effort)
	// Store files are keyed by cloister name, not container name.
	if store, err := getTokenStore(); err == nil {
		cloisterName := container.NameToCloisterName(containerName)
		if removeErr := store.Remove(cloisterName); removeErr != nil {
			clog.Warn("failed to remove token from disk: %v", removeErr)
		}
	}

	// Step 3: Stop and remove the container
	return deps.manager.Stop(containerName)
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
//
// Options can be used to inject dependencies for testing:
//
//	Attach(containerID, WithManager(mockManager))
func Attach(containerID string, options ...Option) (exitCode int, err error) {
	deps := applyOptions(options...)
	return deps.manager.Attach(containerID)
}
