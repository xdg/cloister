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
	agentName      = "claude"
	settingsDir    = ".claude"
	configFileName = ".claude.json"
)

// cloisterRulesPath is the path to the cloister rules file in the container.
const cloisterRulesPath = containerHomeDir + "/.claude/rules/cloister.md"

// cloisterRulesContent is the content of the rules file that explains the
// cloister environment to Claude. This file is always overwritten by cloister.
const cloisterRulesContent = `# Cloister Environment

You are running inside a cloister container - a sandboxed environment for AI coding agents.

## Key Facts

- **Project directory**: /work (this is where you should do all your work)
- **Network**: No direct internet access; HTTP proxy allowlists documentation sites and package registries
- **Host access**: Use hostexec for operations that require the host machine

## Using hostexec

For operations that cannot run inside the container (git push, docker commands, etc.), use the hostexec wrapper. This sends a request to the host for human approval.

Common patterns:
- ` + "`hostexec git push`" + ` - push commits to remote
- ` + "`hostexec git push origin HEAD`" + ` - push current branch
- ` + "`hostexec docker build -t myimage .`" + ` - build Docker image on host
- ` + "`hostexec docker compose up -d`" + ` - start services on host

## What Works Inside the Container

- Reading and writing files in /work
- Running tests, linters, and build tools
- Installing packages (npm, pip, go get, etc.)
- Git operations that don't require network (commit, branch, merge, rebase)
- Fetching allowed documentation and packages through the proxy

## What Requires hostexec

- Git push/pull (network operations)
- Docker commands (no Docker socket in container)
- Any command that needs host filesystem access outside /work
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
	return agentName
}

// settingsExcludePatterns lists directories/files to exclude when copying ~/.claude/
// These are machine-local files that don't need to be in the container.
// Based on ~/.claude/.gitignore patterns.
var settingsExcludePatterns = []string{
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
	if err := CopyDirToContainer(containerName, settingsDir, settingsExcludePatterns); err != nil {
		// Log but don't fail - missing settings is not fatal
		_ = err
	}

	// Step 1.5: Write cloister rules file
	// This explains the cloister environment to Claude and is always overwritten
	if err := writeCloisterRules(containerName); err != nil {
		return nil, fmt.Errorf("failed to write cloister rules: %w", err)
	}

	// Step 2: Inject credentials (if configured)
	var authMethod string
	if agentCfg != nil && agentCfg.AuthMethod != "" {
		authMethod = agentCfg.AuthMethod

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
	// Build conditional fields based on auth method
	var conditionalCopy map[string]any
	if authMethod == claude.AuthMethodExisting {
		// Copy oauthAccount from host when using "existing" auth
		// (it's tied to the credentials being injected)
		conditionalCopy = map[string]any{
			"oauthAccount": nil, // nil means "copy from host if present"
		}
	}

	configJSON, err := MergeJSONConfig(configFileName, configFieldsToCopy, configForcedValues, conditionalCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config: %w", err)
	}

	if err := WriteFileToContainer(containerName, configJSONPath, configJSON); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	// Step 4: Add skip-permissions alias if enabled (default true)
	// SkipPerms is *bool: nil or true means add the alias, only explicit false skips it
	if agentCfg == nil || agentCfg.SkipPerms == nil || *agentCfg.SkipPerms {
		if err := appendSkipPermsAlias(containerName); err != nil {
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
