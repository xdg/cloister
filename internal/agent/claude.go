package agent

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/xdg/cloister/internal/claude"
	"github.com/xdg/cloister/internal/config"
)

func init() {
	Register(NewClaudeAgent())
}

const (
	claudeAgentName      = "claude"
	claudeSettingsDir    = ".claude"
	claudeConfigFileName = ".claude.json"
)

// claudeSkipPermsAlias is the alias line added to bashrc for --dangerously-skip-permissions.
const claudeSkipPermsAlias = `alias claude='claude --dangerously-skip-permissions'`

// cloisterRulesPath is the path to the cloister rules file in the container.
const cloisterRulesPath = containerHomeDir + "/.claude/rules/cloister.md"

// cloisterRulesContent is the content of the rules file that explains the
// cloister environment to Claude. This file is always overwritten by cloister.
const cloisterRulesContent = `# Cloister Environment

You are running inside a cloister container - a sandboxed environment for AI coding agents.
This allows you to safely run with permissions disabled without risk to the user's system.
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

All external web traffic routes through an allowlist proxy configured via HTTP_PROXY and HTTPS_PROXY. Common package registries (npm, PyPI, crates.io, proxy.golang.org, etc.) and documentation sites are pre-allowed. Requests to unlisted domains are either rejected with a 403 error or held for human approval, depending on the user's configuration. There is no direct internet access — if a fetch fails with 403, the domain is not on the allowlist.

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

The command must be in the host user's PATH. Arguments are passed directly to the executable without shell expansion — shell features like redirection (>), piping (|), globbing (*), and variable substitution ($VAR) will not work. Each argument is passed exactly as written.

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

// ClaudeAgent handles setup for Claude Code in containers.
type ClaudeAgent struct {
	// Injector handles credential extraction and injection.
	// If nil, claude.NewInjector() is used.
	Injector *claude.Injector
}

// NewClaudeAgent creates a new ClaudeAgent with default dependencies.
func NewClaudeAgent() *ClaudeAgent {
	return &ClaudeAgent{
		Injector: claude.NewInjector(),
	}
}

// Name returns the agent identifier.
func (a *ClaudeAgent) Name() string {
	return claudeAgentName
}

// GetCredentialEnvVars implements CredentialEnvProvider.
// It returns credential env vars without requiring a running container.
func (a *ClaudeAgent) GetCredentialEnvVars(agentCfg *config.AgentConfig) (map[string]string, error) {
	if agentCfg == nil || agentCfg.AuthMethod == "" {
		return nil, nil
	}

	injector := a.Injector
	if injector == nil {
		injector = claude.NewInjector()
	}

	injectionConfig, err := injector.InjectCredentials(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("credential injection failed: %w", err)
	}

	return injectionConfig.EnvVars, nil
}

// settingsExcludePatterns lists directories/files to exclude when copying ~/.claude/
// These are machine-local files that don't need to be in the container.
// Based on ~/.claude/.gitignore patterns.
var settingsExcludePatterns = []string{
	".claude.json",
	".update.lock",
	"cache",
	"debug/",
	"downloads/",
	"file-history/",
	"history.jsonl",
	"plans/",
	"plugins/install-counts-cache.json",
	"projects/",
	"session-env/",
	"shell-snapshots/",
	"stats-cache.json",
	"statsig/",
	"tasks/",
	"telemetry",
	"todos/",
}

// configJSONPath is the path to the Claude config file in the container.
var configJSONPath = containerHomeDir + "/.claude.json"

// configFieldsToCopy lists the top-level fields from ~/.claude.json that should
// be copied to the container. These are identity fields that ensure consistent user ID.
var configFieldsToCopy = []string{
	"userID",
	"lastOnboardingVersion",
}

// configForcedValues are fields that are always set to specific values.
// These ensure smooth operation in the container environment.
var configForcedValues = map[string]any{
	"hasCompletedOnboarding":        true,
	"bypassPermissionsModeAccepted": true,
	"installMethod":                 "native",
}

// Setup performs Claude-specific container setup.
// This copies the ~/.claude/ settings directory, injects credentials,
// and generates the ~/.claude.json config file.
func (a *ClaudeAgent) Setup(containerName string, agentCfg *config.AgentConfig) (*SetupResult, error) {
	result := &SetupResult{
		EnvVars: make(map[string]string),
	}

	// Step 1: Copy settings directory (~/.claude/)
	// This is a one-way snapshot - writes inside container are isolated
	if err := CopyDirToContainer(containerName, claudeSettingsDir, settingsExcludePatterns); err != nil {
		// Log but don't fail - missing settings is not fatal
		_ = err
	}

	// Step 1.5: Write cloister rules file
	// This explains the cloister environment to Claude and is always overwritten
	if err := writeCloisterRules(containerName); err != nil {
		return nil, fmt.Errorf("failed to write cloister rules: %w", err)
	}

	// Step 2: Inject credentials (if configured)
	if agentCfg != nil && agentCfg.AuthMethod != "" {
		injector := a.Injector
		if injector == nil {
			injector = claude.NewInjector()
		}

		injectionConfig, err := injector.InjectCredentials(agentCfg)
		if err != nil {
			return nil, fmt.Errorf("credential injection failed: %w", err)
		}

		// Collect env vars for container
		for key, value := range injectionConfig.EnvVars {
			result.EnvVars[key] = value
		}

		// Write credential files to container
		for destPath, content := range injectionConfig.Files {
			if err := WriteFileToContainer(containerName, destPath, content); err != nil {
				return nil, fmt.Errorf("failed to write credential file %s: %w", destPath, err)
			}
		}
	}

	// Step 3: Generate ~/.claude.json config file
	configJSON, err := MergeJSONConfig(claudeConfigFileName, configFieldsToCopy, configForcedValues, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config: %w", err)
	}

	if err := WriteFileToContainer(containerName, configJSONPath, configJSON); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	// Step 4: Add skip-permissions alias if enabled (default true)
	// SkipPerms is *bool: nil or true means add the alias, only explicit false skips it
	if agentCfg == nil || agentCfg.SkipPerms == nil || *agentCfg.SkipPerms {
		if err := AppendBashAlias(containerName, claudeSkipPermsAlias); err != nil {
			return nil, fmt.Errorf("failed to add skip-permissions alias: %w", err)
		}
	}

	return result, nil
}

// writeCloisterRules creates the rules directory and writes the cloister.md file.
// This file explains the cloister environment to Claude and is always overwritten.
func writeCloisterRules(containerName string) error {
	// Create rules directory if it doesn't exist
	rulesDir := filepath.Dir(cloisterRulesPath)
	mkdirCmd := exec.Command("docker", "exec", containerName, "mkdir", "-p", rulesDir)
	if output, err := mkdirCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create rules directory: %w: %s", err, output)
	}

	// Write the rules file
	return WriteFileToContainer(containerName, cloisterRulesPath, cloisterRulesContent)
}
