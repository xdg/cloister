package agent

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/xdg/cloister/internal/codex"
	"github.com/xdg/cloister/internal/config"
)

func init() {
	Register(NewCodexAgent())
}

const (
	codexAgentName   = "codex"
	codexSettingsDir = ".codex"
	codexConfigFile  = "config.toml"
	codexAgentsMD    = "AGENTS.md"
)

// codexFullAutoAlias is the alias line added to bashrc for full-auto mode.
// This is equivalent to Claude's --dangerously-skip-permissions.
const codexFullAutoAlias = `alias codex='codex --approval-mode full-auto'`

// codexCloisterRulesContent is the content appended to AGENTS.md that explains
// the cloister environment to Codex. This is appended to any existing AGENTS.md.
const codexCloisterRulesContent = `
# Cloister Environment

You are running inside a cloister container - a sandboxed environment for AI coding agents.
This allows you to safely run with full-auto approval mode without risk to the user's system.
Within the cloister, you have free rein, but external network access is restricted and only
the /work directory is a read/write mount from the host.

A separate cloister guardian process acts as your http proxy and gatekeeper for certain operations.

## Key Facts

- **Project directory**: /work (this is where you should do all your work); it is bound to the host project directory so the user can see your changes and your work persists after the container stops
- **Network**: No direct internet access; HTTP proxy allowlists common documentation sites and package registries
- **Host access**: The 'hostexec' command communicates requests to run commands on the host machine

## What Works Inside the Container

- Reading and writing files in /work
- Normal unix tools and resources available to non-root users (e.g /tmp)
- Running tests, linters, and build tools (build-essential, make, cmake, and common development tools are pre-installed)
- Installing operating system packages via 'sudo apt-get'
- Installing language packages (npm, pip, go get, etc.)
- Git operations that don't require network (commit, branch, merge, rebase)
- Fetching allowed documentation and packages through the proxy

## Network Access

All external web traffic routes through an allowlist proxy configured via HTTP_PROXY and HTTPS_PROXY. Common package registries (npm, PyPI, crates.io, proxy.golang.org, etc.) and documentation sites are pre-allowed. Requests to unlisted domains are either rejected with a 403 error or held for human approval, depending on the user's configuration. There is no direct internet access - if a fetch fails with 403, the domain is not on the allowlist.

Non-HTTP protocols (e.g., git://, ssh://) are not supported.

## What Requires hostexec

- Git push/pull/fetch (network operations)
- Docker commands (no Docker socket in container)
- GitHub CLI (` + "`gh`" + `) commands
- Any command that needs host filesystem access outside /work

## Using hostexec

For operations that absolutely need to be run and cannot run inside the cloister container, use the hostexec wrapper. This sends a request to the guardian for approval and execution on the host.

**USING HOSTEXEC SHOULD BE RARE**: Only use hostexec when absolutely necessary, as it interrupts long-running, unsupervised work. This annoys users and is a security risk.

The general syntax is: hostexec <command> [args...]

The command must be in the host user's PATH. Arguments are passed directly to the executable without shell expansion - shell features like redirection (>), piping (|), globbing (*), and variable substitution ($VAR) will not work. Each argument is passed exactly as written.

### Approval Flow

When you run a hostexec command, the guardian evaluates it against a set of allow/deny patterns configured by the user. There are three possible outcomes:

1. **Auto-approved commands** execute immediately without human intervention
2. **Manual-approve commands** are queued in a web UI for human approval. The command blocks until the human approves, denies, or the request times out (5 minutes by default)
3. **Unrecognized commands** are denied immediately

Check the result by examining the exit code and output:
- Exit code 0 with output: command was approved and executed successfully; hostexec stdout/stderr/exit-code are relayed from the host command
- Exit code non-zero with output: command was approved but failed (check stderr)
- "Command denied: <reason>": the human denied the request or the command pattern was not allowed
- "Command timed out waiting for approval": no response within the timeout period

**IMPORTANT**: If the command is denied or times out, do not retry it. Instead, find a workaround if you can or else inform the user and ask for further instructions.

For example, if 'hostexec git push' is denied, then the user doesn't want you to push changes to the remote. You should respect that decision and not attempt to push again.

### Example Patterns

These are illustrative examples only. The actual allowed/denied patterns are configured by the user based on their security posture.

- ` + "`hostexec git push`" + ` - push commits to remote
- ` + "`hostexec docker build -t myimage .`" + ` - build Docker image on host
- ` + "`hostexec docker logs my-container`" + ` - read host container logs
- ` + "`hostexec gh pr create`" + ` - create GitHub pull request
`

// codexSettingsExcludePatterns lists directories/files to exclude when copying ~/.codex/
// These are machine-local files that don't need to be in the container.
// TODO: Populate with actual patterns once Codex cache/log locations are identified.
var codexSettingsExcludePatterns = []string{
	// Stub - add exclusion patterns as needed
}

// codexConfigForcedValues are TOML key-value pairs that are always set.
// These ensure smooth operation in the container environment.
var codexConfigForcedValues = map[string]any{
	// Network access through proxy is allowed
	"sandbox_workspace_write.network_access": true,
}

// codexConfigSkipPermsForcedValues are additional forced values when skip_permissions is enabled.
var codexConfigSkipPermsForcedValues = map[string]any{
	"approval_policy": "full-auto",
}

// CodexAgent handles setup for Codex CLI in containers.
type CodexAgent struct {
	// Injector handles credential extraction and injection.
	// If nil, codex.NewInjector() is used.
	Injector *codex.Injector
}

// NewCodexAgent creates a new CodexAgent with default dependencies.
func NewCodexAgent() *CodexAgent {
	return &CodexAgent{
		Injector: codex.NewInjector(),
	}
}

// Name returns the agent identifier.
func (a *CodexAgent) Name() string {
	return codexAgentName
}

// GetContainerEnvVars implements ContainerEnvProvider.
// It returns credential env vars without requiring a running container.
func (a *CodexAgent) GetContainerEnvVars(agentCfg *config.AgentConfig) (map[string]string, error) {
	if agentCfg == nil || agentCfg.AuthMethod == "" {
		return nil, nil
	}

	injector := a.Injector
	if injector == nil {
		injector = codex.NewInjector()
	}

	injectionConfig, err := injector.InjectCredentials(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("credential injection failed: %w", err)
	}

	return injectionConfig.EnvVars, nil
}

// Setup performs Codex-specific container setup.
// This copies the ~/.codex/ settings directory, injects credentials,
// generates the config.toml, and appends cloister rules to AGENTS.md.
func (a *CodexAgent) Setup(containerName string, agentCfg *config.AgentConfig) (*SetupResult, error) {
	result := &SetupResult{
		EnvVars: make(map[string]string),
	}

	// Step 1: Copy settings directory (~/.codex/)
	// This is a one-way snapshot - writes inside container are isolated
	if err := CopyDirToContainer(containerName, codexSettingsDir, codexSettingsExcludePatterns); err != nil {
		// Log but don't fail - missing settings is not fatal
		_ = err
	}

	// Step 2: Append cloister rules to AGENTS.md
	if err := appendCodexCloisterRules(containerName); err != nil {
		return nil, fmt.Errorf("failed to write cloister rules: %w", err)
	}

	// Step 3: Inject credentials (if configured)
	if agentCfg != nil && agentCfg.AuthMethod != "" {
		injector := a.Injector
		if injector == nil {
			injector = codex.NewInjector()
		}

		injectionConfig, err := injector.InjectCredentials(agentCfg)
		if err != nil {
			return nil, fmt.Errorf("credential injection failed: %w", err)
		}

		// Collect env vars for container
		for key, value := range injectionConfig.EnvVars {
			result.EnvVars[key] = value
		}

		// Write credential files to container (if any)
		for destPath, content := range injectionConfig.Files {
			if err := WriteFileToContainer(containerName, destPath, content); err != nil {
				return nil, fmt.Errorf("failed to write credential file %s: %w", destPath, err)
			}
		}
	}

	// Step 4: Generate/merge config.toml
	skipPerms := agentCfg == nil || agentCfg.SkipPerms == nil || *agentCfg.SkipPerms
	if err := mergeCodexConfig(containerName, skipPerms); err != nil {
		return nil, fmt.Errorf("failed to merge config.toml: %w", err)
	}

	// Step 5: Add full-auto alias if skip_permissions is enabled (default true)
	if skipPerms {
		if err := AppendBashAlias(containerName, codexFullAutoAlias); err != nil {
			return nil, fmt.Errorf("failed to add full-auto alias: %w", err)
		}
	}

	return result, nil
}

// appendCodexCloisterRules appends the cloister environment documentation to AGENTS.md.
func appendCodexCloisterRules(containerName string) error {
	codexDir := containerHomeDir + "/" + codexSettingsDir
	agentsMDPath := codexDir + "/" + codexAgentsMD

	// Create .codex directory if it doesn't exist
	mkdirCmd := exec.CommandContext(context.Background(), "docker", "exec", containerName, "mkdir", "-p", codexDir) //nolint:gosec // G204: args are not user-controlled
	if output, err := mkdirCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create .codex directory: %w: %s", err, output)
	}

	// Append to AGENTS.md (create if doesn't exist)
	// Use a marker to avoid duplicating the content on repeated runs
	marker := "# Cloister Environment"
	appendCmd := exec.CommandContext(context.Background(), "docker", "exec", containerName, "sh", "-c", //nolint:gosec // G204: args are not user-controlled
		fmt.Sprintf(`grep -qF %q %s 2>/dev/null || echo %q >> %s`,
			marker, agentsMDPath, codexCloisterRulesContent, agentsMDPath))
	if output, err := appendCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to append to AGENTS.md: %w: %s", err, output)
	}

	// Fix ownership
	chownCmd := exec.CommandContext(context.Background(), "docker", "exec", containerName, "chown",
		ContainerUID+":"+ContainerGID, agentsMDPath)
	if output, err := chownCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to fix AGENTS.md ownership: %w: %s", err, output)
	}

	return nil
}

// mergeCodexConfig reads the existing config.toml (if any), merges in forced values,
// and writes the result back to the container.
func mergeCodexConfig(containerName string, skipPerms bool) error {
	configPath := filepath.Join(containerHomeDir, codexSettingsDir, codexConfigFile)

	// Build forced values
	forcedValues := make(map[string]any)
	for k, v := range codexConfigForcedValues {
		forcedValues[k] = v
	}
	if skipPerms {
		for k, v := range codexConfigSkipPermsForcedValues {
			forcedValues[k] = v
		}
	}

	// Use the TOML merge utility
	configTOML, err := MergeTOMLConfig(codexSettingsDir+"/"+codexConfigFile, nil, forcedValues)
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	if err := WriteFileToContainer(containerName, configPath, configTOML); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
