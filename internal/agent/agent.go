// Package agent provides the Agent interface and utilities for AI agent setup
// in cloister containers. Each agent (Claude, Codex, Gemini CLI, etc.) implements
// the Agent interface to handle its specific configuration needs.
package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/docker"
)

// containerHomeDir is the home directory for the cloister user inside containers.
const containerHomeDir = "/home/cloister"

// ContainerUID is the UID of the cloister user inside containers.
const ContainerUID = "1000"

// ContainerGID is the GID of the cloister user inside containers.
const ContainerGID = "1000"

// SetupResult contains the results of agent setup that the orchestration
// code needs to configure the container.
type SetupResult struct {
	// EnvVars contains environment variables to set in the container.
	// Keys are variable names, values are the variable values.
	EnvVars map[string]string
}

// Agent defines the interface for AI agent setup in containers.
// Each agent implementation handles its specific configuration needs
// (settings directories, config files, credentials, etc.).
type Agent interface {
	// Name returns the agent identifier (e.g., "claude", "codex").
	// This is used for config lookups and logging.
	Name() string

	// Setup performs all agent-specific container setup.
	// This is called after the container is created and started, but before
	// the user attaches to it. The implementation should:
	// - Copy settings directories if needed
	// - Generate config files if needed
	// - Inject credentials if needed
	//
	// The agentCfg parameter contains the agent configuration from the global
	// config file. It may be nil if no config exists for this agent.
	//
	// Returns a SetupResult containing env vars to set, or an error if setup fails.
	Setup(containerName string, agentCfg *config.AgentConfig) (*SetupResult, error)
}

// CredentialEnvProvider is an optional interface that agents can implement
// to provide credential environment variables before the container is created.
// This is necessary because env vars must be set at container creation time,
// but Setup() runs after the container starts.
type CredentialEnvProvider interface {
	// GetCredentialEnvVars returns environment variables needed for agent authentication.
	// This is called before container creation and must not require a running container.
	// Returns nil map and nil error if no credentials are configured.
	GetCredentialEnvVars(agentCfg *config.AgentConfig) (map[string]string, error)
}

// GetCredentialEnvVars returns credential env vars for an agent if it implements
// CredentialEnvProvider. Returns nil if the agent doesn't implement the interface
// or if no credentials are configured.
func GetCredentialEnvVars(a Agent, agentCfg *config.AgentConfig) (map[string]string, error) {
	if provider, ok := a.(CredentialEnvProvider); ok {
		return provider.GetCredentialEnvVars(agentCfg)
	}
	return nil, nil
}

// Registry maps agent names to their implementations.
var registry = make(map[string]Agent)

// Register adds an agent to the registry.
// This is typically called from init() in agent implementation files.
func Register(agent Agent) {
	registry[agent.Name()] = agent
}

// Get returns the agent with the given name, or nil if not found.
func Get(name string) Agent {
	return registry[name]
}

// List returns the names of all registered agents.
func List() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// --- Utility Functions for Agent Implementations ---

// UserHomeDirFunc returns the user's home directory.
// Can be overridden in tests to use a mock home directory.
var UserHomeDirFunc = os.UserHomeDir

// CopyDirToContainer copies a directory from the host to the container.
// The directory is copied to containerHomeDir (e.g., ~/.claude -> /home/cloister/.claude).
//
// Parameters:
//   - containerName: the Docker container name
//   - dirName: the directory name under $HOME (e.g., ".claude")
//   - excludePatterns: patterns to exclude (passed to rsync --exclude)
//
// Symlinks are dereferenced during copy so that settings stored in dotfiles
// repositories work correctly inside the container.
//
// Returns nil if the directory doesn't exist on the host (not an error).
// Returns an error only if the directory exists but cannot be copied.
func CopyDirToContainer(containerName, dirName string, excludePatterns []string) error {
	homeDir, err := UserHomeDirFunc()
	if err != nil {
		return err
	}

	srcDir := filepath.Join(homeDir, dirName)

	// Check if directory exists
	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist - skip silently
		}
		return err
	}
	if !info.IsDir() {
		return nil // Not a directory - skip silently
	}

	// Create a temp directory to hold the filtered copy
	tmpDir, err := os.MkdirTemp("", "cloister-settings-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDestDir := filepath.Join(tmpDir, dirName)

	// Try rsync first (fast, supports exclusions natively)
	if err := copyWithRsync(srcDir, tmpDestDir, excludePatterns); err != nil {
		// rsync failed or not available, fall back to cp (no exclusion support)
		log.Printf("cloister: warning: rsync failed, falling back to cp (exclusions will not apply): %v", err)
		if err := copyWithCp(srcDir, tmpDestDir); err != nil {
			return err
		}
	}

	// Clear any pre-existing directory in the container (from image build)
	clearCmd := exec.Command("docker", "exec", containerName, "rm", "-rf", containerHomeDir+"/"+dirName)
	if output, err := clearCmd.CombinedOutput(); err != nil {
		log.Printf("cloister: warning: failed to clear existing %s: %v: %s", dirName, err, output)
	}

	// Copy the filtered directory to the container
	return docker.CopyToContainerWithOwner(tmpDestDir, containerName, containerHomeDir, ContainerUID, ContainerGID)
}

// copyWithRsync copies src to dest using rsync with exclusions and symlink dereferencing.
func copyWithRsync(src, dest string, excludePatterns []string) error {
	args := []string{
		"-rL",             // recursive, dereference symlinks
		"--copy-dirlinks", // also dereference symlinks to directories
	}
	for _, pattern := range excludePatterns {
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
func copyWithCp(src, dest string) error {
	cmd := exec.Command("cp", "-rL", src, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp failed: %w: %s", err, output)
	}
	return nil
}

// WriteFileToContainer writes a file to the container with proper ownership.
//
// Parameters:
//   - containerName: the Docker container name
//   - destPath: absolute path in the container (e.g., "/home/cloister/.claude.json")
//   - content: the file content to write
func WriteFileToContainer(containerName, destPath, content string) error {
	return docker.WriteFileToContainerWithOwner(containerName, destPath, content, ContainerUID, ContainerGID)
}

// MergeJSONConfig reads a JSON config file from the host, copies specified fields,
// applies forced values, and returns the merged JSON.
//
// Parameters:
//   - hostFileName: the config file name under $HOME (e.g., ".claude.json")
//   - fieldsToCopy: top-level fields to copy from host config
//   - forcedValues: fields that are always set to specific values (override host)
//   - conditionalCopy: additional fields to copy from host (e.g., auth-method-specific)
//
// Returns the merged JSON as an indented string with a trailing newline.
// If the host file doesn't exist, only forcedValues are included.
func MergeJSONConfig(hostFileName string, fieldsToCopy []string, forcedValues map[string]any, conditionalCopy map[string]any) (string, error) {
	config := make(map[string]any)

	// Apply forced values first
	for key, value := range forcedValues {
		config[key] = value
	}

	// Try to read host config file
	homeDir, err := UserHomeDirFunc()
	if err == nil {
		hostPath := filepath.Join(homeDir, hostFileName)
		if content, readErr := os.ReadFile(hostPath); readErr == nil {
			var hostConfig map[string]any
			if json.Unmarshal(content, &hostConfig) == nil {
				// Copy specified fields from host
				for _, field := range fieldsToCopy {
					if value, ok := hostConfig[field]; ok {
						config[field] = value
					}
				}
				// Copy conditional fields
				for field, value := range conditionalCopy {
					if value == nil {
						// nil means "copy from host if present"
						if hostValue, ok := hostConfig[field]; ok {
							config[field] = hostValue
						}
					} else {
						// Non-nil means use this value directly
						config[field] = value
					}
				}
			}
		}
	}

	// Marshal the config
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	return string(configJSON) + "\n", nil
}

// skipPermsAlias is the alias line added to bashrc for --dangerously-skip-permissions.
const skipPermsAlias = `alias claude='claude --dangerously-skip-permissions'`

// appendSkipPermsAlias adds the --dangerously-skip-permissions alias to the container's bashrc.
// The alias is only added if not already present (idempotent).
// The container must be running.
func appendSkipPermsAlias(containerName string) error {
	bashrcPath := containerHomeDir + "/.bashrc"

	// Use grep to check if alias already exists, then append if not.
	// The command exits 0 if alias exists, 1 if not. We use || to append only when grep fails.
	cmd := exec.Command("docker", "exec", containerName, "sh", "-c",
		fmt.Sprintf(`grep -qF %q %s || echo %q >> %s`,
			skipPermsAlias, bashrcPath, skipPermsAlias, bashrcPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add alias: %w: %s", err, output)
	}
	return nil
}
