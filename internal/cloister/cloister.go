// Package cloister provides high-level orchestration for starting and stopping
// cloister containers with proper guardian integration.
package cloister

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/xdg/cloister/internal/claude"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/token"
)

// containerHomeDir is the home directory for the cloister user inside containers.
const containerHomeDir = "/home/cloister"

// ContainerManager is the interface for container operations.
// This allows injecting mock implementations for testing.
type ContainerManager interface {
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

func (defaultConfigLoader) LoadGlobalConfig() (*config.GlobalConfig, error) {
	return config.LoadGlobalConfig()
}

// CredentialInjector is the interface for injecting credentials into containers.
// This allows injecting mock implementations for testing.
type CredentialInjector interface {
	InjectCredentials(cfg *config.AgentConfig) (*claude.InjectionConfig, error)
}

// defaultCredentialInjector implements CredentialInjector using the real claude package.
type defaultCredentialInjector struct {
	injector *claude.Injector
}

func (d *defaultCredentialInjector) InjectCredentials(cfg *config.AgentConfig) (*claude.InjectionConfig, error) {
	return d.injector.InjectCredentials(cfg)
}

// FileCopier is the interface for copying files to containers.
// This allows injecting mock implementations for testing.
type FileCopier interface {
	// WriteFileToContainerWithOwner writes a file with the specified ownership.
	// Uses tar piping which doesn't require extra container capabilities.
	WriteFileToContainerWithOwner(containerName, destPath, content, uid, gid string) error
}

// defaultFileCopier implements FileCopier using docker commands.
type defaultFileCopier struct{}

func (defaultFileCopier) WriteFileToContainerWithOwner(containerName, destPath, content, uid, gid string) error {
	return docker.WriteFileToContainerWithOwner(containerName, destPath, content, uid, gid)
}

// GuardianManager is the interface for guardian operations.
// This allows injecting mock implementations for testing.
type GuardianManager interface {
	// EnsureRunning ensures the guardian container is running.
	EnsureRunning() error

	// RegisterToken registers a token with the guardian for a cloister.
	RegisterToken(token, cloisterName, projectName string) error

	// RevokeToken revokes a token from the guardian.
	RevokeToken(token string) error
}

// defaultGuardianManager implements GuardianManager using the real guardian package.
type defaultGuardianManager struct{}

func (defaultGuardianManager) EnsureRunning() error {
	return guardian.EnsureRunning()
}

func (defaultGuardianManager) RegisterToken(token, cloisterName, projectName string) error {
	return guardian.RegisterToken(token, cloisterName, projectName)
}

func (defaultGuardianManager) RevokeToken(token string) error {
	return guardian.RevokeToken(token)
}

// Option configures cloister operations.
type Option func(*options)

type options struct {
	manager            ContainerManager
	guardian           GuardianManager
	configLoader       ConfigLoader
	credentialInjector CredentialInjector
	fileCopier         FileCopier
	stderr             io.Writer
	globalConfig       *config.GlobalConfig // Pre-loaded config (avoids double-load)
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

// WithCredentialInjector sets a custom credential injector for dependency injection.
// If not set, the real claude.Injector is used.
func WithCredentialInjector(c CredentialInjector) Option {
	return func(o *options) {
		o.credentialInjector = c
	}
}

// WithFileCopier sets a custom file copier for dependency injection.
// If not set, docker.WriteFileToContainer is used.
func WithFileCopier(f FileCopier) Option {
	return func(o *options) {
		o.fileCopier = f
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
	if o.credentialInjector == nil {
		o.credentialInjector = &defaultCredentialInjector{injector: claude.NewInjector()}
	}
	if o.fileCopier == nil {
		o.fileCopier = defaultFileCopier{}
	}
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
func Start(opts StartOptions, options ...Option) (containerID string, tok string, err error) {
	deps := applyOptions(options...)
	// Step 1: Ensure guardian is running
	if err := deps.guardian.EnsureRunning(); err != nil {
		return "", "", fmt.Errorf("guardian failed to start: %w", err)
	}

	// Step 2: Generate a new token
	tok = token.Generate()

	// Step 3: Build container name for registration
	containerName := container.GenerateContainerName(opts.ProjectName)

	// Step 4: Persist token to disk (for recovery after guardian restart)
	store, err := getTokenStore()
	if err != nil {
		return "", "", err
	}
	if err := store.Save(containerName, tok, opts.ProjectName); err != nil {
		return "", "", err
	}

	// Step 5: Register the token with guardian (with project name for per-project allowlist)
	if err := deps.guardian.RegisterToken(tok, containerName, opts.ProjectName); err != nil {
		// Clean up persisted token on failure
		_ = store.Remove(containerName)
		return "", "", err
	}

	// If container creation fails after token registration, we should revoke the token
	defer func() {
		if err != nil {
			// Best effort cleanup - ignore revocation errors
			_ = deps.guardian.RevokeToken(tok)
			_ = store.Remove(containerName)
		}
	}()

	// Step 6: Load global config and get credential injection config for Claude
	envVars := token.ProxyEnvVars(tok, "")
	var injectionConfig *claude.InjectionConfig

	// Use pre-loaded config if available, otherwise load it
	globalCfg := deps.globalConfig
	if globalCfg == nil {
		var cfgErr error
		globalCfg, cfgErr = deps.configLoader.LoadGlobalConfig()
		if cfgErr != nil {
			log.Printf("cloister: warning: failed to load global config: %v", cfgErr)
		}
	}

	if globalCfg != nil {
		if claudeCfg, ok := globalCfg.Agents["claude"]; ok && claudeCfg.AuthMethod != "" {
			// Config has Claude credentials configured - use them
			injectionConfig, err = deps.credentialInjector.InjectCredentials(&claudeCfg)
			if err != nil {
				return "", "", fmt.Errorf("credential injection failed: %w", err)
			}

			// Add env vars from injection config
			for key, value := range injectionConfig.EnvVars {
				envVars = append(envVars, key+"="+value)
			}
		} else {
			// No config credentials - fall back to host env vars (Phase 1 behavior)
			// This is deprecated - print warning once per Start() call
			usedEnvVars := token.CredentialEnvVarsUsed()
			if len(usedEnvVars) > 0 {
				fmt.Fprintf(deps.stderr, "Warning: Using %s from environment.\n", usedEnvVars[0])
				fmt.Fprintln(deps.stderr, "Run 'cloister setup claude' to store credentials in config.")
			}
			envVars = append(envVars, token.CredentialEnvVars()...)
		}
	} else {
		// Config load failed - fall back to host env vars
		usedEnvVars := token.CredentialEnvVarsUsed()
		if len(usedEnvVars) > 0 {
			fmt.Fprintf(deps.stderr, "Warning: Using %s from environment.\n", usedEnvVars[0])
			fmt.Fprintln(deps.stderr, "Run 'cloister setup claude' to store credentials in config.")
		}
		envVars = append(envVars, token.CredentialEnvVars()...)
	}

	cfg := &container.Config{
		Project:     opts.ProjectName,
		Branch:      opts.BranchName,
		ProjectPath: opts.ProjectPath,
		Image:       opts.Image,
		Network:     docker.CloisterNetworkName,
		EnvVars:     envVars,
	}

	// Step 7: Create the container (without starting)
	containerID, err = deps.manager.Create(cfg)
	if err != nil {
		return "", "", err
	}

	// If starting fails after container creation, remove the container
	defer func() {
		if err != nil {
			// Best effort cleanup - ignore removal errors
			_ = deps.manager.Stop(containerName)
		}
	}()

	// Step 8: Start the container (needed for docker exec chown in later steps)
	err = deps.manager.StartContainer(containerName)
	if err != nil {
		return "", "", err
	}

	// Step 9: Inject user settings (~/.claude/) into the container
	// This is a one-way snapshot - writes inside container are isolated
	// After copying, chown to cloister user (docker cp preserves host uid)
	if copyErr := injectUserSettings(containerName); copyErr != nil {
		// Log but don't fail - missing settings is not fatal
		// (user might not have ~/.claude/ on a fresh install)
		_ = copyErr
	}

	// Step 10: Inject credential files into the container (for "existing" auth method)
	// Uses tar piping to set ownership during copy (no extra capabilities needed)
	if injectionConfig != nil {
		for destPath, content := range injectionConfig.Files {
			if writeErr := deps.fileCopier.WriteFileToContainerWithOwner(containerName, destPath, content, "1000", "1000"); writeErr != nil {
				return "", "", fmt.Errorf("failed to write credential file %s: %w", destPath, writeErr)
			}
		}
	}

	return containerID, tok, nil
}

// userHomeDirFunc returns the user's home directory.
// Can be overridden in tests to use a mock home directory.
var userHomeDirFunc = os.UserHomeDir

// settingsExcludePatterns lists directories/files to exclude when copying ~/.claude/
// These are machine-local files that don't need to be in the container.
// Based on ~/.claude/.gitignore patterns.
var settingsExcludePatterns = []string{
	".update.lock",
	"debug/",
	"file-history/",
	"history.jsonl",
	"plans/",
	"plugins/install-counts-cache.json",
	"projects/",
	"shell-snapshots/",
	"stats-cache.json",
	"statsig/",
	"tasks/",
	"telemetry",
	"todos/",
	"cache",
	"downloads/",
}

// injectUserSettings copies the host's ~/.claude/ directory into the container.
// This provides a one-way snapshot so the agent inherits user's settings, skills,
// memory, and CLAUDE.md. Writes inside the container are isolated and do not
// modify the host config.
//
// Symlinks are dereferenced during copy so that settings stored in dotfiles
// repositories (e.g., ~/.claude -> ~/dotfiles/claude) work correctly inside
// the container where the original symlink target doesn't exist.
//
// Machine-local files (debug logs, history, todos, etc.) are excluded to keep
// the copy fast and avoid leaking sensitive conversation history.
//
// Returns nil if ~/.claude/ doesn't exist on the host (fresh install).
// Returns an error only if the directory exists but cannot be copied.
func injectUserSettings(containerName string) error {
	// Get the user's home directory
	homeDir, err := userHomeDirFunc()
	if err != nil {
		return err
	}

	claudeDir := filepath.Join(homeDir, ".claude")

	// Check if ~/.claude/ exists
	info, err := os.Stat(claudeDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist - this is fine, just skip
			return nil
		}
		return err
	}

	// Ensure it's a directory
	if !info.IsDir() {
		// ~/.claude is a file, not a directory - skip
		return nil
	}

	// Create a temp directory to hold the filtered copy
	tmpDir, err := os.MkdirTemp("", "cloister-claude-settings-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpClaudeDir := filepath.Join(tmpDir, ".claude")

	// Try rsync first (fast, supports exclusions natively)
	// Fall back to cp -rL if rsync isn't available
	if err := copyWithRsync(claudeDir, tmpClaudeDir); err != nil {
		// rsync failed or not available, fall back to cp
		if err := copyWithCp(claudeDir, tmpClaudeDir); err != nil {
			return err
		}
	}

	// Copy the filtered directory to the container home (creates .claude there)
	// Uses tar piping which sets ownership during copy (no extra capabilities needed)
	return docker.CopyToContainerWithOwner(tmpClaudeDir, containerName, containerHomeDir, "1000", "1000")
}

// copyWithRsync copies src to dest using rsync with exclusions and symlink dereferencing.
// Returns an error if rsync is not available or fails.
func copyWithRsync(src, dest string) error {
	args := []string{
		"-rL",             // recursive, dereference symlinks
		"--copy-dirlinks", // also dereference symlinks to directories
	}
	for _, pattern := range settingsExcludePatterns {
		args = append(args, "--exclude="+pattern)
	}
	// rsync needs trailing slash on source to copy contents, not the directory itself
	args = append(args, src+"/", dest)

	cmd := exec.Command("rsync", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %w: %s", err, output)
	}
	return nil
}

// copyWithCp copies src to dest using cp -rL (no exclusion support).
// This is the fallback when rsync is not available.
func copyWithCp(src, dest string) error {
	cmd := exec.Command("cp", "-rL", src, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp failed: %w: %s", err, output)
	}
	return nil
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
func Stop(containerName string, tok string, options ...Option) error {
	deps := applyOptions(options...)
	// Step 1: Revoke the token from guardian (if provided)
	// We ignore revocation errors and continue with container stop.
	// The token will become orphaned but won't cause security issues
	// since the container will no longer exist.
	if tok != "" {
		_ = deps.guardian.RevokeToken(tok)
	}

	// Step 2: Remove the token from disk (best effort)
	if store, err := getTokenStore(); err == nil {
		_ = store.Remove(containerName)
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
